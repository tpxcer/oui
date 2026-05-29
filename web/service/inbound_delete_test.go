package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
	"github.com/mhsanaei/3x-ui/v3/xray"
)

func TestDelInboundDeletesOrphanClientRecords(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()

	inbound := model.Inbound{
		Tag:      "delete-me",
		Enable:   false,
		Port:     50001,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"email":"orphan@example.com","id":"11111111-1111-1111-1111-111111111111","enable":true,"subId":"sub-orphan"}]}`,
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client := model.ClientRecord{
		Email:  "orphan@example.com",
		SubID:  "sub-orphan",
		UUID:   "11111111-1111-1111-1111-111111111111",
		Enable: true,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{InboundId: inbound.Id, Email: client.Email, Up: 1024}).Error; err != nil {
		t.Fatalf("create traffic: %v", err)
	}
	if err := db.Create(&model.InboundClientIps{ClientEmail: client.Email, Ips: `[]`}).Error; err != nil {
		t.Fatalf("create ips: %v", err)
	}

	svc := InboundService{}
	if _, err := svc.DelInbound(inbound.Id); err != nil {
		t.Fatalf("DelInbound: %v", err)
	}

	assertCount(t, &model.Inbound{}, "id = ?", inbound.Id, 0)
	assertCount(t, &model.ClientRecord{}, "email = ?", client.Email, 0)
	assertCount(t, &model.ClientInbound{}, "client_id = ?", client.Id, 0)
	assertCount(t, &xray.ClientTraffic{}, "email = ?", client.Email, 0)
	assertCount(t, &model.InboundClientIps{}, "client_email = ?", client.Email, 0)
}

func TestDelInboundKeepsClientsAttachedElsewhere(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()

	email := "shared@example.com"
	deletedInbound := model.Inbound{
		Tag:      "delete-one",
		Enable:   false,
		Port:     50002,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"email":"shared@example.com","id":"22222222-2222-2222-2222-222222222222","enable":true,"subId":"sub-shared"}]}`,
	}
	keptInbound := model.Inbound{Tag: "keep-one", Enable: false, Port: 50003, Protocol: model.VLESS}
	if err := db.Create(&deletedInbound).Error; err != nil {
		t.Fatalf("create deleted inbound: %v", err)
	}
	if err := db.Create(&keptInbound).Error; err != nil {
		t.Fatalf("create kept inbound: %v", err)
	}
	client := model.ClientRecord{
		Email:  email,
		SubID:  "sub-shared",
		UUID:   "22222222-2222-2222-2222-222222222222",
		Enable: true,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: deletedInbound.Id}).Error; err != nil {
		t.Fatalf("create deleted mapping: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: keptInbound.Id}).Error; err != nil {
		t.Fatalf("create kept mapping: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{InboundId: keptInbound.Id, Email: email, Up: 2048}).Error; err != nil {
		t.Fatalf("create traffic: %v", err)
	}
	if err := db.Create(&model.InboundClientIps{ClientEmail: email, Ips: `[]`}).Error; err != nil {
		t.Fatalf("create ips: %v", err)
	}

	svc := InboundService{}
	if _, err := svc.DelInbound(deletedInbound.Id); err != nil {
		t.Fatalf("DelInbound: %v", err)
	}

	assertCount(t, &model.Inbound{}, "id = ?", deletedInbound.Id, 0)
	assertCount(t, &model.Inbound{}, "id = ?", keptInbound.Id, 1)
	assertCount(t, &model.ClientRecord{}, "email = ?", email, 1)
	assertCount(t, &model.ClientInbound{}, "client_id = ? AND inbound_id = ?", client.Id, deletedInbound.Id, 0)
	assertCount(t, &model.ClientInbound{}, "client_id = ? AND inbound_id = ?", client.Id, keptInbound.Id, 1)
	assertCount(t, &xray.ClientTraffic{}, "email = ?", email, 1)
	assertCount(t, &model.InboundClientIps{}, "client_email = ?", email, 1)
}

func assertCount(t *testing.T, modelValue any, query string, args ...any) {
	t.Helper()
	want := int64(args[len(args)-1].(int))
	queryArgs := args[:len(args)-1]
	var got int64
	if err := database.GetDB().Model(modelValue).Where(query, queryArgs...).Count(&got).Error; err != nil {
		t.Fatalf("count %T: %v", modelValue, err)
	}
	if got != want {
		t.Fatalf("count %T where %q = %d, want %d", modelValue, query, got, want)
	}
}
