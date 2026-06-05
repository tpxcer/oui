package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
	"github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/xray"
	"gorm.io/gorm"
)

type IPLimitIPWithTimestamp struct {
	IP        string `json:"ip"`
	Timestamp int64  `json:"timestamp"`
}

type ipLimitBanState struct {
	IP        string
	Port      int
	Timestamp int64
}

type ipLimitBanEvent struct {
	ipLimitBanState
	Action string
	Seq    int
}

type IPLimitIncrementResult struct {
	Email         string
	Port          int
	InboundRemark string
	OldLimit      int
	NewLimit      int
}

func (s *InboundService) SyncClientIPLimitBansByEmail(clientEmail string) error {
	clientEmail = strings.TrimSpace(clientEmail)
	if clientEmail == "" {
		return nil
	}

	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).Where("settings LIKE ?", "%"+clientEmail+"%").Find(&inbounds).Error; err != nil {
		return err
	}

	tracked, err := s.getTrackedIPLimitIPs(clientEmail)
	if err != nil {
		logger.Warningf("[LIMIT_IP] failed to load tracked IPs for %s: %v", clientEmail, err)
	}

	var errs []string
	matched := false
	for _, inbound := range inbounds {
		limitIP, clientFound, clientEnabled := ipLimitClientConfig(inbound, clientEmail)
		if !clientFound {
			continue
		}
		matched = true
		if !clientEnabled || !inbound.Enable || s.inboundNodeOffline(inbound) {
			limitIP = 0
		}
		if err := s.SyncClientIPLimitBansForInbound(clientEmail, inbound, limitIP, tracked); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if !matched {
		if err := s.UnbanClientIPLimitByEmail(clientEmail); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (s *InboundService) IncrementClientIPLimitByEmailAndPort(clientEmail string, port int) (IPLimitIncrementResult, error) {
	result := IPLimitIncrementResult{Email: strings.TrimSpace(clientEmail), Port: port}
	if result.Email == "" {
		return result, errors.New("client email is required")
	}
	if port <= 0 || port > 65535 {
		return result, fmt.Errorf("invalid port: %d", port)
	}

	db := database.GetDB()
	var inbounds []model.Inbound
	if err := db.Model(model.Inbound{}).
		Where("port = ? AND settings LIKE ?", port, "%"+result.Email+"%").
		Order("id asc").
		Find(&inbounds).Error; err != nil {
		return result, err
	}
	if len(inbounds) == 0 {
		if err := db.Model(model.Inbound{}).
			Where("settings LIKE ?", "%"+result.Email+"%").
			Order("id asc").
			Find(&inbounds).Error; err != nil {
			return result, err
		}
	}

	var selected *model.Inbound
	var selectedSettings string
	for i := range inbounds {
		nextSettings, oldLimit, newLimit, ok, err := incrementClientLimitInSettings(inbounds[i].Settings, result.Email)
		if err != nil {
			return result, err
		}
		if !ok {
			continue
		}
		selected = &inbounds[i]
		selectedSettings = nextSettings
		result.Port = inbounds[i].Port
		result.InboundRemark = inbounds[i].Remark
		result.OldLimit = oldLimit
		result.NewLimit = newLimit
		break
	}
	if selected == nil {
		return result, fmt.Errorf("client %s not found for port %d", result.Email, port)
	}

	tx := db.Begin()
	if tx.Error != nil {
		return result, tx.Error
	}
	var err error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&model.Inbound{}).
		Where("id = ?", selected.Id).
		Update("settings", selectedSettings).Error; err != nil {
		return result, err
	}
	if err = tx.Model(&model.ClientRecord{}).
		Where("email = ? AND limit_ip < ?", result.Email, result.NewLimit).
		Update("limit_ip", result.NewLimit).Error; err != nil {
		return result, err
	}
	if err = tx.Commit().Error; err != nil {
		return result, err
	}

	return result, nil
}

func (s *InboundService) SyncClientIPLimitBansForInbound(clientEmail string, inbound *model.Inbound, limitIP int, tracked []IPLimitIPWithTimestamp) error {
	if strings.TrimSpace(clientEmail) == "" || inbound == nil {
		return nil
	}

	currentBans := s.currentIPLimitBanStatesForEmail(clientEmail, inbound.Port)
	if len(currentBans) == 0 {
		return nil
	}

	allowed := map[string]bool{}
	if limitIP > 0 && inbound.Enable && !s.inboundNodeOffline(inbound) {
		allowed = allowedIPLimitIPs(limitIP, tracked, currentBans)
	}

	var errs []string
	for _, ban := range currentBans {
		shouldUnban := limitIP <= 0 || !inbound.Enable || s.inboundNodeOffline(inbound) || allowed[ban.IP]
		if !shouldUnban {
			continue
		}
		if err := s.unbanClientIPLimitByEmailAndIP(clientEmail, ban.IP, ban.Port, "automatic IP limit sync"); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func incrementClientLimitInSettings(rawSettings string, clientEmail string) (string, int, int, bool, error) {
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(rawSettings), &settings); err != nil {
		return "", 0, 0, false, err
	}
	rawClients, ok := settings["clients"].([]any)
	if !ok {
		return "", 0, 0, false, nil
	}

	for i, rawClient := range rawClients {
		clientMap, ok := rawClient.(map[string]any)
		if !ok {
			continue
		}
		email, _ := clientMap["email"].(string)
		if email != clientEmail {
			continue
		}
		oldLimit := ipLimitIntValue(clientMap["limitIp"])
		newLimit := oldLimit + 1
		clientMap["limitIp"] = newLimit
		rawClients[i] = clientMap
		settings["clients"] = rawClients
		next, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return "", 0, 0, false, err
		}
		return string(next), oldLimit, newLimit, true, nil
	}

	return "", 0, 0, false, nil
}

func ipLimitIntValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		parsed, _ := v.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(v))
		return parsed
	default:
		return 0
	}
}

func (s *InboundService) UnbanIPLimitBansForNode(nodeID int) error {
	if nodeID <= 0 {
		return nil
	}

	var inbounds []*model.Inbound
	if err := database.GetDB().Model(model.Inbound{}).Where("node_id = ?", nodeID).Find(&inbounds).Error; err != nil {
		return err
	}

	var errs []string
	for _, inbound := range inbounds {
		for _, client := range ipLimitClientsFromInbound(inbound) {
			if strings.TrimSpace(client.Email) == "" {
				continue
			}
			if err := s.SyncClientIPLimitBansForInbound(client.Email, inbound, 0, nil); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (s *InboundService) getTrackedIPLimitIPs(clientEmail string) ([]IPLimitIPWithTimestamp, error) {
	row := &model.InboundClientIps{}
	if err := database.GetDB().Model(model.InboundClientIps{}).Where("client_email = ?", clientEmail).First(row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return parseTrackedIPLimitIPs(row.Ips), nil
}

func parseTrackedIPLimitIPs(raw string) []IPLimitIPWithTimestamp {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var withTime []IPLimitIPWithTimestamp
	if err := json.Unmarshal([]byte(raw), &withTime); err == nil {
		out := make([]IPLimitIPWithTimestamp, 0, len(withTime))
		now := time.Now().Unix()
		for _, item := range withTime {
			if item.IP == "" {
				continue
			}
			if item.Timestamp <= 0 {
				item.Timestamp = now
			}
			out = append(out, item)
		}
		return out
	}

	var oldIPs []string
	if err := json.Unmarshal([]byte(raw), &oldIPs); err != nil {
		return nil
	}
	now := time.Now().Unix()
	out := make([]IPLimitIPWithTimestamp, 0, len(oldIPs))
	for _, ip := range oldIPs {
		if ip == "" {
			continue
		}
		out = append(out, IPLimitIPWithTimestamp{IP: ip, Timestamp: now})
	}
	return out
}

func ipLimitClientsFromInbound(inbound *model.Inbound) []model.Client {
	if inbound == nil || inbound.Settings == "" {
		return nil
	}
	settings := map[string][]model.Client{}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return nil
	}
	return settings["clients"]
}

func ipLimitClientConfig(inbound *model.Inbound, clientEmail string) (limitIP int, found bool, enabled bool) {
	for _, client := range ipLimitClientsFromInbound(inbound) {
		if client.Email == clientEmail {
			return client.LimitIP, true, client.Enable
		}
	}
	return 0, false, false
}

func (s *InboundService) inboundNodeOffline(inbound *model.Inbound) bool {
	if inbound == nil || inbound.NodeID == nil {
		return false
	}
	node := &model.Node{}
	if err := database.GetDB().Model(model.Node{}).Select("enable", "status").Where("id = ?", *inbound.NodeID).First(node).Error; err != nil {
		return false
	}
	return !node.Enable || node.Status == "offline"
}

func allowedIPLimitIPs(limitIP int, tracked []IPLimitIPWithTimestamp, currentBans []ipLimitBanState) map[string]bool {
	allowed := map[string]bool{}
	if limitIP <= 0 {
		return allowed
	}

	firstSeen := map[string]int64{}
	add := func(ip string, timestamp int64) {
		if ip == "" {
			return
		}
		if timestamp <= 0 {
			timestamp = time.Now().Unix()
		}
		if existing, ok := firstSeen[ip]; !ok || timestamp < existing {
			firstSeen[ip] = timestamp
		}
	}

	for _, item := range tracked {
		add(item.IP, item.Timestamp)
	}
	for _, ban := range currentBans {
		add(ban.IP, ban.Timestamp)
	}

	entries := make([]IPLimitIPWithTimestamp, 0, len(firstSeen))
	for ip, timestamp := range firstSeen {
		entries = append(entries, IPLimitIPWithTimestamp{IP: ip, Timestamp: timestamp})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Timestamp == entries[j].Timestamp {
			return entries[i].IP < entries[j].IP
		}
		return entries[i].Timestamp < entries[j].Timestamp
	})

	if len(entries) < limitIP {
		limitIP = len(entries)
	}
	for _, item := range entries[:limitIP] {
		allowed[item.IP] = true
	}
	return allowed
}

func (s *InboundService) currentIPLimitBanStatesForEmail(clientEmail string, defaultPort int) []ipLimitBanState {
	events := parseIPLimitBanEventsForEmail(clientEmail, defaultPort)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp == events[j].Timestamp {
			return events[i].Seq < events[j].Seq
		}
		return events[i].Timestamp < events[j].Timestamp
	})

	current := map[string]ipLimitBanState{}
	for _, event := range events {
		if defaultPort > 0 && event.Port != defaultPort {
			continue
		}
		key := fmt.Sprintf("%s/%d", event.IP, event.Port)
		switch event.Action {
		case "UNBAN":
			delete(current, key)
		case "BAN":
			current[key] = event.ipLimitBanState
		}
	}

	out := make([]ipLimitBanState, 0, len(current))
	for _, state := range current {
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp == out[j].Timestamp {
			if out[i].Port == out[j].Port {
				return out[i].IP < out[j].IP
			}
			return out[i].Port < out[j].Port
		}
		return out[i].Timestamp < out[j].Timestamp
	})
	return out
}

func parseIPLimitBanEventsForEmail(clientEmail string, defaultPort int) []ipLimitBanEvent {
	paths := []string{
		xray.GetIPLimitBannedPrevLogPath(),
		xray.GetIPLimitLogPath(),
		xray.GetIPLimitBannedLogPath(),
	}

	events := []ipLimitBanEvent{}
	seq := 0
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil || len(body) == 0 {
			continue
		}
		for _, line := range strings.Split(string(body), "\n") {
			seq++
			event, ok := parseIPLimitBanEventLine(line, clientEmail, defaultPort, seq)
			if ok {
				events = append(events, event)
			}
		}
	}
	return events
}

func parseIPLimitBanEventLine(line, clientEmail string, defaultPort int, seq int) (ipLimitBanEvent, bool) {
	line = strings.TrimSpace(line)
	if line == "" || !ipLimitLogLineMatchesEmail(line, clientEmail) {
		return ipLimitBanEvent{}, false
	}

	action := ""
	switch {
	case strings.Contains(line, "UNBAN"):
		action = "UNBAN"
	case strings.Contains(line, "[LIMIT_IP]") || strings.Contains(line, " BAN "):
		action = "BAN"
	default:
		return ipLimitBanEvent{}, false
	}

	ip := extractIPLimitLogField(line, regexp.MustCompile(`(?:Disconnecting OLD IP|\[?IP\]?)\s*=\s*([0-9a-fA-F:.]+)`))
	if ip == "" {
		return ipLimitBanEvent{}, false
	}
	port := defaultPort
	if portText := extractIPLimitLogField(line, regexp.MustCompile(`\[?Port\]?\s*=\s*(\d+)`)); portText != "" {
		if parsed, err := strconv.Atoi(portText); err == nil {
			port = parsed
		}
	}
	if port <= 0 || port > 65535 {
		return ipLimitBanEvent{}, false
	}

	timestamp := time.Now().Unix()
	if tsText := extractIPLimitLogField(line, regexp.MustCompile(`Timestamp\s*=\s*(\d+)`)); tsText != "" {
		if parsed, err := strconv.ParseInt(tsText, 10, 64); err == nil {
			timestamp = parsed
		}
	} else if len(line) >= len("2006/01/02 15:04:05") {
		if parsed, err := time.ParseInLocation("2006/01/02 15:04:05", line[:len("2006/01/02 15:04:05")], time.Local); err == nil {
			timestamp = parsed.Unix()
		}
	}

	return ipLimitBanEvent{
		ipLimitBanState: ipLimitBanState{IP: ip, Port: port, Timestamp: timestamp},
		Action:          action,
		Seq:             seq,
	}, true
}

func ipLimitLogLineMatchesEmail(line, clientEmail string) bool {
	return strings.Contains(line, "Email = "+clientEmail+" ||") ||
		strings.Contains(line, "Email = "+clientEmail+" ") ||
		strings.Contains(line, "[Email] = "+clientEmail+" ")
}

func extractIPLimitLogField(line string, re *regexp.Regexp) string {
	matches := re.FindStringSubmatch(line)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}
