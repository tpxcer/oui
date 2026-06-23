package job

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/web/service"
	"github.com/mymmrac/telego"
)

func TestMergeClientIps_EvictsStaleOldEntries(t *testing.T) {
	// #4077: after a ban expires, a single IP that reconnects used to get
	// banned again immediately because a long-disconnected IP stayed in the
	// DB with an ancient timestamp and kept "protecting" itself against
	// eviction. Guard against that regression here.
	old := []IPWithTimestamp{
		{IP: "1.1.1.1", Timestamp: 100},  // stale — client disconnected long ago
		{IP: "2.2.2.2", Timestamp: 1900}, // fresh — still connecting
	}
	new := []IPWithTimestamp{
		{IP: "2.2.2.2", Timestamp: 2000}, // same IP, newer log line
	}

	got := mergeClientIps(old, new, 1000)

	want := map[string]int64{"2.2.2.2": 2000}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stale 1.1.1.1 should have been dropped\ngot:  %v\nwant: %v", got, want)
	}
}

func TestMergeClientIps_KeepsFreshOldEntriesUnchanged(t *testing.T) {
	// Backwards-compat: entries that aren't stale are still carried forward,
	// so enforcement survives access-log rotation.
	old := []IPWithTimestamp{
		{IP: "1.1.1.1", Timestamp: 1500},
	}
	got := mergeClientIps(old, nil, 1000)

	want := map[string]int64{"1.1.1.1": 1500}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fresh old IP should have been retained\ngot:  %v\nwant: %v", got, want)
	}
}

func TestMergeClientIps_PrefersLaterTimestampForSameIp(t *testing.T) {
	old := []IPWithTimestamp{{IP: "1.1.1.1", Timestamp: 1500}}
	new := []IPWithTimestamp{{IP: "1.1.1.1", Timestamp: 1700}}

	got := mergeClientIps(old, new, 1000)

	if got["1.1.1.1"] != 1700 {
		t.Fatalf("expected latest timestamp 1700, got %d", got["1.1.1.1"])
	}
}

func TestMergeClientIps_DropsStaleNewEntries(t *testing.T) {
	// A log line with a clock-skewed old timestamp must not resurrect a
	// stale IP past the cutoff.
	new := []IPWithTimestamp{{IP: "1.1.1.1", Timestamp: 500}}
	got := mergeClientIps(nil, new, 1000)

	if len(got) != 0 {
		t.Fatalf("stale new IP should have been dropped, got %v", got)
	}
}

func TestMergeClientIps_NoStaleCutoffStillWorks(t *testing.T) {
	// Defensive: a zero cutoff (e.g. during very first run on a fresh
	// install) must not over-evict.
	old := []IPWithTimestamp{{IP: "1.1.1.1", Timestamp: 100}}
	new := []IPWithTimestamp{{IP: "2.2.2.2", Timestamp: 200}}

	got := mergeClientIps(old, new, 0)

	want := map[string]int64{"1.1.1.1": 100, "2.2.2.2": 200}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("zero cutoff should keep everything\ngot:  %v\nwant: %v", got, want)
	}
}

func collectIps(entries []IPWithTimestamp) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.IP)
	}
	return out
}

func TestPartitionLiveIps_SingleLiveNotStarvedByStillFreshHistoricals(t *testing.T) {
	// #4091: db holds A, B, C from minutes ago (still in the 30min
	// window) but they're not connecting anymore. only D is. old code
	// merged all four, sorted ascending, kept [A,B,C] and banned D
	// every tick. pin the new rule: only live ips count toward the limit.
	ipMap := map[string]int64{
		"A": 1000,
		"B": 1100,
		"C": 1200,
		"D": 2000,
	}
	observed := map[string]bool{"D": true}

	live, historical := partitionLiveIps(ipMap, observed)

	if got := collectIps(live); !reflect.DeepEqual(got, []string{"D"}) {
		t.Fatalf("live set should only contain the ip observed this scan\ngot:  %v\nwant: [D]", got)
	}
	if got := collectIps(historical); !reflect.DeepEqual(got, []string{"A", "B", "C"}) {
		t.Fatalf("historical set should contain db-only ips in ascending order\ngot:  %v\nwant: [A B C]", got)
	}
}

func TestPartitionLiveIps_ConcurrentLiveIpsStillBanNewcomers(t *testing.T) {
	// keep the "protect original, ban newcomer" policy when several ips
	// are really live. with limit=1, A must stay and B must be banned.
	ipMap := map[string]int64{
		"A": 5000,
		"B": 5500,
	}
	observed := map[string]bool{"A": true, "B": true}

	live, historical := partitionLiveIps(ipMap, observed)

	if got := collectIps(live); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Fatalf("both live ips should be in the live set, ascending\ngot:  %v\nwant: [A B]", got)
	}
	if len(historical) != 0 {
		t.Fatalf("no historical ips expected, got %v", historical)
	}
}

func TestPartitionLiveIps_EmptyScanLeavesDbIntact(t *testing.T) {
	// quiet tick: nothing observed => nothing live. everything merged
	// is historical. keeps the panel from wiping recent-but-idle ips.
	ipMap := map[string]int64{
		"A": 1000,
		"B": 1100,
	}
	observed := map[string]bool{}

	live, historical := partitionLiveIps(ipMap, observed)

	if len(live) != 0 {
		t.Fatalf("no live ips expected, got %v", live)
	}
	if got := collectIps(historical); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Fatalf("all merged entries should flow to historical\ngot:  %v\nwant: [A B]", got)
	}
}

func TestSelectIPLimitExcess_FreshHistoricalIpReservesSlot(t *testing.T) {
	ipMap := map[string]int64{
		"10.0.0.1":  1000,
		"192.0.2.9": 2000,
	}
	live := []IPWithTimestamp{
		{IP: "192.0.2.9", Timestamp: 2000},
	}

	kept, banned := selectIPLimitExcess(ipMap, live, 1)

	if len(kept) != 0 {
		t.Fatalf("new live ip should not be kept while the original ip still reserves the only slot: %v", kept)
	}
	if got := collectIps(banned); !reflect.DeepEqual(got, []string{"192.0.2.9"}) {
		t.Fatalf("new live ip should be banned\ngot:  %v\nwant: [192.0.2.9]", got)
	}
}

func TestSelectIPLimitExcess_LimitThreeBansFourthByFirstSeenTime(t *testing.T) {
	ipMap := map[string]int64{
		"10.0.0.1":  1000,
		"10.0.0.2":  1100,
		"10.0.0.3":  1200,
		"192.0.2.9": 1300,
	}
	live := []IPWithTimestamp{
		{IP: "10.0.0.1", Timestamp: 1000},
		{IP: "10.0.0.2", Timestamp: 1100},
		{IP: "10.0.0.3", Timestamp: 1200},
		{IP: "192.0.2.9", Timestamp: 1300},
	}

	kept, banned := selectIPLimitExcess(ipMap, live, 3)

	if got := collectIps(kept); !reflect.DeepEqual(got, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}) {
		t.Fatalf("oldest three IPs should be kept\ngot:  %v\nwant: [10.0.0.1 10.0.0.2 10.0.0.3]", got)
	}
	if got := collectIps(banned); !reflect.DeepEqual(got, []string{"192.0.2.9"}) {
		t.Fatalf("fourth IP by first-seen time should be banned\ngot:  %v\nwant: [192.0.2.9]", got)
	}
}

func TestBuildIPLimitCutoffNotifyMessage(t *testing.T) {
	msg := buildIPLimitCutoffNotifyMessage(
		"user@example.com",
		nil,
		1,
		[]IPWithTimestamp{{IP: "10.0.0.1", Timestamp: 1000}},
		[]IPWithTimestamp{{IP: "192.0.2.9", Timestamp: 2000}},
		time.Date(2026, 6, 5, 9, 8, 7, 0, time.Local),
	)

	for _, want := range []string{
		"超出 IP 上限，已掐断",
		"用户/节点：<code>user@example.com</code>",
		"IP 限制：<code>1</code>",
		"保留 IP：<code>10.0.0.1</code>",
		"掐断 IP：<code>192.0.2.9</code>",
		"2026-06-05 09:08:07",
	} {
		if !contains(msg, want) {
			t.Fatalf("notification message missing %q\nfull message:\n%s", want, msg)
		}
	}
}

func TestBuildIPLimitCutoffKeyboardUsesIPLimitSettings(t *testing.T) {
	j := &CheckClientIpJob{tgbotService: service.Tgbot{}}
	markup := j.buildIPLimitCutoffKeyboard(
		"user@example.com",
		443,
		[]IPWithTimestamp{{IP: "192.0.2.9", Timestamp: 2000}},
	)

	keyboard, ok := markup.(*telego.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T, want *telego.InlineKeyboardMarkup", markup)
	}
	if len(keyboard.InlineKeyboard) != 2 {
		t.Fatalf("keyboard rows = %d, want 2", len(keyboard.InlineKeyboard))
	}

	var labels []string
	var callbacks []string
	for _, row := range keyboard.InlineKeyboard {
		for _, button := range row {
			labels = append(labels, button.Text)
			callbacks = append(callbacks, button.CallbackData)
		}
	}
	for _, forbidden := range []string{"解除封禁", "手动封禁"} {
		if contains(strings.Join(labels, ","), forbidden) {
			t.Fatalf("keyboard should not contain %q: %v", forbidden, labels)
		}
	}
	for _, want := range []string{"临时解封 1 小时", "临时解封 6 小时", "临时解封 24 小时", "设置IP数量"} {
		if !contains(strings.Join(labels, ","), want) {
			t.Fatalf("keyboard missing %q: %v", want, labels)
		}
	}
	if callbacks[len(callbacks)-1] != "ip_limit user@example.com" {
		t.Fatalf("set IP count callback = %q, want %q", callbacks[len(callbacks)-1], "ip_limit user@example.com")
	}
}

func TestIPLimitCutoffNotifyCooldownSuppressesSameTarget(t *testing.T) {
	j := NewCheckClientIpJob()
	now := time.Date(2026, 6, 23, 10, 0, 0, 0, time.Local)
	banned := []IPWithTimestamp{{IP: "192.0.2.9", Timestamp: now.Unix()}}
	key := ipLimitCutoffNotifyKey("user@example.com", 4321, banned)

	if !j.shouldSendIPLimitCutoffNotify(key, now) {
		t.Fatal("first notification should be sent")
	}
	if j.shouldSendIPLimitCutoffNotify(key, now.Add(time.Minute)) {
		t.Fatal("same target inside cooldown should be suppressed")
	}
	if !j.shouldSendIPLimitCutoffNotify(key, now.Add(ipLimitCutoffNotifyCooldown+time.Second)) {
		t.Fatal("same target after cooldown should be sent again")
	}
}

func TestIPLimitCutoffNotifyCooldownAllowsDifferentIP(t *testing.T) {
	j := NewCheckClientIpJob()
	now := time.Date(2026, 6, 23, 10, 0, 0, 0, time.Local)
	first := []IPWithTimestamp{{IP: "192.0.2.9", Timestamp: now.Unix()}}
	second := []IPWithTimestamp{{IP: "198.51.100.9", Timestamp: now.Add(time.Minute).Unix()}}

	if !j.shouldSendIPLimitCutoffNotify(ipLimitCutoffNotifyKey("user@example.com", 4321, first), now) {
		t.Fatal("first notification should be sent")
	}
	if !j.shouldSendIPLimitCutoffNotify(ipLimitCutoffNotifyKey("user@example.com", 4321, second), now.Add(time.Minute)) {
		t.Fatal("different banned IP should not be suppressed")
	}
}
