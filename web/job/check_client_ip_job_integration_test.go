package job

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
	xuilogger "github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/web/service"
	"github.com/mhsanaei/3x-ui/v3/xray"
	"github.com/op/go-logging"
)

// OUI logger must be initialised once before any code path that can
// log a warning. otherwise log.Warningf panics on a nil logger.
var loggerInitOnce sync.Once

// setupIntegrationDB wires a temp sqlite db and log folder so
// updateInboundClientIps can run end to end. closes the db before
// TempDir cleanup so windows doesn't complain about the file being in
// use.
func setupIntegrationDB(t *testing.T) {
	t.Helper()

	loggerInitOnce.Do(func() {
		xuilogger.InitLogger(logging.ERROR)
	})

	dbDir := t.TempDir()
	logDir := t.TempDir()

	t.Setenv("XUI_DB_FOLDER", dbDir)
	t.Setenv("XUI_LOG_FOLDER", logDir)

	// updateInboundClientIps calls log.SetOutput on the package global,
	// which would leak to other tests in the same binary.
	origLogWriter := log.Writer()
	origLogFlags := log.Flags()
	t.Cleanup(func() {
		log.SetOutput(origLogWriter)
		log.SetFlags(origLogFlags)
	})

	if err := database.InitDB(filepath.Join(dbDir, "3x-ui.db")); err != nil {
		t.Fatalf("database.InitDB failed: %v", err)
	}
	// LIFO cleanup order: this runs before t.TempDir's own cleanup.
	t.Cleanup(func() {
		if err := database.CloseDB(); err != nil {
			t.Logf("database.CloseDB warning: %v", err)
		}
	})
}

// seed an inbound whose settings json has a single client with the
// given email and ip limit.
func seedInboundWithClient(t *testing.T, tag, email string, limitIp int) {
	t.Helper()
	seedInboundWithClientOptions(t, inboundSeedOptions{
		Tag:     tag,
		Email:   email,
		LimitIP: limitIp,
		Enable:  true,
		Port:    4321,
	})
}

type inboundSeedOptions struct {
	Tag           string
	Email         string
	LimitIP       int
	Enable        bool
	ClientEnabled bool
	Port          int
	NodeID        *int
}

func seedInboundWithClientOptions(t *testing.T, opts inboundSeedOptions) {
	t.Helper()
	if opts.Port == 0 {
		opts.Port = 4321
	}
	clientEnabled := opts.ClientEnabled
	if !clientEnabled {
		clientEnabled = true
	}
	settings := map[string]any{
		"clients": []map[string]any{
			{
				"email":   opts.Email,
				"limitIp": opts.LimitIP,
				"enable":  clientEnabled,
			},
		},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	inbound := &model.Inbound{
		Tag:      opts.Tag,
		Enable:   opts.Enable,
		Protocol: model.VLESS,
		Port:     opts.Port,
		NodeID:   opts.NodeID,
		Settings: string(settingsJSON),
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
}

// seed an InboundClientIps row with the given blob.
func seedClientIps(t *testing.T, email string, ips []IPWithTimestamp) *model.InboundClientIps {
	t.Helper()
	blob, err := json.Marshal(ips)
	if err != nil {
		t.Fatalf("marshal ips: %v", err)
	}
	row := &model.InboundClientIps{
		ClientEmail: email,
		Ips:         string(blob),
	}
	if err := database.GetDB().Create(row).Error; err != nil {
		t.Fatalf("seed InboundClientIps: %v", err)
	}
	return row
}

// read the persisted blob and parse it back.
func readClientIps(t *testing.T, email string) []IPWithTimestamp {
	t.Helper()
	row := &model.InboundClientIps{}
	if err := database.GetDB().Where("client_email = ?", email).First(row).Error; err != nil {
		t.Fatalf("read InboundClientIps for %s: %v", email, err)
	}
	if row.Ips == "" {
		return nil
	}
	var out []IPWithTimestamp
	if err := json.Unmarshal([]byte(row.Ips), &out); err != nil {
		t.Fatalf("unmarshal Ips blob %q: %v", row.Ips, err)
	}
	return out
}

// make a lookup map so asserts don't depend on slice order.
func ipSet(entries []IPWithTimestamp) map[string]int64 {
	out := make(map[string]int64, len(entries))
	for _, e := range entries {
		out[e.IP] = e.Timestamp
	}
	return out
}

// A fresh historical IP still reserves one of the configured IP slots.
// With limit=1, a different IP that appears later is the newcomer and
// must be banned even if the original IP did not emit a log line in the
// same scan.
func TestUpdateInboundClientIps_NewIpBannedByFreshHistoricalOriginal(t *testing.T) {
	setupIntegrationDB(t)

	const email = "ip-limit-one"
	seedInboundWithClient(t, "inbound-ip-limit-one", email, 1)

	now := time.Now().Unix()
	row := seedClientIps(t, email, []IPWithTimestamp{
		{IP: "10.0.0.1", Timestamp: now - 10*60},
	})

	j := NewCheckClientIpJob()
	live := []IPWithTimestamp{
		{IP: "192.0.2.9", Timestamp: now},
	}

	shouldCleanLog := j.updateInboundClientIps(row, email, live)

	if !shouldCleanLog {
		t.Fatalf("shouldCleanLog must be true when a new IP exceeds the configured limit")
	}
	if len(j.disAllowedIps) != 1 || j.disAllowedIps[0] != "192.0.2.9" {
		t.Fatalf("expected 192.0.2.9 to be banned; disAllowedIps = %v", j.disAllowedIps)
	}

	persisted := ipSet(readClientIps(t, email))
	if _, ok := persisted["10.0.0.1"]; !ok {
		t.Errorf("original IP 10.0.0.1 must still be persisted; got %v", persisted)
	}
	if _, ok := persisted["192.0.2.9"]; ok {
		t.Errorf("banned IP 192.0.2.9 must NOT be persisted; got %v", persisted)
	}

	body, err := os.ReadFile(readIpLimitLogPath())
	if err != nil {
		t.Fatalf("read 3xipl.log: %v", err)
	}
	wantSubstr := "[LIMIT_IP] Email = ip-limit-one || Port = 4321 || Disconnecting OLD IP = 192.0.2.9"
	if !contains(string(body), wantSubstr) {
		t.Fatalf("3xipl.log missing expected ban line %q\nfull log:\n%s", wantSubstr, body)
	}
}

// opposite invariant: when several ips are actually live and exceed
// the limit, the newcomer still gets banned.
func TestUpdateInboundClientIps_ExcessLiveIpIsStillBanned(t *testing.T) {
	setupIntegrationDB(t)

	const email = "pr4091-abuse"
	seedInboundWithClient(t, "inbound-pr4091-abuse", email, 1)

	now := time.Now().Unix()
	row := seedClientIps(t, email, []IPWithTimestamp{
		{IP: "10.1.0.1", Timestamp: now - 60}, // original connection
	})

	j := NewCheckClientIpJob()
	// both live, limit=1. use distinct timestamps so sort-by-timestamp
	// is deterministic: 10.1.0.1 is the original (older), 192.0.2.9
	// joined later and must get banned.
	live := []IPWithTimestamp{
		{IP: "10.1.0.1", Timestamp: now - 5},
		{IP: "192.0.2.9", Timestamp: now},
	}

	shouldCleanLog := j.updateInboundClientIps(row, email, live)

	if !shouldCleanLog {
		t.Fatalf("shouldCleanLog must be true when the live set exceeds the limit")
	}
	if len(j.disAllowedIps) != 1 || j.disAllowedIps[0] != "192.0.2.9" {
		t.Fatalf("expected 192.0.2.9 to be banned; disAllowedIps = %v", j.disAllowedIps)
	}

	persisted := ipSet(readClientIps(t, email))
	if _, ok := persisted["10.1.0.1"]; !ok {
		t.Errorf("original IP 10.1.0.1 must still be persisted; got %v", persisted)
	}
	if _, ok := persisted["192.0.2.9"]; ok {
		t.Errorf("banned IP 192.0.2.9 must NOT be persisted; got %v", persisted)
	}

	// 3xipl.log must contain the ban line in the exact fail2ban format.
	body, err := os.ReadFile(readIpLimitLogPath())
	if err != nil {
		t.Fatalf("read 3xipl.log: %v", err)
	}
	wantSubstr := "[LIMIT_IP] Email = pr4091-abuse || Port = 4321 || Disconnecting OLD IP = 192.0.2.9"
	if !contains(string(body), wantSubstr) {
		t.Fatalf("3xipl.log missing expected ban line %q\nfull log:\n%s", wantSubstr, body)
	}
}

func TestUpdateInboundClientIps_AlreadyBannedExcessIpDoesNotNotifyAgain(t *testing.T) {
	setupIntegrationDB(t)

	const email = "already-banned-excess"
	seedInboundWithClient(t, "inbound-already-banned-excess", email, 1)

	now := time.Now().Unix()
	row := seedClientIps(t, email, []IPWithTimestamp{
		{IP: "10.1.0.1", Timestamp: now - 60},
	})
	writeIPLimitBannedLog(t, "2026/06/05 10:00:00 BAN   [Email] = already-banned-excess [Port] = 4321 [IP] = 192.0.2.9 exceeded IP limit.\n")

	j := NewCheckClientIpJob()
	live := []IPWithTimestamp{
		{IP: "10.1.0.1", Timestamp: now - 5},
		{IP: "192.0.2.9", Timestamp: now},
	}

	shouldCleanLog := j.updateInboundClientIps(row, email, live)

	if shouldCleanLog {
		t.Fatalf("already-banned excess IP must not trigger another notification/log cleanup")
	}
	if len(j.disAllowedIps) != 0 {
		t.Fatalf("already-banned excess IP must not be queued again, got %v", j.disAllowedIps)
	}

	persisted := ipSet(readClientIps(t, email))
	if _, ok := persisted["10.1.0.1"]; !ok {
		t.Errorf("original IP 10.1.0.1 must still be persisted; got %v", persisted)
	}
	if _, ok := persisted["192.0.2.9"]; ok {
		t.Errorf("already-banned IP 192.0.2.9 must NOT be persisted as allowed; got %v", persisted)
	}

	body := readIPLimitBannedLog(t)
	if got := strings.Count(body, "BAN   [Email] = already-banned-excess [Port] = 4321 [IP] = 192.0.2.9"); got != 1 {
		t.Fatalf("expected no repeated BAN entries, got %d\nfull log:\n%s", got, body)
	}
	if body, err := os.ReadFile(readIpLimitLogPath()); err == nil && strings.Contains(string(body), "Disconnecting OLD IP = 192.0.2.9") {
		t.Fatalf("3xipl.log should not get a repeated ban line\nfull log:\n%s", string(body))
	}
}

func TestUpdateInboundClientIps_TemporaryUnbanSkipsRebanWithoutChangingLimit(t *testing.T) {
	setupIntegrationDB(t)

	const email = "temporary-unban"
	seedInboundWithClient(t, "inbound-temporary-unban", email, 1)

	now := time.Now().Unix()
	row := seedClientIps(t, email, []IPWithTimestamp{
		{IP: "10.1.0.1", Timestamp: now - 60},
	})
	inboundSvc := service.InboundService{}
	if err := inboundSvc.MarkClientIPLimitTemporaryUnban(email, "192.0.2.9", 4321, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("mark temporary unban: %v", err)
	}
	t.Cleanup(func() {
		inboundSvc.ClearClientIPLimitTemporaryUnban(email, "192.0.2.9", 4321)
	})

	j := NewCheckClientIpJob()
	live := []IPWithTimestamp{
		{IP: "10.1.0.1", Timestamp: now - 5},
		{IP: "192.0.2.9", Timestamp: now},
	}

	shouldCleanLog := j.updateInboundClientIps(row, email, live)
	if shouldCleanLog {
		t.Fatalf("temporary-unbanned IP must not trigger a new ban")
	}
	if len(j.disAllowedIps) != 0 {
		t.Fatalf("expected no banned IPs while temporary unban is active, got %v", j.disAllowedIps)
	}

	persisted := ipSet(readClientIps(t, email))
	if _, ok := persisted["192.0.2.9"]; !ok {
		t.Fatalf("temporary-unbanned IP should stay persisted while allowed, got %v", persisted)
	}

	var inbound model.Inbound
	if err := database.GetDB().Where("port = ?", 4321).First(&inbound).Error; err != nil {
		t.Fatalf("read inbound: %v", err)
	}
	settings := map[string][]model.Client{}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if settings["clients"][0].LimitIP != 1 {
		t.Fatalf("temporary unban must not change panel limitIp, settings=%s", inbound.Settings)
	}
}

func TestUpdateInboundClientIps_ExactEmailDoesNotMatchLongerEmail(t *testing.T) {
	setupIntegrationDB(t)

	seedInboundWithClientOptions(t, inboundSeedOptions{
		Tag:     "inbound-user-a-plus",
		Email:   "user-a-plus",
		LimitIP: 1,
		Enable:  true,
		Port:    60401,
	})
	seedInboundWithClientOptions(t, inboundSeedOptions{
		Tag:     "inbound-user-a",
		Email:   "user-a",
		LimitIP: 1,
		Enable:  true,
		Port:    54883,
	})

	now := time.Now().Unix()
	row := seedClientIps(t, "user-a", []IPWithTimestamp{
		{IP: "182.143.200.102", Timestamp: now - ipStaleAfterSeconds - 60},
	})

	j := NewCheckClientIpJob()
	shouldCleanLog := j.updateInboundClientIps(row, "user-a", []IPWithTimestamp{
		{IP: "182.143.200.102", Timestamp: now - ipStaleAfterSeconds - 60},
		{IP: "118.114.148.17", Timestamp: now},
	})

	if shouldCleanLog {
		t.Fatal("stale longer-email regression case should not trigger a ban")
	}

	got := ipSet(readClientIps(t, "user-a"))
	if _, ok := got["182.143.200.102"]; ok {
		t.Fatalf("stale IP from exact-email client should be evicted, got %v", got)
	}
	if got["118.114.148.17"] != now {
		t.Fatalf("fresh exact-email IP should remain with timestamp %d, got %v", now, got)
	}

	inbound, err := j.getInboundByEmail("user-a")
	if err != nil {
		t.Fatalf("get inbound by exact email: %v", err)
	}
	if inbound.Port != 54883 {
		t.Fatalf("short email user-a must not match user-a-plus inbound first, got port %d", inbound.Port)
	}
}

func TestSyncClientIPLimitBans_LimitZeroUnbansImmediately(t *testing.T) {
	setupIntegrationDB(t)

	const email = "limit-zero"
	seedInboundWithClientOptions(t, inboundSeedOptions{
		Tag:     "inbound-limit-zero",
		Email:   email,
		LimitIP: 0,
		Enable:  true,
		Port:    4321,
	})
	seedClientIps(t, email, []IPWithTimestamp{{IP: "10.0.0.1", Timestamp: 1000}})
	writeIPLimitBannedLog(t, "2026/06/05 10:00:00 BAN   [Email] = limit-zero [Port] = 4321 [IP] = 192.0.2.9 exceeded IP limit.\n")

	inboundSvc := service.InboundService{}
	if err := inboundSvc.SyncClientIPLimitBansByEmail(email); err != nil {
		t.Fatalf("sync bans: %v", err)
	}

	body := readIPLimitBannedLog(t)
	if !contains(body, "UNBAN   [Email] = limit-zero [Port] = 4321 [IP] = 192.0.2.9 automatic IP limit sync.") {
		t.Fatalf("expected automatic unban after limit=0\nfull log:\n%s", body)
	}
}

func TestSyncClientIPLimitBans_OfflineNodeUnbans(t *testing.T) {
	setupIntegrationDB(t)

	node := &model.Node{
		Name:     "remote-offline",
		Address:  "example.com",
		Port:     443,
		Scheme:   "https",
		ApiToken: "token",
		Enable:   true,
		Status:   "offline",
	}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}

	const email = "offline-node"
	seedInboundWithClientOptions(t, inboundSeedOptions{
		Tag:     "inbound-offline-node",
		Email:   email,
		LimitIP: 1,
		Enable:  true,
		Port:    4321,
		NodeID:  &node.Id,
	})
	seedClientIps(t, email, []IPWithTimestamp{{IP: "10.0.0.1", Timestamp: 1000}})
	writeIPLimitBannedLog(t, "2026/06/05 10:00:00 BAN   [Email] = offline-node [Port] = 4321 [IP] = 192.0.2.9 exceeded IP limit.\n")

	inboundSvc := service.InboundService{}
	if err := inboundSvc.SyncClientIPLimitBansByEmail(email); err != nil {
		t.Fatalf("sync bans: %v", err)
	}

	body := readIPLimitBannedLog(t)
	if !contains(body, "UNBAN   [Email] = offline-node [Port] = 4321 [IP] = 192.0.2.9 automatic IP limit sync.") {
		t.Fatalf("expected automatic unban for offline node\nfull log:\n%s", body)
	}
}

func TestSyncClientIPLimitBans_LimitIncreaseUnbansByFirstSeenTime(t *testing.T) {
	setupIntegrationDB(t)

	const email = "limit-increase"
	seedInboundWithClientOptions(t, inboundSeedOptions{
		Tag:     "inbound-limit-increase",
		Email:   email,
		LimitIP: 2,
		Enable:  true,
		Port:    4321,
	})
	seedClientIps(t, email, []IPWithTimestamp{{IP: "10.0.0.1", Timestamp: 1000}})
	writeIPLimitLog(t,
		"2026/06/05 10:00:00 [LIMIT_IP] Email = limit-increase || Port = 4321 || Disconnecting OLD IP = 192.0.2.9 || Timestamp = 1100\n"+
			"2026/06/05 10:00:01 [LIMIT_IP] Email = limit-increase || Port = 4321 || Disconnecting OLD IP = 192.0.2.10 || Timestamp = 1200\n",
	)

	inboundSvc := service.InboundService{}
	if err := inboundSvc.SyncClientIPLimitBansByEmail(email); err != nil {
		t.Fatalf("sync bans: %v", err)
	}

	body := readIPLimitBannedLog(t)
	if !contains(body, "UNBAN   [Email] = limit-increase [Port] = 4321 [IP] = 192.0.2.9 automatic IP limit sync.") {
		t.Fatalf("expected earlier banned IP to be unbanned after limit increase\nfull log:\n%s", body)
	}
	if contains(body, "UNBAN   [Email] = limit-increase [Port] = 4321 [IP] = 192.0.2.10 automatic IP limit sync.") {
		t.Fatalf("newer banned IP should remain blocked while limit=2\nfull log:\n%s", body)
	}
}

func TestUnbanClientIPLimitByEmail_IgnoresAlreadyUnbannedTargets(t *testing.T) {
	setupIntegrationDB(t)

	const email = "already-unbanned"
	seedInboundWithClientOptions(t, inboundSeedOptions{
		Tag:     "inbound-already-unbanned",
		Email:   email,
		LimitIP: 1,
		Enable:  true,
		Port:    4321,
	})
	writeIPLimitBannedLog(t,
		"2026/06/05 10:00:00 BAN   [Email] = already-unbanned [Port] = 4321 [IP] = 192.0.2.9 exceeded IP limit.\n"+
			"2026/06/05 10:00:01 UNBAN   [Email] = already-unbanned [Port] = 4321 [IP] = 192.0.2.9 manual firewall rule.\n",
	)

	inboundSvc := service.InboundService{}
	if err := inboundSvc.UnbanClientIPLimitByEmail(email); err != nil {
		t.Fatalf("unban by email: %v", err)
	}

	body := readIPLimitBannedLog(t)
	if got := strings.Count(body, "UNBAN   [Email] = already-unbanned [Port] = 4321 [IP] = 192.0.2.9"); got != 1 {
		t.Fatalf("expected no repeated UNBAN entries, got %d\nfull log:\n%s", got, body)
	}
}

func TestIncrementClientIPLimitByEmailAndPort_UpdatesInboundSettingsAndClientRecord(t *testing.T) {
	setupIntegrationDB(t)

	const email = "tg-unban-increment"
	seedInboundWithClientOptions(t, inboundSeedOptions{
		Tag:     "inbound-tg-unban-increment",
		Email:   email,
		LimitIP: 2,
		Enable:  true,
		Port:    4321,
	})
	if err := database.GetDB().Create(&model.ClientRecord{
		Email:   email,
		LimitIP: 2,
		Enable:  true,
	}).Error; err != nil {
		t.Fatalf("seed client record: %v", err)
	}

	inboundSvc := service.InboundService{}
	result, err := inboundSvc.IncrementClientIPLimitByEmailAndPort(email, 4321)
	if err != nil {
		t.Fatalf("increment limit: %v", err)
	}
	if result.OldLimit != 2 || result.NewLimit != 3 || result.Port != 4321 {
		t.Fatalf("unexpected increment result: %+v", result)
	}

	var inbound model.Inbound
	if err := database.GetDB().Where("port = ?", 4321).First(&inbound).Error; err != nil {
		t.Fatalf("read inbound: %v", err)
	}
	settings := map[string][]model.Client{}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if len(settings["clients"]) != 1 || settings["clients"][0].LimitIP != 3 {
		t.Fatalf("expected inbound client limitIp=3, settings=%s", inbound.Settings)
	}

	var record model.ClientRecord
	if err := database.GetDB().Where("email = ?", email).First(&record).Error; err != nil {
		t.Fatalf("read client record: %v", err)
	}
	if record.LimitIP != 3 {
		t.Fatalf("expected client record limit_ip=3, got %d", record.LimitIP)
	}
}

// readIpLimitLogPath reads the 3xipl.log path the same way the job does.
func readIpLimitLogPath() string {
	return xray.GetIPLimitLogPath()
}

func readIPLimitBannedLogPath() string {
	return xray.GetIPLimitBannedLogPath()
}

func writeIPLimitLog(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(readIpLimitLogPath(), []byte(body), 0o644); err != nil {
		t.Fatalf("write 3xipl.log: %v", err)
	}
}

func writeIPLimitBannedLog(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(readIPLimitBannedLogPath(), []byte(body), 0o644); err != nil {
		t.Fatalf("write 3xipl-banned.log: %v", err)
	}
}

func readIPLimitBannedLog(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(readIPLimitBannedLogPath())
	if err != nil {
		t.Fatalf("read 3xipl-banned.log: %v", err)
	}
	return string(body)
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
