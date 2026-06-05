package job

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/web/service"
	"github.com/mhsanaei/3x-ui/v3/xray"
)

func resetOnlineNotifySessions(t *testing.T) {
	t.Helper()
	onlineNotifyMu.Lock()
	onlineNotifySessions = map[string]onlineNotifySession{}
	onlineNotifyMu.Unlock()
	t.Cleanup(func() {
		onlineNotifyMu.Lock()
		onlineNotifySessions = map[string]onlineNotifySession{}
		onlineNotifyMu.Unlock()
	})
}

func TestOnlineNotifyRequiresFiveMiBWithinFiveMinutes(t *testing.T) {
	resetOnlineNotifySessions(t)

	now := time.Unix(1000, 0)
	trackable := map[string]onlineNotifyInbound{
		"client@example.com": {remark: "node-a"},
	}
	traffic := map[string]*xray.ClientTraffic{
		"client@example.com": {Email: "client@example.com", Up: 100, Down: 200},
	}

	onlineNotifyMu.Lock()
	online, offline := reconcileOnlineNotifySessions(trackable, map[string]bool{"client@example.com": true}, traffic, now)
	onlineNotifyMu.Unlock()
	if len(online) != 0 {
		t.Fatalf("online transitions before traffic threshold = %#v, want none", online)
	}
	if len(offline) != 0 {
		t.Fatalf("offline transitions = %#v, want none", offline)
	}

	traffic["client@example.com"] = &xray.ClientTraffic{
		Email: "client@example.com",
		Up:    3 * 1024 * 1024,
		Down:  3 * 1024 * 1024,
	}
	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{"client@example.com": true}, traffic, now.Add(4*time.Minute))
	onlineNotifyMu.Unlock()
	if len(online) != 1 || online[0].email != "client@example.com" {
		t.Fatalf("online transitions after traffic threshold = %#v, want one client@example.com", online)
	}
	if len(offline) != 0 {
		t.Fatalf("offline transitions after traffic threshold = %#v, want none", offline)
	}

	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{"client@example.com": true}, traffic, now.Add(4*time.Minute+time.Second))
	onlineNotifyMu.Unlock()
	if len(online) != 0 || len(offline) != 0 {
		t.Fatalf("repeat transitions = %#v/%#v, want none", online, offline)
	}

	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{}, traffic, now.Add(5*time.Minute))
	onlineNotifyMu.Unlock()
	if len(online) != 0 {
		t.Fatalf("online transitions after disappearance = %#v, want none", online)
	}
	if len(offline) != 1 || offline[0].email != "client@example.com" {
		t.Fatalf("offline transitions = %#v, want one client@example.com", offline)
	}
	if _, exists := onlineNotifySessions["client@example.com"]; exists {
		t.Fatalf("session still exists after disappearance")
	}
}

func TestOnlineNotifyDoesNotSendOfflineIfOnlineWasNeverAnnounced(t *testing.T) {
	resetOnlineNotifySessions(t)

	now := time.Unix(2000, 0)
	trackable := map[string]onlineNotifyInbound{
		"idle@example.com": {remark: "node-b"},
	}
	traffic := map[string]*xray.ClientTraffic{
		"idle@example.com": {Email: "idle@example.com", Up: 100, Down: 200},
	}

	onlineNotifyMu.Lock()
	online, offline := reconcileOnlineNotifySessions(trackable, map[string]bool{"idle@example.com": true}, traffic, now)
	onlineNotifyMu.Unlock()
	if len(online) != 0 || len(offline) != 0 {
		t.Fatalf("initial idle transitions = %#v/%#v, want none", online, offline)
	}

	traffic["idle@example.com"] = &xray.ClientTraffic{
		Email: "idle@example.com",
		Up:    10 * 1024 * 1024,
		Down:  0,
	}
	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{"idle@example.com": true}, traffic, now.Add(6*time.Minute))
	onlineNotifyMu.Unlock()
	if len(online) != 0 || len(offline) != 0 {
		t.Fatalf("late threshold transitions = %#v/%#v, want none", online, offline)
	}

	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{}, traffic, now.Add(7*time.Minute))
	onlineNotifyMu.Unlock()
	if len(online) != 0 || len(offline) != 0 {
		t.Fatalf("offline without prior online transitions = %#v/%#v, want none", online, offline)
	}
}

func TestOnlineNotifyAnnouncesDifferentIPForSameClient(t *testing.T) {
	resetOnlineNotifySessions(t)

	now := time.Unix(3000, 0)
	trackable := map[string]onlineNotifyInbound{
		onlineNotifySessionKey("client@example.com", "1.1.1.1"): {
			email:  "client@example.com",
			remark: "node-a",
			ip:     "1.1.1.1",
		},
	}
	traffic := map[string]*xray.ClientTraffic{
		"client@example.com": {Email: "client@example.com", Up: 100, Down: 200},
	}

	onlineNotifyMu.Lock()
	online, offline := reconcileOnlineNotifySessions(trackable, map[string]bool{"client@example.com": true}, traffic, now)
	onlineNotifyMu.Unlock()
	if len(online) != 0 || len(offline) != 0 {
		t.Fatalf("initial transitions = %#v/%#v, want none", online, offline)
	}

	traffic["client@example.com"] = &xray.ClientTraffic{
		Email: "client@example.com",
		Up:    6 * 1024 * 1024,
		Down:  0,
	}
	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{"client@example.com": true}, traffic, now.Add(time.Minute))
	onlineNotifyMu.Unlock()
	if len(online) != 1 || online[0].session.ip != "1.1.1.1" || online[0].remark != "node-a" {
		t.Fatalf("first ip online transitions = %#v, want node-a for 1.1.1.1", online)
	}
	if len(offline) != 0 {
		t.Fatalf("first ip offline transitions = %#v, want none", offline)
	}

	trackable[onlineNotifySessionKey("client@example.com", "2.2.2.2")] = onlineNotifyInbound{
		email:  "client@example.com",
		remark: "node-a(ip2)",
		ip:     "2.2.2.2",
	}
	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{"client@example.com": true}, traffic, now.Add(2*time.Minute))
	onlineNotifyMu.Unlock()
	if len(online) != 0 || len(offline) != 0 {
		t.Fatalf("second ip initial transitions = %#v/%#v, want none before traffic threshold", online, offline)
	}

	traffic["client@example.com"] = &xray.ClientTraffic{
		Email: "client@example.com",
		Up:    12 * 1024 * 1024,
		Down:  0,
	}
	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{"client@example.com": true}, traffic, now.Add(3*time.Minute))
	onlineNotifyMu.Unlock()
	if len(online) != 1 || online[0].session.ip != "2.2.2.2" || online[0].remark != "node-a(ip2)" {
		t.Fatalf("second ip online transitions = %#v, want node-a(ip2) for 2.2.2.2", online)
	}
	if len(offline) != 0 {
		t.Fatalf("second ip offline transitions = %#v, want none", offline)
	}
}

func TestOnlineNotifyDifferentIPOfflineKeepsRule(t *testing.T) {
	resetOnlineNotifySessions(t)

	now := time.Unix(4000, 0)
	key := onlineNotifySessionKey("client@example.com", "1.1.1.1")
	onlineNotifySessions[key] = onlineNotifySession{
		email:     "client@example.com",
		remark:    "node-a",
		ip:        "1.1.1.1",
		start:     now,
		up:        0,
		down:      0,
		lastTotal: 6 * 1024 * 1024,
		announced: true,
	}
	trackable := map[string]onlineNotifyInbound{
		"client@example.com": {email: "client@example.com", remark: "node-a"},
	}
	traffic := map[string]*xray.ClientTraffic{
		"client@example.com": {Email: "client@example.com", Up: 6 * 1024 * 1024, Down: 0},
	}

	onlineNotifyMu.Lock()
	online, offline := reconcileOnlineNotifySessions(trackable, map[string]bool{}, traffic, now.Add(time.Minute))
	onlineNotifyMu.Unlock()
	if len(online) != 0 {
		t.Fatalf("online transitions after disappearance = %#v, want none", online)
	}
	if len(offline) != 1 || offline[0].email != "client@example.com" || offline[0].session.ip != "1.1.1.1" {
		t.Fatalf("offline transitions = %#v, want one ip session", offline)
	}
}

func TestLatestOnlineNotifyClientIPPrefersNewestTimestamp(t *testing.T) {
	got, ok := latestOnlineNotifyClientIP(`[
		{"ip":"139.201.252.156","timestamp":1780644977},
		{"ip":"8.8.8.8","timestamp":1780644999}
	]`)
	if !ok {
		t.Fatal("latestOnlineNotifyClientIP returned no IP")
	}
	if got.ip != "8.8.8.8" || got.timestamp != 1780644999 {
		t.Fatalf("latestOnlineNotifyClientIP = %#v, want 8.8.8.8 at 1780644999", got)
	}
}

func TestOnlineNotifyRemarkForIPIndex(t *testing.T) {
	if got := onlineNotifyRemarkForIPIndex("node-a", 0); got != "node-a" {
		t.Fatalf("onlineNotifyRemarkForIPIndex first = %q, want node-a", got)
	}
	if got := onlineNotifyRemarkForIPIndex("node-a", 1); got != "node-a(ip2)" {
		t.Fatalf("onlineNotifyRemarkForIPIndex second = %q, want node-a(ip2)", got)
	}
}

func TestLatestOnlineNotifyClientIPSupportsLegacyStringArray(t *testing.T) {
	got, ok := latestOnlineNotifyClientIP(`["1.1.1.1","139.201.252.156"]`)
	if !ok {
		t.Fatal("latestOnlineNotifyClientIP returned no IP")
	}
	if got.ip != "139.201.252.156" {
		t.Fatalf("latestOnlineNotifyClientIP = %#v, want last legacy IP", got)
	}
}

func TestFormatOnlineNotifyGeoLocationDeduplicatesFields(t *testing.T) {
	got := formatOnlineNotifyGeoLocation(service.NodeGeoLocation{
		Location: "中国四川省成都市",
		Country:  "中国",
		Province: "四川省",
		City:     "成都市",
		District: "成都市",
		Detail:   "锦江区",
	})
	want := "中国四川省成都市"
	if got != want {
		t.Fatalf("formatOnlineNotifyGeoLocation = %q, want %q", got, want)
	}
}

func TestFormatOnlineNotifyGeoLocationFallsBackToParts(t *testing.T) {
	got := formatOnlineNotifyGeoLocation(service.NodeGeoLocation{
		Country:  "中国",
		Province: "四川省",
		City:     "成都市",
		District: "成都市",
		Detail:   "锦江区",
	})
	want := "中国 四川省 成都市 锦江区"
	if got != want {
		t.Fatalf("formatOnlineNotifyGeoLocation = %q, want %q", got, want)
	}
}
