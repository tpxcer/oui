package service

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/config"
	"github.com/mhsanaei/3x-ui/v3/database"
	"github.com/mhsanaei/3x-ui/v3/logger"
	"github.com/mhsanaei/3x-ui/v3/util/common"
	"github.com/mhsanaei/3x-ui/v3/util/sys"
	"github.com/mhsanaei/3x-ui/v3/xray"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// ProcessState represents the current state of a system process.
type ProcessState string

// Process state constants
const (
	Running ProcessState = "running" // Process is running normally
	Stop    ProcessState = "stop"    // Process is stopped
	Error   ProcessState = "error"   // Process is in error state
)

// Status represents comprehensive system and application status information.
// It includes CPU, memory, disk, network statistics, and Xray process status.
type Status struct {
	T           time.Time `json:"-"`
	Cpu         float64   `json:"cpu"`
	CpuCores    int       `json:"cpuCores"`
	LogicalPro  int       `json:"logicalPro"`
	CpuSpeedMhz float64   `json:"cpuSpeedMhz"`
	Mem         struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"mem"`
	Swap struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"swap"`
	Disk struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"disk"`
	Xray struct {
		State    ProcessState `json:"state"`
		ErrorMsg string       `json:"errorMsg"`
		Version  string       `json:"version"`
	} `json:"xray"`
	PanelVersion string    `json:"panelVersion"`
	Uptime       uint64    `json:"uptime"`
	Loads        []float64 `json:"loads"`
	TcpCount     int       `json:"tcpCount"`
	UdpCount     int       `json:"udpCount"`
	NetIO        struct {
		Up   uint64 `json:"up"`
		Down uint64 `json:"down"`
	} `json:"netIO"`
	NetTraffic struct {
		Sent uint64 `json:"sent"`
		Recv uint64 `json:"recv"`
	} `json:"netTraffic"`
	PublicIP struct {
		IPv4 string `json:"ipv4"`
		IPv6 string `json:"ipv6"`
	} `json:"publicIP"`
	ServerInfo struct {
		Source           string            `json:"source"`
		Provider         string            `json:"provider"`
		Error            string            `json:"error"`
		Hostname         string            `json:"hostname"`
		NodeAlias        string            `json:"nodeAlias"`
		NodeLocation     string            `json:"nodeLocation"`
		Plan             string            `json:"plan"`
		PlanMonthlyData  uint64            `json:"planMonthlyData"`
		PlanDisk         uint64            `json:"planDisk"`
		PlanRAM          uint64            `json:"planRam"`
		PlanSwap         uint64            `json:"planSwap"`
		Email            string            `json:"email"`
		DataCounter      uint64            `json:"dataCounter"`
		DataNextReset    int64             `json:"dataNextReset"`
		IPAddresses      []string          `json:"ipAddresses"`
		RDNSAPIAvailable bool              `json:"rdnsApiAvailable"`
		PTR              map[string]string `json:"ptr"`
		VMType           string            `json:"vmType"`
		OS               string            `json:"os"`
		Geo              NodeGeoLocation   `json:"geo"`
	} `json:"serverInfo"`
	AppStats struct {
		Threads uint32 `json:"threads"`
		Mem     uint64 `json:"mem"`
		Uptime  uint64 `json:"uptime"`
	} `json:"appStats"`
}

// Release represents information about a software release from GitHub.
type Release struct {
	TagName string `json:"tag_name"` // The tag name of the release
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
}

// ServerService provides business logic for server monitoring and management.
// It handles system status collection, IP detection, and application statistics.
type ServerService struct {
	xrayService        XrayService
	inboundService     InboundService
	settingService     SettingService
	cachedIPv4         string
	cachedIPv6         string
	noIPv6             bool
	mu                 sync.Mutex
	lastCPUTimes       cpu.TimesStat
	hasLastCPUSample   bool
	hasNativeCPUSample bool
	emaCPU             float64
	cachedCpuSpeedMhz  float64
	lastCpuInfoAttempt time.Time
	cachedGeoIP        string
	cachedGeoLocation  NodeGeoLocation
	lastGeoAttempt     time.Time
	ipGeoCache         map[string]cachedIPGeo
	ipAttributionCache map[string]cachedIPGeo
	cachedCloud64Key   string
	cachedCloud64Info  *cloud64ServiceInfo
	cachedCloud64At    time.Time

	lastStatusMu sync.RWMutex
	lastStatus   *Status

	versionsCacheMu sync.Mutex
	versionsCache   *cachedXrayVersions
}

type cachedXrayVersions struct {
	versions  []string
	fetchedAt time.Time
}

// xrayVersionsCacheTTL bounds how often /getXrayVersion hits GitHub. The list
// is purely informational (rendered in the "switch Xray version" picker) so a
// quarter-hour staleness window is fine and saves the API budget.
const xrayVersionsCacheTTL = 15 * time.Minute

// allowedHistoryBuckets is the bucket-second whitelist for time-series
// aggregation endpoints (server + node metrics). Restricting it prevents
// callers from triggering arbitrary aggregation work and keeps the
// frontend's bucket selector self-documenting.
var allowedHistoryBuckets = map[int]bool{
	2:   true, // Real-time view
	30:  true, // 30s intervals
	60:  true, // 1m intervals
	120: true, // 2m intervals
	180: true, // 3m intervals
	300: true, // 5m intervals
}

// IsAllowedHistoryBucket reports whether a bucket-seconds value is in the
// whitelist used by /server/history, /server/cpuHistory, /server/xrayMetricsHistory,
// /server/xrayObservatoryHistory, and /nodes/history.
func IsAllowedHistoryBucket(bucketSeconds int) bool {
	return allowedHistoryBuckets[bucketSeconds]
}

// LastStatus returns the most recent Status snapshot collected by
// RefreshStatus. Safe for concurrent readers.
func (s *ServerService) LastStatus() *Status {
	s.lastStatusMu.RLock()
	defer s.lastStatusMu.RUnlock()
	return s.lastStatus
}

// RefreshStatus collects a new system snapshot, stores it as LastStatus, and
// appends it to the system-metrics time series. Returns the new snapshot (may
// be nil if collection failed). Called by the background ticker; the caller is
// responsible for any side effects (websocket broadcast, xray metrics sample).
func (s *ServerService) RefreshStatus() *Status {
	next := s.GetStatus(s.LastStatus())
	if next == nil {
		return nil
	}
	s.lastStatusMu.Lock()
	s.lastStatus = next
	s.lastStatusMu.Unlock()
	s.AppendStatusSample(time.Now(), next)
	return next
}

// GetXrayVersionsCached wraps GetXrayVersions with a TTL cache. On fetch
// failure we serve the last successful list (if any) so the UI doesn't go
// blank during a GitHub API hiccup; if there's no cache at all the underlying
// error is surfaced.
func (s *ServerService) GetXrayVersionsCached() ([]string, error) {
	s.versionsCacheMu.Lock()
	cache := s.versionsCache
	s.versionsCacheMu.Unlock()
	if cache != nil && time.Since(cache.fetchedAt) <= xrayVersionsCacheTTL {
		return cache.versions, nil
	}
	versions, err := s.GetXrayVersions()
	if err != nil {
		if cache != nil {
			logger.Warning("GetXrayVersionsCached: serving stale list:", err)
			return cache.versions, nil
		}
		return nil, err
	}
	s.versionsCacheMu.Lock()
	s.versionsCache = &cachedXrayVersions{versions: versions, fetchedAt: time.Now()}
	s.versionsCacheMu.Unlock()
	return versions, nil
}

// GetDefaultLogOutboundTags scans the default Xray config for freedom and
// blackhole outbound tags so /getXrayLogs can colour-code log lines without
// the controller re-doing the JSON walk. Falls back to the historical
// "direct"/"blocked" defaults when the config can't be read.
func (s *ServerService) GetDefaultLogOutboundTags() (freedoms, blackholes []string) {
	config, err := s.settingService.GetDefaultXrayConfig()
	if err == nil && config != nil {
		if cfgMap, ok := config.(map[string]any); ok {
			if outbounds, ok := cfgMap["outbounds"].([]any); ok {
				for _, outbound := range outbounds {
					obMap, ok := outbound.(map[string]any)
					if !ok {
						continue
					}
					tag, _ := obMap["tag"].(string)
					if tag == "" {
						continue
					}
					switch obMap["protocol"] {
					case "freedom":
						freedoms = append(freedoms, tag)
					case "blackhole":
						blackholes = append(blackholes, tag)
					}
				}
			}
		}
	}
	if len(freedoms) == 0 {
		freedoms = []string{"direct"}
	}
	if len(blackholes) == 0 {
		blackholes = []string{"blocked"}
	}
	return freedoms, blackholes
}

// AggregateCpuHistory returns up to maxPoints averaged buckets of size bucketSeconds.
// Kept for back-compat with the original /panel/api/server/cpuHistory/:bucket route;
// the response key is "cpu" (not "v") so legacy consumers parse unchanged.
func (s *ServerService) AggregateCpuHistory(bucketSeconds int, maxPoints int) []map[string]any {
	out := systemMetrics.aggregate("cpu", bucketSeconds, maxPoints)
	for _, p := range out {
		p["cpu"] = p["v"]
		delete(p, "v")
	}
	return out
}

// AggregateSystemMetric returns up to maxPoints averaged buckets for any
// known system metric (see SystemMetricKeys). Output points have keys
// {"t": unixSec, "v": value}; the caller decides how to format the value.
func (s *ServerService) AggregateSystemMetric(metric string, bucketSeconds int, maxPoints int) []map[string]any {
	return systemMetrics.aggregate(metric, bucketSeconds, maxPoints)
}

type LogEntry struct {
	DateTime    time.Time
	FromAddress string
	ToAddress   string
	Inbound     string
	Outbound    string
	Email       string
	Event       int
}

type NodeGeoLocation struct {
	IP        string  `json:"ip"`
	Location  string  `json:"location"`
	Country   string  `json:"country"`
	Province  string  `json:"province"`
	City      string  `json:"city"`
	District  string  `json:"district"`
	Detail    string  `json:"detail"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Source    string  `json:"source"`
	Error     string  `json:"error"`
}

type cachedIPGeo struct {
	location NodeGeoLocation
	at       time.Time
}

type cloud64ServiceInfo struct {
	Hostname         string            `json:"hostname"`
	NodeAlias        string            `json:"node_alias"`
	NodeLocation     string            `json:"node_location"`
	Plan             string            `json:"plan"`
	PlanMonthlyData  uint64            `json:"plan_monthly_data"`
	PlanDisk         uint64            `json:"plan_disk"`
	PlanRAM          uint64            `json:"plan_ram"`
	PlanSwap         uint64            `json:"plan_swap"`
	OS               string            `json:"os"`
	Email            string            `json:"email"`
	DataCounter      uint64            `json:"data_counter"`
	DataNextReset    int64             `json:"data_next_reset"`
	IPAddresses      []string          `json:"ip_addresses"`
	RDNSAPIAvailable json.RawMessage   `json:"rdns_api_available"`
	PTR              map[string]string `json:"ptr"`
	Error            json.RawMessage   `json:"error"`
}

const (
	defaultServerProviderURL = "https://api.64clouds.com/v1/getServiceInfo"
	ipGeoSuccessCacheTTL     = 24 * time.Hour
	ipGeoFailureCacheTTL     = 15 * time.Minute
	ipAttrSuccessCacheTTL    = 48 * time.Hour
	ipAttrFailureCacheTTL    = 5 * time.Minute
)

func getPublicIP(url string) string {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return "N/A"
	}
	defer resp.Body.Close()

	// Don't retry if access is blocked or region-restricted
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnavailableForLegalReasons {
		return "N/A"
	}
	if resp.StatusCode != http.StatusOK {
		return "N/A"
	}

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "N/A"
	}

	ipString := strings.TrimSpace(string(ip))
	if ipString == "" {
		return "N/A"
	}

	return ipString
}

func (s *ServerService) LookupIPGeo(ctx context.Context, ip string) NodeGeoLocation {
	ip = strings.TrimSpace(ip)
	result := NodeGeoLocation{IP: ip, Source: "meituan"}
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.Is4() {
		result.Error = fmt.Sprintf("invalid ipv4: %s", ip)
		return result
	}

	now := time.Now()
	s.mu.Lock()
	if s.ipGeoCache == nil {
		s.ipGeoCache = make(map[string]cachedIPGeo)
	}
	if cached, ok := s.ipGeoCache[ip]; ok {
		ttl := cachedIPGeoTTL(cached.location, ipGeoSuccessCacheTTL, ipGeoFailureCacheTTL)
		if now.Sub(cached.at) < ttl {
			s.mu.Unlock()
			return cached.location
		}
	}
	s.mu.Unlock()

	geo, err := fetchMeituanNodeGeo(ctx, ip)
	if err != nil {
		geo = NodeGeoLocation{IP: ip, Source: "meituan", Error: err.Error()}
	}
	s.mu.Lock()
	s.ipGeoCache[ip] = cachedIPGeo{location: geo, at: time.Now()}
	s.mu.Unlock()
	return geo
}

func (s *ServerService) LookupIPAttribution(ctx context.Context, ip string) NodeGeoLocation {
	ip = strings.TrimSpace(ip)
	result := NodeGeoLocation{IP: ip, Source: "ipwho"}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		result.Error = fmt.Sprintf("invalid ip: %s", ip)
		return result
	}
	ip = addr.String()
	result.IP = ip

	now := time.Now()
	s.mu.Lock()
	if s.ipAttributionCache == nil {
		s.ipAttributionCache = make(map[string]cachedIPGeo)
	}
	if cached, ok := s.ipAttributionCache[ip]; ok {
		ttl := cachedIPGeoTTL(cached.location, ipAttrSuccessCacheTTL, ipAttrFailureCacheTTL)
		if now.Sub(cached.at) < ttl {
			s.mu.Unlock()
			return cached.location
		}
	}
	s.mu.Unlock()

	geo, err := fetchIPAttributionNodeGeo(ctx, ip)
	if err != nil {
		geo = NodeGeoLocation{IP: ip, Source: "ipwho+ip9", Error: err.Error()}
	}
	s.mu.Lock()
	s.ipAttributionCache[ip] = cachedIPGeo{location: geo, at: time.Now()}
	s.mu.Unlock()
	return geo
}

type ipAttributionFetcher func(context.Context, string) (NodeGeoLocation, error)

func fetchIPAttributionNodeGeo(ctx context.Context, ip string) (NodeGeoLocation, error) {
	return fetchIPAttributionNodeGeoWith(ctx, ip, fetchIPWhoNodeGeo, fetchIP9NodeGeo)
}

func fetchIPAttributionNodeGeoWith(ctx context.Context, ip string, primary, fallback ipAttributionFetcher) (NodeGeoLocation, error) {
	geo, primaryErr := primary(ctx, ip)
	if primaryErr == nil && hasNodeGeoLocation(geo) {
		return geo, nil
	}

	geo, fallbackErr := fallback(ctx, ip)
	if fallbackErr == nil && hasNodeGeoLocation(geo) {
		return geo, nil
	}
	if primaryErr == nil {
		primaryErr = fmt.Errorf("empty location")
	}
	if fallbackErr == nil {
		fallbackErr = fmt.Errorf("empty location")
	}
	return NodeGeoLocation{}, fmt.Errorf("ipwho: %v; ip9: %v", primaryErr, fallbackErr)
}

func cachedIPGeoTTL(geo NodeGeoLocation, successTTL time.Duration, failureTTL time.Duration) time.Duration {
	if strings.TrimSpace(geo.Error) != "" || !hasNodeGeoLocation(geo) {
		return failureTTL
	}
	return successTTL
}

func hasNodeGeoLocation(geo NodeGeoLocation) bool {
	return strings.TrimSpace(geo.Location) != "" ||
		strings.TrimSpace(geo.Country) != "" ||
		strings.TrimSpace(geo.Province) != "" ||
		strings.TrimSpace(geo.City) != "" ||
		strings.TrimSpace(geo.District) != "" ||
		strings.TrimSpace(geo.Detail) != ""
}

func joinGeoAddress(values ...string) string {
	parts := []string{}
	for _, value := range values {
		part := strings.TrimSpace(value)
		if part == "" {
			continue
		}
		current := strings.Join(parts, "")
		if slices.Contains(parts, part) || (current != "" && strings.Contains(current, part)) {
			continue
		}
		if current != "" && strings.Contains(part, current) {
			parts = []string{part}
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "")
}

func fetchMeituanNodeGeo(ctx context.Context, ip string) (NodeGeoLocation, error) {
	result := NodeGeoLocation{IP: ip, Source: "meituan"}
	client := &http.Client{Timeout: 5 * time.Second}
	locReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://apimobile.meituan.com/locate/v2/ip/loc?rgeo=true&ip="+url.QueryEscape(ip), nil)
	if err != nil {
		return result, err
	}
	locRespHTTP, err := client.Do(locReq)
	if err != nil {
		return result, err
	}
	defer locRespHTTP.Body.Close()
	if locRespHTTP.StatusCode != http.StatusOK {
		return result, fmt.Errorf("meituan locate status %d", locRespHTTP.StatusCode)
	}
	var locResp struct {
		Code *int `json:"code"`
		Data struct {
			Lat  float64 `json:"lat"`
			Lng  float64 `json:"lng"`
			RGeo struct {
				Country  string `json:"country"`
				Province string `json:"province"`
				City     string `json:"city"`
				District string `json:"district"`
			} `json:"rgeo"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.NewDecoder(locRespHTTP.Body).Decode(&locResp); err != nil {
		return result, err
	}
	if locResp.Code == nil {
		return result, fmt.Errorf("meituan locate missing code")
	}
	if *locResp.Code != 0 {
		return result, fmt.Errorf("meituan locate code %d %s", *locResp.Code, locResp.Msg)
	}
	result.Latitude = locResp.Data.Lat
	result.Longitude = locResp.Data.Lng
	result.Country = strings.TrimSpace(locResp.Data.RGeo.Country)
	result.Province = strings.TrimSpace(locResp.Data.RGeo.Province)
	result.City = strings.TrimSpace(locResp.Data.RGeo.City)
	result.District = strings.TrimSpace(locResp.Data.RGeo.District)
	result.Location = joinGeoAddress(result.Country, result.Province, result.City, result.District)
	if result.Location == "" {
		return result, fmt.Errorf("meituan locate empty location")
	}

	if result.Latitude == 0 && result.Longitude == 0 {
		return result, nil
	}
	groupURL := fmt.Sprintf("https://apimobile.meituan.com/group/v1/city/latlng/%f,%f?tag=0", result.Latitude, result.Longitude)
	groupReq, err := http.NewRequestWithContext(ctx, http.MethodGet, groupURL, nil)
	if err != nil {
		return result, nil
	}
	groupRespHTTP, err := client.Do(groupReq)
	if err != nil {
		return result, nil
	}
	defer groupRespHTTP.Body.Close()
	if groupRespHTTP.StatusCode != http.StatusOK {
		return result, nil
	}
	var groupResp struct {
		Data struct {
			Detail string `json:"detail"`
		} `json:"data"`
	}
	if err := json.NewDecoder(groupRespHTTP.Body).Decode(&groupResp); err == nil {
		result.Detail = strings.TrimSpace(groupResp.Data.Detail)
		if result.Detail != "" {
			result.Location = joinGeoAddress(result.Country, result.Province, result.City, result.District, result.Detail)
		}
	}
	return result, nil
}

func fetchIP9NodeGeo(ctx context.Context, ip string) (NodeGeoLocation, error) {
	result := NodeGeoLocation{IP: ip, Source: "ip9"}
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ip9.com.cn/get?ip="+url.QueryEscape(ip), nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("User-Agent", "OUI Panel")
	respHTTP, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer respHTTP.Body.Close()
	if respHTTP.StatusCode != http.StatusOK {
		return result, fmt.Errorf("ip9 status %d", respHTTP.StatusCode)
	}

	var resp struct {
		Ret  int    `json:"ret"`
		Msg  string `json:"msg"`
		Data *struct {
			Country string `json:"country"`
			Prov    string `json:"prov"`
			City    string `json:"city"`
			Area    string `json:"area"`
			ISP     string `json:"isp"`
			Lng     string `json:"lng"`
			Lat     string `json:"lat"`
		} `json:"data"`
	}
	if err := json.NewDecoder(respHTTP.Body).Decode(&resp); err != nil {
		return result, err
	}
	if resp.Ret != 200 {
		if strings.TrimSpace(resp.Msg) == "" {
			resp.Msg = "unknown error"
		}
		return result, fmt.Errorf("ip9 ret %d %s", resp.Ret, resp.Msg)
	}
	if resp.Data == nil {
		return result, fmt.Errorf("ip9 empty data")
	}

	result.Country = strings.TrimSpace(resp.Data.Country)
	result.Province = strings.TrimSpace(resp.Data.Prov)
	result.City = strings.TrimSpace(resp.Data.City)
	result.District = strings.TrimSpace(resp.Data.Area)
	result.Detail = strings.TrimSpace(resp.Data.ISP)
	if lat, err := strconv.ParseFloat(strings.TrimSpace(resp.Data.Lat), 64); err == nil {
		result.Latitude = lat
	}
	if lng, err := strconv.ParseFloat(strings.TrimSpace(resp.Data.Lng), 64); err == nil {
		result.Longitude = lng
	}
	result.Location = joinGeoAddress(result.Country, result.Province, result.City, result.District, result.Detail)
	if result.Location == "" {
		return result, fmt.Errorf("ip9 empty location")
	}
	return result, nil
}

func fetchIPWhoNodeGeo(ctx context.Context, ip string) (NodeGeoLocation, error) {
	result := NodeGeoLocation{IP: ip, Source: "ipwho"}
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/"+url.PathEscape(ip), nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("User-Agent", "OUI Panel")
	req.Header.Set("Accept", "application/json")
	respHTTP, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer respHTTP.Body.Close()
	if respHTTP.StatusCode != http.StatusOK {
		return result, fmt.Errorf("ipwho status %d", respHTTP.StatusCode)
	}

	return decodeIPWhoNodeGeo(respHTTP.Body, ip)
}

func decodeIPWhoNodeGeo(body io.Reader, ip string) (NodeGeoLocation, error) {
	result := NodeGeoLocation{IP: ip, Source: "ipwho"}
	var resp struct {
		Success    bool    `json:"success"`
		Message    string  `json:"message"`
		Country    string  `json:"country"`
		Region     string  `json:"region"`
		City       string  `json:"city"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		Connection struct {
			ISP          string `json:"isp"`
			Organization string `json:"org"`
		} `json:"connection"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return result, err
	}
	if !resp.Success {
		if strings.TrimSpace(resp.Message) == "" {
			resp.Message = "unknown error"
		}
		return result, fmt.Errorf("ipwho %s", resp.Message)
	}

	result.Country = strings.TrimSpace(resp.Country)
	result.Province = strings.TrimSpace(resp.Region)
	result.City = strings.TrimSpace(resp.City)
	result.Detail = strings.TrimSpace(resp.Connection.ISP)
	if result.Detail == "" {
		result.Detail = strings.TrimSpace(resp.Connection.Organization)
	}
	result.Latitude = resp.Latitude
	result.Longitude = resp.Longitude
	result.Location = joinGeoAddress(result.Country, result.Province, result.City, result.Detail)
	if result.Location == "" {
		return result, fmt.Errorf("ipwho empty location")
	}
	return result, nil
}

func (s *ServerService) GetStatus(lastStatus *Status) *Status {
	now := time.Now()
	status := &Status{
		T: now,
	}

	// CPU stats
	util, err := s.sampleCPUUtilization()
	if err != nil {
		logger.Warning("get cpu percent failed:", err)
	} else {
		status.Cpu = util
	}

	status.CpuCores, err = cpu.Counts(false)
	if err != nil {
		logger.Warning("get cpu cores count failed:", err)
	}

	status.LogicalPro = runtime.NumCPU()

	if status.CpuSpeedMhz = s.cachedCpuSpeedMhz; s.cachedCpuSpeedMhz == 0 && time.Since(s.lastCpuInfoAttempt) > 5*time.Minute {
		s.lastCpuInfoAttempt = time.Now()
		done := make(chan struct{})
		go func() {
			defer close(done)
			cpuInfos, err := cpu.Info()
			if err != nil {
				logger.Warning("get cpu info failed:", err)
				return
			}
			if len(cpuInfos) > 0 {
				s.cachedCpuSpeedMhz = cpuInfos[0].Mhz
				status.CpuSpeedMhz = s.cachedCpuSpeedMhz
			} else {
				logger.Warning("could not find cpu info")
			}
		}()
		select {
		case <-done:
		case <-time.After(1500 * time.Millisecond):
			logger.Warning("cpu info query timed out; will retry later")
		}
	} else if s.cachedCpuSpeedMhz != 0 {
		status.CpuSpeedMhz = s.cachedCpuSpeedMhz
	}

	// Uptime
	upTime, err := host.Uptime()
	if err != nil {
		logger.Warning("get uptime failed:", err)
	} else {
		status.Uptime = upTime
	}

	// Memory stats
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		logger.Warning("get virtual memory failed:", err)
	} else {
		status.Mem.Current = memInfo.Used
		status.Mem.Total = memInfo.Total
	}

	swapInfo, err := mem.SwapMemory()
	if err != nil {
		logger.Warning("get swap memory failed:", err)
	} else {
		status.Swap.Current = swapInfo.Used
		status.Swap.Total = swapInfo.Total
	}

	// Disk stats
	diskInfo, err := disk.Usage("/")
	if err != nil {
		logger.Warning("get disk usage failed:", err)
	} else {
		status.Disk.Current = diskInfo.Used
		status.Disk.Total = diskInfo.Total
	}

	// Load averages
	avgState, err := load.Avg()
	if err != nil {
		logger.Warning("get load avg failed:", err)
	} else {
		status.Loads = []float64{avgState.Load1, avgState.Load5, avgState.Load15}
	}

	// Network stats
	ioStats, err := net.IOCounters(true)
	if err != nil {
		logger.Warning("get io counters failed:", err)
	} else {
		var totalSent, totalRecv uint64
		for _, iface := range ioStats {
			name := strings.ToLower(iface.Name)
			if isVirtualInterface(name) {
				continue
			}
			totalSent += iface.BytesSent
			totalRecv += iface.BytesRecv
		}
		status.NetTraffic.Sent = totalSent
		status.NetTraffic.Recv = totalRecv

		if lastStatus != nil {
			duration := now.Sub(lastStatus.T)
			seconds := float64(duration) / float64(time.Second)
			up := uint64(float64(status.NetTraffic.Sent-lastStatus.NetTraffic.Sent) / seconds)
			down := uint64(float64(status.NetTraffic.Recv-lastStatus.NetTraffic.Recv) / seconds)
			status.NetIO.Up = up
			status.NetIO.Down = down
		}
	}

	// TCP/UDP connections
	status.TcpCount, err = sys.GetTCPCount()
	if err != nil {
		logger.Warning("get tcp connections failed:", err)
	}

	status.UdpCount, err = sys.GetUDPCount()
	if err != nil {
		logger.Warning("get udp connections failed:", err)
	}

	// IP fetching with caching
	showIp4ServiceLists := []string{
		"https://api4.ipify.org",
		"https://ipv4.icanhazip.com",
		"https://v4.api.ipinfo.io/ip",
		"https://ipv4.myexternalip.com/raw",
		"https://4.ident.me",
		"https://check-host.net/ip",
	}
	showIp6ServiceLists := []string{
		"https://api6.ipify.org",
		"https://ipv6.icanhazip.com",
		"https://v6.api.ipinfo.io/ip",
		"https://ipv6.myexternalip.com/raw",
		"https://6.ident.me",
	}

	if s.cachedIPv4 == "" {
		for _, ip4Service := range showIp4ServiceLists {
			s.cachedIPv4 = getPublicIP(ip4Service)
			if s.cachedIPv4 != "N/A" {
				break
			}
		}
	}

	if s.cachedIPv6 == "" && !s.noIPv6 {
		for _, ip6Service := range showIp6ServiceLists {
			s.cachedIPv6 = getPublicIP(ip6Service)
			if s.cachedIPv6 != "N/A" {
				break
			}
		}
	}

	if s.cachedIPv6 == "N/A" {
		s.noIPv6 = true
	}

	status.PublicIP.IPv4 = s.cachedIPv4
	status.PublicIP.IPv6 = s.cachedIPv6
	s.applySystemServerInfo(status)
	s.applyNodeGeoLocation(status, now)

	// Xray status
	if s.xrayService.IsXrayRunning() {
		status.Xray.State = Running
		status.Xray.ErrorMsg = ""
	} else {
		err := s.xrayService.GetXrayErr()
		if err != nil {
			status.Xray.State = Error
		} else {
			status.Xray.State = Stop
		}
		status.Xray.ErrorMsg = s.xrayService.GetXrayResult()
	}
	status.Xray.Version = s.xrayService.GetXrayVersion()
	status.PanelVersion = config.GetVersion()

	// Application stats
	var rtm runtime.MemStats
	runtime.ReadMemStats(&rtm)
	status.AppStats.Mem = rtm.Sys
	status.AppStats.Threads = uint32(runtime.NumGoroutine())
	if p != nil && p.IsRunning() {
		status.AppStats.Uptime = p.GetUptime()
	} else {
		status.AppStats.Uptime = 0
	}

	return status
}

func (s *ServerService) applySystemServerInfo(status *Status) {
	status.ServerInfo.Source = "system"
	status.ServerInfo.Provider = "本机系统"
	if hostname, err := os.Hostname(); err == nil {
		status.ServerInfo.Hostname = hostname
	}
	if info, err := host.Info(); err == nil {
		if status.ServerInfo.Hostname == "" {
			status.ServerInfo.Hostname = info.Hostname
		}
		status.ServerInfo.VMType = strings.ToUpper(strings.TrimSpace(info.VirtualizationSystem))
		osLabel := strings.TrimSpace(info.Platform)
		if info.PlatformVersion != "" {
			osLabel = strings.TrimSpace(osLabel + " " + info.PlatformVersion)
		}
		status.ServerInfo.OS = osLabel
	}
	if status.ServerInfo.VMType == "" {
		status.ServerInfo.VMType = "物理/未知"
	}
	if status.ServerInfo.OS == "" {
		status.ServerInfo.OS = runtime.GOOS
	}
	s.applyConfiguredServerProvider(status)
}

func (s *ServerService) applyConfiguredServerProvider(status *Status) {
	provider, _ := s.settingService.GetServerProvider()
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return
	}
	if provider != "custom" && provider != "64clouds" {
		status.ServerInfo.Provider = provider
		status.ServerInfo.Error = "暂不支持该服务器商"
		return
	}

	status.ServerInfo.Source = "custom"
	status.ServerInfo.Provider = "自定义"
	providerURL, _ := s.settingService.GetServerProviderURL()
	veid, _ := s.settingService.GetServerProviderVEID()
	apiKey, _ := s.settingService.GetServerProviderAPIKey()
	providerURL = strings.TrimSpace(providerURL)
	veid = strings.TrimSpace(veid)
	apiKey = strings.TrimSpace(apiKey)
	if providerURL == "" {
		if provider == "64clouds" {
			providerURL = defaultServerProviderURL
		} else {
			status.ServerInfo.Error = "请在设置中填写自定义拉取链接"
			return
		}
	}
	if veid == "" || apiKey == "" {
		status.ServerInfo.Error = "请在设置中填写自定义 VEID 和自定义 API KEY"
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	info, err := s.getServerProviderServiceInfo(ctx, providerURL, veid, apiKey)
	if err != nil {
		status.ServerInfo.Error = err.Error()
		return
	}
	status.ServerInfo.Error = ""
	if info.Hostname != "" {
		status.ServerInfo.Hostname = info.Hostname
	}
	status.ServerInfo.NodeAlias = info.NodeAlias
	status.ServerInfo.NodeLocation = info.NodeLocation
	status.ServerInfo.Plan = info.Plan
	status.ServerInfo.PlanMonthlyData = info.PlanMonthlyData
	status.ServerInfo.PlanDisk = info.PlanDisk
	status.ServerInfo.PlanRAM = info.PlanRAM
	status.ServerInfo.PlanSwap = info.PlanSwap
	if info.OS != "" {
		status.ServerInfo.OS = info.OS
	}
	status.ServerInfo.Email = info.Email
	status.ServerInfo.DataCounter = info.DataCounter
	status.ServerInfo.DataNextReset = info.DataNextReset
	status.ServerInfo.IPAddresses = info.IPAddresses
	status.ServerInfo.RDNSAPIAvailable = cloud64Truthy(info.RDNSAPIAvailable)
	status.ServerInfo.PTR = info.PTR
}

func (s *ServerService) getServerProviderServiceInfo(ctx context.Context, providerURL string, veid string, apiKey string) (*cloud64ServiceInfo, error) {
	cacheKey := providerURL + ":" + veid + ":" + apiKey
	now := time.Now()

	s.mu.Lock()
	if s.cachedCloud64Info != nil && s.cachedCloud64Key == cacheKey && now.Sub(s.cachedCloud64At) < 15*time.Minute {
		info := s.cachedCloud64Info
		s.mu.Unlock()
		return info, nil
	}
	s.mu.Unlock()

	endpoint, err := buildServerProviderURL(providerURL, veid, apiKey)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.settingService.NewProxiedHTTPClient(8 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器商 API 返回 HTTP %d", resp.StatusCode)
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	info := parseServerProviderInfo(raw)
	if msg := cloud64ErrorMessage(info.Error); msg != "" {
		return nil, fmt.Errorf("服务器商 API 错误：%s", msg)
	}

	s.mu.Lock()
	s.cachedCloud64Key = cacheKey
	s.cachedCloud64Info = info
	s.cachedCloud64At = time.Now()
	s.mu.Unlock()
	return info, nil
}

func buildServerProviderURL(providerURL string, veid string, apiKey string) (string, error) {
	providerURL = strings.TrimSpace(providerURL)
	if providerURL == "" {
		return "", fmt.Errorf("自定义拉取链接不能为空")
	}
	replaced := strings.NewReplacer(
		"{veid}", url.QueryEscape(veid),
		"{VEID}", url.QueryEscape(veid),
		"%7Bveid%7D", url.QueryEscape(veid),
		"%7BVEID%7D", url.QueryEscape(veid),
		"{api_key}", url.QueryEscape(apiKey),
		"{API_KEY}", url.QueryEscape(apiKey),
		"{apiKey}", url.QueryEscape(apiKey),
		"{APIKey}", url.QueryEscape(apiKey),
		"%7Bapi_key%7D", url.QueryEscape(apiKey),
		"%7BAPI_KEY%7D", url.QueryEscape(apiKey),
		"%7BapiKey%7D", url.QueryEscape(apiKey),
		"%7BAPIKey%7D", url.QueryEscape(apiKey),
	).Replace(providerURL)
	endpoint, err := url.Parse(replaced)
	if err != nil {
		return "", err
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return "", fmt.Errorf("自定义拉取链接必须以 http:// 或 https:// 开头")
	}
	if !hasServerProviderPlaceholder(providerURL, "veid", "VEID") {
		q := endpoint.Query()
		q.Set("veid", veid)
		endpoint.RawQuery = q.Encode()
	}
	if !hasServerProviderPlaceholder(providerURL, "api_key", "API_KEY", "apiKey", "APIKey") {
		q := endpoint.Query()
		q.Set("api_key", apiKey)
		endpoint.RawQuery = q.Encode()
	}
	return endpoint.String(), nil
}

func hasServerProviderPlaceholder(providerURL string, names ...string) bool {
	for _, name := range names {
		if strings.Contains(providerURL, "{"+name+"}") || strings.Contains(providerURL, "%7B"+name+"%7D") {
			return true
		}
	}
	return false
}

func parseServerProviderInfo(raw map[string]json.RawMessage) *cloud64ServiceInfo {
	infoRaw := raw
	if nested, ok := raw["data"]; ok {
		var nestedMap map[string]json.RawMessage
		if err := json.Unmarshal(nested, &nestedMap); err == nil && len(nestedMap) > 0 {
			infoRaw = nestedMap
		}
	}
	info := &cloud64ServiceInfo{
		Hostname:         rawString(infoRaw, "hostname", "hostName"),
		NodeAlias:        rawString(infoRaw, "node_alias", "nodeAlias", "node"),
		NodeLocation:     rawString(infoRaw, "node_location", "nodeLocation", "location"),
		Plan:             rawString(infoRaw, "plan"),
		PlanMonthlyData:  rawUint64(infoRaw, "plan_monthly_data", "planMonthlyData", "monthly_data", "monthlyData"),
		PlanDisk:         rawUint64(infoRaw, "plan_disk", "planDisk", "disk"),
		PlanRAM:          rawUint64(infoRaw, "plan_ram", "planRam", "ram", "memory"),
		PlanSwap:         rawUint64(infoRaw, "plan_swap", "planSwap", "swap"),
		OS:               rawString(infoRaw, "os", "operatingSystem"),
		Email:            rawString(infoRaw, "email"),
		DataCounter:      rawUint64(infoRaw, "data_counter", "dataCounter", "used_data", "usedData"),
		DataNextReset:    rawInt64(infoRaw, "data_next_reset", "dataNextReset", "next_reset", "nextReset"),
		IPAddresses:      rawStringSlice(infoRaw, "ip_addresses", "ipAddresses", "ips"),
		RDNSAPIAvailable: rawMessage(infoRaw, "rdns_api_available", "rdnsApiAvailable"),
		PTR:              rawStringMap(infoRaw, "ptr", "rdns"),
		Error:            rawMessage(raw, "error"),
	}
	if len(info.Error) == 0 {
		info.Error = rawMessage(infoRaw, "error")
	}
	if len(info.Error) == 0 {
		if success, ok := rawBool(raw, "success", "ok"); ok && !success {
			info.Error = rawMessage(raw, "message", "msg")
		}
	}
	return info
}

func rawMessage(raw map[string]json.RawMessage, names ...string) json.RawMessage {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			return value
		}
	}
	return nil
}

func rawString(raw map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			var s string
			if err := json.Unmarshal(value, &s); err == nil {
				return strings.TrimSpace(s)
			}
			var n json.Number
			if err := json.Unmarshal(value, &n); err == nil {
				return n.String()
			}
		}
	}
	return ""
}

func rawBool(raw map[string]json.RawMessage, names ...string) (bool, bool) {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			var b bool
			if err := json.Unmarshal(value, &b); err == nil {
				return b, true
			}
			s := strings.ToLower(rawString(raw, name))
			switch s {
			case "true", "1", "yes", "ok", "success":
				return true, true
			case "false", "0", "no", "fail", "failed", "error":
				return false, true
			}
		}
	}
	return false, false
}

func rawUint64(raw map[string]json.RawMessage, names ...string) uint64 {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			var n uint64
			if err := json.Unmarshal(value, &n); err == nil {
				return n
			}
			var f float64
			if err := json.Unmarshal(value, &f); err == nil && f >= 0 {
				return uint64(f)
			}
			s := rawString(raw, name)
			if parsed, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func rawInt64(raw map[string]json.RawMessage, names ...string) int64 {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			var n int64
			if err := json.Unmarshal(value, &n); err == nil {
				return n
			}
			var f float64
			if err := json.Unmarshal(value, &f); err == nil {
				return int64(f)
			}
			s := rawString(raw, name)
			if parsed, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func rawStringSlice(raw map[string]json.RawMessage, names ...string) []string {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			var list []string
			if err := json.Unmarshal(value, &list); err == nil {
				return list
			}
			s := rawString(raw, name)
			if s == "" {
				continue
			}
			parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == '\n' })
			out := []string{}
			for _, part := range parts {
				if item := strings.TrimSpace(part); item != "" {
					out = append(out, item)
				}
			}
			return out
		}
	}
	return nil
}

func rawStringMap(raw map[string]json.RawMessage, names ...string) map[string]string {
	for _, name := range names {
		if value, ok := raw[name]; ok {
			var out map[string]string
			if err := json.Unmarshal(value, &out); err == nil {
				return out
			}
		}
	}
	return nil
}

func cloud64ErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if !b {
			return ""
		}
		return "true"
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" || s == "0" {
			return ""
		}
		return s
	}
	return string(raw)
}

func cloud64Truthy(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n != 0
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(strings.ToLower(s))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	}
	return false
}

func (s *ServerService) applyNodeGeoLocation(status *Status, now time.Time) {
	ip := strings.TrimSpace(status.PublicIP.IPv4)
	if ip == "" || ip == "N/A" {
		status.ServerInfo.Geo = NodeGeoLocation{IP: ip, Source: "ip9", Error: "no public IPv4"}
		return
	}

	s.mu.Lock()
	if s.cachedGeoIP == ip && !s.lastGeoAttempt.IsZero() && now.Sub(s.lastGeoAttempt) < 6*time.Hour {
		status.ServerInfo.Geo = s.cachedGeoLocation
		s.mu.Unlock()
		return
	}
	if s.cachedGeoIP == ip && !s.lastGeoAttempt.IsZero() && now.Sub(s.lastGeoAttempt) < 30*time.Second {
		status.ServerInfo.Geo = s.cachedGeoLocation
		s.mu.Unlock()
		return
	}
	s.cachedGeoIP = ip
	s.lastGeoAttempt = now
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	geo := s.LookupIPAttribution(ctx, ip)

	s.mu.Lock()
	s.cachedGeoIP = ip
	s.cachedGeoLocation = geo
	s.lastGeoAttempt = time.Now()
	s.mu.Unlock()
	status.ServerInfo.Geo = geo
}

// AppendCpuSample is preserved for callers that only have the CPU number.
// New callers should prefer AppendStatusSample which writes the full set.
func (s *ServerService) AppendCpuSample(t time.Time, v float64) {
	systemMetrics.append("cpu", t, v)
}

// AppendStatusSample writes one tick of every metric we keep — CPU, memory
// percent, network throughput (bytes/s), online client count, and the three
// load averages. Called by RefreshStatus on the same @2s cadence as
// AppendCpuSample, so all series stay aligned.
func (s *ServerService) AppendStatusSample(t time.Time, status *Status) {
	if status == nil {
		return
	}
	systemMetrics.append("cpu", t, status.Cpu)
	if status.Mem.Total > 0 {
		systemMetrics.append("mem", t, float64(status.Mem.Current)*100.0/float64(status.Mem.Total))
	}
	systemMetrics.append("netUp", t, float64(status.NetIO.Up))
	systemMetrics.append("netDown", t, float64(status.NetIO.Down))
	online := 0
	if p != nil && p.IsRunning() {
		online = len(p.GetOnlineClients())
	}
	systemMetrics.append("online", t, float64(online))
	if len(status.Loads) >= 3 {
		systemMetrics.append("load1", t, status.Loads[0])
		systemMetrics.append("load5", t, status.Loads[1])
		systemMetrics.append("load15", t, status.Loads[2])
	}
}

func (s *ServerService) sampleCPUUtilization() (float64, error) {
	// Try native platform-specific CPU implementation first (Windows, Linux, macOS)
	if pct, err := sys.CPUPercentRaw(); err == nil {
		s.mu.Lock()
		// First call to native method returns 0 (initializes baseline)
		if !s.hasNativeCPUSample {
			s.hasNativeCPUSample = true
			s.mu.Unlock()
			return 0, nil
		}
		// Smooth with EMA
		const alpha = 0.3
		if s.emaCPU == 0 {
			s.emaCPU = pct
		} else {
			s.emaCPU = alpha*pct + (1-alpha)*s.emaCPU
		}
		val := s.emaCPU
		s.mu.Unlock()
		return val, nil
	}
	// If native call fails, fall back to gopsutil times
	// Read aggregate CPU times (all CPUs combined)
	times, err := cpu.Times(false)
	if err != nil {
		return 0, err
	}
	if len(times) == 0 {
		return 0, fmt.Errorf("no cpu times available")
	}

	cur := times[0]

	s.mu.Lock()
	defer s.mu.Unlock()

	// If this is the first sample, initialize and return current EMA (0 by default)
	if !s.hasLastCPUSample {
		s.lastCPUTimes = cur
		s.hasLastCPUSample = true
		return s.emaCPU, nil
	}

	// Compute busy and total deltas
	// Note: Guest and GuestNice times are already included in User and Nice respectively,
	// so we exclude them to avoid double-counting (Linux kernel accounting)
	idleDelta := cur.Idle - s.lastCPUTimes.Idle
	busyDelta := (cur.User - s.lastCPUTimes.User) +
		(cur.System - s.lastCPUTimes.System) +
		(cur.Nice - s.lastCPUTimes.Nice) +
		(cur.Iowait - s.lastCPUTimes.Iowait) +
		(cur.Irq - s.lastCPUTimes.Irq) +
		(cur.Softirq - s.lastCPUTimes.Softirq) +
		(cur.Steal - s.lastCPUTimes.Steal)

	totalDelta := busyDelta + idleDelta

	// Update last sample for next time
	s.lastCPUTimes = cur

	// Guard against division by zero or negative deltas (e.g., counter resets)
	if totalDelta <= 0 {
		return s.emaCPU, nil
	}

	raw := 100.0 * (busyDelta / totalDelta)
	if raw < 0 {
		raw = 0
	}
	if raw > 100 {
		raw = 100
	}

	// Exponential moving average to smooth spikes
	const alpha = 0.3 // smoothing factor (0<alpha<=1). Higher = more responsive, lower = smoother
	if s.emaCPU == 0 {
		// Initialize EMA with the first real reading to avoid long warm-up from zero
		s.emaCPU = raw
	} else {
		s.emaCPU = alpha*raw + (1-alpha)*s.emaCPU
	}

	return s.emaCPU, nil
}

const (
	maxXrayArchiveBytes = 200 << 20
	maxXrayBinaryBytes  = 200 << 20
)

func (s *ServerService) GetXrayVersions() ([]string, error) {
	const (
		XrayURL    = "https://api.github.com/repos/XTLS/Xray-core/releases"
		bufferSize = 8192
	)

	resp, err := s.settingService.NewProxiedHTTPClient(10 * time.Second).Get(XrayURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check HTTP status code - GitHub API returns object instead of array on error
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		var errorResponse struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(bodyBytes, &errorResponse) == nil && errorResponse.Message != "" {
			return nil, fmt.Errorf("GitHub API error: %s", errorResponse.Message)
		}
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, resp.Status)
	}

	buffer := bytes.NewBuffer(make([]byte, bufferSize))
	buffer.Reset()
	if _, err := buffer.ReadFrom(resp.Body); err != nil {
		return nil, err
	}

	var releases []Release
	if err := json.Unmarshal(buffer.Bytes(), &releases); err != nil {
		return nil, err
	}

	var versions []string
	for _, release := range releases {
		tagVersion := strings.TrimPrefix(release.TagName, "v")
		tagParts := strings.Split(tagVersion, ".")
		if len(tagParts) != 3 {
			continue
		}

		major, err1 := strconv.Atoi(tagParts[0])
		minor, err2 := strconv.Atoi(tagParts[1])
		patch, err3 := strconv.Atoi(tagParts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}

		if major > 26 || (major == 26 && minor > 4) || (major == 26 && minor == 4 && patch >= 25) {
			versions = append(versions, release.TagName)
		}
	}
	return versions, nil
}

func (s *ServerService) StopXrayService() error {
	err := s.xrayService.StopXray()
	if err != nil {
		logger.Error("stop xray failed:", err)
		return err
	}
	return nil
}

func (s *ServerService) RestartXrayService() error {
	err := s.xrayService.RestartXray(true)
	if err != nil {
		logger.Error("start xray failed:", err)
		return err
	}
	return nil
}

func (s *ServerService) downloadXRay(version string) (string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	switch osName {
	case "darwin":
		osName = "macos"
	case "windows":
		osName = "windows"
	}

	switch arch {
	case "amd64":
		arch = "64"
	case "arm64":
		arch = "arm64-v8a"
	case "armv7":
		arch = "arm32-v7a"
	case "armv6":
		arch = "arm32-v6"
	case "armv5":
		arch = "arm32-v5"
	case "386":
		arch = "32"
	case "s390x":
		arch = "s390x"
	}

	fileName := fmt.Sprintf("Xray-%s-%s.zip", osName, arch)
	url := fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/download/%s/%s", version, fileName)
	client := s.settingService.NewProxiedHTTPClient(60 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download xray: unexpected HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxXrayArchiveBytes {
		return "", fmt.Errorf("download xray: archive exceeds %d bytes", maxXrayArchiveBytes)
	}

	file, err := os.CreateTemp("", "xray-*.zip")
	if err != nil {
		return "", err
	}
	path := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()

	n, err := io.Copy(file, io.LimitReader(resp.Body, maxXrayArchiveBytes+1))
	if err != nil {
		return "", err
	}
	if n > maxXrayArchiveBytes {
		return "", fmt.Errorf("download xray: archive exceeds %d bytes", maxXrayArchiveBytes)
	}

	ok = true
	return path, nil
}

func (s *ServerService) UpdateXray(version string) error {
	versions, err := s.GetXrayVersions()
	if err != nil {
		return err
	}
	if !slices.Contains(versions, version) {
		return fmt.Errorf("xray version %q is not in the fetched release list", version)
	}

	// 1. Stop xray before doing anything
	if err := s.StopXrayService(); err != nil {
		logger.Warning("failed to stop xray before update:", err)
	}

	// 2. Download the zip
	zipFileName, err := s.downloadXRay(version)
	if err != nil {
		return err
	}
	defer os.Remove(zipFileName)

	zipFile, err := os.Open(zipFileName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	stat, err := zipFile.Stat()
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(zipFile, stat.Size())
	if err != nil {
		return err
	}

	// 3. Helper to extract files
	copyZipFile := func(zipName string, fileName string) error {
		zipFile, err := reader.Open(zipName)
		if err != nil {
			return err
		}
		defer zipFile.Close()
		if err := os.MkdirAll(filepath.Dir(fileName), 0755); err != nil {
			return err
		}
		tmpFile, err := os.CreateTemp(filepath.Dir(fileName), ".xray-*")
		if err != nil {
			return err
		}
		tmpPath := tmpFile.Name()
		ok := false
		defer func() {
			_ = tmpFile.Close()
			if !ok {
				_ = os.Remove(tmpPath)
			}
		}()
		n, err := io.Copy(tmpFile, io.LimitReader(zipFile, maxXrayBinaryBytes+1))
		if err != nil {
			return err
		}
		if n > maxXrayBinaryBytes {
			return fmt.Errorf("xray binary exceeds %d bytes", maxXrayBinaryBytes)
		}
		if err := tmpFile.Chmod(0755); err != nil {
			return err
		}
		if err := tmpFile.Close(); err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			_ = os.Remove(fileName)
		}
		if err := os.Rename(tmpPath, fileName); err != nil {
			return err
		}
		ok = true
		return nil
	}

	// 4. Extract correct binary
	if runtime.GOOS == "windows" {
		targetBinary := filepath.Join(config.GetBinFolderPath(), "xray-windows-amd64.exe")
		err = copyZipFile("xray.exe", targetBinary)
	} else {
		err = copyZipFile("xray", xray.GetBinaryPath())
	}
	if err != nil {
		return err
	}

	// 5. Restart xray
	if err := s.xrayService.RestartXray(true); err != nil {
		logger.Error("start xray failed:", err)
		return err
	}

	return nil
}

func (s *ServerService) GetLogs(count string, level string, syslog string) []string {
	c, _ := strconv.Atoi(count)
	var lines []string

	if syslog == "true" {
		// Check if running on Windows - journalctl is not available
		if runtime.GOOS == "windows" {
			return []string{"Syslog is not supported on Windows. Please use application logs instead by unchecking the 'Syslog' option."}
		}

		// Validate and sanitize count parameter
		countInt, err := strconv.Atoi(count)
		if err != nil || countInt < 1 || countInt > 10000 {
			return []string{"Invalid count parameter - must be a number between 1 and 10000"}
		}

		// Validate level parameter - only allow valid syslog levels
		validLevels := map[string]bool{
			"0": true, "emerg": true,
			"1": true, "alert": true,
			"2": true, "crit": true,
			"3": true, "err": true,
			"4": true, "warning": true,
			"5": true, "notice": true,
			"6": true, "info": true,
			"7": true, "debug": true,
		}
		if !validLevels[level] {
			return []string{"Invalid level parameter - must be a valid syslog level"}
		}

		// Use hardcoded command with validated parameters
		cmd := exec.Command("journalctl", "-u", "x-ui", "--no-pager", "-n", strconv.Itoa(countInt), "-p", level)
		var out bytes.Buffer
		cmd.Stdout = &out
		err = cmd.Run()
		if err != nil {
			return []string{"Failed to run journalctl command! Make sure systemd is available and x-ui service is registered."}
		}
		lines = strings.Split(out.String(), "\n")
	} else {
		lines = logger.GetLogs(c, level)
	}

	return lines
}

func (s *ServerService) GetXrayLogs(
	count string,
	filter string,
	showDirect string,
	showBlocked string,
	showProxy string,
	freedoms []string,
	blackholes []string) []LogEntry {

	const (
		Direct = iota
		Blocked
		Proxied
	)

	countInt, _ := strconv.Atoi(count)
	var entries []LogEntry

	pathToAccessLog, err := xray.GetAccessLogPath()
	if err != nil {
		return nil
	}

	file, err := os.Open(pathToAccessLog)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.Contains(line, "api -> api") {
			//skipping empty lines and api calls
			continue
		}

		if filter != "" && !strings.Contains(line, filter) {
			//applying filter if it's not empty
			continue
		}

		var entry LogEntry
		parts := strings.Fields(line)

		for i, part := range parts {

			if i == 0 {
				dateTime, err := time.ParseInLocation("2006/01/02 15:04:05.999999", parts[0]+" "+parts[1], time.Local)
				if err != nil {
					continue
				}
				entry.DateTime = dateTime.UTC()
			}

			if part == "from" {
				entry.FromAddress = strings.TrimLeft(parts[i+1], "/")
			} else if part == "accepted" {
				entry.ToAddress = strings.TrimLeft(parts[i+1], "/")
			} else if strings.HasPrefix(part, "[") {
				entry.Inbound = part[1:]
			} else if strings.HasSuffix(part, "]") {
				entry.Outbound = part[:len(part)-1]
			} else if part == "email:" {
				entry.Email = parts[i+1]
			}
		}

		if logEntryContains(line, freedoms) {
			if showDirect == "false" {
				continue
			}
			entry.Event = Direct
		} else if logEntryContains(line, blackholes) {
			if showBlocked == "false" {
				continue
			}
			entry.Event = Blocked
		} else {
			if showProxy == "false" {
				continue
			}
			entry.Event = Proxied
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil
	}

	if len(entries) > countInt {
		entries = entries[len(entries)-countInt:]
	}

	return entries
}

// isVirtualInterface returns true for loopback and virtual/tunnel interfaces
// that should be excluded from network traffic statistics.
func isVirtualInterface(name string) bool {
	// Exact matches
	if name == "lo" || name == "lo0" {
		return true
	}
	// Prefix matches for virtual/tunnel interfaces
	virtualPrefixes := []string{
		"loopback",
		"docker",
		"br-",
		"veth",
		"virbr",
		"tun",
		"tap",
		"wg",
		"tailscale",
		"zt",
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func logEntryContains(line string, suffixes []string) bool {
	for _, sfx := range suffixes {
		if strings.Contains(line, sfx+"]") {
			return true
		}
	}
	return false
}

func (s *ServerService) GetConfigJson() (any, error) {
	config, err := s.xrayService.GetXrayConfig()
	if err != nil {
		return nil, err
	}
	contents, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}

	var jsonData any
	err = json.Unmarshal(contents, &jsonData)
	if err != nil {
		return nil, err
	}

	return jsonData, nil
}

func (s *ServerService) GetDb() ([]byte, error) {
	// Update by manually trigger a checkpoint operation
	err := database.Checkpoint()
	if err != nil {
		return nil, err
	}
	// Open the file for reading
	file, err := os.Open(config.GetDBPath())
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the file contents
	fileContents, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return fileContents, nil
}

func (s *ServerService) ImportDB(file multipart.File) error {
	// Check if the file is a SQLite database
	isValidDb, err := database.IsSQLiteDB(file)
	if err != nil {
		return common.NewErrorf("Error checking db file format: %v", err)
	}
	if !isValidDb {
		return common.NewError("Invalid db file format")
	}

	// Reset the file reader to the beginning
	_, err = file.Seek(0, 0)
	if err != nil {
		return common.NewErrorf("Error resetting file reader: %v", err)
	}

	// Save the file as a temporary file
	tempPath := fmt.Sprintf("%s.temp", config.GetDBPath())

	// Remove the existing temporary file (if any)
	if _, err := os.Stat(tempPath); err == nil {
		if errRemove := os.Remove(tempPath); errRemove != nil {
			return common.NewErrorf("Error removing existing temporary db file: %v", errRemove)
		}
	}

	// Create the temporary file
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return common.NewErrorf("Error creating temporary db file: %v", err)
	}

	// Robust deferred cleanup for the temporary file
	defer func() {
		if tempFile != nil {
			if cerr := tempFile.Close(); cerr != nil {
				logger.Warningf("Warning: failed to close temp file: %v", cerr)
			}
		}
		if _, err := os.Stat(tempPath); err == nil {
			if rerr := os.Remove(tempPath); rerr != nil {
				logger.Warningf("Warning: failed to remove temp file: %v", rerr)
			}
		}
	}()

	// Save uploaded file to temporary file
	if _, err = io.Copy(tempFile, file); err != nil {
		return common.NewErrorf("Error saving db: %v", err)
	}

	// Close temp file before opening via sqlite
	if err = tempFile.Close(); err != nil {
		return common.NewErrorf("Error closing temporary db file: %v", err)
	}
	tempFile = nil

	// Validate integrity (no migrations / side effects)
	if err = database.ValidateSQLiteDB(tempPath); err != nil {
		return common.NewErrorf("Invalid or corrupt db file: %v", err)
	}

	xrayStopped := true
	defer func() {
		if xrayStopped {
			if errR := s.RestartXrayService(); errR != nil {
				logger.Warningf("Failed to restart Xray after DB import error: %v", errR)
			}
		}
	}()
	if errStop := s.StopXrayService(); errStop != nil {
		logger.Warningf("Failed to stop Xray before DB import: %v", errStop)
	}

	if errClose := database.CloseDB(); errClose != nil {
		logger.Warningf("Failed to close existing DB before replacement: %v", errClose)
	}

	// Backup the current database for fallback
	fallbackPath := fmt.Sprintf("%s.backup", config.GetDBPath())

	// Remove the existing fallback file (if any)
	if _, err := os.Stat(fallbackPath); err == nil {
		if errRemove := os.Remove(fallbackPath); errRemove != nil {
			return common.NewErrorf("Error removing existing fallback db file: %v", errRemove)
		}
	}

	// Move the current database to the fallback location
	if err = os.Rename(config.GetDBPath(), fallbackPath); err != nil {
		return common.NewErrorf("Error backing up current db file: %v", err)
	}

	// Defer fallback cleanup ONLY if everything goes well
	defer func() {
		if _, err := os.Stat(fallbackPath); err == nil {
			if rerr := os.Remove(fallbackPath); rerr != nil {
				logger.Warningf("Warning: failed to remove fallback file: %v", rerr)
			}
		}
	}()

	// Move temp to DB path
	if err = os.Rename(tempPath, config.GetDBPath()); err != nil {
		// Restore from fallback
		if errRename := os.Rename(fallbackPath, config.GetDBPath()); errRename != nil {
			return common.NewErrorf("Error moving db file and restoring fallback: %v", errRename)
		}
		return common.NewErrorf("Error moving db file: %v", err)
	}

	// Open & migrate new DB
	if err = database.InitDB(config.GetDBPath()); err != nil {
		if errRename := os.Rename(fallbackPath, config.GetDBPath()); errRename != nil {
			return common.NewErrorf("Error migrating db and restoring fallback: %v", errRename)
		}
		return common.NewErrorf("Error migrating db: %v", err)
	}

	s.inboundService.MigrateDB()

	xrayStopped = false
	if err = s.RestartXrayService(); err != nil {
		return common.NewErrorf("Imported DB but failed to start Xray: %v", err)
	}

	return nil
}

// IsValidGeofileName validates that the filename is safe for geofile operations.
// It checks for path traversal attempts and ensures the filename contains only safe characters.
func (s *ServerService) IsValidGeofileName(filename string) bool {
	if filename == "" {
		return false
	}

	// Check for path traversal attempts
	if strings.Contains(filename, "..") {
		return false
	}

	// Check for path separators (both forward and backward slash)
	if strings.ContainsAny(filename, `/\`) {
		return false
	}

	// Check for absolute path indicators
	if filepath.IsAbs(filename) {
		return false
	}

	// Additional security: only allow alphanumeric, dots, underscores, and hyphens
	// This is stricter than the general filename regex
	validGeofilePattern := `^[a-zA-Z0-9._-]+\.dat$`
	matched, _ := regexp.MatchString(validGeofilePattern, filename)
	return matched
}

func (s *ServerService) UpdateGeofile(fileName string) error {
	type geofileEntry struct {
		URL      string
		FileName string
	}
	geofileAllowlist := map[string]geofileEntry{
		"geoip.dat":      {"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat", "geoip.dat"},
		"geosite.dat":    {"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat", "geosite.dat"},
		"geoip_IR.dat":   {"https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat", "geoip_IR.dat"},
		"geosite_IR.dat": {"https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat", "geosite_IR.dat"},
		"geoip_RU.dat":   {"https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat", "geoip_RU.dat"},
		"geosite_RU.dat": {"https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat", "geosite_RU.dat"},
	}

	// Strict allowlist check to avoid writing uncontrolled files
	if fileName != "" {
		if _, ok := geofileAllowlist[fileName]; !ok {
			return common.NewErrorf("Invalid geofile name: %q not in allowlist", fileName)
		}
	}

	client := s.settingService.NewProxiedHTTPClient(0)

	downloadFile := func(url, destPath string) error {
		var req *http.Request
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return common.NewErrorf("Failed to create HTTP request for %s: %v", url, err)
		}

		var localFileModTime time.Time
		if fileInfo, err := os.Stat(destPath); err == nil {
			localFileModTime = fileInfo.ModTime()
			if !localFileModTime.IsZero() {
				req.Header.Set("If-Modified-Since", localFileModTime.UTC().Format(http.TimeFormat))
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return common.NewErrorf("Failed to download Geofile from %s: %v", url, err)
		}
		defer resp.Body.Close()

		// Parse Last-Modified header from server
		var serverModTime time.Time
		serverModTimeStr := resp.Header.Get("Last-Modified")
		if serverModTimeStr != "" {
			parsedTime, err := time.Parse(http.TimeFormat, serverModTimeStr)
			if err != nil {
				logger.Warningf("Failed to parse Last-Modified header for %s: %v", url, err)
			} else {
				serverModTime = parsedTime
			}
		}

		// Function to update local file's modification time
		updateFileModTime := func() {
			if !serverModTime.IsZero() {
				if err := os.Chtimes(destPath, serverModTime, serverModTime); err != nil {
					logger.Warningf("Failed to update modification time for %s: %v", destPath, err)
				}
			}
		}

		// Handle 304 Not Modified
		if resp.StatusCode == http.StatusNotModified {
			updateFileModTime()
			return nil
		}

		if resp.StatusCode != http.StatusOK {
			return common.NewErrorf("Failed to download Geofile from %s: received status code %d", url, resp.StatusCode)
		}

		file, err := os.Create(destPath)
		if err != nil {
			return common.NewErrorf("Failed to create Geofile %s: %v", destPath, err)
		}
		defer file.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return common.NewErrorf("Failed to save Geofile %s: %v", destPath, err)
		}

		updateFileModTime()
		return nil
	}

	var errorMessages []string

	if fileName == "" {
		// Download all geofiles
		for _, entry := range geofileAllowlist {
			destPath := filepath.Join(config.GetBinFolderPath(), entry.FileName)
			if err := downloadFile(entry.URL, destPath); err != nil {
				errorMessages = append(errorMessages, fmt.Sprintf("Error downloading Geofile '%s': %v", entry.FileName, err))
			}
		}
	} else {
		entry := geofileAllowlist[fileName]
		destPath := filepath.Join(config.GetBinFolderPath(), entry.FileName)
		if err := downloadFile(entry.URL, destPath); err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("Error downloading Geofile '%s': %v", entry.FileName, err))
		}
	}

	err := s.RestartXrayService()
	if err != nil {
		errorMessages = append(errorMessages, fmt.Sprintf("Updated Geofile '%s' but Failed to start Xray: %v", fileName, err))
	}

	if len(errorMessages) > 0 {
		return common.NewErrorf("%s", strings.Join(errorMessages, "\r\n"))
	}

	return nil
}

func (s *ServerService) GetNewX25519Cert() (any, error) {
	// Run the command
	cmd := exec.Command(xray.GetBinaryPath(), "x25519")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out.String(), "\n")

	privateKeyLine := strings.Split(lines[0], ":")
	publicKeyLine := strings.Split(lines[1], ":")

	privateKey := strings.TrimSpace(privateKeyLine[1])
	publicKey := strings.TrimSpace(publicKeyLine[1])

	keyPair := map[string]any{
		"privateKey": privateKey,
		"publicKey":  publicKey,
	}

	return keyPair, nil
}

func (s *ServerService) GetNewmldsa65() (any, error) {
	// Run the command
	cmd := exec.Command(xray.GetBinaryPath(), "mldsa65")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out.String(), "\n")

	SeedLine := strings.Split(lines[0], ":")
	VerifyLine := strings.Split(lines[1], ":")

	seed := strings.TrimSpace(SeedLine[1])
	verify := strings.TrimSpace(VerifyLine[1])

	keyPair := map[string]any{
		"seed":   seed,
		"verify": verify,
	}

	return keyPair, nil
}

func (s *ServerService) GetNewEchCert(sni string) (any, error) {
	// Run the command
	cmd := exec.Command(xray.GetBinaryPath(), "tls", "ech", "--serverName", sni)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out.String(), "\n")
	if len(lines) < 4 {
		return nil, common.NewError("invalid ech cert")
	}

	configList := lines[1]
	serverKeys := lines[3]

	return map[string]any{
		"echServerKeys": serverKeys,
		"echConfigList": configList,
	}, nil
}

func (s *ServerService) GetNewVlessEnc() (any, error) {
	cmd := exec.Command(xray.GetBinaryPath(), "vlessenc")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return map[string]any{
		"auths": parseVlessEncAuths(out.String()),
	}, nil
}

func parseVlessEncAuths(output string) []map[string]string {
	lines := strings.Split(output, "\n")
	var auths []map[string]string
	var current map[string]string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Authentication:") {
			if current != nil {
				auths = append(auths, current)
			}
			label := strings.TrimSpace(strings.TrimPrefix(line, "Authentication:"))
			current = map[string]string{
				"id":    vlessEncAuthID(label),
				"label": label,
			}
		} else if strings.HasPrefix(line, `"decryption"`) || strings.HasPrefix(line, `"encryption"`) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && current != nil {
				key := strings.Trim(parts[0], `" `)
				val := strings.TrimSpace(parts[1])
				val = strings.TrimSuffix(val, ",")
				val = strings.Trim(val, `" `)
				current[key] = val
			}
		}
	}

	if current != nil {
		auths = append(auths, current)
	}

	return auths
}

func vlessEncAuthID(label string) string {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(label))
	switch {
	case strings.Contains(normalized, "mlkem768"):
		return "mlkem768"
	case strings.Contains(normalized, "x25519"):
		return "x25519"
	default:
		return normalized
	}
}

func (s *ServerService) GetNewUUID() (map[string]string, error) {
	newUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}

	return map[string]string{
		"uuid": newUUID.String(),
	}, nil
}

func (s *ServerService) GetNewmlkem768() (any, error) {
	// Run the command
	cmd := exec.Command(xray.GetBinaryPath(), "mlkem768")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out.String(), "\n")

	SeedLine := strings.Split(lines[0], ":")
	ClientLine := strings.Split(lines[1], ":")

	seed := strings.TrimSpace(SeedLine[1])
	client := strings.TrimSpace(ClientLine[1])

	keyPair := map[string]any{
		"seed":   seed,
		"client": client,
	}

	return keyPair, nil
}
