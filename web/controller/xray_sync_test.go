package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	xuilogger "github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/web/entity"
	"github.com/mhsanaei/3x-ui/v3/web/locale"
	"github.com/mhsanaei/3x-ui/v3/web/service"

	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"
)

func TestSyncXrayAfterMutationReturnsAPIErrorOnReloadFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	xuilogger.InitLogger(logging.ERROR)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/xui/inbound/add", nil)
	c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })

	oldApply := applyXrayConfigChange
	applyXrayConfigChange = func(_ *service.XrayService, _ string) error {
		return errors.New("synthetic reload failure")
	}
	t.Cleanup(func() { applyXrayConfigChange = oldApply })

	if syncXrayAfterMutation(c, &service.XrayService{}, "test api failure") {
		t.Fatalf("syncXrayAfterMutation returned true on reload failure")
	}

	var msg entity.Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if msg.Success {
		t.Fatalf("response success = true, want false")
	}
	if !strings.Contains(msg.Msg, "数据已保存，但 Xray 配置重载失败") {
		t.Fatalf("response message %q does not expose reload failure", msg.Msg)
	}
	if !strings.Contains(msg.Msg, "synthetic reload failure") {
		t.Fatalf("response message %q does not include concrete reload error", msg.Msg)
	}
}
