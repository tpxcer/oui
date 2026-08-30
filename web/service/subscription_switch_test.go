package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
)

type directClientLinkProvider struct{}

func (directClientLinkProvider) SubLinksForSubId(string, string) ([]string, error) {
	return nil, nil
}

func (directClientLinkProvider) LinksForClient(_ string, _ *model.Inbound, email string) []string {
	return []string{"vless://direct#" + email}
}

func TestDisabledSubscriptionDoesNotGenerateSubID(t *testing.T) {
	setupConflictDB(t)
	if err := (&SettingService{}).saveSetting("subEnable", "false"); err != nil {
		t.Fatalf("disable subscription: %v", err)
	}

	inbound := &model.Inbound{
		Tag:            "no-sub-id",
		Enable:         false,
		Port:           56001,
		Protocol:       model.VLESS,
		StreamSettings: `{"network":"tcp"}`,
		Settings:       `{"clients":[{"email":"no-sub@example.com","id":"11111111-1111-1111-1111-111111111111","enable":true}]}`,
	}
	created, _, err := (&InboundService{}).AddInbound(inbound)
	if err != nil {
		t.Fatalf("AddInbound: %v", err)
	}

	record := &model.ClientRecord{}
	if err := database.GetDB().Where("email = ?", "no-sub@example.com").First(record).Error; err != nil {
		t.Fatalf("load client record: %v", err)
	}
	if record.SubID != "" {
		t.Fatalf("client record subId = %q, want empty", record.SubID)
	}

	loaded, err := (&InboundService{}).GetInbound(created.Id)
	if err != nil {
		t.Fatalf("GetInbound: %v", err)
	}
	var settings struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(loaded.Settings), &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if len(settings.Clients) != 1 || settings.Clients[0].SubID != "" {
		t.Fatalf("inbound clients = %+v, want empty subId", settings.Clients)
	}
}

func TestDisabledSubscriptionSkipsSubscriptionQRCodesWithoutError(t *testing.T) {
	setupConflictDB(t)
	if err := (&SettingService{}).saveSetting("subEnable", "false"); err != nil {
		t.Fatalf("disable subscription: %v", err)
	}
	bot := &Tgbot{settingService: SettingService{}}
	subURL, subJSONURL, err := bot.optionalSubscriptionURLs("missing@example.com")
	if err != nil {
		t.Fatalf("optionalSubscriptionURLs: %v", err)
	}
	if subURL != "" || subJSONURL != "" {
		t.Fatalf("subscription URLs = %q, %q; want empty", subURL, subJSONURL)
	}
}

func TestDisabledSubscriptionStillProvidesIndividualClientLinks(t *testing.T) {
	setupConflictDB(t)
	if err := (&SettingService{}).saveSetting("subEnable", "false"); err != nil {
		t.Fatalf("disable subscription: %v", err)
	}

	inbound := &model.Inbound{
		Tag:            "direct-link-without-sub-id",
		Enable:         false,
		Port:           56002,
		Protocol:       model.VLESS,
		StreamSettings: `{"network":"tcp"}`,
		Settings:       `{"clients":[{"email":"direct@example.com","id":"22222222-2222-2222-2222-222222222222","enable":true}]}`,
	}
	if _, _, err := (&InboundService{}).AddInbound(inbound); err != nil {
		t.Fatalf("AddInbound: %v", err)
	}

	originalProvider := registeredSubLinkProvider
	RegisterSubLinkProvider(directClientLinkProvider{})
	t.Cleanup(func() { registeredSubLinkProvider = originalProvider })

	links, err := (&InboundService{}).GetAllClientLinks("panel.example.com", "direct@example.com")
	if err != nil {
		t.Fatalf("GetAllClientLinks: %v", err)
	}
	if len(links) != 1 || links[0] != "vless://direct#direct@example.com" {
		t.Fatalf("individual links = %#v", links)
	}
}
