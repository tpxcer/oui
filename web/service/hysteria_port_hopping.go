package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/database/model"
	"github.com/mhsanaei/3x-ui/v3/logger"
)

const (
	hysteriaPortHoppingNftTable = "oui_port_hopping"
	hysteriaPortHoppingChain    = "OUI_PORT_HOPPING"
	hysteriaPortHoppingTimeout  = 8 * time.Second
)

type hysteriaPortHoppingRule struct {
	InboundID int
	Start     int
	End       int
	Target    int
	Range     string
}

type hysteriaPortHoppingStream struct {
	Network          string `json:"network"`
	HysteriaSettings struct {
		Version     int `json:"version"`
		PortHoping  any `json:"portHoping"`
		PortHopping struct {
			Enable bool   `json:"enable"`
			Range  string `json:"range"`
		} `json:"portHopping"`
	} `json:"hysteriaSettings"`
}

func parseHysteriaPortRange(raw string) (start int, end int, normalized string, err error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return 0, 0, "", fmt.Errorf("端口跳跃范围不能为空")
	}
	cleaned = strings.ReplaceAll(cleaned, "：", ":")
	cleaned = strings.ReplaceAll(cleaned, "－", "-")
	cleaned = strings.ReplaceAll(cleaned, "—", "-")
	cleaned = strings.ReplaceAll(cleaned, "–", "-")
	cleaned = strings.ReplaceAll(cleaned, " ", "")

	sep := "-"
	if strings.Contains(cleaned, ":") && !strings.Contains(cleaned, "-") {
		sep = ":"
	}
	parts := strings.Split(cleaned, sep)
	if len(parts) == 1 {
		parts = []string{parts[0], parts[0]}
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, "", fmt.Errorf("端口跳跃范围格式应为 48000-50000")
	}
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, "", fmt.Errorf("端口跳跃起始端口无效")
	}
	end, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, "", fmt.Errorf("端口跳跃结束端口无效")
	}
	if start < 1 || start > 65535 || end < 1 || end > 65535 {
		return 0, 0, "", fmt.Errorf("端口跳跃范围必须在 1-65535 之间")
	}
	if start > end {
		return 0, 0, "", fmt.Errorf("端口跳跃起始端口不能大于结束端口")
	}
	return start, end, fmt.Sprintf("%d-%d", start, end), nil
}

func hysteriaPortHoppingRuleFromInbound(inbound *model.Inbound) (*hysteriaPortHoppingRule, error) {
	if inbound == nil || inbound.Protocol != model.Hysteria || !inbound.Enable || inbound.NodeID != nil {
		return nil, nil
	}
	if strings.TrimSpace(inbound.StreamSettings) == "" {
		return nil, nil
	}
	var stream hysteriaPortHoppingStream
	if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
		return nil, err
	}
	if stream.Network != "" && stream.Network != "hysteria" {
		return nil, nil
	}
	if stream.HysteriaSettings.Version != 0 && stream.HysteriaSettings.Version != 2 {
		return nil, nil
	}
	cfg := stream.HysteriaSettings.PortHopping
	if !cfg.Enable {
		return nil, nil
	}
	start, end, normalized, err := parseHysteriaPortRange(cfg.Range)
	if err != nil {
		return nil, err
	}
	if inbound.Port < 1 || inbound.Port > 65535 {
		return nil, fmt.Errorf("Hysteria2 真实监听端口无效：%d", inbound.Port)
	}
	return &hysteriaPortHoppingRule{
		InboundID: inbound.Id,
		Start:     start,
		End:       end,
		Target:    inbound.Port,
		Range:     normalized,
	}, nil
}

func hysteriaPortHoppingRangeFromInbound(inbound *model.Inbound) string {
	return HysteriaPortHoppingRangeFromInbound(inbound)
}

func HysteriaPortHoppingRangeFromInbound(inbound *model.Inbound) string {
	rule, err := hysteriaPortHoppingRuleFromInbound(inbound)
	if err != nil || rule == nil {
		return ""
	}
	return rule.Range
}

func validateHysteriaPortHopping(inbound *model.Inbound) error {
	_, err := hysteriaPortHoppingRuleFromInbound(inbound)
	return err
}

func (s *InboundService) checkHysteriaPortHoppingConflict(inbound *model.Inbound, exceptInboundID int) error {
	rule, err := hysteriaPortHoppingRuleFromInbound(inbound)
	if err != nil || rule == nil {
		return err
	}
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).Find(&inbounds).Error; err != nil {
		return err
	}
	for _, existing := range inbounds {
		if existing == nil || existing.Id == exceptInboundID {
			continue
		}
		existingRule, err := hysteriaPortHoppingRuleFromInbound(existing)
		if err != nil || existingRule == nil {
			continue
		}
		if rule.Start <= existingRule.End && existingRule.Start <= rule.End {
			return fmt.Errorf("端口跳跃范围 %s 与入站 %s(id=%d) 的范围 %s 重叠", rule.Range, existing.Remark, existing.Id, existingRule.Range)
		}
	}
	return nil
}

func (s *InboundService) syncHysteriaPortHoppingRulesBestEffort() {
	if err := s.SyncHysteriaPortHoppingRules(); err != nil {
		logger.Warning("hysteria port hopping sync failed:", err)
	}
}

func (s *InboundService) SyncHysteriaPortHoppingRules() error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).Find(&inbounds).Error; err != nil {
		return err
	}
	rules := make([]hysteriaPortHoppingRule, 0)
	for _, inbound := range inbounds {
		rule, err := hysteriaPortHoppingRuleFromInbound(inbound)
		if err != nil {
			logger.Warningf("skip hysteria port hopping inbound id=%d: %v", inbound.Id, err)
			continue
		}
		if rule != nil {
			rules = append(rules, *rule)
		}
	}
	if nftPath, err := exec.LookPath("nft"); err == nil {
		if err := syncHysteriaPortHoppingNft(nftPath, rules); err == nil {
			return nil
		} else {
			logger.Warning("hysteria port hopping nft sync failed, falling back to iptables:", err)
		}
	}
	return syncHysteriaPortHoppingIptables(rules)
}

func syncHysteriaPortHoppingNft(nftPath string, rules []hysteriaPortHoppingRule) error {
	ctx, cancel := context.WithTimeout(context.Background(), hysteriaPortHoppingTimeout)
	_ = exec.CommandContext(ctx, nftPath, "delete", "table", "inet", hysteriaPortHoppingNftTable).Run()
	cancel()

	var buf bytes.Buffer
	buf.WriteString("table inet " + hysteriaPortHoppingNftTable + " {\n")
	buf.WriteString("  chain prerouting {\n")
	buf.WriteString("    type nat hook prerouting priority dstnat; policy accept;\n")
	for _, rule := range rules {
		buf.WriteString(fmt.Sprintf(
			"    udp dport %d-%d counter redirect to :%d comment \"oui-hy2:%d\"\n",
			rule.Start, rule.End, rule.Target, rule.InboundID,
		))
	}
	buf.WriteString("  }\n")
	buf.WriteString("}\n")

	ctx, cancel = context.WithTimeout(context.Background(), hysteriaPortHoppingTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, nftPath, "-f", "-")
	cmd.Stdin = strings.NewReader(buf.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func syncHysteriaPortHoppingIptables(rules []hysteriaPortHoppingRule) error {
	paths := make([]string, 0, 2)
	if path, err := exec.LookPath("iptables"); err == nil {
		paths = append(paths, path)
	}
	if path, err := exec.LookPath("ip6tables"); err == nil {
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return fmt.Errorf("未找到 nft/iptables/ip6tables，无法自动部署 Hysteria2 端口跳跃")
	}
	var errs []string
	for _, path := range paths {
		if err := syncHysteriaPortHoppingIptablesOne(path, rules); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", path, err))
		}
	}
	if len(errs) == len(paths) {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func syncHysteriaPortHoppingIptablesOne(path string, rules []hysteriaPortHoppingRule) error {
	_ = runHysteriaPortHoppingCommand(path, "-t", "nat", "-N", hysteriaPortHoppingChain)
	if err := runHysteriaPortHoppingCommand(path, "-t", "nat", "-F", hysteriaPortHoppingChain); err != nil {
		return err
	}
	if err := runHysteriaPortHoppingCommand(path, "-t", "nat", "-C", "PREROUTING", "-j", hysteriaPortHoppingChain); err != nil {
		if err := runHysteriaPortHoppingCommand(path, "-t", "nat", "-A", "PREROUTING", "-j", hysteriaPortHoppingChain); err != nil {
			return err
		}
	}
	for _, rule := range rules {
		if err := runHysteriaPortHoppingCommand(
			path, "-t", "nat", "-A", hysteriaPortHoppingChain,
			"-p", "udp", "--dport", fmt.Sprintf("%d:%d", rule.Start, rule.End),
			"-m", "comment", "--comment", fmt.Sprintf("oui-hy2:%d", rule.InboundID),
			"-j", "REDIRECT", "--to-ports", strconv.Itoa(rule.Target),
		); err != nil {
			return err
		}
	}
	return nil
}

func runHysteriaPortHoppingCommand(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), hysteriaPortHoppingTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
