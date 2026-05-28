package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeRealityShortIdsInStreamSettings(t *testing.T) {
	stream := `{
  "network": "tcp",
  "security": "reality",
  "realitySettings": {
    "shortIds": ["gksimxr8", "ABCD1234", ""]
  }
}`

	normalized, changed := normalizeRealityShortIdsInStreamSettings(stream)
	if !changed {
		t.Fatal("expected invalid shortId to be normalized")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(normalized), &parsed); err != nil {
		t.Fatal(err)
	}
	reality := parsed["realitySettings"].(map[string]any)
	shortIds := reality["shortIds"].([]any)
	if got := shortIds[0].(string); got != "676b73696d787238" {
		t.Fatalf("unexpected normalized shortId: %s", got)
	}
	if got := shortIds[1].(string); got != "abcd1234" {
		t.Fatalf("expected valid shortId to be lower-cased, got %s", got)
	}
	if got := shortIds[2].(string); got != "" {
		t.Fatalf("empty shortId should remain empty, got %q", got)
	}
}

func TestNormalizeRealityShortIdsInStreamSettingsNoop(t *testing.T) {
	stream := `{"network":"tcp","security":"reality","realitySettings":{"shortIds":["abcd1234"]}}`
	normalized, changed := normalizeRealityShortIdsInStreamSettings(stream)
	if changed {
		t.Fatalf("valid shortId should not change: %s", normalized)
	}
}
