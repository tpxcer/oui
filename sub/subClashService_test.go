package sub

import (
	"reflect"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database/model"
)

func TestApplyTransport_XHTTP(t *testing.T) {
	svc := &SubClashService{}
	proxy := map[string]any{}
	stream := map[string]any{
		"xhttpSettings": map[string]any{
			"path": "/xh",
			"host": "example.com",
			"mode": "auto",
		},
	}

	if !svc.applyTransport(proxy, "xhttp", stream) {
		t.Fatalf("applyTransport returned false for xhttp (#4531: would drop the inbound and yield an empty Clash YAML)")
	}
	if proxy["network"] != "xhttp" {
		t.Fatalf("network = %v, want xhttp", proxy["network"])
	}
	opts, ok := proxy["xhttp-opts"].(map[string]any)
	if !ok {
		t.Fatalf("xhttp-opts missing or wrong type: %#v", proxy["xhttp-opts"])
	}
	want := map[string]any{"path": "/xh", "host": "example.com", "mode": "auto"}
	if !reflect.DeepEqual(opts, want) {
		t.Fatalf("xhttp-opts = %#v, want %#v", opts, want)
	}
}

func TestApplyTransport_XHTTP_HostFromHeaders(t *testing.T) {
	svc := &SubClashService{}
	proxy := map[string]any{}
	stream := map[string]any{
		"xhttpSettings": map[string]any{
			"path":    "/xh",
			"headers": map[string]any{"Host": "via-header.example.com"},
		},
	}

	if !svc.applyTransport(proxy, "xhttp", stream) {
		t.Fatalf("applyTransport returned false for xhttp")
	}
	opts, _ := proxy["xhttp-opts"].(map[string]any)
	if opts["host"] != "via-header.example.com" {
		t.Fatalf("host should fall back to headers.Host, got %v", opts["host"])
	}
}

func TestApplyTransport_HTTPUpgrade(t *testing.T) {
	svc := &SubClashService{}
	proxy := map[string]any{}
	stream := map[string]any{
		"httpupgradeSettings": map[string]any{
			"path": "/hu",
			"host": "example.com",
		},
	}

	if !svc.applyTransport(proxy, "httpupgrade", stream) {
		t.Fatalf("applyTransport returned false for httpupgrade")
	}
	if proxy["network"] != "httpupgrade" {
		t.Fatalf("network = %v, want httpupgrade", proxy["network"])
	}
	opts, ok := proxy["http-upgrade-opts"].(map[string]any)
	if !ok {
		t.Fatalf("http-upgrade-opts missing: %#v", proxy["http-upgrade-opts"])
	}
	if opts["path"] != "/hu" {
		t.Fatalf("path = %v, want /hu", opts["path"])
	}
	headers, _ := opts["headers"].(map[string]any)
	if headers["Host"] != "example.com" {
		t.Fatalf("headers.Host = %v, want example.com", headers["Host"])
	}
}

func TestBuildHysteriaProxy_IncludesPortHoppingPorts(t *testing.T) {
	svc := &SubClashService{SubService: &SubService{remarkModel: "-io"}}
	inbound := &model.Inbound{
		Remark:   "hy2",
		Listen:   "example.com",
		Port:     51744,
		Protocol: model.Hysteria,
		Enable:   true,
		Settings: `{"version":2,"clients":[{"email":"user","auth":"secret"}]}`,
		StreamSettings: `{
			"network":"hysteria",
			"hysteriaSettings":{
				"version":2,
				"auth":"secret",
				"portHopping":{"enable":true,"range":"47612-47811"}
			},
			"security":"tls",
			"tlsSettings":{"serverName":"example.com","alpn":["h3"],"settings":{"fingerprint":"chrome"}}
		}`,
	}

	proxy := svc.buildHysteriaProxy(inbound, model.Client{Email: "user", Auth: "secret"}, "")
	if proxy["type"] != "hysteria2" {
		t.Fatalf("type = %v, want hysteria2", proxy["type"])
	}
	if proxy["ports"] != "47612-47811" {
		t.Fatalf("ports = %v, want 47612-47811", proxy["ports"])
	}
}

func TestBuildHysteriaProxy_DoesNotAddPortsWhenPortHoppingDisabled(t *testing.T) {
	svc := &SubClashService{SubService: &SubService{remarkModel: "-io"}}
	inbound := &model.Inbound{
		Remark:   "hy2",
		Listen:   "example.com",
		Port:     51744,
		Protocol: model.Hysteria,
		Enable:   true,
		Settings: `{"version":2,"clients":[{"email":"user","auth":"secret"}]}`,
		StreamSettings: `{
			"network":"hysteria",
			"hysteriaSettings":{
				"version":2,
				"auth":"secret",
				"portHopping":{"enable":false,"range":"47612-47811"}
			},
			"security":"tls",
			"tlsSettings":{"serverName":"example.com","alpn":["h3"],"settings":{"fingerprint":"chrome"}}
		}`,
	}

	proxy := svc.buildHysteriaProxy(inbound, model.Client{Email: "user", Auth: "secret"}, "")
	if _, ok := proxy["ports"]; ok {
		t.Fatalf("ports should be omitted when port hopping is disabled: %#v", proxy)
	}
}
