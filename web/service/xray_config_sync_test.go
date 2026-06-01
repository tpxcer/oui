package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database/model"
	panelruntime "github.com/mhsanaei/3x-ui/v3/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/xray"
)

func setupXrayConfigSyncTest(t *testing.T) {
	t.Helper()
	setupConflictDB(t)
	panelruntime.SetManager(panelruntime.NewManager(panelruntime.LocalDeps{
		APIPort:        func() int { return 0 },
		SetNeedRestart: func() {},
	}))
	p = nil
	isNeedXrayRestart.Store(false)
	isManuallyStopped.Store(false)
	result = ""
	t.Cleanup(func() {
		p = nil
		isNeedXrayRestart.Store(false)
		isManuallyStopped.Store(false)
		panelruntime.SetManager(nil)
	})
}

func addConfigSyncInbound(t *testing.T, svc *InboundService, remark string, port int, enable bool) *model.Inbound {
	t.Helper()
	email := fmt.Sprintf("%s@example.com", remark)
	inbound := &model.Inbound{
		UserId:         1,
		Remark:         remark,
		Enable:         enable,
		Port:           port,
		Protocol:       model.VLESS,
		Settings:       fmt.Sprintf(`{"clients":[{"email":%q,"id":"11111111-1111-1111-1111-%012d","enable":true}],"decryption":"none"}`, email, port),
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Sniffing:       `{"enabled":false}`,
	}
	created, _, err := svc.AddInbound(inbound)
	if err != nil {
		t.Fatalf("AddInbound(%s): %v", remark, err)
	}
	return created
}

func getTestXrayConfig(t *testing.T) *xray.Config {
	t.Helper()
	cfg, err := (&XrayService{}).GetXrayConfig()
	if err != nil {
		t.Fatalf("GetXrayConfig: %v", err)
	}
	return cfg
}

func inboundByPort(cfg *xray.Config, port int) *xray.InboundConfig {
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Port == port {
			return &cfg.InboundConfigs[i]
		}
	}
	return nil
}

func TestXrayConfigSync_AddInboundConfigContainsPort(t *testing.T) {
	setupXrayConfigSyncTest(t)
	svc := &InboundService{}

	addConfigSyncInbound(t, svc, "tw", 50461, true)

	if got := inboundByPort(getTestXrayConfig(t), 50461); got == nil {
		t.Fatalf("generated config does not contain port 50461")
	}
}

func TestXrayConfigSync_UpdateInboundPortReplacesOldPort(t *testing.T) {
	setupXrayConfigSyncTest(t)
	svc := &InboundService{}
	created := addConfigSyncInbound(t, svc, "move", 50462, true)

	updated := *created
	updated.Port = 50463
	if _, _, err := svc.UpdateInbound(&updated); err != nil {
		t.Fatalf("UpdateInbound: %v", err)
	}

	cfg := getTestXrayConfig(t)
	if got := inboundByPort(cfg, 50462); got != nil {
		t.Fatalf("old port 50462 still exists in generated config")
	}
	if got := inboundByPort(cfg, 50463); got == nil {
		t.Fatalf("new port 50463 does not exist in generated config")
	}
}

func TestXrayConfigSync_DeleteInboundRemovesPort(t *testing.T) {
	setupXrayConfigSyncTest(t)
	svc := &InboundService{}
	created := addConfigSyncInbound(t, svc, "delete", 50464, true)

	if _, err := svc.DelInbound(created.Id); err != nil {
		t.Fatalf("DelInbound: %v", err)
	}

	if got := inboundByPort(getTestXrayConfig(t), 50464); got != nil {
		t.Fatalf("deleted port 50464 still exists in generated config")
	}
}

func TestXrayConfigSync_DisabledInboundExcluded(t *testing.T) {
	setupXrayConfigSyncTest(t)
	svc := &InboundService{}

	addConfigSyncInbound(t, svc, "disabled", 50465, false)

	if got := inboundByPort(getTestXrayConfig(t), 50465); got != nil {
		t.Fatalf("disabled inbound port 50465 exists in generated config")
	}
}

func TestXrayConfigSync_ConcurrentCreateKeepsBothInbounds(t *testing.T) {
	setupXrayConfigSyncTest(t)
	svc := &InboundService{}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, port := range []int{50466, 50467} {
		wg.Add(1)
		go func(i, port int) {
			defer wg.Done()
			_, _, err := svc.AddInbound(&model.Inbound{
				UserId:         1,
				Remark:         fmt.Sprintf("concurrent-%d", i),
				Enable:         true,
				Port:           port,
				Protocol:       model.VLESS,
				Settings:       fmt.Sprintf(`{"clients":[{"email":"concurrent-%d@example.com","id":"22222222-2222-2222-2222-%012d","enable":true}],"decryption":"none"}`, i, port),
				StreamSettings: `{"network":"tcp","security":"none"}`,
				Sniffing:       `{"enabled":false}`,
			})
			errs <- err
		}(i, port)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent AddInbound: %v", err)
		}
	}

	cfg := getTestXrayConfig(t)
	for _, port := range []int{50466, 50467} {
		if got := inboundByPort(cfg, port); got == nil {
			t.Fatalf("port %d missing from final generated config", port)
		}
	}
}

func TestXrayConfigSync_VlessXHTTPRealityRegression(t *testing.T) {
	setupXrayConfigSyncTest(t)
	svc := &InboundService{}
	inbound := &model.Inbound{
		UserId:   1,
		Remark:   "xhttp-reality",
		Enable:   true,
		Port:     50468,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"email":"xhttp-reality@example.com","id":"33333333-3333-3333-3333-333333333333","enable":true}],"decryption":"none","encryption":"none"}`,
		StreamSettings: `{
			"network":"xhttp",
			"security":"reality",
			"realitySettings":{"show":false,"dest":"www.microsoft.com:443","xver":0,"serverNames":["www.microsoft.com"],"privateKey":"test-private-key","minClientVer":"","maxClientVer":"","maxTimeDiff":0,"shortIds":["0123456789abcdef"]},
			"xhttpSettings":{"path":"/xhttp-test","host":"www.microsoft.com","mode":"auto","headers":{}}
		}`,
		Sniffing: `{"enabled":false}`,
	}
	if _, _, err := svc.AddInbound(inbound); err != nil {
		t.Fatalf("AddInbound xhttp reality: %v", err)
	}

	got := inboundByPort(getTestXrayConfig(t), 50468)
	if got == nil {
		t.Fatalf("xhttp reality inbound missing from generated config")
	}
	var stream map[string]any
	if err := json.Unmarshal(got.StreamSettings, &stream); err != nil {
		t.Fatalf("unmarshal streamSettings: %v", err)
	}
	if stream["network"] != "xhttp" || stream["security"] != "reality" {
		t.Fatalf("streamSettings = %#v, want network=xhttp security=reality", stream)
	}
}

func TestXrayConfigSync_ApplyConfigChangeWritesConfigAndStartsOnce(t *testing.T) {
	setupXrayConfigSyncTest(t)
	addConfigSyncInbound(t, &InboundService{}, "reload", 50469, true)

	restore := mockXrayRestartHooks(t)
	defer restore()

	var tests, writes, starts int
	var written []byte
	testXrayConfigFile = func(data []byte) error {
		tests++
		return nil
	}
	writeXrayConfigFile = func(_ string, data []byte) error {
		writes++
		written = append([]byte(nil), data...)
		return nil
	}
	startXrayProcess = func(_ *xray.Process) error {
		starts++
		return nil
	}

	if err := (&XrayService{}).ApplyConfigChange("test reload"); err != nil {
		t.Fatalf("ApplyConfigChange: %v", err)
	}
	if tests != 1 || writes != 1 || starts != 1 {
		t.Fatalf("calls tests/writes/starts = %d/%d/%d, want 1/1/1", tests, writes, starts)
	}
	if !strings.Contains(string(written), `"port": 50469`) {
		t.Fatalf("written config does not contain port 50469: %s", string(written))
	}
}

func TestXrayConfigSync_ReloadFailureReturnsErrorAndDoesNotStart(t *testing.T) {
	setupXrayConfigSyncTest(t)
	addConfigSyncInbound(t, &InboundService{}, "bad", 50470, true)

	restore := mockXrayRestartHooks(t)
	defer restore()

	var starts int
	testXrayConfigFile = func(_ []byte) error {
		return errors.New("synthetic config test failure")
	}
	startXrayProcess = func(_ *xray.Process) error {
		starts++
		return nil
	}

	err := (&XrayService{}).ApplyConfigChange("test failure")
	if err == nil {
		t.Fatalf("ApplyConfigChange returned nil on config test failure")
	}
	if starts != 0 {
		t.Fatalf("start called %d times after config test failure, want 0", starts)
	}
}

func mockXrayRestartHooks(t *testing.T) func() {
	t.Helper()
	oldMarshal := marshalXrayConfig
	oldTest := testXrayConfigFile
	oldWrite := writeXrayConfigFile
	oldNew := newXrayProcess
	oldStart := startXrayProcess
	oldStop := stopXrayProcess
	return func() {
		marshalXrayConfig = oldMarshal
		testXrayConfigFile = oldTest
		writeXrayConfigFile = oldWrite
		newXrayProcess = oldNew
		startXrayProcess = oldStart
		stopXrayProcess = oldStop
	}
}
