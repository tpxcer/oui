package service

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

const (
	ipLimitTrafficInputChain  = "oui-ipacct-in"
	ipLimitTrafficOutputChain = "oui-ipacct-out"
)

type ipLimitTrafficRule struct {
	chain       string
	addressFlag string
	portFlag    string
	protocol    string
}

func ipLimitTrafficRules() []ipLimitTrafficRule {
	return []ipLimitTrafficRule{
		{chain: ipLimitTrafficInputChain, addressFlag: "-s", portFlag: "--dport", protocol: "tcp"},
		{chain: ipLimitTrafficInputChain, addressFlag: "-s", portFlag: "--dport", protocol: "udp"},
		{chain: ipLimitTrafficOutputChain, addressFlag: "-d", portFlag: "--sport", protocol: "tcp"},
		{chain: ipLimitTrafficOutputChain, addressFlag: "-d", portFlag: "--sport", protocol: "udp"},
	}
}

func ipLimitTrafficComment(ip string, port int) string {
	digest := sha256.Sum256([]byte(ip))
	return fmt.Sprintf("oui-ipacct:%d:%x", port, digest[:8])
}

func ipLimitTrafficRuleArgs(rule ipLimitTrafficRule, ip string, port int, comment string) []string {
	return []string{
		rule.addressFlag, ip,
		"-p", rule.protocol,
		rule.portFlag, strconv.Itoa(port),
		"-m", "comment", "--comment", comment,
		"-j", "RETURN",
	}
}

func runIPLimitTrafficCommand(command string, args ...string) ([]byte, error) {
	return exec.Command(command, append([]string{"-w", "2", "-t", "filter"}, args...)...).CombinedOutput()
}

func ensureIPLimitTrafficChain(command, chain, parent string) error {
	if output, err := runIPLimitTrafficCommand(command, "-N", chain); err != nil {
		if checkOutput, checkErr := runIPLimitTrafficCommand(command, "-L", chain, "-n"); checkErr != nil {
			return fmt.Errorf("create traffic chain %s: %w: %s %s", chain, err, strings.TrimSpace(string(output)), strings.TrimSpace(string(checkOutput)))
		}
	}
	if _, err := runIPLimitTrafficCommand(command, "-C", parent, "-j", chain); err == nil {
		return nil
	}
	if output, err := runIPLimitTrafficCommand(command, "-I", parent, "1", "-j", chain); err != nil {
		return fmt.Errorf("attach traffic chain %s to %s: %w: %s", chain, parent, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ensureIPLimitPortTrafficCounter(ip string, port int) (string, string, error) {
	if net.ParseIP(ip) == nil {
		return "", "", fmt.Errorf("invalid IP address: %s", ip)
	}
	if port <= 0 || port > 65535 {
		return "", "", fmt.Errorf("invalid port: %d", port)
	}

	command := ipLimitTableCommand(ip)
	if _, err := exec.LookPath(command); err != nil {
		return "", "", fmt.Errorf("%s is required for IP traffic sampling", command)
	}
	if err := ensureIPLimitTrafficChain(command, ipLimitTrafficInputChain, "INPUT"); err != nil {
		return "", "", err
	}
	if err := ensureIPLimitTrafficChain(command, ipLimitTrafficOutputChain, "OUTPUT"); err != nil {
		return "", "", err
	}

	comment := ipLimitTrafficComment(ip, port)
	for _, rule := range ipLimitTrafficRules() {
		args := ipLimitTrafficRuleArgs(rule, ip, port, comment)
		if _, err := runIPLimitTrafficCommand(command, append([]string{"-C", rule.chain}, args...)...); err == nil {
			continue
		}
		if output, err := runIPLimitTrafficCommand(command, append([]string{"-A", rule.chain}, args...)...); err != nil {
			return "", "", fmt.Errorf("add traffic counter for %s:%d: %w: %s", ip, port, err, strings.TrimSpace(string(output)))
		}
	}
	return command, comment, nil
}

func parseIPLimitTrafficBytes(output []byte, comment string) (uint64, int, error) {
	var total uint64
	matches := 0
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	marker := "/* " + comment + " */"
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, marker) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		bytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parse traffic bytes %q: %w", fields[1], err)
		}
		total += bytes
		matches++
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return total, matches, nil
}

// ReadIPLimitPortTrafficBytes returns the TCP and UDP bytes exchanged by one
// client IP on one inbound port since its short-lived accounting rules were
// created. The rules only count packets and do not change their verdict.
func ReadIPLimitPortTrafficBytes(ip string, port int) (uint64, error) {
	command, comment, err := ensureIPLimitPortTrafficCounter(ip, port)
	if err != nil {
		return 0, err
	}

	var total uint64
	matches := 0
	for _, chain := range []string{ipLimitTrafficInputChain, ipLimitTrafficOutputChain} {
		output, err := runIPLimitTrafficCommand(command, "-n", "-v", "-x", "-L", chain)
		if err != nil {
			return 0, fmt.Errorf("read traffic chain %s: %w: %s", chain, err, strings.TrimSpace(string(output)))
		}
		bytes, count, err := parseIPLimitTrafficBytes(output, comment)
		if err != nil {
			return 0, err
		}
		total += bytes
		matches += count
	}
	if matches != len(ipLimitTrafficRules()) {
		return 0, fmt.Errorf("traffic counter for %s:%d is incomplete: got %d rules", ip, port, matches)
	}
	return total, nil
}

// RemoveIPLimitPortTrafficCounter removes only the temporary accounting rules
// for one IP and port. Empty shared chains remain attached for future samples.
func RemoveIPLimitPortTrafficCounter(ip string, port int) {
	if net.ParseIP(ip) == nil || port <= 0 || port > 65535 {
		return
	}
	command := ipLimitTableCommand(ip)
	if _, err := exec.LookPath(command); err != nil {
		return
	}
	comment := ipLimitTrafficComment(ip, port)
	for _, rule := range ipLimitTrafficRules() {
		args := ipLimitTrafficRuleArgs(rule, ip, port, comment)
		for {
			if _, err := runIPLimitTrafficCommand(command, append([]string{"-D", rule.chain}, args...)...); err != nil {
				break
			}
		}
	}
}

// ResetIPLimitPortTrafficCounters clears accounting rules left behind by an
// interrupted sample or a process restart. The dedicated chains contain no
// allow or deny rules.
func ResetIPLimitPortTrafficCounters() {
	for _, command := range []string{"iptables", "ip6tables"} {
		if _, err := exec.LookPath(command); err != nil {
			continue
		}
		for _, chain := range []string{ipLimitTrafficInputChain, ipLimitTrafficOutputChain} {
			_, _ = runIPLimitTrafficCommand(command, "-F", chain)
		}
	}
}
