package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
)

func TestManagedFirewallRuleConsumer(t *testing.T) {
	rule := managedFirewallRule{Protocol: "udp", Start: 40000, End: 40199}
	consumer := &model.Inbound{
		Id:             8,
		Enable:         false,
		Port:           53000,
		Protocol:       model.Hysteria,
		StreamSettings: `{"network":"hysteria","hysteriaSettings":{"version":2,"portHopping":{"enable":true,"range":"40100-40299"}}}`,
	}
	if got := managedFirewallRuleConsumer(rule, []*model.Inbound{consumer}); got == nil || got.Id != consumer.Id {
		t.Fatalf("disabled configured consumer = %+v, want inbound %d", got, consumer.Id)
	}

	tcpOnly := &model.Inbound{Id: 9, Port: 40050, Protocol: model.VLESS, StreamSettings: `{"network":"tcp"}`}
	if got := managedFirewallRuleConsumer(rule, []*model.Inbound{tcpOnly}); got != nil {
		t.Fatalf("udp rule must not be retained by tcp inbound: %+v", got)
	}

	nodeID := 1
	remote := &model.Inbound{Id: 10, NodeID: &nodeID, Port: 40050, Protocol: model.Hysteria, StreamSettings: `{"network":"hysteria"}`}
	if got := managedFirewallRuleConsumer(rule, []*model.Inbound{remote}); got != nil {
		t.Fatalf("remote inbound must not retain a local firewall rule: %+v", got)
	}
}

func TestDelInboundRemovesOnlyTrackedFirewallRuleAfterLastConsumer(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()

	first := model.Inbound{Tag: "managed-one", Enable: false, Port: 55001, Protocol: model.VLESS, StreamSettings: `{"network":"tcp"}`, Settings: `{}`}
	second := model.Inbound{Tag: "managed-two", Enable: false, Port: 55001, Protocol: model.VLESS, StreamSettings: `{"network":"tcp"}`, Settings: `{}`}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first inbound: %v", err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("create second inbound: %v", err)
	}
	rule := managedFirewallRule{InboundID: first.Id, Backend: "ufw", Protocol: "tcp", Start: 55001, End: 55001}
	if err := saveManagedFirewallRules(db, []managedFirewallRule{rule}); err != nil {
		t.Fatalf("save rules: %v", err)
	}

	originalRemove := removeManagedFirewallRuleFn
	removed := 0
	removeManagedFirewallRuleFn = func(got managedFirewallRule) error {
		removed++
		return nil
	}
	t.Cleanup(func() { removeManagedFirewallRuleFn = originalRemove })

	svc := &InboundService{}
	if _, err := svc.DelInbound(first.Id); err != nil {
		t.Fatalf("delete first inbound: %v", err)
	}
	if removed != 0 {
		t.Fatalf("shared rule removed %d times, want 0", removed)
	}
	rules, err := loadManagedFirewallRules(db)
	if err != nil || len(rules) != 1 || rules[0].InboundID != second.Id {
		t.Fatalf("rules after first delete = %+v, err=%v", rules, err)
	}

	if _, err := svc.DelInbound(second.Id); err != nil {
		t.Fatalf("delete second inbound: %v", err)
	}
	if removed != 1 {
		t.Fatalf("rule removed %d times, want 1", removed)
	}
	rules, err = loadManagedFirewallRules(db)
	if err != nil || len(rules) != 0 {
		t.Fatalf("rules after last delete = %+v, err=%v", rules, err)
	}
}

func TestUnchangedFirewallRuleIsNotTracked(t *testing.T) {
	setupConflictDB(t)
	msg := trackManagedFirewallChange(1, 52000, 52000, "tcp", firewallMutationResult{
		Message: "already open",
		Backend: "ufw",
		Changed: false,
	})
	if msg != "already open" {
		t.Fatalf("message = %q", msg)
	}
	rules, err := loadManagedFirewallRules(database.GetDB())
	if err != nil || len(rules) != 0 {
		t.Fatalf("unmodified rule must not be tracked: %+v err=%v", rules, err)
	}
}

func TestResetSettingsPreservesManagedFirewallRules(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	rule := managedFirewallRule{InboundID: 1, Backend: "ufw", Protocol: "tcp", Start: 52000, End: 52000}
	if err := saveManagedFirewallRules(db, []managedFirewallRule{rule}); err != nil {
		t.Fatalf("save managed rule: %v", err)
	}
	if err := (&SettingService{}).saveSetting("pageSize", "99"); err != nil {
		t.Fatalf("save ordinary setting: %v", err)
	}

	if err := (&SettingService{}).ResetSettings(); err != nil {
		t.Fatalf("ResetSettings: %v", err)
	}
	rules, err := loadManagedFirewallRules(db)
	if err != nil || len(rules) != 1 || rules[0] != rule {
		t.Fatalf("managed rules after reset = %+v, err=%v", rules, err)
	}
	var count int64
	if err := db.Model(&model.Setting{}).Where("key = ?", "pageSize").Count(&count).Error; err != nil {
		t.Fatalf("count ordinary setting: %v", err)
	}
	if count != 0 {
		t.Fatalf("ordinary setting count after reset = %d, want 0", count)
	}
}
