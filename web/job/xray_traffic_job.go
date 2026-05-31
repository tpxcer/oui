package job

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
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
}

type onlineNotifySession struct {
	remark       string
	start        time.Time
	lastTotal    int64
	up           int64
	down         int64
	missingSince time.Time
}

type onlineNotifyInbound struct {
	remark string
	ports  []int
}

var (
	onlineNotifyMu       sync.Mutex
	onlineNotifySessions = map[string]onlineNotifySession{}
)

const onlineNotifyOfflineConfirm = 5 * time.Minute

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
	trackable := make(map[string]onlineNotifyInbound)
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
			meta := trackable[client.Email]
			meta.remark = inbound.Remark
			if inbound.Port > 0 {
				meta.ports = append(meta.ports, inbound.Port)
			}
			trackable[client.Email] = meta
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
	if tcpConnected, err := activeTCPClientEmails(trackable); err == nil {
		for email := range tcpConnected {
			onlineSet[email] = true
		}
	} else {
		logger.Debug("tg online notify tcp connection check failed:", err)
	}

	onlineNotifyMu.Lock()
	defer onlineNotifyMu.Unlock()

	for email, meta := range trackable {
		if !onlineSet[email] {
			continue
		}
		if session, exists := onlineNotifySessions[email]; exists {
			session.remark = meta.remark
			session.missingSince = time.Time{}
			onlineNotifySessions[email] = session
			continue
		}
		st := trafficByEmail[email]
		session := onlineNotifySession{remark: meta.remark, start: now}
		if st != nil {
			session.up = st.Up
			session.down = st.Down
			session.lastTotal = st.Up + st.Down
		}
		onlineNotifySessions[email] = session
		j.tgbotService.SendMsgToTgbotAdmins(fmt.Sprintf(
			"💎 <b>OUI 用户通知</b>\n"+
				"🚀 <b>客户端上线</b>\n"+
				"📧 用户/节点：<code>%s</code>\n"+
				"🧩 节点名称：<code>%s</code>\n"+
				"⏰ 上线时间：<code>%s</code>",
			html.EscapeString(email),
			html.EscapeString(meta.remark),
			now.Format("2006-01-02 15:04:05"),
		))
	}

	for email, session := range onlineNotifySessions {
		if _, ok := trackable[email]; !ok {
			delete(onlineNotifySessions, email)
			continue
		}

		st := trafficByEmail[email]
		currentTotal := session.lastTotal
		if st != nil {
			currentTotal = st.Up + st.Down
		}
		hasTrafficChange := st != nil && currentTotal > session.lastTotal

		if hasTrafficChange {
			session.lastTotal = currentTotal
		}
		if !onlineSet[email] {
			if session.missingSince.IsZero() {
				session.missingSince = now
				onlineNotifySessions[email] = session
				continue
			}
			if now.Sub(session.missingSince) >= onlineNotifyOfflineConfirm {
				j.sendInboundOfflineNotify(email, session, st, now)
				delete(onlineNotifySessions, email)
				continue
			}
			onlineNotifySessions[email] = session
			continue
		}
		session.missingSince = time.Time{}
		onlineNotifySessions[email] = session
	}
}

func activeTCPClientEmails(trackable map[string]onlineNotifyInbound) (map[string]bool, error) {
	if len(trackable) == 0 {
		return map[string]bool{}, nil
	}
	ports := map[int]struct{}{}
	for _, meta := range trackable {
		for _, port := range meta.ports {
			if port > 0 {
				ports[port] = struct{}{}
			}
		}
	}
	if len(ports) == 0 {
		return map[string]bool{}, nil
	}

	activeByPort, err := activeTCPRemoteIPsByLocalPort(ports)
	if err != nil {
		return nil, err
	}
	if len(activeByPort) == 0 {
		return map[string]bool{}, nil
	}

	var rows []model.InboundClientIps
	if err := database.GetDB().Find(&rows).Error; err != nil {
		return nil, err
	}

	result := map[string]bool{}
	for _, row := range rows {
		meta, ok := trackable[row.ClientEmail]
		if !ok || row.Ips == "" {
			continue
		}
		var ips []IPWithTimestamp
		if err := json.Unmarshal([]byte(row.Ips), &ips); err != nil {
			continue
		}
		for _, item := range ips {
			ip := normalizeIP(item.IP)
			if ip == "" {
				continue
			}
			for _, port := range meta.ports {
				if activeByPort[port][ip] {
					result[row.ClientEmail] = true
					break
				}
			}
			if result[row.ClientEmail] {
				break
			}
		}
	}
	return result, nil
}

func activeTCPRemoteIPsByLocalPort(ports map[int]struct{}) (map[int]map[string]bool, error) {
	result := map[int]map[string]bool{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if err := readActiveTCPFile(path, ports, result); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
	}
	return result, nil
}

func readActiveTCPFile(path string, ports map[int]struct{}, result map[int]map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || fields[0] == "sl" {
			continue
		}
		if fields[3] != "01" {
			continue
		}
		localPort, ok := parseProcTCPPort(fields[1])
		if !ok {
			continue
		}
		if _, watched := ports[localPort]; !watched {
			continue
		}
		remoteIP := parseProcTCPIP(fields[2])
		if remoteIP == "" {
			continue
		}
		if result[localPort] == nil {
			result[localPort] = map[string]bool{}
		}
		result[localPort][remoteIP] = true
	}
	return sc.Err()
}

func parseProcTCPPort(addr string) (int, bool) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return 0, false
	}
	port64, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return 0, false
	}
	return int(port64), true
}

func parseProcTCPIP(addr string) string {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return ""
	}
	raw, err := hex.DecodeString(parts[0])
	if err != nil {
		return ""
	}
	switch len(raw) {
	case net.IPv4len:
		return net.IPv4(raw[3], raw[2], raw[1], raw[0]).String()
	case net.IPv6len:
		ip := make(net.IP, net.IPv6len)
		for i := 0; i < net.IPv6len; i += 4 {
			ip[i] = raw[i+3]
			ip[i+1] = raw[i+2]
			ip[i+2] = raw[i+1]
			ip[i+3] = raw[i]
		}
		return ip.String()
	default:
		return ""
	}
}

func normalizeIP(value string) string {
	ip := net.ParseIP(strings.Trim(value, "[]"))
	if ip == nil {
		return ""
	}
	return ip.String()
}

func (j *XrayTrafficJob) sendInboundOfflineNotify(email string, session onlineNotifySession, st *xray.ClientTraffic, now time.Time) {
	up, down := sessionDelta(session, st)
	j.tgbotService.SendMsgToTgbotAdmins(fmt.Sprintf(
		"💎 <b>OUI 用户通知</b>\n"+
			"📴 <b>客户端下线</b>\n"+
			"📧 用户/节点：<code>%s</code>\n"+
			"🧩 节点名称：<code>%s</code>\n"+
			"⏱ 在线时长：<code>%s</code>\n"+
			"⏰ 下线时间：<code>%s</code>\n"+
			"📊 流量：<code>↑%s / ↓%s / 合计%s</code>",
		html.EscapeString(email),
		html.EscapeString(session.remark),
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
