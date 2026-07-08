package job

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/xray"
)

const (
	maxXrayLogFileBytes      int64 = 100 * 1024 * 1024
	maxXrayRotatedLogBackups       = 3
)

// XrayLogRotateJob copy-truncates large Xray access/error logs while keeping
// the active file path unchanged for Xray and IP-limit readers.
type XrayLogRotateJob struct {
	maxBytes  int64
	maxBackup int
}

func NewXrayLogRotateJob() *XrayLogRotateJob {
	return &XrayLogRotateJob{
		maxBytes:  maxXrayLogFileBytes,
		maxBackup: maxXrayRotatedLogBackups,
	}
}

func (j *XrayLogRotateJob) Run() {
	paths, err := xray.GetConfiguredLogPaths()
	if err != nil {
		logger.Warning("Failed to read Xray log paths:", err)
		return
	}

	for _, path := range paths {
		if !filepath.IsAbs(path) {
			continue
		}
		if err := rotateLogFileIfNeeded(path, j.maxBytes, j.maxBackup); err != nil {
			logger.Warning("Failed to rotate Xray log file:", path, "-", err)
		}
	}
}

func rotateLogFileIfNeeded(path string, maxBytes int64, maxBackup int) error {
	if maxBytes <= 0 || maxBackup <= 0 {
		return nil
	}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() || info.Size() < maxBytes {
		return nil
	}

	return copyTruncateLogFile(path, maxBackup)
}

func copyTruncateLogFile(path string, maxBackup int) error {
	for i := maxBackup; i >= 1; i-- {
		current := rotatedLogPath(path, i)
		if i == maxBackup {
			if err := os.Remove(current); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}

		next := rotatedLogPath(path, i+1)
		if err := os.Rename(current, next); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(rotatedLogPath(path, 1), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err = io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	if err = dst.Close(); err != nil {
		return err
	}

	return os.Truncate(path, 0)
}

func rotatedLogPath(path string, n int) string {
	return path + "." + strconv.Itoa(n)
}
