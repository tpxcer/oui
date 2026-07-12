package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestJoinGeoAddressKeepsFullAddress(t *testing.T) {
	got := joinGeoAddress("中国", "台湾省", "彰化县", "埔盐乡")
	if got != "中国台湾省彰化县埔盐乡" {
		t.Fatalf("joinGeoAddress() = %q", got)
	}
}

func TestJoinGeoAddressDoesNotDuplicateDetail(t *testing.T) {
	got := joinGeoAddress("中国", "台湾省", "彰化县", "埔盐乡", "埔盐乡")
	if got != "中国台湾省彰化县埔盐乡" {
		t.Fatalf("joinGeoAddress() with duplicate detail = %q", got)
	}
}

func TestJoinGeoAddressUsesFullDetailWhenProvided(t *testing.T) {
	got := joinGeoAddress("中国", "台湾省", "彰化县", "埔盐乡", "中国台湾省彰化县埔盐乡")
	if got != "中国台湾省彰化县埔盐乡" {
		t.Fatalf("joinGeoAddress() with full detail = %q", got)
	}
}

func TestCachedIPGeoTTLSeparatesSuccessAndFailure(t *testing.T) {
	successTTL := 48 * time.Hour
	failureTTL := 15 * time.Minute
	if got := cachedIPGeoTTL(NodeGeoLocation{Location: "中国四川资阳中国电信"}, successTTL, failureTTL); got != successTTL {
		t.Fatalf("cachedIPGeoTTL success = %s, want %s", got, successTTL)
	}
	if got := cachedIPGeoTTL(NodeGeoLocation{Error: "lookup failed"}, successTTL, failureTTL); got != failureTTL {
		t.Fatalf("cachedIPGeoTTL error = %s, want %s", got, failureTTL)
	}
	if got := cachedIPGeoTTL(NodeGeoLocation{}, successTTL, failureTTL); got != failureTTL {
		t.Fatalf("cachedIPGeoTTL empty = %s, want %s", got, failureTTL)
	}
}

func TestIPAttributionFailureCacheTTL(t *testing.T) {
	if ipAttrFailureCacheTTL != 5*time.Minute {
		t.Fatalf("ip attribution failure cache TTL = %s, want 5m", ipAttrFailureCacheTTL)
	}
}

func TestDecodeIPWhoNodeGeo(t *testing.T) {
	geo, err := decodeIPWhoNodeGeo(strings.NewReader(`{
		"success": true,
		"country": "Netherlands",
		"country_code": "NL",
		"region": "Groningen",
		"city": "Groningen",
		"latitude": 30.67,
		"longitude": 104.06,
		"connection": {"isp": "Example ISP", "org": "Example Org"}
	}`), "203.0.113.1")
	if err != nil {
		t.Fatal(err)
	}
	if geo.Source != "ipwho" || geo.Country != "荷兰" || geo.Province != "Groningen" || geo.City != "Groningen" {
		t.Fatalf("unexpected ipwho geo: %#v", geo)
	}
	if geo.Detail != "Example ISP" || geo.Location != "荷兰GroningenExample ISP" {
		t.Fatalf("unexpected ipwho detail/location: %#v", geo)
	}
}

func TestNormalizeChineseAttributionDropsEnglishGeoParts(t *testing.T) {
	geo := normalizeChineseAttribution(NodeGeoLocation{
		Country:  "美国",
		Province: "加利福尼亚州",
		City:     "San Francisco",
		Detail:   "Google Cloud",
		Location: "美国加利福尼亚州San FranciscoGoogle Cloud",
	})
	if geo.Country != "美国" || geo.Province != "加利福尼亚州" || geo.City != "" {
		t.Fatalf("unexpected normalized geo: %#v", geo)
	}
	if geo.Location != "美国加利福尼亚州" || geo.Detail != "Google Cloud" {
		t.Fatalf("unexpected normalized location/detail: %#v", geo)
	}
}

func TestFetchIPAttributionPrefersChinesePrimary(t *testing.T) {
	fallbackCalled := false
	primary := func(context.Context, string) (NodeGeoLocation, error) {
		return NodeGeoLocation{Source: "ip9", Country: "荷兰", Province: "格罗宁根", City: "埃姆斯哈文"}, nil
	}
	fallback := func(context.Context, string) (NodeGeoLocation, error) {
		fallbackCalled = true
		return NodeGeoLocation{Source: "ipwho", Country: "荷兰"}, nil
	}
	geo, err := fetchIPAttributionNodeGeoWith(context.Background(), "34.6.139.141", primary, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if fallbackCalled || geo.Source != "ip9" || geo.Location != "荷兰格罗宁根埃姆斯哈文" {
		t.Fatalf("unexpected primary geo: %#v, fallbackCalled=%v", geo, fallbackCalled)
	}
}

func TestFetchIPAttributionFallsBackToIP9(t *testing.T) {
	primary := func(context.Context, string) (NodeGeoLocation, error) {
		return NodeGeoLocation{}, errors.New("primary unavailable")
	}
	fallback := func(context.Context, string) (NodeGeoLocation, error) {
		return NodeGeoLocation{Source: "ip9", Country: "中国", Province: "四川", City: "成都"}, nil
	}
	geo, err := fetchIPAttributionNodeGeoWith(context.Background(), "203.0.113.1", primary, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if geo.Source != "ip9" || geo.Location != "中国四川成都" {
		t.Fatalf("unexpected fallback geo: %#v", geo)
	}
}

func TestFetchIPAttributionReturnsBothErrors(t *testing.T) {
	primary := func(context.Context, string) (NodeGeoLocation, error) {
		return NodeGeoLocation{}, errors.New("primary unavailable")
	}
	fallback := func(context.Context, string) (NodeGeoLocation, error) {
		return NodeGeoLocation{}, errors.New("fallback unavailable")
	}
	_, err := fetchIPAttributionNodeGeoWith(context.Background(), "203.0.113.1", primary, fallback)
	if err == nil || !strings.Contains(err.Error(), "ip9: primary unavailable") || !strings.Contains(err.Error(), "ipwho: fallback unavailable") {
		t.Fatalf("unexpected attribution error: %v", err)
	}
}

func TestBuildServerProviderURLAddsCredentials(t *testing.T) {
	got, err := buildServerProviderURL("https://api.example.com/service?x=1", "123", "abc")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"https://api.example.com/service?", "x=1", "veid=123", "api_key=abc"} {
		if !strings.Contains(got, part) {
			t.Fatalf("provider url %q missing %q", got, part)
		}
	}
}

func TestBuildServerProviderURLSupportsPlaceholders(t *testing.T) {
	got, err := buildServerProviderURL("https://api.example.com/{veid}?key={apiKey}", "12 3", "a/b")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.example.com/12+3?key=a%2Fb" {
		t.Fatalf("provider placeholder url = %q", got)
	}
}

func TestBuildServerProviderURLSupportsEncodedPlaceholders(t *testing.T) {
	got, err := buildServerProviderURL("https://api.example.com/%7Bveid%7D?key=%7Bapi_key%7D", "123", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.example.com/123?key=abc" {
		t.Fatalf("encoded placeholder url = %q", got)
	}
}

func TestParseServerProviderInfoSupportsSnakeAndCamelCase(t *testing.T) {
	rawJSON := []byte(`{
		"error": false,
		"data": {
			"hostname": "srv.example",
			"nodeAlias": "NodeA",
			"nodeLocation": "TW",
			"planMonthlyData": "1000",
			"dataCounter": 250,
			"ipAddresses": "1.1.1.1,2.2.2.2",
			"rdnsApiAvailable": true,
			"ptr": {"1.1.1.1": "one.example"}
		}
	}`)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		t.Fatal(err)
	}
	info := parseServerProviderInfo(raw)
	if info.Hostname != "srv.example" || info.NodeAlias != "NodeA" || info.NodeLocation != "TW" {
		t.Fatalf("unexpected provider info: %#v", info)
	}
	if info.PlanMonthlyData != 1000 || info.DataCounter != 250 {
		t.Fatalf("unexpected traffic fields: %#v", info)
	}
	if len(info.IPAddresses) != 2 || info.IPAddresses[0] != "1.1.1.1" || info.PTR["1.1.1.1"] != "one.example" {
		t.Fatalf("unexpected ip fields: %#v", info)
	}
	if msg := cloud64ErrorMessage(info.Error); msg != "" {
		t.Fatalf("false error should be empty, got %q", msg)
	}
}
