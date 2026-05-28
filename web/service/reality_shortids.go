package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeRealityShortIdsInStreamSettings(streamSettings string) (string, bool) {
	if strings.TrimSpace(streamSettings) == "" {
		return streamSettings, false
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(streamSettings), &stream); err != nil {
		return streamSettings, false
	}
	realitySettings, ok := stream["realitySettings"].(map[string]any)
	if !ok {
		return streamSettings, false
	}
	if !normalizeRealityShortIds(realitySettings) {
		return streamSettings, false
	}
	out, err := json.MarshalIndent(stream, "", "  ")
	if err != nil {
		return streamSettings, false
	}
	return string(out), true
}

func normalizeRealityShortIds(realitySettings map[string]any) bool {
	raw, ok := realitySettings["shortIds"]
	if !ok {
		return false
	}
	shortIds, ok := raw.([]any)
	if !ok {
		return false
	}
	changed := false
	for i, item := range shortIds {
		value := strings.TrimSpace(fmt.Sprint(item))
		normalized := normalizeRealityShortID(value)
		if normalized != value {
			shortIds[i] = normalized
			changed = true
		}
	}
	if changed {
		realitySettings["shortIds"] = shortIds
	}
	return changed
}

func normalizeRealityShortID(value string) string {
	if value == "" {
		return value
	}
	lower := strings.ToLower(value)
	if isValidRealityShortID(lower) {
		return lower
	}
	encoded := hex.EncodeToString([]byte(value))
	if len(encoded) > 16 {
		sum := sha256.Sum256([]byte(value))
		encoded = hex.EncodeToString(sum[:8])
	}
	if len(encoded)%2 != 0 {
		encoded = "0" + encoded
	}
	return encoded
}

func isValidRealityShortID(value string) bool {
	if len(value) > 16 || len(value)%2 != 0 {
		return false
	}
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') {
			continue
		}
		return false
	}
	return true
}
