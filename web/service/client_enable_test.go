package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
)

func TestSetClientEnableByEmailPreservesCredentialsAcrossInbounds(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "3x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const (
		email      = "client@example.com"
		uuid       = "11111111-2222-4333-8444-555555555555"
		subID      = "fixed-sub-id"
		flow       = "xtls-rprx-vision"
		limitIP    = 2
		expiryTime = int64(1893456000000)
	)
	original := model.Client{
		ID: uuid, Email: email, SubID: subID, Flow: flow, Security: "auto",
		LimitIP: limitIP, TotalGB: 10 << 30, ExpiryTime: expiryTime,
		Enable: true, Comment: "preserve-me", Reset: 15,
	}

	db := database.GetDB()
	inbounds := []*model.Inbound{
		{Tag: "first", Enable: true, Port: 10001, Protocol: model.VLESS},
		{Tag: "second", Enable: true, Port: 10002, Protocol: model.VLESS},
	}
	clientService := ClientService{}
	inboundService := InboundService{}
	for _, inbound := range inbounds {
		settings, err := json.Marshal(map[string]any{
			"clients":    []model.Client{original},
			"decryption": "none",
		})
		if err != nil {
			t.Fatalf("marshal settings: %v", err)
		}
		inbound.Settings = string(settings)
		if err := db.Create(inbound).Error; err != nil {
			t.Fatalf("create inbound: %v", err)
		}
		if err := clientService.SyncInbound(nil, inbound.Id, []model.Client{original}); err != nil {
			t.Fatalf("SyncInbound: %v", err)
		}
	}

	assertState := func(wantEnable bool) {
		t.Helper()
		for _, inbound := range inbounds {
			stored, err := inboundService.GetInbound(inbound.Id)
			if err != nil {
				t.Fatalf("GetInbound(%d): %v", inbound.Id, err)
			}
			clients, err := inboundService.GetClients(stored)
			if err != nil {
				t.Fatalf("GetClients(%d): %v", inbound.Id, err)
			}
			if len(clients) != 1 {
				t.Fatalf("inbound %d client count = %d, want 1", inbound.Id, len(clients))
			}
			got := clients[0]
			if got.Enable != wantEnable {
				t.Errorf("inbound %d enable = %t, want %t", inbound.Id, got.Enable, wantEnable)
			}
			if got.ID != uuid || got.SubID != subID || got.Flow != flow || got.Security != "auto" {
				t.Errorf("inbound %d credentials changed: %#v", inbound.Id, got)
			}
			if got.LimitIP != limitIP || got.ExpiryTime != expiryTime || got.TotalGB != 10<<30 || got.Comment != "preserve-me" || got.Reset != 15 {
				t.Errorf("inbound %d client settings changed: %#v", inbound.Id, got)
			}
		}

		record, err := clientService.GetRecordByEmail(nil, email)
		if err != nil {
			t.Fatalf("GetRecordByEmail: %v", err)
		}
		if record.Enable != wantEnable {
			t.Errorf("record enable = %t, want %t", record.Enable, wantEnable)
		}
		if record.UUID != uuid || record.SubID != subID || record.Flow != flow || record.Security != "auto" {
			t.Errorf("record credentials changed: %#v", record)
		}
		if record.LimitIP != limitIP || record.ExpiryTime != expiryTime || record.TotalGB != 10<<30 || record.Comment != "preserve-me" || record.Reset != 15 {
			t.Errorf("record settings changed: %#v", record)
		}
	}

	changed, _, err := clientService.SetClientEnableByEmail(&inboundService, email, false)
	if err != nil {
		t.Fatalf("disable client: %v", err)
	}
	if !changed {
		t.Fatal("disable client reported no change")
	}
	assertState(false)

	changed, _, err = clientService.SetClientEnableByEmail(&inboundService, email, true)
	if err != nil {
		t.Fatalf("enable client: %v", err)
	}
	if !changed {
		t.Fatal("enable client reported no change")
	}
	assertState(true)
}
