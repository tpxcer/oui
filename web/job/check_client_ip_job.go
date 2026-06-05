package job

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
	"github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/web/service"
	"github.com/mhsanaei/3x-ui/v3/xray"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// IPWithTimestamp tracks an IP address with its last seen timestamp
type IPWithTimestamp struct {
	IP        string `json:"ip"`
	Timestamp int64  `json:"timestamp"`
}

// CheckClientIpJob monitors client IP addresses from access logs and manages IP blocking based on configured limits.
type CheckClientIpJob struct {
	lastClear     int64
	disAllowedIps []string
	tgbotService  service.Tgbot
}

var job *CheckClientIpJob

// ipStaleAfterSeconds controls how long a client IP kept in the
// per-client tracking table (model.InboundClientIps.Ips) is considered
// still "active" before it's evicted during the next scan.
//
// Without this eviction, an IP that connected once and then went away
// keeps sitting in the table with its old timestamp. Because the
// excess-IP selector sorts ascending ("oldest wins, newest loses") to
// protect the original/current connections, that stale entry keeps
// occupying a slot and the IP that is *actually* currently using the
// config gets classified as "new excess" and banned by fail2ban on
// every single run — producing the continuous ban loop from #4077.
//
// 30 minutes is chosen so an actively-streaming client (where xray
// emits a fresh `accepted` log line whenever it opens a new TCP) will
// always refresh its timestamp well within the window, but a client
// that has really stopped using the config will drop out of the table
// in a bounded time and free its slot.
const ipStaleAfterSeconds = int64(30 * 60)

// NewCheckClientIpJob creates a new client IP monitoring job instance.
func NewCheckClientIpJob() *CheckClientIpJob {
	job = new(CheckClientIpJob)
	return job
}

func (j *CheckClientIpJob) Run() {
	if j.lastClear == 0 {
		j.lastClear = time.Now().Unix()
	}

	j.syncStoredIPLimitBans()

	shouldClearAccessLog := false
	iplimitActive := j.hasLimitIp()
	f2bInstalled := j.checkFail2BanInstalled()
	isAccessLogAvailable := j.checkAccessLogAvailable(iplimitActive)

	if isAccessLogAvailable {
		if runtime.GOOS == "windows" {
			if iplimitActive {
				shouldClearAccessLog = j.processLogFile()
			}
		} else {
			if iplimitActive {
				if !f2bInstalled {
					logger.Warning("[LimitIP] Fail2Ban is not installed, IP limit can disconnect users but cannot firewall-ban excess IPs.")
				}
				shouldClearAccessLog = j.processLogFile()
			}
		}
	}

	if shouldClearAccessLog || (isAccessLogAvailable && time.Now().Unix()-j.lastClear > 3600) {
		j.clearAccessLog()
	}
}

func (j *CheckClientIpJob) clearAccessLog() {
	logAccessP, err := os.OpenFile(xray.GetAccessPersistentLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	j.checkError(err)
	defer logAccessP.Close()

	accessLogPath, err := xray.GetAccessLogPath()
	j.checkError(err)

	file, err := os.Open(accessLogPath)
	j.checkError(err)
	defer file.Close()

	_, err = io.Copy(logAccessP, file)
	j.checkError(err)

	err = os.Truncate(accessLogPath, 0)
	j.checkError(err)

	j.lastClear = time.Now().Unix()
}

func (j *CheckClientIpJob) hasLimitIp() bool {
	db := database.GetDB()
	var inbounds []*model.Inbound

	err := db.Model(model.Inbound{}).Find(&inbounds).Error
	if err != nil {
		return false
	}

	for _, inbound := range inbounds {
		if inbound.Settings == "" {
			continue
		}

		settings := map[string][]model.Client{}
		json.Unmarshal([]byte(inbound.Settings), &settings)
		clients := settings["clients"]

		for _, client := range clients {
			limitIp := client.LimitIP
			if limitIp > 0 {
				return true
			}
		}
	}

	return false
}

func (j *CheckClientIpJob) processLogFile() bool {

	ipRegex := regexp.MustCompile(`from (?:tcp:|udp:)?\[?([0-9a-fA-F\.:]+)\]?:\d+ accepted`)
	emailRegex := regexp.MustCompile(`email: (.+)$`)
	timestampRegex := regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})`)

	accessLogPath, _ := xray.GetAccessLogPath()
	file, _ := os.Open(accessLogPath)
	defer file.Close()

	// Track IPs with their last seen timestamp
	inboundClientIps := make(map[string]map[string]int64, 100)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		ipMatches := ipRegex.FindStringSubmatch(line)
		if len(ipMatches) < 2 {
			continue
		}

		ip := ipMatches[1]

		if ip == "127.0.0.1" || ip == "::1" {
			continue
		}

		emailMatches := emailRegex.FindStringSubmatch(line)
		if len(emailMatches) < 2 {
			continue
		}
		email := emailMatches[1]

		// Extract timestamp from log line
		var timestamp int64
		timestampMatches := timestampRegex.FindStringSubmatch(line)
		if len(timestampMatches) >= 2 {
			t, err := time.ParseInLocation("2006/01/02 15:04:05", timestampMatches[1], time.Local)
			if err == nil {
				timestamp = t.Unix()
			} else {
				timestamp = time.Now().Unix()
			}
		} else {
			timestamp = time.Now().Unix()
		}

		if _, exists := inboundClientIps[email]; !exists {
			inboundClientIps[email] = make(map[string]int64)
		}
		// Update timestamp - keep the latest
		if existingTime, ok := inboundClientIps[email][ip]; !ok || timestamp > existingTime {
			inboundClientIps[email][ip] = timestamp
		}
	}
	if err := scanner.Err(); err != nil {
		j.checkError(err)
	}

	shouldCleanLog := false
	for email, ipTimestamps := range inboundClientIps {

		// Convert to IPWithTimestamp slice
		ipsWithTime := make([]IPWithTimestamp, 0, len(ipTimestamps))
		for ip, timestamp := range ipTimestamps {
			ipsWithTime = append(ipsWithTime, IPWithTimestamp{IP: ip, Timestamp: timestamp})
		}

		clientIpsRecord, err := j.getInboundClientIps(email)
		if err != nil {
			j.addInboundClientIps(email, ipsWithTime)
			continue
		}

		shouldCleanLog = j.updateInboundClientIps(clientIpsRecord, email, ipsWithTime) || shouldCleanLog
	}

	return shouldCleanLog
}

// mergeClientIps combines the persisted (old) and freshly observed (new)
// IP-with-timestamp lists for a single client into a map. An entry is
// dropped if its last-seen timestamp is older than staleCutoff.
//
// Extracted as a helper so updateInboundClientIps can stay DB-oriented
// and the merge policy can be exercised by a unit test.
func mergeClientIps(old, new []IPWithTimestamp, staleCutoff int64) map[string]int64 {
	ipMap := make(map[string]int64, len(old)+len(new))
	for _, ipTime := range old {
		if ipTime.Timestamp < staleCutoff {
			continue
		}
		ipMap[ipTime.IP] = ipTime.Timestamp
	}
	for _, ipTime := range new {
		if ipTime.Timestamp < staleCutoff {
			continue
		}
		if existingTime, ok := ipMap[ipTime.IP]; !ok || ipTime.Timestamp > existingTime {
			ipMap[ipTime.IP] = ipTime.Timestamp
		}
	}
	return ipMap
}

// partitionLiveIps splits the merged ip map into live (seen in the
// current scan) and historical (only in the db blob, still inside the
// staleness window).
//
// only live ips count toward the per-client limit. historical ones stay
// in the db so the panel keeps showing them, but they must not take a
// protected slot. the 30min cutoff alone isn't tight enough: an ip that
// stopped connecting a few minutes ago still looks fresh to
// mergeClientIps, and since the over-limit picker sorts ascending and
// keeps the oldest, those idle entries used to win the slot while the
// ip actually connecting got classified as excess and sent to fail2ban
// every tick. see #4077 / #4091.
//
// live is sorted ascending so the "protect original, ban newcomer"
// rule still holds when several ips are really connecting at once.
func partitionLiveIps(ipMap map[string]int64, observedThisScan map[string]bool) (live, historical []IPWithTimestamp) {
	live = make([]IPWithTimestamp, 0, len(observedThisScan))
	historical = make([]IPWithTimestamp, 0, len(ipMap))
	for ip, ts := range ipMap {
		entry := IPWithTimestamp{IP: ip, Timestamp: ts}
		if observedThisScan[ip] {
			live = append(live, entry)
		} else {
			historical = append(historical, entry)
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i].Timestamp < live[j].Timestamp })
	sort.Slice(historical, func(i, j int) bool { return historical[i].Timestamp < historical[j].Timestamp })
	return live, historical
}

func sortedIps(ipMap map[string]int64) []IPWithTimestamp {
	ips := make([]IPWithTimestamp, 0, len(ipMap))
	for ip, ts := range ipMap {
		ips = append(ips, IPWithTimestamp{IP: ip, Timestamp: ts})
	}
	sort.Slice(ips, func(i, k int) bool { return ips[i].Timestamp < ips[k].Timestamp })
	return ips
}

func selectIPLimitExcess(ipMap map[string]int64, liveIps []IPWithTimestamp, limitIp int) (keptLive, bannedLive []IPWithTimestamp) {
	if limitIp <= 0 {
		return liveIps, nil
	}

	allIps := sortedIps(ipMap)
	if len(allIps) <= limitIp {
		return liveIps, nil
	}

	protected := make(map[string]bool, limitIp)
	for _, ipTime := range allIps[:limitIp] {
		protected[ipTime.IP] = true
	}

	for _, ipTime := range liveIps {
		if protected[ipTime.IP] {
			keptLive = append(keptLive, ipTime)
		} else {
			bannedLive = append(bannedLive, ipTime)
		}
	}

	return keptLive, bannedLive
}

func toServiceIPLimitIPs(entries []IPWithTimestamp) []service.IPLimitIPWithTimestamp {
	out := make([]service.IPLimitIPWithTimestamp, 0, len(entries))
	for _, entry := range entries {
		out = append(out, service.IPLimitIPWithTimestamp{IP: entry.IP, Timestamp: entry.Timestamp})
	}
	return out
}

func (j *CheckClientIpJob) syncStoredIPLimitBans() {
	db := database.GetDB()
	var rows []model.InboundClientIps
	if err := db.Model(model.InboundClientIps{}).Where("client_email <> ''").Find(&rows).Error; err != nil {
		logger.Warning("[LIMIT_IP] failed to load stored client IP rows:", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	inboundSvc := service.InboundService{}
	for _, row := range rows {
		if err := inboundSvc.SyncClientIPLimitBansByEmail(row.ClientEmail); err != nil {
			logger.Warningf("[LIMIT_IP] failed to sync stored bans for %s: %v", row.ClientEmail, err)
		}
	}
}

func (j *CheckClientIpJob) checkFail2BanInstalled() bool {
	cmd := "fail2ban-client"
	args := []string{"-h"}
	err := exec.Command(cmd, args...).Run()
	return err == nil
}

func (j *CheckClientIpJob) checkAccessLogAvailable(iplimitActive bool) bool {
	accessLogPath, err := xray.GetAccessLogPath()
	if err != nil {
		return false
	}

	if accessLogPath == "none" || accessLogPath == "" {
		if iplimitActive {
			logger.Warning("[LimitIP] Access log path is not set, Please configure the access log path in Xray configs.")
		}
		return false
	}

	return true
}

func (j *CheckClientIpJob) checkError(e error) {
	if e != nil {
		logger.Warning("client ip job err:", e)
	}
}

func (j *CheckClientIpJob) getInboundClientIps(clientEmail string) (*model.InboundClientIps, error) {
	db := database.GetDB()
	InboundClientIps := &model.InboundClientIps{}
	err := db.Model(model.InboundClientIps{}).Where("client_email = ?", clientEmail).First(InboundClientIps).Error
	if err != nil {
		return nil, err
	}
	return InboundClientIps, nil
}

func (j *CheckClientIpJob) addInboundClientIps(clientEmail string, ipsWithTime []IPWithTimestamp) error {
	inboundClientIps := &model.InboundClientIps{}
	jsonIps, err := json.Marshal(ipsWithTime)
	j.checkError(err)

	inboundClientIps.ClientEmail = clientEmail
	inboundClientIps.Ips = string(jsonIps)

	db := database.GetDB()
	tx := db.Begin()

	defer func() {
		if err == nil {
			tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	err = tx.Save(inboundClientIps).Error
	if err != nil {
		return err
	}
	return nil
}

func (j *CheckClientIpJob) updateInboundClientIps(inboundClientIps *model.InboundClientIps, clientEmail string, newIpsWithTime []IPWithTimestamp) bool {
	// Get the inbound configuration
	inbound, err := j.getInboundByEmail(clientEmail)
	if err != nil {
		logger.Errorf("failed to fetch inbound settings for email %s: %s", clientEmail, err)
		return false
	}

	if inbound.Settings == "" {
		logger.Debug("wrong data:", inbound)
		return false
	}

	settings := map[string][]model.Client{}
	json.Unmarshal([]byte(inbound.Settings), &settings)
	clients := settings["clients"]

	// Find the client's IP limit
	var limitIp int
	var clientFound bool
	var clientEnabled bool
	for _, client := range clients {
		if client.Email == clientEmail {
			limitIp = client.LimitIP
			clientFound = true
			clientEnabled = client.Enable
			break
		}
	}

	// Parse old IPs from database before the early-exit path so the sync
	// step can release bans when the limit is disabled or the inbound/client
	// goes offline.
	var oldIpsWithTime []IPWithTimestamp
	if inboundClientIps.Ips != "" {
		json.Unmarshal([]byte(inboundClientIps.Ips), &oldIpsWithTime)
	}

	if !clientFound || limitIp <= 0 || !inbound.Enable || !clientEnabled {
		inboundSvc := service.InboundService{}
		if err := inboundSvc.SyncClientIPLimitBansForInbound(clientEmail, inbound, 0, toServiceIPLimitIPs(oldIpsWithTime)); err != nil {
			logger.Warningf("[LIMIT_IP] failed to release bans for %s: %v", clientEmail, err)
		}
		// No limit or inbound disabled, just update and return
		jsonIps, _ := json.Marshal(newIpsWithTime)
		inboundClientIps.Ips = string(jsonIps)
		db := database.GetDB()
		db.Save(inboundClientIps)
		return false
	}

	// Merge old and new IPs, evicting entries that haven't been
	// re-observed in a while. See mergeClientIps / #4077 for why.
	ipMap := mergeClientIps(oldIpsWithTime, newIpsWithTime, time.Now().Unix()-ipStaleAfterSeconds)

	// only ips seen in this scan count toward the limit. see
	// partitionLiveIps.
	observedThisScan := make(map[string]bool, len(newIpsWithTime))
	for _, ipTime := range newIpsWithTime {
		observedThisScan[ipTime.IP] = true
	}
	liveIps, historicalIps := partitionLiveIps(ipMap, observedThisScan)

	shouldCleanLog := false
	j.disAllowedIps = []string{}

	var keptLive []IPWithTimestamp
	var bannedLive []IPWithTimestamp
	keptLive, bannedLive = selectIPLimitExcess(ipMap, liveIps, limitIp)
	inboundSvc := service.InboundService{}
	if err := inboundSvc.SyncClientIPLimitBansForInbound(clientEmail, inbound, limitIp, toServiceIPLimitIPs(sortedIps(ipMap))); err != nil {
		logger.Warningf("[LIMIT_IP] failed to sync allowed bans for %s: %v", clientEmail, err)
	}
	if len(bannedLive) > 0 {
		shouldCleanLog = true

		// Open log file only when a ban entry needs to be written.
		// Use a local logger to avoid mutating the global log.* state,
		// which would redirect all standard-library logging to this file
		// and leave a dangling closed-file handle after the defer fires.
		logIpFile, err := os.OpenFile(xray.GetIPLimitLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			logger.Errorf("failed to open IP limit log file: %s", err)
			return false
		}
		defer logIpFile.Close()
		ipLogger := log.New(logIpFile, "", log.LstdFlags)

		// log format is load-bearing: x-ui.sh create_iplimit_jails builds
		// filter.d/3x-ipl.conf from this wording. Keep Port before IP so
		// Fail2Ban can block only this inbound port instead of all VPS ports.
		for _, ipTime := range bannedLive {
			j.disAllowedIps = append(j.disAllowedIps, ipTime.IP)
			ipLogger.Printf("[LIMIT_IP] Email = %s || Port = %d || Disconnecting OLD IP = %s || Timestamp = %d", clientEmail, inbound.Port, ipTime.IP, ipTime.Timestamp)
			if err := service.BanIPLimitPort(ipTime.IP, inbound.Port); err != nil {
				logger.Warningf("[LIMIT_IP] Failed to ban IP %s on port %d: %v", ipTime.IP, inbound.Port, err)
			} else {
				service.AppendIPLimitBanLog("BAN", clientEmail, ipTime.IP, inbound.Port, "exceeded IP limit")
			}
		}

		j.sendIPLimitCutoffNotify(clientEmail, inbound, limitIp, keptLive, bannedLive)

		// The firewall rule above cuts only the excess source IP on this
		// inbound port. Do not remove/re-add the Xray user here: that is a
		// client-level operation and can drop every device using this client.
	} else {
		keptLive = liveIps
	}

	// keep kept-live + historical in the blob so the panel keeps showing
	// recently seen ips. banned live ips are already in the fail2ban log
	// and will reappear in the next scan if they reconnect.
	dbIps := make([]IPWithTimestamp, 0, len(keptLive)+len(historicalIps))
	dbIps = append(dbIps, keptLive...)
	dbIps = append(dbIps, historicalIps...)
	jsonIps, _ := json.Marshal(dbIps)
	inboundClientIps.Ips = string(jsonIps)

	db := database.GetDB()
	err = db.Save(inboundClientIps).Error
	if err != nil {
		logger.Error("failed to save inboundClientIps:", err)
		return false
	}

	if len(j.disAllowedIps) > 0 {
		logger.Infof("[LIMIT_IP] Client %s: Kept %d live IPs, queued %d new IPs for fail2ban", clientEmail, len(keptLive), len(j.disAllowedIps))
	}

	return shouldCleanLog
}

func (j *CheckClientIpJob) sendIPLimitCutoffNotify(clientEmail string, inbound *model.Inbound, limitIp int, keptLive, bannedLive []IPWithTimestamp) {
	if len(bannedLive) == 0 {
		return
	}
	j.tgbotService.SendMsgToTgbotAdmins(
		buildIPLimitCutoffNotifyMessage(clientEmail, inbound, limitIp, keptLive, bannedLive, time.Now()),
		j.buildIPLimitCutoffKeyboard(clientEmail, inbound.Port, bannedLive),
	)
}

func buildIPLimitCutoffNotifyMessage(clientEmail string, inbound *model.Inbound, limitIp int, keptLive, bannedLive []IPWithTimestamp, now time.Time) string {
	remark := ""
	if inbound != nil {
		remark = inbound.Remark
	}

	return fmt.Sprintf(
		"💎 <b>OUI 用户通知</b>\n"+
			"⛔ <b>超出 IP 上限，已掐断</b>\n"+
			"📧 用户/节点：<code>%s</code>\n"+
			"🧩 节点名称：<code>%s</code>\n"+
			"🔢 IP 限制：<code>%d</code>\n"+
			"✅ 保留 IP：<code>%s</code>\n"+
			"🚫 掐断 IP：<code>%s</code>\n"+
			"⏰ 时间：<code>%s</code>",
		html.EscapeString(clientEmail),
		html.EscapeString(remark),
		limitIp,
		html.EscapeString(formatIPLimitNotifyIPs(keptLive)),
		html.EscapeString(formatIPLimitNotifyIPs(bannedLive)),
		now.Format("2006-01-02 15:04:05"),
	)
}

func formatIPLimitNotifyIPs(ips []IPWithTimestamp) string {
	if len(ips) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(ips))
	for _, ipTime := range ips {
		parts = append(parts, ipTime.IP)
	}
	return strings.Join(parts, ", ")
}

func (j *CheckClientIpJob) buildIPLimitCutoffKeyboard(clientEmail string, port int, bannedLive []IPWithTimestamp) telego.ReplyMarkup {
	if len(bannedLive) == 0 {
		return nil
	}
	ip := bannedLive[0].IP
	portText := fmt.Sprint(port)
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("临时解封 1 小时").WithCallbackData(j.tgbotService.EncodeQuery("iplimit_unban "+clientEmail+" "+ip+" "+portText+" 1")),
			tu.InlineKeyboardButton("临时解封 6 小时").WithCallbackData(j.tgbotService.EncodeQuery("iplimit_unban "+clientEmail+" "+ip+" "+portText+" 6")),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("临时解封 24 小时").WithCallbackData(j.tgbotService.EncodeQuery("iplimit_unban "+clientEmail+" "+ip+" "+portText+" 24")),
			tu.InlineKeyboardButton("解除封禁").WithCallbackData(j.tgbotService.EncodeQuery("iplimit_unban "+clientEmail+" "+ip+" "+portText+" 0")),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("手动封禁").WithCallbackData(j.tgbotService.EncodeQuery("iplimit_ban "+clientEmail+" "+ip+" "+portText)),
		),
	)
}

func (j *CheckClientIpJob) getInboundByEmail(clientEmail string) (*model.Inbound, error) {
	db := database.GetDB()
	inbound := &model.Inbound{}

	err := db.Model(&model.Inbound{}).Where("settings LIKE ?", "%"+clientEmail+"%").First(inbound).Error
	if err != nil {
		return nil, err
	}

	return inbound, nil
}
