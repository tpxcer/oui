package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
)

func TestGetInboundTagsFiltersCurrentEnabledLocalUser(t *testing.T) {
	setupConflictDB(t)

	nodeID := 7
	rows := []model.Inbound{
		{UserId: 1, Enable: true, Tag: "inbound-50001", Remark: "Hp"},
		{UserId: 1, Enable: false, Tag: "inbound-disabled", Remark: "Disabled"},
		{UserId: 2, Enable: true, Tag: "inbound-other-user", Remark: "Other"},
		{UserId: 1, Enable: true, Tag: "inbound-remote", Remark: "Remote", NodeID: &nodeID},
		{UserId: 1, Enable: true, Tag: ""},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatalf("seed inbounds: %v", err)
	}

	raw, err := (&InboundService{}).GetInboundTags(1)
	if err != nil {
		t.Fatalf("GetInboundTags: %v", err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		t.Fatalf("unmarshal tags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "inbound-50001" {
		t.Fatalf("tags = %#v, want only current enabled local user's tag", tags)
	}

	rawRemarks, err := (&InboundService{}).GetInboundTagRemarks(1)
	if err != nil {
		t.Fatalf("GetInboundTagRemarks: %v", err)
	}
	var remarks map[string]string
	if err := json.Unmarshal([]byte(rawRemarks), &remarks); err != nil {
		t.Fatalf("unmarshal remarks: %v", err)
	}
	if len(remarks) != 1 || remarks["inbound-50001"] != "Hp" {
		t.Fatalf("remarks = %#v, want only current enabled local user's inbound remark", remarks)
	}
}
