package job

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotateLogFileIfNeededCopyTruncatesAndShiftsBackups(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "access.log")
	mustWriteFile(t, logPath, "current")
	mustWriteFile(t, rotatedLogPath(logPath, 1), "old-1")
	mustWriteFile(t, rotatedLogPath(logPath, 2), "old-2")

	if err := rotateLogFileIfNeeded(logPath, 1, 2); err != nil {
		t.Fatalf("rotateLogFileIfNeeded: %v", err)
	}

	if got := mustReadFile(t, logPath); got != "" {
		t.Fatalf("active log = %q, want truncated", got)
	}
	if got := mustReadFile(t, rotatedLogPath(logPath, 1)); got != "current" {
		t.Fatalf("backup .1 = %q, want current", got)
	}
	if got := mustReadFile(t, rotatedLogPath(logPath, 2)); got != "old-1" {
		t.Fatalf("backup .2 = %q, want old-1", got)
	}
}

func TestRotateLogFileIfNeededSkipsSmallOrMissingFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "error.log")
	mustWriteFile(t, logPath, "small")

	if err := rotateLogFileIfNeeded(logPath, 100, 2); err != nil {
		t.Fatalf("rotate small log: %v", err)
	}
	if got := mustReadFile(t, logPath); got != "small" {
		t.Fatalf("small active log changed to %q", got)
	}
	if _, err := os.Stat(rotatedLogPath(logPath, 1)); !os.IsNotExist(err) {
		t.Fatalf("small log backup exists or stat error: %v", err)
	}

	if err := rotateLogFileIfNeeded(filepath.Join(dir, "missing.log"), 1, 2); err != nil {
		t.Fatalf("rotate missing log: %v", err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
