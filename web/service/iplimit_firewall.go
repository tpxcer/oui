package service

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/xray"
)

const ipLimitChain = "f2b-3x-ipl"

func ipLimitTableCommand(ip string) string {
	if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() == nil {
		return "ip6tables"
	}
	return "iptables"
}

func EnsureIPLimitFirewallChain(ip string) error {
	cmdName := ipLimitTableCommand(ip)
	if _, err := exec.LookPath(cmdName); err != nil {
		return nil
	}

	_ = exec.Command(cmdName, "-N", ipLimitChain).Run()
	if err := exec.Command(cmdName, "-C", "INPUT", "-p", "tcp", "-j", ipLimitChain).Run(); err == nil {
		return nil
	}
	if output, err := exec.Command(cmdName, "-I", "INPUT", "-p", "tcp", "-j", ipLimitChain).CombinedOutput(); err != nil {
		return fmt.Errorf("%s attach %s: %w: %s", cmdName, ipLimitChain, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func BanIPLimitPort(ip string, port int) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	cmdName := ipLimitTableCommand(ip)
	if _, err := exec.LookPath(cmdName); err != nil {
		return nil
	}
	if err := EnsureIPLimitFirewallChain(ip); err != nil {
		return err
	}
	UnbanIPLimitLegacyAllPorts(ip)

	args := []string{"-s", ip, "-p", "tcp", "--dport", fmt.Sprint(port), "-j", "REJECT"}
	checkArgs := append([]string{"-C", ipLimitChain}, args...)
	if err := exec.Command(cmdName, checkArgs...).Run(); err == nil {
		return nil
	}
	insertArgs := append([]string{"-I", ipLimitChain, "1"}, args...)
	if output, err := exec.Command(cmdName, insertArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s ban %s:%d: %w: %s", cmdName, ip, port, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func UnbanIPLimitLegacyAllPorts(ip string) {
	if net.ParseIP(ip) == nil {
		return
	}
	cmdName := ipLimitTableCommand(ip)
	if _, err := exec.LookPath(cmdName); err != nil {
		return
	}

	rejectWith := []string{"icmp-port-unreachable", "tcp-reset"}
	if cmdName == "ip6tables" {
		rejectWith = []string{"icmp6-port-unreachable", "tcp-reset", "icmp6-adm-prohibited"}
	}

	rules := [][]string{
		{"-s", ip, "-j", "REJECT"},
		{"-s", ip, "-j", "DROP"},
	}
	for _, value := range rejectWith {
		rules = append(rules, []string{"-s", ip, "-j", "REJECT", "--reject-with", value})
	}
	for _, rule := range rules {
		args := append([]string{"-D", ipLimitChain}, rule...)
		for exec.Command(cmdName, args...).Run() == nil {
		}
	}
}

func UnbanIPLimitPort(ip string, port int) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	cmdName := ipLimitTableCommand(ip)
	if _, err := exec.LookPath(cmdName); err != nil {
		return nil
	}

	args := []string{"-D", ipLimitChain, "-s", ip, "-p", "tcp", "--dport", fmt.Sprint(port), "-j", "REJECT"}
	for {
		if err := exec.Command(cmdName, args...).Run(); err != nil {
			break
		}
	}
	UnbanIPLimitLegacyAllPorts(ip)
	return nil
}

func AppendIPLimitBanLog(action, email, ip string, port int, note string) {
	file, err := os.OpenFile(xray.GetIPLimitBannedLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()

	emailPart := email
	if emailPart == "" {
		emailPart = "-"
	}
	_, _ = fmt.Fprintf(file, "%s   %s   [Email] = %s [Port] = %d [IP] = %s %s.\n",
		time.Now().Format("2006/01/02 15:04:05"),
		action,
		emailPart,
		port,
		ip,
		note,
	)
}
