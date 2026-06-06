package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mhsanaei/3x-ui/v3/config"
	"github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/web/global"
)

// PanelService provides business logic for panel management operations.
// It handles panel restart, updates, and system-level panel controls.
type PanelService struct{}

// PanelUpdateInfo contains the current and latest available panel versions.
type PanelUpdateInfo struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

// PendingPanelUpdateNotice remembers a Telegram-triggered update across the panel restart.
type PendingPanelUpdateNotice struct {
	ChatID        int64  `json:"chatId"`
	TargetVersion string `json:"targetVersion"`
	RequestedAt   int64  `json:"requestedAt"`
}

const (
	panelUpdaterURL      = "https://raw.githubusercontent.com/tpxcer/oui/main/update.sh"
	maxPanelUpdaterBytes = 2 << 20
	panelUpdateNotice    = "panel-update-pending.json"
)

func (s *PanelService) RestartPanel(delay time.Duration) error {
	go func() {
		time.Sleep(delay)
		if global.TriggerRestart() {
			return
		}
		if runtime.GOOS == "windows" {
			logger.Error("panel restart: no restart hook registered (SIGHUP unsupported on Windows)")
			return
		}
		p, err := os.FindProcess(syscall.Getpid())
		if err != nil {
			logger.Error("panel restart: FindProcess failed:", err)
			return
		}
		if err := p.Signal(syscall.SIGHUP); err != nil {
			logger.Error("failed to send SIGHUP signal:", err)
		}
	}()
	return nil
}

// GetUpdateInfo checks GitHub for the latest OUI release.
func (s *PanelService) GetUpdateInfo() (*PanelUpdateInfo, error) {
	latest, err := fetchLatestPanelVersion()
	if err != nil {
		return nil, err
	}
	current := config.GetVersion()
	return &PanelUpdateInfo{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: isNewerVersion(latest, current),
	}, nil
}

// StartUpdate starts the official updater outside of the current web request.
func (s *PanelService) StartUpdate() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("面板网页更新仅支持 Linux 安装环境")
	}

	latest, err := fetchLatestPanelVersion()
	if err != nil {
		return fmt.Errorf("检查最新版本失败: %w", err)
	}
	if !isNewerVersion(latest, config.GetVersion()) {
		return fmt.Errorf("当前已是最新版本: %s", config.GetVersion())
	}

	bash, err := exec.LookPath("bash")
	if err != nil {
		return fmt.Errorf("运行面板更新器需要 bash: %w", err)
	}

	scriptPath, err := downloadPanelUpdater()
	if err != nil {
		return err
	}

	mainFolder, serviceFolder := resolveUpdateFolders()
	updateScript := fmt.Sprintf("set -e; trap 'rm -f %s' EXIT; %s %s %s", shellQuote(scriptPath), shellQuote(bash), shellQuote(scriptPath), shellQuote(latest))

	if systemdRun, err := exec.LookPath("systemd-run"); err == nil {
		unitName := fmt.Sprintf("x-ui-web-update-%d", time.Now().Unix())
		cmd := exec.Command(systemdRun,
			"--unit", unitName,
			"--setenv", "XUI_MAIN_FOLDER="+mainFolder,
			"--setenv", "XUI_SERVICE="+serviceFolder,
			bash, "-c", updateScript,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			output := strings.TrimSpace(string(out))
			if !strings.Contains(output, "System has not been booted with systemd") &&
				!strings.Contains(output, "Failed to connect to bus") {
				_ = os.Remove(scriptPath)
				return fmt.Errorf("启动面板更新任务失败: %w: %s", err, output)
			}
			logger.Warning("systemd-run is unavailable, falling back to detached update process:", output)
		} else {
			logger.Infof("started panel update job via systemd-run unit %s", unitName)
			return nil
		}
	}

	cmd := exec.Command(bash, "-c", updateScript)
	cmd.Env = append(os.Environ(),
		"XUI_MAIN_FOLDER="+mainFolder,
		"XUI_SERVICE="+serviceFolder,
	)
	setDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("启动面板更新任务失败: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		logger.Warning("failed to release panel update process:", err)
	}
	logger.Infof("started panel update job with pid %d", cmd.Process.Pid)
	return nil
}

// SavePendingUpdateNotice persists the Telegram chat that should receive the success notice.
func (s *PanelService) SavePendingUpdateNotice(chatID int64, targetVersion string) error {
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		return fmt.Errorf("目标版本为空")
	}

	folder := config.GetDBFolderPath()
	if err := os.MkdirAll(folder, 0700); err != nil {
		return fmt.Errorf("创建更新通知目录失败: %w", err)
	}

	notice := PendingPanelUpdateNotice{
		ChatID:        chatID,
		TargetVersion: targetVersion,
		RequestedAt:   time.Now().Unix(),
	}
	payload, err := json.MarshalIndent(notice, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	tmp, err := os.CreateTemp(folder, ".panel-update-pending-*.json")
	if err != nil {
		return fmt.Errorf("创建更新通知临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(payload); err != nil {
		return fmt.Errorf("写入更新通知失败: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, panelUpdateNoticePath()); err != nil {
		return fmt.Errorf("保存更新通知失败: %w", err)
	}
	ok = true
	return nil
}

// GetPendingUpdateNotice loads a pending Telegram update success notice, if one exists.
func (s *PanelService) GetPendingUpdateNotice() (*PendingPanelUpdateNotice, error) {
	payload, err := os.ReadFile(panelUpdateNoticePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var notice PendingPanelUpdateNotice
	if err := json.Unmarshal(payload, &notice); err != nil {
		return nil, err
	}
	if strings.TrimSpace(notice.TargetVersion) == "" {
		return nil, fmt.Errorf("更新通知目标版本为空")
	}
	return &notice, nil
}

// ClearPendingUpdateNotice removes a pending Telegram update success notice.
func (s *PanelService) ClearPendingUpdateNotice() error {
	err := os.Remove(panelUpdateNoticePath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func panelUpdateNoticePath() string {
	return filepath.Join(config.GetDBFolderPath(), panelUpdateNotice)
}

func panelUpdateNoticeReached(targetVersion string, currentVersion string) bool {
	targetVersion = strings.TrimSpace(targetVersion)
	currentVersion = strings.TrimSpace(currentVersion)
	if targetVersion == "" || currentVersion == "" {
		return false
	}
	if normalizeVersionTag(targetVersion) == normalizeVersionTag(currentVersion) {
		return true
	}
	return !isNewerVersion(targetVersion, currentVersion)
}

func downloadPanelUpdater() (string, error) {
	client := (&SettingService{}).NewProxiedHTTPClient(15 * time.Second)
	resp, err := client.Get(panelUpdaterURL)
	if err != nil {
		return "", fmt.Errorf("下载面板更新器失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载面板更新器失败: HTTP %d", resp.StatusCode)
	}

	file, err := os.CreateTemp("", "oui-update-*.sh")
	if err != nil {
		return "", err
	}
	path := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()

	n, err := io.Copy(file, io.LimitReader(resp.Body, maxPanelUpdaterBytes+1))
	if err != nil {
		return "", fmt.Errorf("写入面板更新器失败: %w", err)
	}
	if n > maxPanelUpdaterBytes {
		return "", fmt.Errorf("面板更新器超过 %d 字节", maxPanelUpdaterBytes)
	}
	if err := file.Chmod(0700); err != nil {
		return "", err
	}
	ok = true
	return path, nil
}

func fetchLatestPanelVersion() (string, error) {
	client := (&SettingService{}).NewProxiedHTTPClient(10 * time.Second)
	resp, err := client.Get("https://api.github.com/repos/tpxcer/oui/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 返回状态 %d: %s", resp.StatusCode, resp.Status)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("最新面板发布标签为空")
	}
	return release.TagName, nil
}

func resolveUpdateFolders() (string, string) {
	mainFolder := os.Getenv("XUI_MAIN_FOLDER")
	if mainFolder == "" {
		if exePath, err := os.Executable(); err == nil {
			mainFolder = filepath.Dir(exePath)
		}
	}
	if mainFolder == "" {
		mainFolder = "/usr/local/x-ui"
	}

	serviceFolder := os.Getenv("XUI_SERVICE")
	if serviceFolder == "" {
		serviceFolder = "/etc/systemd/system"
	}
	return mainFolder, serviceFolder
}

func isNewerVersion(latest string, current string) bool {
	cmp, ok := compareVersionStrings(latest, current)
	if !ok {
		return normalizeVersionTag(latest) != normalizeVersionTag(current)
	}
	return cmp > 0
}

func compareVersionStrings(a string, b string) (int, bool) {
	aParts, okA := parseVersionParts(a)
	bParts, okB := parseVersionParts(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range len(aParts) {
		if aParts[i] > bParts[i] {
			return 1, true
		}
		if aParts[i] < bParts[i] {
			return -1, true
		}
	}
	return 0, true
}

func parseVersionParts(version string) ([4]int, bool) {
	var result [4]int
	clean := normalizeVersionTag(version)
	if clean == "" {
		return result, false
	}
	if base, suffix, ok := strings.Cut(clean, "-"); ok {
		clean = base
		n, err := strconv.Atoi(suffix)
		if err != nil {
			return result, false
		}
		result[3] = n
	}
	parts := strings.Split(clean, ".")
	if len(parts) != 3 {
		return result, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return result, false
		}
		result[i] = n
	}
	return result, true
}

func normalizeVersionTag(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
