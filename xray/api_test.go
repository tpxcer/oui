package xray

import (
	"context"
	"github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/shadowsocks_2022"
	"google.golang.org/grpc"
	"strings"
	"testing"
)

func TestGetRequiredUserString_Present(t *testing.T) {
	user := map[string]any{"email": "alice@example.com"}
	got, err := getRequiredUserString(user, "email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alice@example.com" {
		t.Fatalf("got %q, want %q", got, "alice@example.com")
	}
}

func TestGetRequiredUserString_Missing(t *testing.T) {
	user := map[string]any{}
	if _, err := getRequiredUserString(user, "email"); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestGetRequiredUserString_NilValue(t *testing.T) {
	user := map[string]any{"email": nil}
	if _, err := getRequiredUserString(user, "email"); err == nil {
		t.Fatal("expected error for nil value")
	}
}

func TestGetRequiredUserString_WrongType(t *testing.T) {
	user := map[string]any{"email": 42}
	_, err := getRequiredUserString(user, "email")
	if err == nil {
		t.Fatal("expected error for non-string value")
	}
	if !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("expected %q in error, got: %v", "invalid type", err)
	}
}

func TestGetOptionalUserString_Present(t *testing.T) {
	user := map[string]any{"flow": "xtls-rprx-vision"}
	got, err := getOptionalUserString(user, "flow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "xtls-rprx-vision" {
		t.Fatalf("got %q, want %q", got, "xtls-rprx-vision")
	}
}

func TestGetOptionalUserString_MissingReturnsEmptyNoError(t *testing.T) {
	user := map[string]any{}
	got, err := getOptionalUserString(user, "flow")
	if err != nil {
		t.Fatalf("unexpected error for missing optional field: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

func TestGetOptionalUserString_NilReturnsEmptyNoError(t *testing.T) {
	user := map[string]any{"flow": nil}
	got, err := getOptionalUserString(user, "flow")
	if err != nil {
		t.Fatalf("unexpected error for nil optional field: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

func TestGetOptionalUserString_WrongTypeErrors(t *testing.T) {
	user := map[string]any{"flow": []string{"a", "b"}}
	if _, err := getOptionalUserString(user, "flow"); err == nil {
		t.Fatal("expected error for non-string optional value")
	}
}

// Capture the protobuf sent to the existing external core, rather than requiring a live server.
type captureUserHandler struct {
	command.HandlerServiceClient
	request *command.AlterInboundRequest
}

func (h *captureUserHandler) AlterInbound(_ context.Context, req *command.AlterInboundRequest, _ ...grpc.CallOption) (*command.AlterInboundResponse, error) {
	h.request = req
	return &command.AlterInboundResponse{}, nil
}
func TestAddShadowsocksUserAccountCompatibility(t *testing.T) {
	for _, cipher := range []string{"aes-256-gcm", "chacha20-poly1305", "xchacha20-poly1305", "2022-blake3-aes-128-gcm"} {
		t.Run(cipher, func(t *testing.T) {
			capture := &captureUserHandler{}
			var handler command.HandlerServiceClient = capture
			api := &XrayAPI{HandlerServiceClient: &handler}
			if err := api.AddUser("shadowsocks", "test-inbound", map[string]any{"email": "test-only-user", "password": "test-only-password", "cipher": cipher}); err != nil {
				t.Fatal(err)
			}
			if capture.request == nil || capture.request.Tag != "test-inbound" {
				t.Fatal("missing inbound operation")
			}
			raw, err := capture.request.Operation.GetInstance()
			if err != nil {
				t.Fatal(err)
			}
			operation, ok := raw.(*command.AddUserOperation)
			if !ok || operation.User.Email != "test-only-user" {
				t.Fatal("invalid add-user operation")
			}
			account, err := operation.User.Account.GetInstance()
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(cipher, "2022-") {
				value, ok := account.(*shadowsocks_2022.ServerConfig)
				if !ok || value.Key != "test-only-password" {
					t.Fatal("2022 account changed")
				}
			} else {
				value, ok := account.(*shadowsocks.Account)
				if !ok || value.Password != "test-only-password" || value.CipherType == shadowsocks.CipherType_UNKNOWN {
					t.Fatal("AEAD account changed")
				}
			}
		})
	}
}
