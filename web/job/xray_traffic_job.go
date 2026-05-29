package job

import (
	"encoding/json"
	"fmt"
	"html"
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
}

type onlineNotifySession struct {
	remark          string
	start           time.Time
	idleWindowStart time.Time
	idleWindowTotal int64
	lastTotal       int64
	up              int64
	down            int64
	idleNotified    bool
}

type onlineNotifyInbound struct {
	remark string
}

var (
	onlineNotifyMu       sync.Mutex
	onlineNotifySessions = map[string]onlineNotifySession{}
)

const (
	onlineNotifyIdleGrace               = 15 * time.Minute
	onlineNotifyLowTrafficThresholdByte = 5 * 1024 * 1024
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
			trackable[client.Email] = onlineNotifyInbound{remark: inbound.Remark}
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

	onlineNotifyMu.Lock()
	defer onlineNotifyMu.Unlock()

	for email, meta := range trackable {
		if !onlineSet[email] {
			continue
		}
		if _, exists := onlineNotifySessions[email]; exists {
			continue
		}
		st := trafficByEmail[email]
		session := onlineNotifySession{remark: meta.remark, start: now}
		if st != nil {
			session.up = st.Up
			session.down = st.Down
			session.lastTotal = st.Up + st.Down
			session.idleWindowTotal = session.lastTotal
		}
		session.idleWindowStart = now
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
			st := trafficByEmail[email]
			j.sendInboundOfflineNotify(email, session, st, now)
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
		if session.idleWindowStart.IsZero() {
			session.idleWindowStart = now
			session.idleWindowTotal = currentTotal
		}
		windowTraffic := currentTotal - session.idleWindowTotal
		if windowTraffic < 0 {
			session.idleWindowStart = now
			session.idleWindowTotal = currentTotal
			windowTraffic = 0
			session.idleNotified = false
		}
		if windowTraffic >= onlineNotifyLowTrafficThresholdByte {
			if session.idleNotified {
				j.sendInboundActiveNotify(email, session, windowTraffic, st, now)
			}
			session.idleWindowStart = now
			session.idleWindowTotal = currentTotal
			session.idleNotified = false
		} else if !session.idleNotified && now.Sub(session.idleWindowStart) >= onlineNotifyIdleGrace {
			j.sendInboundKeepAliveNotify(email, session, windowTraffic, st, now)
			session.idleNotified = true
		}
		onlineNotifySessions[email] = session
	}
}

func (j *XrayTrafficJob) sendInboundActiveNotify(email string, session onlineNotifySession, windowTraffic int64, st *xray.ClientTraffic, now time.Time) {
	up, down := sessionDelta(session, st)
	j.tgbotService.SendMsgToTgbotAdmins(fmt.Sprintf(
		"💎 <b>OUI 用户通知</b>\n"+
			"🔵 <b>已恢复使用在线中</b>\n"+
			"📧 用户/节点：<code>%s</code>\n"+
			"🧩 节点名称：<code>%s</code>\n"+
			"⏱ 在线时长：<code>%s</code>\n"+
			"⏰ 恢复时间：<code>%s</code>\n"+
			"📊 当前窗口流量：<code>%s / 阈值 %s</code>\n"+
			"📈 本次累计：<code>↑%s / ↓%s / 合计%s</code>",
		html.EscapeString(email),
		html.EscapeString(session.remark),
		formatOnlineDuration(now.Sub(session.start)),
		now.Format("2006-01-02 15:04:05"),
		common.FormatTraffic(windowTraffic),
		common.FormatTraffic(onlineNotifyLowTrafficThresholdByte),
		common.FormatTraffic(up),
		common.FormatTraffic(down),
		common.FormatTraffic(up+down),
	))
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

func (j *XrayTrafficJob) sendInboundKeepAliveNotify(email string, session onlineNotifySession, windowTraffic int64, st *xray.ClientTraffic, now time.Time) {
	up, down := sessionDelta(session, st)
	j.tgbotService.SendMsgToTgbotAdmins(fmt.Sprintf(
		"💎 <b>OUI 用户通知</b>\n"+
			"🟢 <b>保持连接中</b>\n"+
			"📧 用户/节点：<code>%s</code>\n"+
			"🧩 节点名称：<code>%s</code>\n"+
			"⏱ 在线时长：<code>%s</code>\n"+
			"⏰ 检测时间：<code>%s</code>\n"+
			"📊 15分钟流量：<code>%s / 阈值 %s</code>\n"+
			"📈 本次累计：<code>↑%s / ↓%s / 合计%s</code>",
		html.EscapeString(email),
		html.EscapeString(session.remark),
		formatOnlineDuration(now.Sub(session.start)),
		now.Format("2006-01-02 15:04:05"),
		common.FormatTraffic(windowTraffic),
		common.FormatTraffic(onlineNotifyLowTrafficThresholdByte),
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
