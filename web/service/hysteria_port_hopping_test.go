package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database/model"
)

func TestParseHysteriaPortRange(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantStart  int
		wantEnd    int
		wantNormal string
		wantErr    bool
	}{
		{name: "hyphen range", input: "48000-50000", wantStart: 48000, wantEnd: 50000, wantNormal: "48000-50000"},
		{name: "iptables colon range", input: "48000:50000", wantStart: 48000, wantEnd: 50000, wantNormal: "48000-50000"},
		{name: "single port", input: "50000", wantStart: 50000, wantEnd: 50000, wantNormal: "50000-50000"},
		{name: "reversed", input: "50000-48000", wantErr: true},
		{name: "too high", input: "48000-70000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, normal, err := parseHysteriaPortRange(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHysteriaPortRange: %v", err)
			}
			if start != tt.wantStart || end != tt.wantEnd || normal != tt.wantNormal {
				t.Fatalf("range = %d-%d %q, want %d-%d %q", start, end, normal, tt.wantStart, tt.wantEnd, tt.wantNormal)
			}
		})
	}
}

func TestHysteriaPortHoppingRuleFromInbound(t *testing.T) {
	inbound := &model.Inbound{
		Id:       7,
		Enable:   true,
		Port:     35406,
		Protocol: model.Hysteria,
		StreamSettings: `{
			"network":"hysteria",
			"hysteriaSettings":{
				"version":2,
				"portHopping":{"enable":true,"range":"48000-50000"}
			}
		}`,
	}

	rule, err := hysteriaPortHoppingRuleFromInbound(inbound)
	if err != nil {
		t.Fatalf("hysteriaPortHoppingRuleFromInbound: %v", err)
	}
	if rule == nil {
		t.Fatalf("expected rule")
	}
	if rule.Start != 48000 || rule.End != 50000 || rule.Target != 35406 || rule.InboundID != 7 {
		t.Fatalf("unexpected rule: %+v", rule)
	}
}

func TestHysteriaPortHoppingRuleDisabled(t *testing.T) {
	inbound := &model.Inbound{
		Id:       8,
		Enable:   true,
		Port:     35406,
		Protocol: model.Hysteria,
		StreamSettings: `{
			"network":"hysteria",
			"hysteriaSettings":{
				"version":2,
				"portHopping":{"enable":false,"range":"48000-50000"}
			}
		}`,
	}

	rule, err := hysteriaPortHoppingRuleFromInbound(inbound)
	if err != nil {
		t.Fatalf("hysteriaPortHoppingRuleFromInbound: %v", err)
	}
	if rule != nil {
		t.Fatalf("expected nil rule, got %+v", rule)
	}
}
