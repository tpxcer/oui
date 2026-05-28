package service

import (
	"github.com/mhsanaei/3x-ui/v3/database/model"
	"github.com/mhsanaei/3x-ui/v3/util/common"
	"github.com/mhsanaei/3x-ui/v3/web/websocket"
)

type QuickInboundPresetInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type QuickInboundResult struct {
	Inbound     *model.Inbound `json:"inbound"`
	Email       string         `json:"email"`
	Firewall    string         `json:"firewall"`
	PresetKey   string         `json:"presetKey"`
	PresetLabel string         `json:"presetLabel"`
}

type QuickInboundService struct {
	inboundService InboundService
	settingService SettingService
	serverService  ServerService
	xrayService    XrayService
	userService    UserService
}

func (s *QuickInboundService) Presets() []QuickInboundPresetInfo {
	items := make([]QuickInboundPresetInfo, 0, len(tgQuickPresetOrder))
	for _, key := range tgQuickPresetOrder {
		preset, ok := tgQuickPresets[key]
		if !ok {
			continue
		}
		items = append(items, QuickInboundPresetInfo{Key: key, Label: preset.Label})
	}
	return items
}

func (s *QuickInboundService) Create(key string, userID int) (*QuickInboundResult, error) {
	preset, ok := tgQuickPresets[key]
	if !ok {
		return nil, common.NewError("未知的一键节点类型:", key)
	}
	if userID <= 0 {
		user, err := s.userService.GetFirstUser()
		if err != nil {
			return nil, err
		}
		userID = user.Id
	}

	helper := &Tgbot{
		inboundService: s.inboundService,
		settingService: s.settingService,
		serverService:  s.serverService,
		xrayService:    s.xrayService,
	}
	helper.SetHostname()

	inbound, email, err := helper.buildQuickInbound(key)
	if err != nil {
		return nil, err
	}
	inbound.UserId = userID

	created, needRestart, err := s.inboundService.AddInbound(inbound)
	if err != nil {
		return nil, err
	}
	if needRestart {
		s.xrayService.SetToNeedRestart()
	}
	websocket.BroadcastInvalidate(websocket.MessageTypeInbounds)

	return &QuickInboundResult{
		Inbound:     created,
		Email:       email,
		Firewall:    allowInboundPort(created.Port, preset.Transport),
		PresetKey:   preset.Key,
		PresetLabel: preset.Label,
	}, nil
}
