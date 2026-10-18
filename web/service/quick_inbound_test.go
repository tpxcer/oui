package service

import (
	"regexp"
	"testing"
)

func TestQuickClientNameIsRandomSuffix(t *testing.T) {
	client := quickClient("client-id", "", "", false)
	if !regexp.MustCompile(`^[a-z0-9]{6}$`).MatchString(client.Email) {
		t.Fatalf("client email = %q, want a six-character lowercase alphanumeric suffix", client.Email)
	}
}

func TestQuickPresetTypeNamesAreShortASCII(t *testing.T) {
	expected := map[string]string{
		"hysteria2":         "HY2",
		"vlessReality":      "Reality",
		"vlessXhttpTls":     "XHTTP-TLS",
		"vlessXhttpReality": "XHTTP-Reality",
	}

	for key, want := range expected {
		preset, ok := tgQuickPresets[key]
		if !ok {
			t.Fatalf("missing quick preset %q", key)
		}
		if preset.Remark != want {
			t.Fatalf("preset %q remark = %q, want %q", key, preset.Remark, want)
		}
		for _, r := range preset.Remark {
			if r > 127 {
				t.Fatalf("preset %q remark contains non-ASCII rune %q", key, r)
			}
		}
		if len(preset.Remark) > 13 {
			t.Fatalf("preset %q remark is too long: %q", key, preset.Remark)
		}
	}
}

func TestQuickInboundRemarkIncludesFlagCountryTypeAndClient(t *testing.T) {
	cases := []struct {
		geo  NodeGeoLocation
		want string
	}{
		{NodeGeoLocation{Country: "美国", CountryCode: "US", Province: "加利福尼亚州", City: "洛杉矶"}, "🇺🇸 美国-Reality-dyv675"},
		{NodeGeoLocation{Country: "中国", CountryCode: "CN", Province: "台湾", City: "彰化"}, "🇹🇼 台湾-Reality-dyv675"},
		{NodeGeoLocation{Country: "中国", CountryCode: "CN", Province: "香港"}, "🇭🇰 香港-Reality-dyv675"},
		{NodeGeoLocation{Country: "中国", CountryCode: "CN", Province: "澳门特别行政区"}, "🇲🇴 澳门-Reality-dyv675"},
		{NodeGeoLocation{Country: "中国", CountryCode: "CN", Province: "广东", City: "广州市"}, "🇨🇳 中国-Reality-dyv675"},
		{NodeGeoLocation{Country: "台湾", CountryCode: "TW"}, "🇹🇼 台湾-Reality-dyv675"},
		{NodeGeoLocation{Country: "某地", CountryCode: "X1"}, "某地-Reality-dyv675"},
		{NodeGeoLocation{Error: "lookup failed"}, "Reality-dyv675"},
	}
	for _, c := range cases {
		if got := quickInboundRemark(c.geo, "Reality", "dyv675"); got != c.want {
			t.Fatalf("quickInboundRemark(%+v) = %q, want %q", c.geo, got, c.want)
		}
	}
}

func TestQuickCountryNameDoesNotUseCityOrProvince(t *testing.T) {
	cases := []struct {
		geo  NodeGeoLocation
		want string
	}{
		{NodeGeoLocation{Country: "美国", Province: "加利福尼亚州", City: "洛杉矶"}, "美国"},
		{NodeGeoLocation{Country: "日本", Province: "东京都"}, "日本"},
		{NodeGeoLocation{Country: " 新加坡 "}, "新加坡"},
		{NodeGeoLocation{Country: "新加坡"}, "新加坡"},
		{NodeGeoLocation{Province: "东京都", City: "东京"}, ""},
		{NodeGeoLocation{Error: "lookup failed"}, ""},
	}
	for _, c := range cases {
		if got := quickCountryName(c.geo); got != c.want {
			t.Fatalf("quickCountryName(%+v) = %q, want %q", c.geo, got, c.want)
		}
	}
}

func TestQuickPresetRemarkUsesGeneratedClientName(t *testing.T) {
	geo := NodeGeoLocation{Country: "美国", CountryCode: "US", City: "洛杉矶"}
	for key, preset := range tgQuickPresets {
		t.Run(key, func(t *testing.T) {
			client := quickClient("client-id", "", "", false)
			want := "🇺🇸 美国-" + preset.Remark + "-" + client.Email
			if got := quickInboundRemark(geo, preset.Remark, client.Email); got != want {
				t.Fatalf("remark = %q, want %q", got, want)
			}
		})
	}
}
