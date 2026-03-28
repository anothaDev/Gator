package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/anothaDev/gator/internal/models"
)

type OPNsenseStore interface {
	GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error)
	GetSimpleVPNConfig(ctx context.Context) (*models.SimpleVPNConfig, error)
	ListVPNConfigs(ctx context.Context) ([]*models.SimpleVPNConfig, error)
	ListSiteTunnels(ctx context.Context) ([]*models.SiteTunnel, error)
	GetCache(ctx context.Context, key string) (string, error)
	SetCache(ctx context.Context, key, value string) error
}

type OPNsenseHandler struct {
	store  OPNsenseStore
	stopCh chan struct{}
}

func NewOPNsenseHandler(store OPNsenseStore) *OPNsenseHandler {
	h := &OPNsenseHandler{store: store, stopCh: make(chan struct{})}
	go h.refreshFirmwareLoop()
	return h
}

// Stop signals the background firmware cache loop to exit.
func (h *OPNsenseHandler) Stop() {
	close(h.stopCh)
}

func (h *OPNsenseHandler) refreshFirmwareLoop() {
	// Initial fetch after a short delay to let the server start.
	select {
	case <-time.After(2 * time.Second):
	case <-h.stopCh:
		return
	}
	h.refreshFirmwareCache()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.refreshFirmwareCache()
		case <-h.stopCh:
			return
		}
	}
}

func (h *OPNsenseHandler) refreshFirmwareCache() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	firmware, err := api.Post(ctx, "/api/core/firmware/status", map[string]any{})
	if err != nil {
		firmware, err = api.Get(ctx, "/api/core/firmware/status")
	}
	if err != nil || len(firmware) == 0 {
		return
	}

	name := asString(firmware["product_name"])
	version := asString(firmware["product_version"])
	if name == "" && version == "" {
		return
	}

	_ = h.store.SetCache(ctx, "firmware_name", name)
	_ = h.store.SetCache(ctx, "firmware_version", version)

	// Cache update availability.
	fwStatus := asString(firmware["status"])
	switch fwStatus {
	case "update", "upgrade":
		latest := firmwareTargetVersion(firmware)
		if latest == "" {
			latest = "available"
		}
		_ = h.store.SetCache(ctx, "firmware_needs_update", latest)
		_ = h.store.SetCache(ctx, "firmware_status_msg", asString(firmware["status_msg"]))
	default:
		_ = h.store.SetCache(ctx, "firmware_needs_update", "")
		_ = h.store.SetCache(ctx, "firmware_status_msg", "")
	}
	reboot := asString(firmware["needs_reboot"])
	if reboot == "" {
		reboot = asString(firmware["upgrade_needs_reboot"])
	}
	_ = h.store.SetCache(ctx, "firmware_needs_reboot", reboot)
}

// FirmwareStatus returns cached firmware update availability (lightweight, no OPNsense call).
func (h *OPNsenseHandler) FirmwareStatus(c *gin.Context) {
	ctx := c.Request.Context()
	needsUpdate, _ := h.store.GetCache(ctx, "firmware_needs_update")
	version, _ := h.store.GetCache(ctx, "firmware_version")
	statusMsg, _ := h.store.GetCache(ctx, "firmware_status_msg")
	needsReboot, _ := h.store.GetCache(ctx, "firmware_needs_reboot")

	c.JSON(http.StatusOK, gin.H{
		"current_version": version,
		"needs_update":    needsUpdate != "",
		"latest_version":  needsUpdate,
		"status_msg":      statusMsg,
		"needs_reboot":    needsReboot == "1",
	})
}

// Overview returns a single snapshot of the dashboard data.
func (h *OPNsenseHandler) Overview(c *gin.Context) {
	overview := h.buildOverview(c.Request.Context())
	c.JSON(http.StatusOK, overview)
}

// OverviewStream provides a Server-Sent Events stream of dashboard data.
// The client connects once and receives updates every 5 seconds until
// the connection is closed. Uses Gin's c.Stream for proper chunked
// streaming that works through reverse proxies and the Vite dev proxy.
func (h *OPNsenseHandler) OverviewStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// c.Stream handles flushing after each callback invocation and
	// exits when the callback returns false or the client disconnects.
	first := true
	c.Stream(func(w io.Writer) bool {
		if first {
			first = false
			h.writeSSEEvent(c)
			return true
		}
		select {
		case <-h.stopCh:
			return false
		case <-ticker.C:
			h.writeSSEEvent(c)
			return true
		}
	})
}

func (h *OPNsenseHandler) writeSSEEvent(c *gin.Context) {
	overview := h.buildOverview(c.Request.Context())
	data, _ := json.Marshal(overview)
	c.SSEvent("message", string(data))
}

// buildOverview collects all dashboard data from the DB and OPNsense API.
func (h *OPNsenseHandler) buildOverview(ctx context.Context) models.OPNsenseOverview {
	overview := models.OPNsenseOverview{}

	// Pick the most relevant VPN for the dashboard: active route > managed > any.
	vpnConfigs, err := h.store.ListVPNConfigs(ctx)
	if err == nil && len(vpnConfigs) > 0 {
		overview.VPNCount = len(vpnConfigs)
		overview.VPN.Configured = true
		vpnCfg := vpnConfigs[0]
		for _, cfg := range vpnConfigs {
			managed := models.IsOwnershipManaged(cfg.OwnershipStatus)
			if managed && cfg.RoutingApplied {
				vpnCfg = cfg
				break
			}
			if managed && !models.IsOwnershipManaged(vpnCfg.OwnershipStatus) {
				vpnCfg = cfg
			}
		}
		overview.VPN.Name = vpnCfg.Name
		managed := models.IsOwnershipManaged(vpnCfg.OwnershipStatus)
		overview.VPN.Applied = managed
		overview.VPN.RoutingApplied = managed && vpnCfg.RoutingApplied
		overview.VPN.LastAppliedAt = vpnCfg.LastAppliedAt
	}

	// Tunnel summary for the dashboard.
	if tunnels, tErr := h.store.ListSiteTunnels(ctx); tErr == nil {
		overview.Tunnels.Total = len(tunnels)
		for _, t := range tunnels {
			switch t.Status {
			case "deployed":
				overview.Tunnels.Deployed++
			case "error":
				overview.Tunnels.Errors++
			}
		}
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil {
		overview.Error = "Firewall setup is not configured."
		return overview
	}
	overview.Host = firewallCfg.Host
	overview.FirewallType = firewallCfg.Type
	if firewallCfg.Type != "opnsense" {
		overview.Error = "Dashboard overview is currently available for OPNsense only."
		return overview
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Load firmware info from cache (refreshed in background every 5 min).
	if name, _ := h.store.GetCache(ctx, "firmware_name"); name != "" {
		overview.Name = name
	}
	if version, _ := h.store.GetCache(ctx, "firmware_version"); version != "" {
		overview.Version = version
	}
	if latest, _ := h.store.GetCache(ctx, "firmware_needs_update"); latest != "" {
		if latest != "available" {
			overview.Updates = "Update to " + latest + " is available"
		} else if msg, _ := h.store.GetCache(ctx, "firmware_status_msg"); msg != "" {
			overview.Updates = msg
		} else {
			overview.Updates = "Updates available"
		}
	}

	// Fire remaining OPNsense API calls in parallel (no firmware — it's cached).
	type apiResult struct {
		data map[string]any
		err  error
	}

	var (
		mu                sync.Mutex
		permissionWarning bool
	)

	systemTime := make(chan apiResult, 1)
	resourcesCh := make(chan apiResult, 1)
	diskCh := make(chan apiResult, 1)
	gatewayCh := make(chan apiResult, 1)
	wireguardCh := make(chan apiResult, 1)

	go func() {
		data, err := api.Get(ctx, "/api/diagnostics/system/system_time")
		systemTime <- apiResult{data, err}
	}()
	go func() {
		data, err := api.Get(ctx, "/api/diagnostics/system/system_resources")
		resourcesCh <- apiResult{data, err}
	}()
	go func() {
		data, err := api.Get(ctx, "/api/diagnostics/system/system_disk")
		diskCh <- apiResult{data, err}
	}()
	go func() {
		data, err := api.Get(ctx, "/api/routes/gateway/status")
		gatewayCh <- apiResult{data, err}
	}()
	go func() {
		data, err := api.Get(ctx, "/api/wireguard/service/show")
		wireguardCh <- apiResult{data, err}
	}()

	addPermWarn := func(e error) {
		mu.Lock()
		defer mu.Unlock()
		permissionWarning = true
		if overview.ErrorDetail == "" && e != nil {
			overview.ErrorDetail = e.Error()
		}
	}

	// Use system_time as the connectivity check (fast, lightweight).
	anySuccess := false

	if r := <-systemTime; r.err == nil {
		anySuccess = true
		overview.Connected = true
		overview.Uptime = asString(r.data["uptime"])
		overview.DateTime = asString(r.data["datetime"])
		overview.LoadAvg = asString(r.data["loadavg"])
	} else {
		addPermWarn(r.err)
	}

	if r := <-resourcesCh; r.err == nil {
		anySuccess = true
		overview.Connected = true
		memory := asMap(r.data["memory"])
		overview.Memory.TotalMB = parseInt(memory["total_frmt"])
		overview.Memory.UsedMB = parseInt(memory["used_frmt"])
		if overview.Memory.TotalMB == 0 {
			overview.Memory.TotalMB = parseInt(memory["total"]) / 1024 / 1024
		}
		if overview.Memory.UsedMB == 0 {
			overview.Memory.UsedMB = parseInt(memory["used"]) / 1024 / 1024
		}
		// CPU core count from the cpu.used field ("N/M" format) or cpu object.
		cpu := asMap(r.data["cpu"])
		if total := parseInt(cpu["total"]); total > 0 {
			overview.CPUCount = total
		}
		// Fallback: count from top-level cpu_count or headers.
		if overview.CPUCount == 0 {
			overview.CPUCount = parseInt(r.data["cpu_count"])
		}
	} else {
		addPermWarn(r.err)
	}

	if r := <-diskCh; r.err == nil {
		anySuccess = true
		overview.Connected = true
		devices := asSlice(r.data["devices"])
		for _, raw := range devices {
			device := asMap(raw)
			mount := asString(device["mountpoint"])
			usedPct := parsePercent(device["used_pct"])
			size := asString(device["size"])
			used := asString(device["used"])
			if mount == "/" {
				overview.Disk.Mountpoint = mount
				overview.Disk.UsedPct = usedPct
				overview.Disk.Size = size
				overview.Disk.Used = used
				break
			}
			if overview.Disk.Mountpoint == "" {
				overview.Disk.Mountpoint = mount
				overview.Disk.UsedPct = usedPct
				overview.Disk.Size = size
				overview.Disk.Used = used
			}
		}
	} else {
		addPermWarn(r.err)
	}

	if r := <-gatewayCh; r.err == nil {
		anySuccess = true
		overview.Connected = true
		items := asSlice(r.data["items"])
		overview.Gateways.Total = len(items)
		for _, raw := range items {
			item := asMap(raw)
			status := strings.ToLower(asString(item["status_translated"]))
			if status == "" {
				status = strings.ToLower(asString(item["status"]))
			}
			if strings.Contains(status, "online") || strings.Contains(status, "active") || status == "up" {
				overview.Gateways.Online++
			} else {
				overview.Gateways.Offline++
			}
		}
	} else {
		addPermWarn(r.err)
	}

	if r := <-wireguardCh; r.err == nil {
		anySuccess = true
		overview.Connected = true
		rows := asSlice(r.data["rows"])
		if len(rows) == 0 {
			rows = asSlice(r.data["records"])
		}
		for _, raw := range rows {
			row := asMap(raw)
			typ := asString(row["type"])
			switch typ {
			case "interface":
				overview.WireGuard.Interfaces++
			case "peer":
				overview.WireGuard.Peers++
				if strings.EqualFold(asString(row["peer-status"]), "online") {
					overview.WireGuard.Online++
				}
			}
		}
	} else {
		addPermWarn(r.err)
	}

	if !anySuccess {
		overview.Error = "Could not reach OPNsense API with current setup credentials."
	} else if permissionWarning {
		overview.Error = "Connected with limited API permissions. Grant Diagnostics/Routes read access for full dashboard stats."
	}

	return overview
}

func asMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asSlice(value any) []any {
	if value == nil {
		return []any{}
	}
	if arr, ok := value.([]any); ok {
		return arr
	}
	return []any{}
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return ""
	}
}

func parseInt(value any) int {
	text := asString(value)
	if text == "" {
		return 0
	}

	builder := strings.Builder{}
	for _, r := range text {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}

	if builder.Len() == 0 {
		return 0
	}

	n, err := strconv.Atoi(builder.String())
	if err != nil {
		return 0
	}
	return n
}

func parsePercent(value any) int {
	return parseInt(value)
}

// firmwareTargetVersion extracts the target OPNsense version from the firmware
// status response. OPNsense nests the version info in upgrade_sets or all_sets
// arrays rather than a simple top-level field.
func firmwareTargetVersion(firmware map[string]any) string {
	// Try upgrade_sets first (base system upgrades), then all_sets.
	for _, key := range []string{"upgrade_sets", "all_sets"} {
		if sets, ok := firmware[key].([]any); ok {
			for _, item := range sets {
				entry, ok := item.(map[string]any)
				if !ok {
					continue
				}
				name := strings.ToLower(asString(entry["name"]))
				if strings.Contains(name, "opnsense") {
					if v := asString(entry["new_version"]); v != "" {
						return v
					}
				}
			}
		}
		// all_sets may also be a map keyed by package name.
		if sets, ok := firmware[key].(map[string]any); ok {
			for name, item := range sets {
				if !strings.Contains(strings.ToLower(name), "opnsense") {
					continue
				}
				if entry, ok := item.(map[string]any); ok {
					if v := asString(entry["new"]); v != "" {
						return v
					}
					if v := asString(entry["new_version"]); v != "" {
						return v
					}
				}
			}
		}
	}
	return ""
}
