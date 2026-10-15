package service

import "testing"

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

func TestQuickInboundRemarkIncludesFlagRegionTypeAndPort(t *testing.T) {
	cases := []struct {
		geo  NodeGeoLocation
		want string
	}{
		{NodeGeoLocation{Country: "美国", CountryCode: "US", Province: "加利福尼亚州", City: "洛杉矶"}, "🇺🇸 洛杉矶-Reality-52171"},
		{NodeGeoLocation{Country: "中国", CountryCode: "CN", Province: "台湾", City: "彰化"}, "🇹🇼 彰化-Reality-52171"},
		{NodeGeoLocation{Country: "中国", CountryCode: "CN", Province: "香港"}, "🇭🇰 香港-Reality-52171"},
		{NodeGeoLocation{Country: "中国", CountryCode: "CN", Province: "广东", City: "广州市"}, "🇨🇳 广州-Reality-52171"},
		{NodeGeoLocation{Country: "台湾", CountryCode: "TW"}, "🇹🇼 台湾-Reality-52171"},
		{NodeGeoLocation{Country: "某地", CountryCode: "X1"}, "某地-Reality-52171"},
		{NodeGeoLocation{Error: "lookup failed"}, "Reality-52171"},
	}
	for _, c := range cases {
		if got := quickInboundRemark(c.geo, "Reality", 52171); got != c.want {
			t.Fatalf("quickInboundRemark(%+v) = %q, want %q", c.geo, got, c.want)
		}
	}
}

func TestQuickRegionNamePrefersCityAndFallsBack(t *testing.T) {
	cases := []struct {
		geo  NodeGeoLocation
		want string
	}{
		{NodeGeoLocation{Country: "美国", Province: "加利福尼亚州", City: "洛杉矶"}, "洛杉矶"},
		{NodeGeoLocation{Country: "中国", Province: "台湾", City: "台北市"}, "台北"},
		{NodeGeoLocation{Country: "日本", Province: "东京都"}, "东京都"},
		{NodeGeoLocation{Country: "新加坡"}, "新加坡"},
		{NodeGeoLocation{Error: "lookup failed"}, ""},
	}
	for _, c := range cases {
		if got := quickRegionName(c.geo); got != c.want {
			t.Fatalf("quickRegionName(%+v) = %q, want %q", c.geo, got, c.want)
		}
	}
}
