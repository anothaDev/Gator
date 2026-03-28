package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/anothaDev/gator/internal/models"
)

const (
	tailscalePackageName  = "os-tailscale"
	defaultLoginServer    = "https://controlplane.tailscale.com"
	defaultLoginTimeout   = "10"
	defaultListenPort     = "41641"
	tailscaleDevicePrefix = "tailscale"
)

type TailscaleStore interface {
	GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error)
}

type TailscaleHandler struct {
	store TailscaleStore
}

func NewTailscaleHandler(store TailscaleStore) *TailscaleHandler {
	return &TailscaleHandler{store: store}
}

func (h *TailscaleHandler) Status(c *gin.Context) {
	status, err := h.buildStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *TailscaleHandler) Install(c *gin.Context) {
	ctx := c.Request.Context()
	api, err := h.opnsenseAPI(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	installed, _, err := h.detectInstalled(ctx, api)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to check Tailscale plugin: " + err.Error()})
		return
	}
	if installed {
		status, _ := h.buildStatus(ctx)
		c.JSON(http.StatusOK, gin.H{"status": "already_installed", "tailscale": status})
		return
	}

	api.http.Timeout = 30 * time.Second
	resp, err := api.Post(ctx, "/api/core/firmware/install/"+tailscalePackageName, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to install Tailscale plugin: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "install_started",
		"msg_uuid": stringValue(resp["msg_uuid"]),
	})
}

// InstallStatus checks OPNsense firmware progress and whether the plugin is available yet.
func (h *TailscaleHandler) InstallStatus(c *gin.Context) {
	ctx := c.Request.Context()
	api, err := h.opnsenseAPI(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if a firmware task is still running.
	firmwareRunning := false
	if running, err := api.Get(ctx, "/api/core/firmware/running"); err == nil {
		firmwareRunning = stringValue(running["status"]) == "1"
	}

	// Get the upgrade/install log.
	var firmwareStatus, firmwareLog string
	if upgrade, err := api.Get(ctx, "/api/core/firmware/upgradestatus"); err == nil {
		firmwareStatus = stringValue(upgrade["status"])
		firmwareLog = stringValue(upgrade["log"])
	}

	// Check if the plugin API is reachable now.
	installed, _, _ := h.detectInstalled(ctx, api)

	c.JSON(http.StatusOK, gin.H{
		"firmware_running": firmwareRunning,
		"firmware_status":  firmwareStatus,
		"firmware_log":     firmwareLog,
		"plugin_ready":     installed,
	})
}

func (h *TailscaleHandler) Configure(c *gin.Context) {
	var body struct {
		PreAuthKey  string `json:"pre_auth_key" binding:"required"`
		LoginServer string `json:"login_server"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	preAuthKey := strings.TrimSpace(body.PreAuthKey)
	if preAuthKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pre-authentication key is required"})
		return
	}

	loginServer := strings.TrimSpace(body.LoginServer)
	if loginServer == "" {
		loginServer = defaultLoginServer
	}

	ctx := c.Request.Context()
	api, err := h.opnsenseAPI(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	installed, _, err := h.detectInstalled(ctx, api)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to check Tailscale plugin: " + err.Error()})
		return
	}
	if !installed {
		c.JSON(http.StatusConflict, gin.H{"error": "Tailscale plugin is not installed yet"})
		return
	}

	_, err = api.Post(ctx, "/api/tailscale/authentication/set", map[string]any{
		"authentication": map[string]any{
			"loginServer": loginServer,
			"preAuthKey":  preAuthKey,
		},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to save Tailscale authentication: " + err.Error()})
		return
	}

	_, err = api.Post(ctx, "/api/tailscale/settings/set", map[string]any{
		"settings": map[string]any{
			"enabled":            "1",
			"loginTimeout":       defaultLoginTimeout,
			"listenPort":         defaultListenPort,
			"acceptDNS":          "1",
			"advertiseExitNode":  "0",
			"useExitNode":        "",
			"acceptSubnetRoutes": "0",
			"enableSSH":          "0",
			"disableSNAT":        "0",
		},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to save Tailscale settings: " + err.Error()})
		return
	}

	// Reconfigure and start can be slow on first setup (Tailscale authenticates
	// with the control plane, generates keys, etc.).
	api.http.Timeout = 60 * time.Second

	if _, err := api.Post(ctx, "/api/tailscale/service/reconfigure", nil); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reconfigure Tailscale service: " + err.Error()})
		return
	}
	if _, err := api.Post(ctx, "/api/tailscale/service/start", nil); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to start Tailscale service: " + err.Error()})
		return
	}

	status, err := h.buildStatus(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "configured", "warning": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "configured", "tailscale": status})
}

// ListSubnets returns the current advertised subnets.
func (h *TailscaleHandler) ListSubnets(c *gin.Context) {
	ctx := c.Request.Context()
	api, err := h.opnsenseAPI(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := api.Post(ctx, "/api/tailscale/settings/search_subnet", map[string]any{
		"current": 1, "rowCount": -1, "searchPhrase": "",
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list subnets: " + err.Error()})
		return
	}

	rows, _ := resp["rows"].([]any)
	if rows == nil {
		rows = []any{}
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows, "total": len(rows)})
}

// AddSubnet adds a new advertised subnet and reconfigures the service.
func (h *TailscaleHandler) AddSubnet(c *gin.Context) {
	var body struct {
		Subnet      string `json:"subnet" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	api, err := h.opnsenseAPI(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Add the subnet entry.
	addResp, err := api.Post(ctx, "/api/tailscale/settings/add_subnet", map[string]any{
		"subnet4": map[string]any{
			"subnet":      body.Subnet,
			"description": body.Description,
		},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to add subnet: " + err.Error()})
		return
	}

	// Check for validation errors.
	if validations, ok := addResp["validations"].(map[string]any); ok && len(validations) > 0 {
		// Return the first validation error message.
		for _, msg := range validations {
			c.JSON(http.StatusBadRequest, gin.H{"error": stringValue(msg)})
			return
		}
	}

	// Reconfigure to apply.
	api.http.Timeout = 30 * time.Second
	if _, err := api.Post(ctx, "/api/tailscale/service/reconfigure", nil); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "subnet added but reconfigure failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "uuid": stringValue(addResp["uuid"])})
}

// DeleteSubnet removes an advertised subnet and reconfigures the service.
func (h *TailscaleHandler) DeleteSubnet(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid is required"})
		return
	}

	ctx := c.Request.Context()
	api, err := h.opnsenseAPI(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := api.Post(ctx, "/api/tailscale/settings/del_subnet/"+uuid, nil); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete subnet: " + err.Error()})
		return
	}

	api.http.Timeout = 30 * time.Second
	if _, err := api.Post(ctx, "/api/tailscale/service/reconfigure", nil); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "subnet deleted but reconfigure failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *TailscaleHandler) buildStatus(ctx context.Context) (gin.H, error) {
	api, err := h.opnsenseAPI(ctx)
	if err != nil {
		return nil, err
	}

	installed, rawSettings, err := h.detectInstalled(ctx, api)
	if err != nil {
		return nil, err
	}

	status := gin.H{
		"package_name":  tailscalePackageName,
		"installed":     installed,
		"opnsense_host": api.baseURL,
	}
	if !installed {
		status["message"] = "Tailscale plugin is not installed on this OPNsense instance."
		return status, nil
	}

	authData, err := api.Get(ctx, "/api/tailscale/authentication/get")
	if err != nil {
		return nil, fmt.Errorf("failed to load Tailscale authentication settings: %w", err)
	}

	settings := nestedMap(rawSettings, "settings")
	auth := nestedMap(authData, "authentication")
	loginServer := stringValue(auth["loginServer"])
	preAuthKey := strings.TrimSpace(stringValue(auth["preAuthKey"]))
	if loginServer == "" {
		loginServer = defaultLoginServer
	}

	serviceStatus, err := api.Get(ctx, "/api/tailscale/service/status")
	if err != nil {
		return nil, fmt.Errorf("failed to load Tailscale service status: %w", err)
	}
	statusInfo, err := api.Get(ctx, "/api/tailscale/status/status")
	if err != nil {
		statusInfo = map[string]any{"error": err.Error()}
	}
	ipInfo, err := api.Get(ctx, "/api/tailscale/status/ip")
	if err != nil {
		ipInfo = map[string]any{}
	}

	ifaceStatus, err := h.inspectTailscaleInterface(ctx, api)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect Tailscale interface: %w", err)
	}

	status["configured"] = preAuthKey != ""
	status["login_server"] = loginServer
	status["service_enabled"] = boolValue(settings["enabled"])
	status["accept_dns"] = boolValue(settings["acceptDNS"])
	status["service_running"] = strings.EqualFold(stringValue(serviceStatus["status"]), "running")
	status["service_status"] = stringValue(serviceStatus["status"])
	status["tailscale_ip"] = stringValue(ipInfo["result"])
	status["interface"] = ifaceStatus
	status["plugin_status"] = statusInfo

	if !ifaceStatus["found"].(bool) {
		status["message"] = "Tailscale is configured, but the tailscale0 interface has not appeared yet."
	} else if !ifaceStatus["assigned"].(bool) {
		status["message"] = "Assign tailscale0 as an OPNsense interface in Interfaces > Assignments, then refresh this page."
	} else if !ifaceStatus["enabled"].(bool) {
		status["message"] = "Enable the assigned Tailscale interface in OPNsense, then refresh this page."
	} else if !strings.EqualFold(stringValue(serviceStatus["status"]), "running") {
		status["message"] = "Tailscale is installed but the service is not running yet."
	} else {
		status["message"] = "Tailscale is installed, configured, and ready to use."
	}

	return status, nil
}

func (h *TailscaleHandler) detectInstalled(ctx context.Context, api *opnsenseAPIClient) (bool, map[string]any, error) {
	settings, err := api.Get(ctx, "/api/tailscale/settings/get")
	if err == nil {
		return true, settings, nil
	}
	if isMissingPluginError(err) {
		return false, nil, nil
	}
	return false, nil, err
}

func (h *TailscaleHandler) inspectTailscaleInterface(ctx context.Context, api *opnsenseAPIClient) (gin.H, error) {
	resp, err := api.Get(ctx, "/api/interfaces/overview/interfaces_info/0")
	if err != nil {
		return nil, err
	}

	rows, ok := resp["rows"].([]any)
	if !ok {
		return gin.H{
			"found":    false,
			"assigned": false,
			"enabled":  false,
		}, nil
	}

	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		device := stringValue(row["device"])
		if !strings.HasPrefix(strings.ToLower(device), tailscaleDevicePrefix) {
			continue
		}
		identifier := stringValue(row["identifier"])
		assigned := identifier != "" && identifier != device
		enabled := stringValue(row["enabled"]) == "1" || stringValue(row["status"]) != "disabled"
		return gin.H{
			"found":       true,
			"assigned":    assigned,
			"enabled":     enabled,
			"device":      device,
			"identifier":  identifier,
			"description": stringValue(row["description"]),
			"status":      stringValue(row["status"]),
		}, nil
	}

	return gin.H{
		"found":    false,
		"assigned": false,
		"enabled":  false,
	}, nil
}

func (h *TailscaleHandler) opnsenseAPI(ctx context.Context) (*opnsenseAPIClient, error) {
	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load firewall config")
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		return nil, fmt.Errorf("Tailscale setup requires an active OPNsense instance")
	}
	return newOPNsenseAPIClient(*firewallCfg), nil
}

func isMissingPluginError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status code 404") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "controller")
}

func nestedMap(root map[string]any, key string) map[string]any {
	if root == nil {
		return map[string]any{}
	}
	if nested, ok := root[key].(map[string]any); ok {
		return nested
	}
	return root
}

func boolValue(v any) bool {
	s := strings.TrimSpace(strings.ToLower(stringValue(v)))
	return s == "1" || s == "true" || s == "yes" || s == "running"
}
