package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/xray"
)

func TestRefreshOnlineClientsUsesUpstreamTwentySecondWindow(t *testing.T) {
	oldProcess := p
	p = xray.NewProcess(&xray.Config{})
	t.Cleanup(func() {
		p.Stop()
		p = oldProcess
	})

	now := time.Now().UnixMilli()
	svc := InboundService{}
	svc.RefreshOnlineClientsFromMap(map[string]int64{
		"fresh@example.com": now - int64(19*time.Second/time.Millisecond),
		"stale@example.com": now - int64(21*time.Second/time.Millisecond),
	})

	got := svc.GetOnlineClients()
	if len(got) != 1 || got[0] != "fresh@example.com" {
		t.Fatalf("online clients = %#v, want only fresh@example.com", got)
	}
}
