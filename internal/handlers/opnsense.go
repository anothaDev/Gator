package handlers

import (
	"context"
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
	GetCache(ctx context.Context, key string) (string, error)
	SetCache(ctx context.Context, key, value string) error
}

type OPNsenseHandler struct {
	store OPNsenseStore
}

func NewOPNsenseHandler(store OPNsenseStore) *OPNsenseHandler {
	h := &OPNsenseHandler{store: store}
	// Start background firmware cache refresh.
	go h.refreshFirmwareLoop()
	return h
}

func (h *OPNsenseHandler) refreshFirmwareLoop() {
	// Initial fetch after a short delay to let the server start.
	time.Sleep(2 * time.Second)
	h.refreshFirmwareCache()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		h.refreshFirmwareCache()
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
}

func (h *OPNsenseHandler) Overview(c *gin.Context) {
	overview := models.OPNsenseOverview{}
	ctx := c.Request.Context()

	vpnCfg, err := h.store.GetSimpleVPNConfig(ctx)
	if err == nil && vpnCfg != nil {
		overview.VPN.Configured = true
		overview.VPN.Name = vpnCfg.Name
		overview.VPN.Applied = vpnCfg.OPNsenseServerUUID != ""
		overview.VPN.RoutingApplied = vpnCfg.RoutingApplied
		overview.VPN.LastAppliedAt = vpnCfg.LastAppliedAt
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil {
		overview.Error = "Firewall setup is not configured."
		c.JSON(http.StatusOK, overview)
		return
	}
	if firewallCfg.Type != "opnsense" {
		overview.Error = "Dashboard overview is currently available for OPNsense only."
		c.JSON(http.StatusOK, overview)
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Load firmware info from cache (refreshed in background every 5 min).
	if name, _ := h.store.GetCache(ctx, "firmware_name"); name != "" {
		overview.Name = name
	}
	if version, _ := h.store.GetCache(ctx, "firmware_version"); version != "" {
		overview.Version = version
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
			if mount == "/" {
				overview.Disk.Mountpoint = mount
				overview.Disk.UsedPct = usedPct
				break
			}
			if overview.Disk.Mountpoint == "" {
				overview.Disk.Mountpoint = mount
				overview.Disk.UsedPct = usedPct
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

	c.JSON(http.StatusOK, overview)
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
