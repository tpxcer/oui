package service

import "testing"

func TestQuickPresetRemarksAreShortASCII(t *testing.T) {
	expected := map[string]string{
		"hysteria2":         "HY2",
		"vlessReality":      "VL-RV",
		"vlessXhttpTls":     "VL-XHTTP-TLS",
		"vlessXhttpReality": "VL-XHTTP-R",
	}

	for key, want := range expected {
		preset, ok := tgQuickPresets[key]
		if !ok {
			t.Fatalf("missing quick preset %q", key)
		}
		if preset.Remark != want {
			t.Fatalf("preset %q remark = %q, want %q", key, preset.Remark, want)
		}
		for _, r := range preset.Remark {
			if r > 127 {
				t.Fatalf("preset %q remark contains non-ASCII rune %q", key, r)
			}
		}
		if len(preset.Remark) > 12 {
			t.Fatalf("preset %q remark is too long: %q", key, preset.Remark)
		}
	}
}
