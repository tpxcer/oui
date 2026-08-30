package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
	"github.com/mhsanaei/3x-ui/v3/logger"

	"gorm.io/gorm"
)

const managedFirewallRulesSettingKey = "quickInboundManagedFirewallRules"

type managedFirewallRule struct {
	InboundID int    `json:"inboundId"`
	Backend   string `json:"backend"`
	Protocol  string `json:"protocol"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
}

type firewallMutationResult struct {
	Message string
	Backend string
	Changed bool
}

var (
	managedFirewallMu           sync.Mutex
	removeManagedFirewallRuleFn = removeManagedFirewallRule
)

func trackManagedFirewallChange(inboundID, start, end int, protocol string, result firewallMutationResult) string {
	if !result.Changed || result.Backend == "" {
		return result.Message
	}
	rule := managedFirewallRule{
		InboundID: inboundID,
		Backend:   result.Backend,
		Protocol:  strings.ToLower(strings.TrimSpace(protocol)),
		Start:     start,
		End:       end,
	}
	if err := rememberManagedFirewallRule(rule); err != nil {
		if rollbackErr := removeManagedFirewallRuleFn(rule); rollbackErr != nil {
			return fmt.Sprintf("%s；自动回收记录失败：%v；回滚失败：%v", result.Message, err, rollbackErr)
		}
		return fmt.Sprintf("防火墙放行已回滚：无法保存自动回收记录：%v", err)
	}
	return result.Message
}

func rememberManagedFirewallRule(rule managedFirewallRule) error {
	managedFirewallMu.Lock()
	defer managedFirewallMu.Unlock()

	db := database.GetDB()
	rules, err := loadManagedFirewallRules(db)
	if err != nil {
		return err
	}
	for _, existing := range rules {
		if existing == rule {
			return nil
		}
	}
	rules = append(rules, rule)
	return saveManagedFirewallRules(db, rules)
}

func loadManagedFirewallRules(db *gorm.DB) ([]managedFirewallRule, error) {
	if db == nil {
		db = database.GetDB()
	}
	setting := &model.Setting{}
	err := db.Model(model.Setting{}).Where("key = ?", managedFirewallRulesSettingKey).First(setting).Error
	if database.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rules []managedFirewallRule
	if strings.TrimSpace(setting.Value) == "" {
		return rules, nil
	}
	if err := json.Unmarshal([]byte(setting.Value), &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func saveManagedFirewallRules(db *gorm.DB, rules []managedFirewallRule) error {
	if db == nil {
		db = database.GetDB()
	}
	value, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	setting := &model.Setting{}
	err = db.Model(model.Setting{}).Where("key = ?", managedFirewallRulesSettingKey).First(setting).Error
	if database.IsNotFound(err) {
		return db.Create(&model.Setting{Key: managedFirewallRulesSettingKey, Value: string(value)}).Error
	}
	if err != nil {
		return err
	}
	setting.Value = string(value)
	return db.Save(setting).Error
}

func (s *InboundService) cleanupManagedFirewallRulesBestEffort(inboundID int) {
	if err := s.cleanupManagedFirewallRules(inboundID); err != nil {
		logger.Warning("cleanup managed firewall rules failed for inbound", inboundID, ":", err)
	}
}

func (s *InboundService) cleanupManagedFirewallRules(inboundID int) error {
	managedFirewallMu.Lock()
	defer managedFirewallMu.Unlock()

	db := database.GetDB()
	rules, err := loadManagedFirewallRules(db)
	if err != nil || len(rules) == 0 {
		return err
	}
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).Find(&inbounds).Error; err != nil {
		return err
	}

	next := make([]managedFirewallRule, 0, len(rules))
	var cleanupErrs []error
	for _, rule := range rules {
		if inboundID != 0 && rule.InboundID != inboundID {
			next = append(next, rule)
			continue
		}
		if consumer := managedFirewallRuleConsumer(rule, inbounds); consumer != nil {
			rule.InboundID = consumer.Id
			next = append(next, rule)
			continue
		}
		if err := removeManagedFirewallRuleFn(rule); err != nil {
			next = append(next, rule)
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	if err := saveManagedFirewallRules(db, next); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	return errors.Join(cleanupErrs...)
}

// ReconcileManagedFirewallRules retries stale cleanup after an interrupted
// delete or a temporary firewall command failure.
func (s *InboundService) ReconcileManagedFirewallRules() error {
	return s.cleanupManagedFirewallRules(0)
}

func managedFirewallRuleConsumer(rule managedFirewallRule, inbounds []*model.Inbound) *model.Inbound {
	for _, inbound := range inbounds {
		if inbound == nil || !managedFirewallRuleUsedByInbound(rule, inbound) {
			continue
		}
		return inbound
	}
	return nil
}

func managedFirewallRuleUsedByInbound(rule managedFirewallRule, inbound *model.Inbound) bool {
	if inbound == nil || inbound.NodeID != nil || rule.Start <= 0 || rule.End < rule.Start {
		return false
	}
	transports := inboundTransports(inbound.Protocol, inbound.StreamSettings, inbound.Settings)
	switch rule.Protocol {
	case "tcp":
		if transports&transportTCP == 0 {
			return false
		}
	case "udp":
		if transports&transportUDP == 0 {
			return false
		}
	default:
		return false
	}
	if inbound.Port >= rule.Start && inbound.Port <= rule.End {
		return true
	}
	if inbound.Protocol != model.Hysteria || strings.TrimSpace(inbound.StreamSettings) == "" {
		return false
	}
	var stream hysteriaPortHoppingStream
	if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
		return false
	}
	cfg := stream.HysteriaSettings.PortHopping
	if !cfg.Enable {
		return false
	}
	start, end, _, err := parseHysteriaPortRange(cfg.Range)
	if err != nil {
		return false
	}
	return rule.Start <= end && start <= rule.End
}

func removeManagedFirewallRule(rule managedFirewallRule) error {
	spec := managedFirewallRuleSpec(rule)
	switch rule.Backend {
	case "ufw":
		path, err := exec.LookPath("ufw")
		if err != nil {
			return err
		}
		exists, err := ufwRuleExistsWithError(path, spec)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		output, err := exec.CommandContext(ctx, path, "--force", "delete", "allow", spec).CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("ufw remove %s: %w: %s", spec, err, strings.TrimSpace(string(output)))
		}
		return nil
	case "firewalld":
		path, err := exec.LookPath("firewall-cmd")
		if err != nil {
			return err
		}
		exists, err := firewalldPortExistsWithError(path, spec)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		output, err := exec.CommandContext(ctx, path, "--permanent", "--remove-port="+spec).CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("firewalld remove %s: %w: %s", spec, err, strings.TrimSpace(string(output)))
		}
		ctx, cancel = context.WithTimeout(context.Background(), 8*time.Second)
		output, err = exec.CommandContext(ctx, path, "--reload").CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("firewalld reload: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	case "host-hardening":
		return removeHostHardeningFirewallRule(rule)
	default:
		return fmt.Errorf("unsupported managed firewall backend: %s", rule.Backend)
	}
}

func managedFirewallRuleSpec(rule managedFirewallRule) string {
	if rule.Start == rule.End {
		return fmt.Sprintf("%d/%s", rule.Start, rule.Protocol)
	}
	separator := "-"
	if rule.Backend == "ufw" {
		separator = ":"
	}
	return fmt.Sprintf("%d%s%d/%s", rule.Start, separator, rule.End, rule.Protocol)
}

func removeHostHardeningFirewallRule(rule managedFirewallRule) error {
	original, err := os.ReadFile(hostHardeningNftPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := exec.LookPath("nft"); err != nil {
		return err
	}
	content := string(original)
	var changed bool
	if rule.Start == rule.End {
		content, changed, err = removePortFromNftDportRule(content, rule.Protocol, rule.Start)
	} else {
		content, changed, err = removePortRangeFromNftDportRule(content, rule.Protocol, rule.Start, rule.End)
	}
	if err != nil || !changed {
		return err
	}

	dir := filepath.Dir(hostHardeningNftPath)
	tmp, err := os.CreateTemp(dir, ".host_hardening-*.nft")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := runNftConfigCheck(tmpPath); err != nil {
		return err
	}
	backupPath := fmt.Sprintf("%s.bak.%s", hostHardeningNftPath, time.Now().Format("20060102150405"))
	if err := os.WriteFile(backupPath, original, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, hostHardeningNftPath); err != nil {
		return err
	}
	ok = true
	if err := reloadHostHardeningFirewall(); err != nil {
		_ = os.WriteFile(hostHardeningNftPath, original, 0644)
		_ = reloadHostHardeningFirewall()
		return err
	}
	return nil
}
