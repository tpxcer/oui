package job

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/xray"
)

func TestOnlineNotifyUsesUpstreamOnlineSetTransitions(t *testing.T) {
	onlineNotifyMu.Lock()
	onlineNotifySessions = map[string]onlineNotifySession{}
	onlineNotifyMu.Unlock()
	t.Cleanup(func() {
		onlineNotifyMu.Lock()
		onlineNotifySessions = map[string]onlineNotifySession{}
		onlineNotifyMu.Unlock()
	})

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
	if len(online) != 1 || online[0].email != "client@example.com" {
		t.Fatalf("online transitions = %#v, want one client@example.com", online)
	}
	if len(offline) != 0 {
		t.Fatalf("offline transitions = %#v, want none", offline)
	}

	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{"client@example.com": true}, traffic, now.Add(time.Second))
	onlineNotifyMu.Unlock()
	if len(online) != 0 || len(offline) != 0 {
		t.Fatalf("repeat online transitions = %#v/%#v, want none", online, offline)
	}

	onlineNotifyMu.Lock()
	online, offline = reconcileOnlineNotifySessions(trackable, map[string]bool{}, traffic, now.Add(21*time.Second))
	onlineNotifyMu.Unlock()
	if len(online) != 0 {
		t.Fatalf("online transitions after disappearance = %#v, want none", online)
	}
	if len(offline) != 1 || offline[0].email != "client@example.com" {
		t.Fatalf("offline transitions = %#v, want one client@example.com", offline)
	}
	if _, exists := onlineNotifySessions["client@example.com"]; exists {
		t.Fatalf("session still exists after upstream online-set disappearance")
	}
}
