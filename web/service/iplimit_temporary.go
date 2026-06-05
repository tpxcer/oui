package service

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ipLimitTemporaryUnbans sync.Map

func (s *InboundService) MarkClientIPLimitTemporaryUnban(clientEmail, ip string, port int, until time.Time) error {
	clientEmail = strings.TrimSpace(clientEmail)
	ip = strings.TrimSpace(ip)
	if clientEmail == "" {
		return fmt.Errorf("client email is required")
	}
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	if !until.After(time.Now()) {
		return fmt.Errorf("temporary unban expiry must be in the future")
	}
	ipLimitTemporaryUnbans.Store(ipLimitTemporaryUnbanKey(clientEmail, ip, port), until)
	return nil
}

func (s *InboundService) ClearClientIPLimitTemporaryUnban(clientEmail, ip string, port int) {
	ipLimitTemporaryUnbans.Delete(ipLimitTemporaryUnbanKey(clientEmail, ip, port))
}

func (s *InboundService) IsClientIPLimitTemporarilyUnbanned(clientEmail, ip string, port int, now time.Time) bool {
	value, ok := ipLimitTemporaryUnbans.Load(ipLimitTemporaryUnbanKey(clientEmail, ip, port))
	if !ok {
		return false
	}
	until, ok := value.(time.Time)
	if !ok || !now.Before(until) {
		ipLimitTemporaryUnbans.Delete(ipLimitTemporaryUnbanKey(clientEmail, ip, port))
		return false
	}
	return true
}

func ipLimitTemporaryUnbanKey(clientEmail, ip string, port int) string {
	return strings.TrimSpace(clientEmail) + "\x00" + strings.TrimSpace(ip) + "\x00" + strconv.Itoa(port)
}
