package job

import (
	"testing"
	"time"

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
