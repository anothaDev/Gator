package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/anothaDev/gator/internal/models"
	"github.com/gin-gonic/gin"
)

type GatewayStore interface {
	GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error)
	ListVPNConfigs(ctx context.Context) ([]*models.SimpleVPNConfig, error)
	ListSiteTunnels(ctx context.Context) ([]*models.SiteTunnel, error)
	UpsertAppRoute(ctx context.Context, r models.AppRoute) error
}

type GatewayHandler struct {
	store GatewayStore
}

func NewGatewayHandler(store GatewayStore) *GatewayHandler {
	return &GatewayHandler{store: store}
}

// ListGateways returns all gateways from OPNsense via the routing API plus gateway status.
func (h *GatewayHandler) ListGateways(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway management requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Get gateway configuration list.
	configResp, err := api.Get(ctx, "/api/routing/settings/search_gateway")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list gateways: " + err.Error()})
		return
	}

	rows := asSlice(configResp["rows"])

	// Get runtime status.
	statusMap := map[string]string{}
	statusResp, err := api.Get(ctx, "/api/routes/gateway/status")
	if err == nil {
		for _, raw := range asSlice(statusResp["items"]) {
			item := asMap(raw)
			name := asString(item["name"])
			if name == "" {
				continue
			}
			status := strings.ToLower(asString(item["status_translated"]))
			if status == "" {
				status = strings.ToLower(asString(item["status"]))
			}
			statusMap[name] = status
		}
	}

	gateways := make([]gin.H, 0, len(rows))
	for _, raw := range rows {
		row := asMap(raw)
		name := asString(row["name"])
		status := statusMap[name]
		if status == "" {
			status = "unknown"
		}

		gateways = append(gateways, gin.H{
			"uuid":       asString(row["uuid"]),
			"name":       name,
			"interface":  asString(row["interface"]),
			"gateway":    asString(row["gateway"]),
			"ipprotocol": asString(row["ipprotocol"]),
			"disabled":   asString(row["disabled"]),
			"defaultgw":  asString(row["defaultgw"]),
			"descr":      asString(row["descr"]),
			"status":     status,
		})
	}

	c.JSON(http.StatusOK, gin.H{"gateways": gateways})
}

// DeleteGateway removes a gateway by UUID.
func (h *GatewayHandler) DeleteGateway(c *gin.Context) {
	ctx := c.Request.Context()
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid is required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway management requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Post(ctx, "/api/routing/settings/del_gateway/"+uuid, map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete gateway: " + err.Error()})
		return
	}
	if err := expectOPNsenseResult(resp, "deleted", "ok", ""); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "delete gateway failed: " + err.Error()})
		return
	}

	// Apply routing changes.
	if _, err := api.Post(ctx, "/api/routing/settings/reconfigure", map[string]any{}); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "deleted", "warning": "deleted but failed to reconfigure routing: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ListInterfaces returns all OPNsense interfaces with assignment and WireGuard detection.
func (h *GatewayHandler) ListInterfaces(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "interface listing requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Get(ctx, "/api/interfaces/overview/interfaces_info/0")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list interfaces: " + err.Error()})
		return
	}

	// The response uses the standard OPNsense grid pattern:
	// { "rows": [...], "rowCount": N, "total": N, "current": 1 }
	rows := asSlice(resp["rows"])

	interfaces := make([]gin.H, 0, len(rows))
	for _, raw := range rows {
		iface := asMap(raw)
		if len(iface) == 0 {
			continue
		}

		device := asString(iface["device"])
		if device == "" {
			continue
		}

		identifier := asString(iface["identifier"])
		if identifier == "" {
			identifier = device
		}

		// enabled can be "1", "0", or "" depending on how OPNsense returns it.
		enabled := asString(iface["enabled"])
		if enabled == "" {
			// Check if there are addresses — if so, it's likely enabled.
			if asString(iface["addr4"]) != "" || asString(iface["addr6"]) != "" {
				enabled = "1"
			}
		}

		description := asString(iface["description"])
		status := asString(iface["status"])
		if status == "" {
			status = asString(iface["link_type"])
		}
		ifType := asString(iface["type"])

		isWG := strings.HasPrefix(device, "wg")
		// An interface is "assigned" if it has an identifier different from the device name.
		assigned := identifier != device && identifier != ""

		// Gather addresses.
		addrs := make([]string, 0)
		if addr4 := asString(iface["addr4"]); addr4 != "" {
			addrs = append(addrs, addr4)
		}
		if addr6 := asString(iface["addr6"]); addr6 != "" {
			addrs = append(addrs, addr6)
		}
		if ipv4 := asSlice(iface["ipv4"]); len(ipv4) > 0 {
			for _, entry := range ipv4 {
				e := asMap(entry)
				if addr := asString(e["ipaddr"]); addr != "" {
					addrs = append(addrs, addr)
				}
			}
		}

		macAddr := asString(iface["macaddr"])

		interfaces = append(interfaces, gin.H{
			"identifier":   identifier,
			"device":       device,
			"description":  description,
			"status":       status,
			"enabled":      enabled,
			"type":         ifType,
			"is_wireguard": isWG,
			"assigned":     assigned,
			"addresses":    addrs,
			"macaddr":      macAddr,
		})
	}

	c.JSON(http.StatusOK, gin.H{"interfaces": interfaces})
}

// ListSelectableInterfaces returns interfaces that can be used as source interfaces
// for VPN routing (LAN-type interfaces, excluding WAN, WG tunnels, and unassigned devices).
func (h *GatewayHandler) ListSelectableInterfaces(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "interface listing requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Get(ctx, "/api/interfaces/overview/interfaces_info/0")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list interfaces: " + err.Error()})
		return
	}

	rows := asSlice(resp["rows"])

	type selectableIface struct {
		Identifier  string `json:"identifier"`
		Device      string `json:"device"`
		Description string `json:"description"`
	}

	var selectable []selectableIface
	for _, raw := range rows {
		iface := asMap(raw)
		device := asString(iface["device"])
		if device == "" {
			continue
		}

		identifier := asString(iface["identifier"])
		// Skip unassigned interfaces.
		if identifier == "" || identifier == device {
			continue
		}
		// Skip WireGuard tunnel interfaces — these are destinations, not sources.
		if strings.HasPrefix(device, "wg") {
			continue
		}
		// Skip WAN (or anything containing "wan").
		if identifier == "wan" {
			continue
		}

		description := asString(iface["description"])
		if description == "" {
			description = identifier
		}

		selectable = append(selectable, selectableIface{
			Identifier:  identifier,
			Device:      device,
			Description: description,
		})
	}

	c.JSON(http.StatusOK, gin.H{"interfaces": selectable})
}

// --- Helpers used by VPN routing automation ---

// discoverWGInterface finds the WireGuard interface on OPNsense.
// It returns (identifier, device, error).
// The identifier (e.g. "opt7") is what gateway/filter APIs need.
// The device (e.g. "wg1") is the kernel interface.
// WireGuard interfaces must be assigned in Interfaces > Assignments
// before they can be used for gateways and routing.
func discoverWGInterface(ctx context.Context, api *opnsenseAPIClient) (string, string, error) {
	resp, err := api.Get(ctx, "/api/interfaces/overview/interfaces_info/0")
	if err != nil {
		return "", "", err
	}

	// Response is { "rows": [...], "rowCount": N, ... }
	rows := asSlice(resp["rows"])

	var assignedNotEnabled string
	var assignedNotEnabledDevice string
	var unassignedDevice string
	for _, raw := range rows {
		iface := asMap(raw)
		device := asString(iface["device"])
		if device == "" || !strings.HasPrefix(device, "wg") {
			continue
		}
		identifier := asString(iface["identifier"])
		enabled := asString(iface["enabled"])
		assigned := identifier != "" && identifier != device

		if assigned && enabled != "0" {
			// Assigned and not explicitly disabled — this is our interface.
			// Accepts enabled="1", enabled="" (ambiguous), or any truthy value.
			return identifier, device, nil
		}
		if assigned && assignedNotEnabled == "" {
			// Assigned but explicitly disabled — remember as fallback.
			assignedNotEnabled = identifier
			assignedNotEnabledDevice = device
		}
		if !assigned && unassignedDevice == "" {
			unassignedDevice = device
		}
	}

	// Prefer an assigned-but-disabled interface over an unassigned one.
	if assignedNotEnabled != "" {
		return assignedNotEnabled, assignedNotEnabledDevice, nil
	}

	if unassignedDevice != "" {
		return "", unassignedDevice, nil
	}

	return "", "", nil
}

// ensureGateway creates or updates a gateway on OPNsense for the VPN interface.
// ipprotocol should be "inet" for IPv4 or "inet6" for IPv6.
// monitorIP should be a tunnel-reachable IP (e.g. provider DNS) for health checks.
func ensureGateway(
	ctx context.Context,
	api *opnsenseAPIClient,
	gwName string,
	ifaceName string,
	gatewayIP string,
	ipprotocol string,
	monitorIP string,
	preferredUUID string,
) (string, bool, error) {
	gwInner := map[string]any{
		"disabled":   "0",
		"name":       gwName,
		"interface":  ifaceName,
		"ipprotocol": ipprotocol,
		"gateway":    gatewayIP,
		"fargw":      "1",
		"priority":   "255",
		"weight":     "1",
		"descr":      "GATOR_GW (auto-managed)",
	}
	if monitorIP != "" {
		gwInner["monitor_disable"] = "0"
		gwInner["monitor"] = monitorIP
	} else {
		gwInner["monitor_disable"] = "1"
	}
	payload := map[string]any{"gateway_item": gwInner}

	// Try updating by preferred UUID first.
	if preferredUUID != "" {
		resp, err := api.Get(ctx, "/api/routing/settings/get_gateway/"+preferredUUID)
		if err == nil && len(asMap(resp["gateway_item"])) > 0 {
			setResp, err := api.Post(ctx, "/api/routing/settings/set_gateway/"+preferredUUID, payload)
			if err == nil {
				if err := expectOPNsenseResult(setResp, "saved", "ok"); err == nil {
					return preferredUUID, false, nil
				}
			}
		}
	}

	// Search for existing gateway by name.
	searchResp, err := api.Get(ctx, "/api/routing/settings/search_gateway")
	if err == nil {
		for _, raw := range asSlice(searchResp["rows"]) {
			row := asMap(raw)
			if asString(row["name"]) == gwName {
				uuid := asString(row["uuid"])
				if uuid != "" {
					setResp, err := api.Post(ctx, "/api/routing/settings/set_gateway/"+uuid, payload)
					if err == nil {
						if err := expectOPNsenseResult(setResp, "saved", "ok"); err == nil {
							return uuid, false, nil
						}
					}
				}
			}
		}
	}

	// Create new gateway.
	resp, err := api.Post(ctx, "/api/routing/settings/add_gateway", payload)
	if err != nil {
		return "", false, err
	}
	if err := expectOPNsenseResult(resp, "saved", "ok"); err != nil {
		return "", false, err
	}

	uuid, err := extractUUID(resp)
	if err != nil {
		return "", false, err
	}

	return uuid, true, nil
}

// ensureSNATRule creates or updates an outbound NAT rule for VPN traffic.
func ensureSNATRule(
	ctx context.Context,
	api *opnsenseAPIClient,
	ifaceName string,
	sourceLAN string,
	ipprotocol string,
	description string,
	preferredUUID string,
) (string, bool, error) {
	payload := map[string]any{
		"rule": map[string]any{
			"enabled":         "1",
			"interface":       ifaceName,
			"ipprotocol":      ipprotocol,
			"protocol":        "any",
			"source_net":      sourceLAN,
			"source_not":      "0",
			"destination_net": "any",
			"destination_not": "0",
			"target":          ifaceName + "ip",
			"nonat":           "0",
			"log":             "0",
			"description":     description,
		},
	}

	// Try updating by preferred UUID.
	if preferredUUID != "" {
		resp, err := api.Get(ctx, "/api/firewall/source_nat/get_rule/"+preferredUUID)
		if err == nil && len(asMap(resp["rule"])) > 0 {
			setResp, err := api.Post(ctx, "/api/firewall/source_nat/set_rule/"+preferredUUID, payload)
			if err == nil {
				if err := expectOPNsenseResult(setResp, "saved", "ok"); err == nil {
					return preferredUUID, false, nil
				}
			}
		}
	}

	// Search for existing rule by description.
	searchResp, err := api.Post(ctx, "/api/firewall/source_nat/search_rule", map[string]any{})
	if err == nil {
		for _, raw := range asSlice(searchResp["rows"]) {
			row := asMap(raw)
			if asString(row["description"]) == description {
				uuid := asString(row["uuid"])
				if uuid != "" {
					setResp, err := api.Post(ctx, "/api/firewall/source_nat/set_rule/"+uuid, payload)
					if err == nil {
						if err := expectOPNsenseResult(setResp, "saved", "ok"); err == nil {
							return uuid, false, nil
						}
					}
				}
			}
		}
	}

	// Create new rule.
	resp, err := api.Post(ctx, "/api/firewall/source_nat/add_rule", payload)
	if err != nil {
		return "", false, err
	}
	if err := expectOPNsenseResult(resp, "saved", "ok"); err != nil {
		return "", false, err
	}

	uuid, err := extractUUID(resp)
	if err != nil {
		return "", false, err
	}

	return uuid, true, nil
}

// resolveGatewayKey queries OPNsense for the valid gateway keys accepted by the
// filter rule model and returns the matching key for the given gateway name.
// The filter model uses a JsonKeyValueStoreField populated by `interface gateways list -g`,
// which returns keys like "Heroic_Ant_gw" or "Inet_Heroic_Ant_gw" depending on the version.
// If no match is found, the original name is returned as a fallback.
func resolveGatewayKey(ctx context.Context, api *opnsenseAPIClient, gatewayName string) string {
	if gatewayName == "" {
		return ""
	}

	// GET /api/firewall/filter/get_rule (no UUID) returns a blank rule template
	// with all field options, including the valid gateway keys.
	resp, err := api.Get(ctx, "/api/firewall/filter/get_rule")
	if err != nil {
		log.Printf("[resolveGatewayKey] failed to fetch rule template: %v, using name as-is: %s", err, gatewayName)
		return gatewayName
	}

	ruleData := asMap(resp["rule"])
	gwField := asMap(ruleData["gateway"])

	if len(gwField) == 0 {
		log.Printf("[resolveGatewayKey] gateway field empty or not a map, using name as-is: %s", gatewayName)
		return gatewayName
	}

	// Collect valid keys for debugging.
	var validKeys []string
	for key := range gwField {
		if key != "" { // skip empty/default entry
			validKeys = append(validKeys, key)
		}
	}

	// Exact match.
	if _, ok := gwField[gatewayName]; ok {
		return gatewayName
	}

	// Try suffix/contains match: OPNsense may prefix with address family like "Inet_" or "Inet6_".
	for key := range gwField {
		if key != "" && (strings.HasSuffix(key, "_"+gatewayName) || strings.Contains(key, gatewayName)) {
			log.Printf("[resolveGatewayKey] resolved %q -> %q (from %v)", gatewayName, key, validKeys)
			return key
		}
	}

	// Try matching gateway name as substring of display value (for nested structures).
	for key, val := range gwField {
		if key == "" {
			continue
		}
		valMap := asMap(val)
		displayVal := asString(valMap["value"])
		if displayVal == "" {
			displayVal = asString(val)
		}
		if strings.Contains(displayVal, gatewayName) {
			log.Printf("[resolveGatewayKey] resolved %q -> %q via display value (from %v)", gatewayName, key, validKeys)
			return key
		}
	}

	log.Printf("[resolveGatewayKey] no match for %q in valid keys: %v, using name as-is", gatewayName, validKeys)
	return gatewayName
}

// ensureFilterRule creates or updates a firewall policy routing rule.
func ensureFilterRule(
	ctx context.Context,
	api *opnsenseAPIClient,
	ifaceName string,
	sourceLAN string,
	gatewayName string,
	ipprotocol string,
	description string,
	preferredUUID string,
) (string, bool, error) {
	// Resolve the gateway name to the key OPNsense actually accepts.
	gwKey := resolveGatewayKey(ctx, api, gatewayName)
	log.Printf("[ensureFilterRule] gateway %q resolved to key %q for ipprotocol %s", gatewayName, gwKey, ipprotocol)

	payload := map[string]any{
		"rule": map[string]any{
			"enabled":         "1",
			"action":          "pass",
			"quick":           "1",
			"interface":       ifaceName,
			"direction":       "in",
			"ipprotocol":      ipprotocol,
			"protocol":        "any",
			"source_net":      sourceLAN,
			"source_not":      "0",
			"destination_net": "any",
			"destination_not": "0",
			"gateway":         gwKey,
			"log":             "0",
			"description":     description,
		},
	}

	// Try updating by preferred UUID.
	if preferredUUID != "" {
		resp, err := api.Get(ctx, "/api/firewall/filter/get_rule/"+preferredUUID)
		if err == nil && len(asMap(resp["rule"])) > 0 {
			setResp, err := api.Post(ctx, "/api/firewall/filter/set_rule/"+preferredUUID, payload)
			if err == nil {
				if err := expectOPNsenseResult(setResp, "saved", "ok"); err == nil {
					return preferredUUID, false, nil
				}
			}
		}
	}

	// Search for existing rule by description.
	searchResp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{})
	if err == nil {
		for _, raw := range asSlice(searchResp["rows"]) {
			row := asMap(raw)
			if asString(row["description"]) == description {
				uuid := asString(row["uuid"])
				if uuid != "" {
					setResp, err := api.Post(ctx, "/api/firewall/filter/set_rule/"+uuid, payload)
					if err == nil {
						if err := expectOPNsenseResult(setResp, "saved", "ok"); err == nil {
							return uuid, false, nil
						}
					}
				}
			}
		}
	}

	// Create new rule.
	resp, err := api.Post(ctx, "/api/firewall/filter/add_rule", payload)
	if err != nil {
		return "", false, err
	}
	if err := expectOPNsenseResult(resp, "saved", "ok"); err != nil {
		return "", false, err
	}

	uuid, err := extractUUID(resp)
	if err != nil {
		return "", false, err
	}

	return uuid, true, nil
}

// applyFirewallPending creates a savepoint and applies changes WITHOUT confirming.
// The 60-second auto-rollback timer starts ticking. The caller must confirm via
// cancel_rollback or let it expire to auto-revert.
// Returns the revision string (empty if savepoints are unsupported).
func applyFirewallPending(ctx context.Context, api *opnsenseAPIClient) (string, error) {
	spResp, err := api.Post(ctx, "/api/firewall/filter/savepoint", map[string]any{})
	if err != nil {
		// Savepoint not supported — apply directly and return empty revision.
		_, applyErr := api.Post(ctx, "/api/firewall/filter/apply", map[string]any{})
		return "", applyErr
	}

	revision := asString(spResp["revision"])
	if revision == "" {
		_, applyErr := api.Post(ctx, "/api/firewall/filter/apply", map[string]any{})
		return "", applyErr
	}

	_, err = api.Post(ctx, "/api/firewall/filter/apply/"+revision, map[string]any{})
	if err != nil {
		return "", err
	}

	return revision, nil
}

// confirmFirewall cancels the auto-rollback for a pending revision.
func confirmFirewall(ctx context.Context, api *opnsenseAPIClient, revision string) error {
	if revision == "" {
		return nil // Nothing to confirm — was a direct apply.
	}
	_, err := api.Post(ctx, "/api/firewall/filter/cancel_rollback/"+revision, map[string]any{})
	return err
}

// revertFirewall explicitly reverts to a pending revision.
func revertFirewall(ctx context.Context, api *opnsenseAPIClient, revision string) error {
	if revision == "" {
		return nil
	}
	_, err := api.Post(ctx, "/api/firewall/filter/revert/"+revision, map[string]any{})
	return err
}

// applyFirewallWithRollback creates a savepoint, applies firewall changes, then confirms.
// This is the safe apply pattern recommended by OPNsense docs.
func applyFirewallWithRollback(ctx context.Context, api *opnsenseAPIClient) error {
	// Create savepoint.
	spResp, err := api.Post(ctx, "/api/firewall/filter/savepoint", map[string]any{})
	if err != nil {
		// If savepoint fails, try applying directly (older OPNsense).
		_, applyErr := api.Post(ctx, "/api/firewall/filter/apply", map[string]any{})
		return applyErr
	}

	revision := asString(spResp["revision"])
	if revision == "" {
		// No revision returned, apply directly.
		_, applyErr := api.Post(ctx, "/api/firewall/filter/apply", map[string]any{})
		return applyErr
	}

	// Apply with rollback timer.
	_, err = api.Post(ctx, "/api/firewall/filter/apply/"+revision, map[string]any{})
	if err != nil {
		return err
	}

	// Confirm (cancel rollback) — if we can still reach the API, the rules are fine.
	// Retry once on failure; if confirmation never lands OPNsense will auto-revert
	// the ruleset after ~60 s, silently desynchronising Gator's DB state.
	if _, err := api.Post(ctx, "/api/firewall/filter/cancel_rollback/"+revision, map[string]any{}); err != nil {
		log.Printf("[warn] cancel_rollback failed (attempt 1): %v — retrying", err)
		if _, err2 := api.Post(ctx, "/api/firewall/filter/cancel_rollback/"+revision, map[string]any{}); err2 != nil {
			log.Printf("[error] cancel_rollback failed (attempt 2): %v — OPNsense may auto-revert in ~60s", err2)
			return err2
		}
	}

	return nil
}
