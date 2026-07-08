package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAddPortToNftDportRuleAddsMissingPort(t *testing.T) {
	input := `table inet host_hardening {
  chain input {
    udp dport {4955, 12108, 26745, 62508} accept
    counter drop
  }
}`

	got, changed, err := addPortToNftDportRule(input, "udp", 61958)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected config to change")
	}
	if !strings.Contains(got, "udp dport {4955, 12108, 26745, 61958, 62508} accept") {
		t.Fatalf("expected udp port to be inserted and sorted, got:\n%s", got)
	}
}

func TestAddPortToNftDportRuleSkipsExistingPort(t *testing.T) {
	input := `table inet host_hardening {
  chain input {
    udp dport {4955, 61958, 62508} accept
    counter drop
  }
}`

	got, changed, err := addPortToNftDportRule(input, "udp", 61958)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("expected existing port to be left unchanged")
	}
	if got != input {
		t.Fatalf("expected content to stay identical, got:\n%s", got)
	}
}

func TestAddPortToNftDportRuleInsertsBeforeDrop(t *testing.T) {
	input := `table inet host_hardening {
  chain input {
    tcp dport 5522 accept
    counter drop
  }
}`

	got, changed, err := addPortToNftDportRule(input, "udp", 61958)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected config to change")
	}
	if !strings.Contains(got, "    udp dport {61958} accept\n    counter drop") {
		t.Fatalf("expected udp rule before drop, got:\n%s", got)
	}
}

func TestQuickHysteriaClientGeneratesUUIDAndAuth(t *testing.T) {
	client := quickHysteriaClient("hy2", "auth-token")

	if client.ID == "" {
		t.Fatal("expected Hysteria2 quick client UUID to be generated")
	}
	if _, err := uuid.Parse(client.ID); err != nil {
		t.Fatalf("expected valid UUID, got %q: %v", client.ID, err)
	}
	if client.Auth != "auth-token" {
		t.Fatalf("auth = %q, want auth-token", client.Auth)
	}
	if !strings.HasPrefix(client.Email, "hy2-") {
		t.Fatalf("email = %q, want hy2-*", client.Email)
	}
}

func TestQuickRealityTargetsAvoidKnownBadDefault(t *testing.T) {
	if len(quickRealityTargets) == 0 {
		t.Fatal("expected at least one quick Reality target")
	}
	for _, target := range quickRealityTargets {
		if strings.Contains(target.Target, "microsoft.com") {
			t.Fatalf("quick Reality target %q should not use microsoft.com", target.Target)
		}
		if !strings.HasSuffix(target.Target, ":443") {
			t.Fatalf("quick Reality target %q should use port 443", target.Target)
		}
		if len(target.ServerNames) == 0 || strings.TrimSpace(target.ServerNames[0]) == "" {
			t.Fatalf("quick Reality target %q should include a primary SNI", target.Target)
		}
		if len(target.ServerNames) != 1 {
			t.Fatalf("quick Reality target %q should keep one verified SNI, got %v", target.Target, target.ServerNames)
		}
	}
}

func TestQuickRealityServerNameUsesPrimarySNI(t *testing.T) {
	got := quickRealityServerName(map[string]any{
		"serverNames": []string{"meta.com", "www.meta.com"},
	})
	if got != "meta.com" {
		t.Fatalf("quickRealityServerName() = %q, want meta.com", got)
	}
}
