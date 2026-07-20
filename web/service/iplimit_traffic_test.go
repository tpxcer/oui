package service

import (
	"strings"
	"testing"
)

func TestParseIPLimitTrafficBytes(t *testing.T) {
	comment := ipLimitTrafficComment("192.0.2.10", 4321)
	output := strings.Join([]string{
		"Chain oui-ipacct-in (1 references)",
		"    pkts      bytes target     prot opt in out source destination",
		"       3       4096 RETURN     tcp  --  *  *   192.0.2.10 0.0.0.0/0 tcp dpt:4321 /* " + comment + " */",
		"       2       1024 RETURN     udp  --  *  *   192.0.2.10 0.0.0.0/0 udp dpt:4321 /* " + comment + " */",
		"       9       9999 RETURN     tcp  --  *  *   192.0.2.11 0.0.0.0/0 tcp dpt:4321 /* other */",
	}, "\n")

	bytes, matches, err := parseIPLimitTrafficBytes([]byte(output), comment)
	if err != nil {
		t.Fatalf("parseIPLimitTrafficBytes failed: %v", err)
	}
	if bytes != 5120 || matches != 2 {
		t.Fatalf("bytes/matches = %d/%d, want 5120/2", bytes, matches)
	}
}

func TestIPLimitTrafficRulesCoverBothDirectionsAndProtocols(t *testing.T) {
	rules := ipLimitTrafficRules()
	if len(rules) != 4 {
		t.Fatalf("rule count = %d, want 4", len(rules))
	}

	want := map[string]bool{
		ipLimitTrafficInputChain + "/tcp/--dport":  true,
		ipLimitTrafficInputChain + "/udp/--dport":  true,
		ipLimitTrafficOutputChain + "/tcp/--sport": true,
		ipLimitTrafficOutputChain + "/udp/--sport": true,
	}
	for _, rule := range rules {
		key := rule.chain + "/" + rule.protocol + "/" + rule.portFlag
		if !want[key] {
			t.Fatalf("unexpected traffic rule: %s", key)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing traffic rules: %v", want)
	}
}
