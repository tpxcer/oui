package controller

import (
	"fmt"

	"github.com/mhsanaei/3x-ui/v3/web/service"

	"github.com/gin-gonic/gin"
)

var applyXrayConfigChange = func(xrayService *service.XrayService, operation string) error {
	return xrayService.ApplyConfigChange(operation)
}

func syncXrayAfterMutation(c *gin.Context, xrayService *service.XrayService, operation string) bool {
	if err := applyXrayConfigChange(xrayService, operation); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), fmt.Errorf("数据已保存，但 Xray 配置重载失败: %w", err))
		return false
	}
	return true
}
