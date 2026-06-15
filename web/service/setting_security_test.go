package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
)

func setupSettingTestDB(t *testing.T) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.CloseDB(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestGetAllSettingViewRedactsSecrets(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}
	if err := s.saveSetting("tgBotToken", "telegram-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("twoFactorToken", "totp-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("ldapPassword", "ldap-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("serverProviderAPIKey", "provider-secret"); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.ApiToken{Name: "test", Token: "api-secret", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}

	view, err := s.GetAllSettingView()
	if err != nil {
		t.Fatal(err)
	}
	if view.TgBotToken != "" || view.TwoFactorToken != "" || view.LdapPassword != "" || view.ServerProviderAPIKey != "" {
		t.Fatalf("settings view leaked secrets: %#v", view)
	}
	if !view.HasTgBotToken || !view.HasTwoFactorToken || !view.HasLdapPassword || !view.HasServerProviderAPIKey || !view.HasApiToken {
		t.Fatalf("settings view did not report configured secret flags: %#v", view)
	}
}

func TestGetDisplaySecretAllowsOnlyWhitelistedSecrets(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}
	if err := s.saveSetting("tgBotToken", "telegram-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("serverProviderAPIKey", "provider-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("ldapPassword", "ldap-secret"); err != nil {
		t.Fatal(err)
	}

	if got, err := s.GetDisplaySecret("tgBotToken"); err != nil || got != "telegram-secret" {
		t.Fatalf("tgBotToken secret = %q, %v", got, err)
	}
	if got, err := s.GetDisplaySecret("serverProviderAPIKey"); err != nil || got != "provider-secret" {
		t.Fatalf("serverProviderAPIKey secret = %q, %v", got, err)
	}
	if got, err := s.GetDisplaySecret("ldapPassword"); err == nil || got != "" {
		t.Fatalf("ldapPassword should not be readable, got %q, %v", got, err)
	}
}

func TestUpdateAllSettingPreservesTelegramTokenWhenBotDisabled(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}
	if err := s.saveSetting("tgBotToken", "telegram-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("ldapPassword", "ldap-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("twoFactorEnable", "true"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("twoFactorToken", "totp-secret"); err != nil {
		t.Fatal(err)
	}

	view, err := s.GetAllSettingView()
	if err != nil {
		t.Fatal(err)
	}
	settings := &view.AllSetting
	if err := s.UpdateAllSetting(settings); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetTgBotToken(); got != "telegram-secret" {
		t.Fatalf("tg token = %q, want preserved secret", got)
	}
	if got, _ := s.GetLdapPassword(); got != "ldap-secret" {
		t.Fatalf("ldap password = %q, want preserved secret", got)
	}
	if got, _ := s.GetTwoFactorToken(); got != "totp-secret" {
		t.Fatalf("2fa token = %q, want preserved secret", got)
	}
}

func TestUpdateAllSettingUsesSavedTelegramTokenWhenEnabled(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}
	if err := s.saveSetting("tgBotToken", "telegram-secret"); err != nil {
		t.Fatal(err)
	}

	view, err := s.GetAllSettingView()
	if err != nil {
		t.Fatal(err)
	}
	settings := &view.AllSetting
	settings.TgBotEnable = true
	settings.TgBotToken = ""
	settings.TgBotChatId = "123456"

	if err := s.UpdateAllSetting(settings); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetTgBotToken(); got != "telegram-secret" {
		t.Fatalf("tg token = %q, want saved secret", got)
	}
}

func TestUpdateAllSettingRequiresTelegramTokenWhenEnabledWithoutSavedToken(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}

	view, err := s.GetAllSettingView()
	if err != nil {
		t.Fatal(err)
	}
	settings := &view.AllSetting
	settings.TgBotEnable = true
	settings.TgBotToken = ""
	settings.TgBotChatId = "123456"

	if err := s.UpdateAllSetting(settings); err == nil {
		t.Fatal("expected empty Telegram token to be rejected when bot is enabled without saved token")
	}
}

func TestUpdateAllSettingRequiresTelegramChatIDWhenEnabled(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}
	if err := s.saveSetting("tgBotToken", "telegram-secret"); err != nil {
		t.Fatal(err)
	}

	view, err := s.GetAllSettingView()
	if err != nil {
		t.Fatal(err)
	}
	settings := &view.AllSetting
	settings.TgBotEnable = true
	settings.TgBotToken = "new-telegram-secret"
	settings.TgBotChatId = ""

	if err := s.UpdateAllSetting(settings); err == nil {
		t.Fatal("expected empty Telegram chat ID to be rejected when bot is enabled")
	}
	if got, _ := s.GetTgBotToken(); got != "telegram-secret" {
		t.Fatalf("tg token = %q, want unchanged secret", got)
	}
}

func TestWebBasePathIsStoredWithoutTrailingSlash(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}

	cases := []struct {
		in   string
		want string
	}{
		{"", "/"},
		{"/", "/"},
		{"abc", "/abc"},
		{"/abc", "/abc"},
		{"/abc/", "/abc"},
		{"///abc///", "/abc"},
	}

	for _, tc := range cases {
		if err := s.SetBasePath(tc.in); err != nil {
			t.Fatalf("SetBasePath(%q): %v", tc.in, err)
		}
		got, err := s.GetBasePath()
		if err != nil {
			t.Fatalf("GetBasePath(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("base path for %q = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizePublicHTTPURLBlocksPrivateAddressUnlessAllowed(t *testing.T) {
	if _, err := SanitizePublicHTTPURL("http://127.0.0.1:8080/hook", false); err == nil {
		t.Fatal("expected localhost URL to be blocked")
	}
	if got, err := SanitizePublicHTTPURL("http://127.0.0.1:8080/hook", true); err != nil || got != "http://127.0.0.1:8080/hook" {
		t.Fatalf("allowPrivate result = %q, %v", got, err)
	}
}
