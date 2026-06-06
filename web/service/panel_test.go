package service

import (
	"os"
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v2.9.4", "2.9.3", true},
		{"v2.10.0", "2.9.9", true},
		{"v2.9.3", "2.9.3", false},
		{"v2.9.2", "2.9.3", false},
		{"v3.0.0", "2.9.3", true},
		{"2026.5.28-5", "2026.5.28-4", true},
		{"2026.5.29", "2026.5.28-5", true},
		{"2026.5.28-4", "2026.5.28-5", false},
	}

	for _, tc := range cases {
		if got := isNewerVersion(tc.latest, tc.current); got != tc.want {
			t.Fatalf("isNewerVersion(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestCompareVersionStringsRejectsUnexpectedFormats(t *testing.T) {
	if _, ok := compareVersionStrings("latest", "2.9.3"); ok {
		t.Fatal("expected non-semver latest tag to be rejected")
	}
	if _, ok := compareVersionStrings("v2.9", "2.9.3"); ok {
		t.Fatal("expected short version to be rejected")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/usr/bin/curl"); got != "'/usr/bin/curl'" {
		t.Fatalf("unexpected quote result: %s", got)
	}
	if got := shellQuote("/tmp/a'b"); got != "'/tmp/a'\\''b'" {
		t.Fatalf("unexpected quote result with single quote: %s", got)
	}
}

func TestPendingUpdateNoticeLifecycle(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	service := &PanelService{}

	notice, err := service.GetPendingUpdateNotice()
	if err != nil {
		t.Fatalf("expected empty notice without error, got %v", err)
	}
	if notice != nil {
		t.Fatalf("expected no notice, got %#v", notice)
	}

	if err := service.SavePendingUpdateNotice(12345, "2026.6.6-3"); err != nil {
		t.Fatalf("failed to save pending notice: %v", err)
	}
	if _, err := os.Stat(panelUpdateNoticePath()); err != nil {
		t.Fatalf("expected pending notice file: %v", err)
	}

	notice, err = service.GetPendingUpdateNotice()
	if err != nil {
		t.Fatalf("failed to load pending notice: %v", err)
	}
	if notice == nil || notice.ChatID != 12345 || notice.TargetVersion != "2026.6.6-3" || notice.RequestedAt == 0 {
		t.Fatalf("unexpected pending notice: %#v", notice)
	}

	if err := service.ClearPendingUpdateNotice(); err != nil {
		t.Fatalf("failed to clear pending notice: %v", err)
	}
	notice, err = service.GetPendingUpdateNotice()
	if err != nil {
		t.Fatalf("expected cleared notice without error, got %v", err)
	}
	if notice != nil {
		t.Fatalf("expected notice to be cleared, got %#v", notice)
	}
}

func TestPanelUpdateNoticeReached(t *testing.T) {
	cases := []struct {
		target  string
		current string
		want    bool
	}{
		{"2026.6.6-3", "2026.6.6-3", true},
		{"2026.6.6-3", "2026.6.6-4", true},
		{"2026.6.6-3", "2026.6.6-2", false},
		{"v2.9.4", "2.9.4", true},
		{"", "2026.6.6-3", false},
	}

	for _, tc := range cases {
		if got := panelUpdateNoticeReached(tc.target, tc.current); got != tc.want {
			t.Fatalf("panelUpdateNoticeReached(%q, %q) = %v, want %v", tc.target, tc.current, got, tc.want)
		}
	}
}
