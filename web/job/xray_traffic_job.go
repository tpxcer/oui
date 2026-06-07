package job

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/util/common"
	"github.com/mhsanaei/3x-ui/v3/web/service"
	"github.com/mhsanaei/3x-ui/v3/web/websocket"
	"github.com/mhsanaei/3x-ui/v3/xray"

	"github.com/valyala/fasthttp"
)

// XrayTrafficJob collects and processes traffic statistics from Xray, updating the database and optionally informing external APIs.
type XrayTrafficJob struct {
	settingService  service.SettingService
	xrayService     service.XrayService
	inboundService  service.InboundService
	outboundService service.OutboundService
	tgbotService    service.Tgbot
	serverService   service.ServerService
}

type onlineNotifySession struct {
	email     string
	remark    string
	ip        string
	start     time.Time
	lastTotal int64
	up        int64
	down      int64
	announced bool
}

type onlineNotifyInbound struct {
	email  string
	remark string
	ip     string
}

type onlineNotifyTransition struct {
	email   string
	remark  string
	session onlineNotifySession
	traffic *xray.ClientTraffic
}

type onlineNotifyClientIP struct {
	ip        string
	timestamp int64
}

var (
	onlineNotifyMu       sync.Mutex
	onlineNotifySessions = map[string]onlineNotifySession{}
)

const (
	onlineNotifyConfirmWindow = 5 * time.Minute
	onlineNotifyMinTraffic    = int64(1 * 1024 * 1024)
)

// NewXrayTrafficJob creates a new traffic collection job instance.
func NewXrayTrafficJob() *XrayTrafficJob {
	return new(XrayTrafficJob)
}

// Run collects traffic statistics from Xray, updates the database, and pushes
// real-time updates over WebSocket using compact delta payloads — no REST
// fallback, scales to 10k–20k+ clients per inbound.
func (j *XrayTrafficJob) Run() {
	if !j.xrayService.IsXrayRunning() {
		return
	}
	traffics, clientTraffics, err := j.xrayService.GetXrayTraffic()
	if err != nil {
		return
	}
	needRestart0, clientsDisabled, err := j.inboundService.AddTraffic(traffics, clientTraffics)
	if err != nil {
		logger.Warning("add inbound traffic failed:", err)
	}
	err, needRestart1 := j.outboundService.AddTraffic(traffics, clientTraffics)
	if err != nil {
		logger.Warning("add outbound traffic failed:", err)
	}
	if clientsDisabled {
		restartOnDisable, settingErr := j.settingService.GetRestartXrayOnClientDisable()
		if settingErr != nil {
			logger.Warning("get RestartXrayOnClientDisable failed:", settingErr)
		}
		if restartOnDisable {
			if err := j.xrayService.RestartXray(true); err != nil {
				logger.Warning("restart xray after disabling clients failed:", err)
				j.xrayService.SetToNeedRestart()
			}
		}
		websocket.BroadcastInvalidate(websocket.MessageTypeInbounds)
	}
	if ExternalTrafficInformEnable, err := j.settingService.GetExternalTrafficInformEnable(); ExternalTrafficInformEnable {
		j.informTrafficToExternalAPI(traffics, clientTraffics)
	} else if err != nil {
		logger.Warning("get ExternalTrafficInformEnable failed:", err)
	}
	if needRestart0 || needRestart1 {
		j.xrayService.SetToNeedRestart()
	}

	lastOnlineMap, err := j.inboundService.GetClientsLastOnline()
	if err != nil {
		logger.Warning("get clients last online failed:", err)
	}
	if lastOnlineMap == nil {
		lastOnlineMap = make(map[string]int64)
	}
	j.inboundService.RefreshOnlineClientsFromMap(lastOnlineMap)

	onlineClients := j.inboundService.GetOnlineClients()
	if onlineClients == nil {
		onlineClients = []string{}
	}
	j.notifyInboundOnlineChanges(onlineClients, lastOnlineMap)

	if !websocket.HasClients() {
		return
	}

	websocket.BroadcastTraffic(map[string]any{
		"traffics":       traffics,
		"clientTraffics": clientTraffics,
		"onlineClients":  onlineClients,
		"lastOnlineMap":  lastOnlineMap,
	})

	clientStatsPayload := map[string]any{}
	if stats, err := j.inboundService.GetAllClientTraffics(); err != nil {
		logger.Warning("get all client traffics for websocket failed:", err)
	} else if len(stats) > 0 {
		clientStatsPayload["clients"] = stats
	}
	if inboundSummary, err := j.inboundService.GetInboundsTrafficSummary(); err != nil {
		logger.Warning("get inbounds traffic summary for websocket failed:", err)
	} else if len(inboundSummary) > 0 {
		clientStatsPayload["inbounds"] = inboundSummary
	}
	if len(clientStatsPayload) > 0 {
		websocket.BroadcastClientStats(clientStatsPayload)
	}

	if updatedOutbounds, err := j.outboundService.GetOutboundsTraffic(); err == nil && updatedOutbounds != nil {
		websocket.BroadcastOutbounds(updatedOutbounds)
	} else if err != nil {
		logger.Warning("get all outbounds for websocket failed:", err)
	}
}

func (j *XrayTrafficJob) notifyInboundOnlineChanges(onlineClients []string, lastOnlineMap map[string]int64) {
	if !j.tgbotService.IsRunning() {
		return
	}

	inbounds, err := j.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("get inbounds for tg online notify failed:", err)
		return
	}
	trackableByEmail := make(map[string]onlineNotifyInbound)
	for _, inbound := range inbounds {
		if inbound == nil || !inbound.Enable || !inbound.TgOnlineNotify {
			continue
		}
		clients, err := j.inboundService.GetClients(inbound)
		if err != nil {
			continue
		}
		for _, client := range clients {
			if client.Email == "" || !client.Enable {
				continue
			}
			meta := trackableByEmail[client.Email]
			meta.email = client.Email
			meta.remark = inbound.Remark
			trackableByEmail[client.Email] = meta
		}
	}

	trafficByEmail := map[string]*xray.ClientTraffic{}
	if stats, err := j.inboundService.GetAllClientTraffics(); err == nil {
		for _, st := range stats {
			if st != nil && st.Email != "" {
				trafficByEmail[st.Email] = st
			}
		}
	}

	now := time.Now()
	onlineSet := make(map[string]bool, len(onlineClients))
	for _, email := range onlineClients {
		onlineSet[email] = true
	}
	trackable := j.expandOnlineNotifyTrackable(trackableByEmail, onlineSet)

	onlineNotifyMu.Lock()
	onlineTransitions, offlineTransitions := reconcileOnlineNotifySessions(trackable, onlineSet, trafficByEmail, now)
	onlineNotifyMu.Unlock()

	for _, ev := range onlineTransitions {
		ipLines := j.buildOnlineNotifyIPLines(ev.email, ev.session.ip)
		j.tgbotService.SendMsgToTgbotAdmins(fmt.Sprintf(
			"💎 <b>OUI 用户通知</b>\n"+
				"🚀 <b>客户端上线</b>\n"+
				"📧 用户/节点：<code>%s</code>\n"+
				"🧩 节点名称：<code>%s</code>\n"+
				"%s"+
				"⏰ 上线时间：<code>%s</code>",
			html.EscapeString(ev.email),
			html.EscapeString(ev.remark),
			ipLines,
			now.Format("2006-01-02 15:04:05"),
		))
	}

	for _, ev := range offlineTransitions {
		j.sendInboundOfflineNotify(ev.email, ev.session, ev.traffic, now)
	}
}

func (j *XrayTrafficJob) expandOnlineNotifyTrackable(trackableByEmail map[string]onlineNotifyInbound, onlineSet map[string]bool) map[string]onlineNotifyInbound {
	trackable := make(map[string]onlineNotifyInbound, len(trackableByEmail))
	for email, meta := range trackableByEmail {
		if meta.email == "" {
			meta.email = email
		}
		if !onlineSet[email] {
			trackable[email] = meta
			continue
		}
		ips := j.onlineNotifyClientIPs(email)
		if len(ips) == 0 {
			trackable[email] = meta
			continue
		}
		for i, clientIP := range ips {
			ipMeta := meta
			ipMeta.ip = clientIP.ip
			ipMeta.remark = onlineNotifyRemarkForIPIndex(meta.remark, i)
			trackable[onlineNotifySessionKey(email, clientIP.ip)] = ipMeta
		}
	}
	return trackable
}

func reconcileOnlineNotifySessions(
	trackable map[string]onlineNotifyInbound,
	onlineSet map[string]bool,
	trafficByEmail map[string]*xray.ClientTraffic,
	now time.Time,
) ([]onlineNotifyTransition, []onlineNotifyTransition) {
	onlineTransitions := make([]onlineNotifyTransition, 0)
	offlineTransitions := make([]onlineNotifyTransition, 0)
	trackableEmails := make(map[string]struct{}, len(trackable))
	for key, meta := range trackable {
		if meta.email == "" {
			meta.email = key
			trackable[key] = meta
		}
		trackableEmails[meta.email] = struct{}{}
	}

	for key, meta := range trackable {
		email := meta.email
		if !onlineSet[email] {
			continue
		}
		if session, exists := onlineNotifySessions[key]; exists {
			session.email = email
			session.remark = meta.remark
			session.ip = meta.ip
			st := trafficByEmail[email]
			if st != nil {
				session.lastTotal = st.Up + st.Down
			}
			if !session.announced && shouldAnnounceOnline(session, now) {
				session.announced = true
				onlineTransitions = append(onlineTransitions, onlineNotifyTransition{
					email:   email,
					remark:  meta.remark,
					session: session,
					traffic: st,
				})
			}
			onlineNotifySessions[key] = session
			continue
		}
		st := trafficByEmail[email]
		session := onlineNotifySession{email: email, remark: meta.remark, ip: meta.ip, start: now}
		if st != nil {
			session.up = st.Up
			session.down = st.Down
			session.lastTotal = st.Up + st.Down
		}
		onlineNotifySessions[key] = session
	}

	for key, session := range onlineNotifySessions {
		if session.email == "" {
			session.email = key
		}
		if _, ok := trackable[key]; !ok {
			delete(onlineNotifySessions, key)
			if _, emailStillTrackable := trackableEmails[session.email]; !emailStillTrackable {
				continue
			}
			if !session.announced {
				continue
			}
			if !onlineSet[session.email] || session.ip != "" {
				st := trafficByEmail[session.email]
				offlineTransitions = append(offlineTransitions, onlineNotifyTransition{
					email:   session.email,
					remark:  session.remark,
					session: session,
					traffic: st,
				})
			}
			continue
		}

		st := trafficByEmail[session.email]
		if st != nil {
			session.lastTotal = st.Up + st.Down
		}
		if !onlineSet[session.email] {
			delete(onlineNotifySessions, key)
			if !session.announced {
				continue
			}
			offlineTransitions = append(offlineTransitions, onlineNotifyTransition{
				email:   session.email,
				remark:  session.remark,
				session: session,
				traffic: st,
			})
			continue
		}
		onlineNotifySessions[key] = session
	}

	return onlineTransitions, offlineTransitions
}

func (j *XrayTrafficJob) buildOnlineNotifyIPLines(email string, ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = j.latestOnlineNotifyClientIP(email)
	}
	if ip == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var attribution service.NodeGeoLocation
	var trace service.NodeGeoLocation
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		attribution = j.serverService.LookupIPAttribution(ctx, ip)
	}()
	go func() {
		defer wg.Done()
		trace = j.serverService.LookupIPGeo(ctx, ip)
	}()
	wg.Wait()

	lines := fmt.Sprintf("🌐 IP 地址：<code>%s</code>\n", html.EscapeString(ip))
	lines += fmt.Sprintf("📍 IP归属地址：<code>%s</code>\n", html.EscapeString(formatOnlineNotifyLocationOrUnknown(attribution)))
	lines += fmt.Sprintf("🧭 IP溯源：<code>%s</code>\n", html.EscapeString(formatOnlineNotifyLocationOrUnknown(trace)))
	return lines
}

func (j *XrayTrafficJob) latestOnlineNotifyClientIP(email string) string {
	raw, err := j.inboundService.GetInboundClientIps(email)
	if err != nil || strings.TrimSpace(raw) == "" {
		return ""
	}
	clientIP, ok := latestOnlineNotifyClientIP(raw)
	if !ok {
		return ""
	}
	return clientIP.ip
}

func (j *XrayTrafficJob) onlineNotifyClientIPs(email string) []onlineNotifyClientIP {
	raw, err := j.inboundService.GetInboundClientIps(email)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	return onlineNotifyClientIPsFromRaw(raw)
}

func latestOnlineNotifyClientIP(raw string) (onlineNotifyClientIP, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return onlineNotifyClientIP{}, false
	}

	type ipWithTimestamp struct {
		IP        string `json:"ip"`
		Timestamp int64  `json:"timestamp"`
	}

	var ipsWithTime []ipWithTimestamp
	if err := json.Unmarshal([]byte(raw), &ipsWithTime); err == nil {
		var latest onlineNotifyClientIP
		for _, item := range ipsWithTime {
			ip := strings.TrimSpace(item.IP)
			if ip == "" {
				continue
			}
			if latest.ip == "" || item.Timestamp >= latest.timestamp {
				latest = onlineNotifyClientIP{ip: ip, timestamp: item.Timestamp}
			}
		}
		if latest.ip != "" {
			return latest, true
		}
	}

	var oldIps []string
	if err := json.Unmarshal([]byte(raw), &oldIps); err == nil {
		for i := len(oldIps) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(oldIps[i])
			if ip != "" {
				return onlineNotifyClientIP{ip: ip}, true
			}
		}
	}

	return onlineNotifyClientIP{}, false
}

func onlineNotifyClientIPsFromRaw(raw string) []onlineNotifyClientIP {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	type ipWithTimestamp struct {
		IP        string `json:"ip"`
		Timestamp int64  `json:"timestamp"`
	}

	var ipsWithTime []ipWithTimestamp
	if err := json.Unmarshal([]byte(raw), &ipsWithTime); err == nil {
		ips := make([]onlineNotifyClientIP, 0, len(ipsWithTime))
		for _, item := range ipsWithTime {
			ip := strings.TrimSpace(item.IP)
			if ip == "" {
				continue
			}
			ips = append(ips, onlineNotifyClientIP{ip: ip, timestamp: item.Timestamp})
		}
		sortOnlineNotifyClientIPs(ips)
		return ips
	}

	var oldIps []string
	if err := json.Unmarshal([]byte(raw), &oldIps); err == nil {
		ips := make([]onlineNotifyClientIP, 0, len(oldIps))
		for _, item := range oldIps {
			ip := strings.TrimSpace(item)
			if ip == "" {
				continue
			}
			ips = append(ips, onlineNotifyClientIP{ip: ip})
		}
		return ips
	}

	return nil
}

func sortOnlineNotifyClientIPs(ips []onlineNotifyClientIP) {
	sort.SliceStable(ips, func(i, j int) bool {
		if ips[i].timestamp == ips[j].timestamp {
			return ips[i].ip < ips[j].ip
		}
		if ips[i].timestamp == 0 {
			return false
		}
		if ips[j].timestamp == 0 {
			return true
		}
		return ips[i].timestamp < ips[j].timestamp
	})
}

func onlineNotifySessionKey(email string, ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return email
	}
	return email + "\x00" + ip
}

func onlineNotifyRemarkForIPIndex(remark string, index int) string {
	if index <= 0 {
		return remark
	}
	return fmt.Sprintf("%s(ip%d)", remark, index+1)
}

func formatOnlineNotifyGeoLocation(geo service.NodeGeoLocation) string {
	if location := strings.TrimSpace(geo.Location); location != "" {
		return location
	}
	parts := []string{
		geo.Country,
		geo.Province,
		geo.City,
		geo.District,
		geo.Detail,
	}
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return strings.Join(out, " ")
}

func formatOnlineNotifyLocationOrUnknown(geo service.NodeGeoLocation) string {
	location := strings.TrimSpace(formatOnlineNotifyGeoLocation(geo))
	if location == "" {
		return "未知"
	}
	return location
}

func shouldAnnounceOnline(session onlineNotifySession, now time.Time) bool {
	if session.announced {
		return false
	}
	if now.Sub(session.start) > onlineNotifyConfirmWindow {
		return false
	}
	return session.lastTotal-session.up-session.down >= onlineNotifyMinTraffic
}

func (j *XrayTrafficJob) sendInboundOfflineNotify(email string, session onlineNotifySession, st *xray.ClientTraffic, now time.Time) {
	up, down := sessionDelta(session, st)
	ipLine := ""
	if ip := strings.TrimSpace(session.ip); ip != "" {
		ipLine = fmt.Sprintf("🌐 离线 IP 地址：<code>%s</code>\n", html.EscapeString(ip))
	}
	j.tgbotService.SendMsgToTgbotAdmins(fmt.Sprintf(
		"💎 <b>OUI 用户通知</b>\n"+
			"📴 <b>客户端下线</b>\n"+
			"📧 用户/节点：<code>%s</code>\n"+
			"🧩 节点名称：<code>%s</code>\n"+
			"%s"+
			"⏱ 在线时长：<code>%s</code>\n"+
			"⏰ 下线时间：<code>%s</code>\n"+
			"📊 流量：<code>↑%s / ↓%s / 合计%s</code>",
		html.EscapeString(email),
		html.EscapeString(session.remark),
		ipLine,
		formatOnlineDuration(now.Sub(session.start)),
		now.Format("2006-01-02 15:04:05"),
		common.FormatTraffic(up),
		common.FormatTraffic(down),
		common.FormatTraffic(up+down),
	))
}

func sessionDelta(session onlineNotifySession, st *xray.ClientTraffic) (int64, int64) {
	if st == nil {
		return 0, 0
	}
	up := st.Up - session.up
	down := st.Down - session.down
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	return up, down
}

func formatOnlineDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int64(d.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d小时%d分钟%d秒", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%d分钟%d秒", minutes, seconds)
	}
	return fmt.Sprintf("%d秒", seconds)
}

func (j *XrayTrafficJob) informTrafficToExternalAPI(inboundTraffics []*xray.Traffic, clientTraffics []*xray.ClientTraffic) {
	informURL, err := j.settingService.GetExternalTrafficInformURI()
	if err != nil {
		logger.Warning("get ExternalTrafficInformURI failed:", err)
		return
	}
	informURL, err = service.SanitizePublicHTTPURL(informURL, false)
	if err != nil {
		logger.Warning("ExternalTrafficInformURI blocked:", err)
		return
	}
	requestBody, err := json.Marshal(map[string]any{"clientTraffics": clientTraffics, "inboundTraffics": inboundTraffics})
	if err != nil {
		logger.Warning("parse client/inbound traffic failed:", err)
		return
	}
	request := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(request)
	request.Header.SetMethod("POST")
	request.Header.SetContentType("application/json; charset=UTF-8")
	request.SetBody([]byte(requestBody))
	request.SetRequestURI(informURL)
	response := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(response)
	if err := fasthttp.Do(request, response); err != nil {
		logger.Warning("POST ExternalTrafficInformURI failed:", err)
	}
}
