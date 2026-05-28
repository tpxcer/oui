package service

import (
	"encoding/json"
	"strings"
	"testing"
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
