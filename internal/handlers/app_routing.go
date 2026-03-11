package handlers

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/models"
)

// AppRoutingStore is the storage interface needed by AppRoutingHandler.
type AppRoutingStore interface {
	GetActiveInstanceID(ctx context.Context) (int64, error)
	GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error)
	GetVPNConfigByID(ctx context.Context, id int64) (*models.SimpleVPNConfig, error)
	SaveSimpleVPNConfig(ctx context.Context, cfg models.SimpleVPNConfig) error
	ListAppRoutes(ctx context.Context, vpnConfigID int64) ([]*models.AppRoute, error)
	GetAppRoute(ctx context.Context, vpnConfigID int64, appID string) (*models.AppRoute, error)
	UpsertAppRoute(ctx context.Context, r models.AppRoute) error
	ListCustomAppProfiles(ctx context.Context) ([]models.AppProfile, error)
	GetCustomAppProfile(ctx context.Context, id string) (*models.AppProfile, error)
	CreateCustomAppProfile(ctx context.Context, p models.AppProfile) error
	DeleteCustomAppProfile(ctx context.Context, id string) error
	ListAppRoutesByAppID(ctx context.Context, appID string) ([]*models.AppRoute, error)
	DeleteAppRoutesByAppID(ctx context.Context, appID string) error
	GetCache(ctx context.Context, key string) (string, error)
	SetCache(ctx context.Context, key, value string) error
}

// AppRoutingHandler serves the app profile + per-VPN routing endpoints.
type AppRoutingHandler struct {
	store AppRoutingStore
}

// NewAppRoutingHandler creates a new handler.
func NewAppRoutingHandler(store AppRoutingStore) *AppRoutingHandler {
	return &AppRoutingHandler{store: store}
}

// getVPNByParam reads the :id URL parameter, loads the VPN config, and verifies
// it belongs to the active instance. Returns nil, false if the lookup fails.
func (h *AppRoutingHandler) getVPNByParam(c *gin.Context) (*models.SimpleVPNConfig, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vpn id"})
		return nil, false
	}
	cfg, err := h.store.GetVPNConfigByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read vpn config"})
		return nil, false
	}
	if cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "vpn config not found"})
		return nil, false
	}
	activeID, _ := h.store.GetActiveInstanceID(c.Request.Context())
	if activeID != 0 && cfg.InstanceID != activeID {
		c.JSON(http.StatusNotFound, gin.H{"error": "vpn config not found"})
		return nil, false
	}
	return cfg, true
}

// ListProfiles returns the merged list of builtin + custom app profiles and presets.
func (h *AppRoutingHandler) ListProfiles(c *gin.Context) {
	ctx := c.Request.Context()

	custom, err := h.store.ListCustomAppProfiles(ctx)
	if err != nil {
		custom = nil // non-fatal
	}

	allApps := make([]models.AppProfile, 0, len(builtinAppProfiles)+len(custom))
	allApps = append(allApps, builtinAppProfiles...)
	allApps = append(allApps, custom...)

	c.JSON(http.StatusOK, gin.H{
		"apps":    allApps,
		"presets": builtinAppPresets,
	})
}

// CreateCustomProfile creates a user-defined app profile.
func (h *AppRoutingHandler) CreateCustomProfile(c *gin.Context) {
	ctx := c.Request.Context()

	var body struct {
		Name     string            `json:"name"`
		Category string            `json:"category"`
		Rules    []models.PortRule `json:"rules"`
		ASNs     []int             `json:"asns"`
		Note     string            `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if len(body.Rules) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one port rule is required"})
		return
	}

	// Validate rules.
	for _, r := range body.Rules {
		if r.Protocol == "" || r.Ports == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "each rule must have protocol and ports"})
			return
		}
	}

	// Generate a safe ID from the name.
	id := "custom_" + sanitizeID(body.Name)

	// Check for collisions with builtins.
	if appProfileByID(id) != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "a profile with this name already exists"})
		return
	}

	// Check for collisions with other custom profiles.
	existing, _ := h.store.GetCustomAppProfile(ctx, id)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "a custom profile with this name already exists"})
		return
	}

	category := body.Category
	if category == "" {
		category = "custom"
	}

	profile := models.AppProfile{
		ID:       id,
		Name:     body.Name,
		Category: category,
		Rules:    body.Rules,
		ASNs:     body.ASNs,
		Note:     body.Note,
		IsCustom: true,
	}

	if err := h.store.CreateCustomAppProfile(ctx, profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save custom profile: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "created", "profile": profile})
}

// DeleteCustomProfile removes a user-defined app profile and cleans up associated
// OPNsense filter rules and app routing rows.
func (h *AppRoutingHandler) DeleteCustomProfile(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("profileId")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile id is required"})
		return
	}

	// Only allow deleting custom profiles.
	existing, err := h.store.GetCustomAppProfile(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check profile"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "custom profile not found"})
		return
	}

	// Clean up OPNsense filter rules associated with this profile's app routes.
	var warnings []string
	routes, err := h.store.ListAppRoutesByAppID(ctx, id)
	if err != nil {
		warnings = append(warnings, "failed to list app routes for cleanup: "+err.Error())
	} else if len(routes) > 0 {
		firewallCfg, fwErr := h.store.GetFirewallConfig(ctx)
		if fwErr == nil && firewallCfg != nil && firewallCfg.Type == "opnsense" {
			api := newOPNsenseAPIClient(*firewallCfg)
			needApply := false
			for _, route := range routes {
				if route.OPNsenseRuleUUIDs == "" {
					continue
				}
				for _, uuid := range strings.Split(route.OPNsenseRuleUUIDs, ",") {
					uuid = strings.TrimSpace(uuid)
					if uuid == "" {
						continue
					}
					deleted, delErr := safeDeleteFilterRule(ctx, api, uuid)
					if delErr != nil {
						warnings = append(warnings, "failed to delete rule "+uuid+": "+delErr.Error())
					} else if deleted {
						needApply = true
					}
				}
			}
			if needApply {
				if err := applyFirewallWithRollback(ctx, api); err != nil {
					warnings = append(warnings, "failed to apply firewall after rule cleanup: "+err.Error())
				}
			}
		}
	}

	// Delete app routing rows referencing this profile.
	if err := h.store.DeleteAppRoutesByAppID(ctx, id); err != nil {
		warnings = append(warnings, "failed to delete app route rows: "+err.Error())
	}

	// Delete the profile itself.
	if err := h.store.DeleteCustomAppProfile(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete profile: " + err.Error()})
		return
	}

	resp := gin.H{"status": "deleted"}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	c.JSON(http.StatusOK, resp)
}

// resolveProfile looks up an app profile by ID, checking builtins first then custom profiles.
func (h *AppRoutingHandler) resolveProfile(ctx context.Context, id string) *models.AppProfile {
	if p := appProfileByID(id); p != nil {
		return p
	}
	p, _ := h.store.GetCustomAppProfile(ctx, id)
	return p
}

// sanitizeID converts a name to a safe lowercase ID.
func sanitizeID(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if len(result) > 40 {
		result = result[:40]
	}
	return result
}

// SetRoutingMode updates the routing mode for a VPN config.
func (h *AppRoutingHandler) SetRoutingMode(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}

	var body struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if body.Mode != "all" && body.Mode != "selective" && body.Mode != "bypass" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'all', 'selective', or 'bypass'"})
		return
	}

	// Toggle the catch-all policy rule on OPNsense based on the mode.
	// "selective" -> disable catch-all (only app-specific rules route through VPN)
	// "all" or "bypass" -> enable catch-all
	var revision string
	if vpnCfg.OPNsenseFilterUUIDs != "" {
		firewallCfg, err := h.store.GetFirewallConfig(ctx)
		if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
			return
		}

		api := newOPNsenseAPIClient(*firewallCfg)

		enabledState := "1" // enable catch-all
		if body.Mode == "selective" {
			enabledState = "0" // disable catch-all
		}

		for _, uuid := range splitUUIDs(vpnCfg.OPNsenseFilterUUIDs) {
			if err := safeToggleFilterRule(ctx, api, uuid, enabledState); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to toggle catch-all rule: " + err.Error()})
				return
			}
		}

		// Apply with pending confirmation so the user can verify connectivity.
		revision, err = applyFirewallPending(ctx, api)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "toggled rule but failed to apply firewall: " + err.Error()})
			return
		}
	}

	vpnCfg.RoutingMode = body.Mode
	if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save routing mode"})
		return
	}

	resp := gin.H{"status": "updated", "routing_mode": body.Mode}
	if revision != "" {
		resp["revision"] = revision
	}
	c.JSON(http.StatusOK, resp)
}

// ListAppRoutes returns which apps are enabled/applied for a given VPN.
func (h *AppRoutingHandler) ListAppRoutes(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}
	vpnID := vpnCfg.ID

	routes, err := h.store.ListAppRoutes(ctx, vpnID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list app routes"})
		return
	}

	routeMap := map[string]*models.AppRoute{}
	for _, r := range routes {
		routeMap[r.AppID] = r
	}

	// Merge builtin + custom profiles for status listing.
	allProfiles := make([]models.AppProfile, 0, len(builtinAppProfiles))
	allProfiles = append(allProfiles, builtinAppProfiles...)
	if custom, err := h.store.ListCustomAppProfiles(ctx); err == nil {
		allProfiles = append(allProfiles, custom...)
	}

	statuses := make([]models.AppRouteStatus, 0, len(allProfiles))
	for _, app := range allProfiles {
		status := models.AppRouteStatus{AppID: app.ID}
		if r, ok := routeMap[app.ID]; ok {
			status.Enabled = r.Enabled
			status.Applied = r.OPNsenseRuleUUIDs != ""
		}
		statuses = append(statuses, status)
	}

	routingMode := vpnCfg.RoutingMode
	if routingMode == "" {
		routingMode = "all"
	}

	c.JSON(http.StatusOK, gin.H{"routes": statuses, "routing_mode": routingMode})
}

// EnableAppRoute creates OPNsense filter rules for a specific app's ports through the VPN gateway.
func (h *AppRoutingHandler) EnableAppRoute(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}
	vpnID := vpnCfg.ID
	appID := c.Param("appId")

	profile := h.resolveProfile(ctx, appID)
	if profile == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown app profile: " + appID})
		return
	}
	if vpnCfg.OPNsenseGatewayName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VPN must have a gateway deployed before enabling app routing"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "app routing requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)
	ipProto := opnsenseIPProtocol(vpnCfg.IPVersion)

	// Determine gateway based on routing mode.
	// "bypass" mode: app rules use default route (empty gateway) to ESCAPE the VPN.
	// "selective" or "all" mode: app rules use VPN gateway to route THROUGH the VPN.
	routingMode := vpnCfg.RoutingMode
	if routingMode == "" {
		routingMode = "all"
	}
	gwName := vpnCfg.OPNsenseGatewayName
	if routingMode == "bypass" {
		gwName = "" // empty = default route, bypassing VPN
	}

	// Auto-switch from "all" to "selective" when enabling an app route.
	// In "all" mode the catch-all handles everything, making app rules redundant.
	// Switching to "selective" disables the catch-all so only app-specific rules route through VPN.
	modeChanged := false
	if routingMode == "all" {
		routingMode = "selective"
		modeChanged = true

		// Disable the catch-all policy rules (verified as Gator-owned).
		for _, uuid := range splitUUIDs(vpnCfg.OPNsenseFilterUUIDs) {
			if err := safeToggleFilterRule(ctx, api, uuid, "0"); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to disable catch-all rule: " + err.Error()})
				return
			}
		}
	}

	// Resolve to the key OPNsense actually accepts in the filter model.
	gwKey := resolveGatewayKey(ctx, api, gwName)

	// If the profile has ASNs, resolve them to CIDRs and create/update an OPNsense alias.
	// The alias name is used as destination_net in filter rules instead of "any".
	//
	// For profiles with a URLTableHint (large providers like Amazon), we use an OPNsense
	// URL Table alias pointing to a locally-hosted IP ranges file instead of a static
	// network alias. The user must upload the file first.
	destNet := "any"
	aliasName := aliasNameForApp(profile.ID)

	if profile.URLTableHint != nil {
		// Large provider — use URL table alias with locally-hosted file.
		hint := profile.URLTableHint
		if !ipRangesFileExists(hint.Filename) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":           "IP ranges file required for " + profile.Name,
				"needs_upload":    true,
				"download_url":    hint.DownloadURL,
				"upload_filename": hint.Filename,
				"description":     hint.Description,
			})
			return
		}

		// Determine the URL that OPNsense can reach Gator at.
		// Prefer GATOR_URL env var if set, otherwise derive from the
		// local address of the incoming request (avoids Host header injection).
		gatorBaseURL := os.Getenv("GATOR_URL")
		if gatorBaseURL == "" {
			// Use the server-side local address from the connection.
			if addr, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
				gatorBaseURL = "http://" + addr.String()
			} else {
				port := os.Getenv("PORT")
				if port == "" {
					port = "8080"
				}
				gatorBaseURL = "http://localhost:" + port
			}
		}

		_, _, aliasErr := ensureURLTableAlias(ctx, api, aliasName, gatorBaseURL, hint.Filename, hint.JQFilter)
		if aliasErr != nil {
			log.Printf("[EnableAppRoute] URL table alias creation failed for %s: %v, falling back to port-only", profile.ID, aliasErr)
		} else {
			if err := applyAliases(ctx, api); err != nil {
				log.Printf("[EnableAppRoute] alias reconfigure failed: %v", err)
			}
			destNet = aliasName
			log.Printf("[EnableAppRoute] using URL table alias for %s via %s", profile.ID, aliasName)
		}
	} else if len(profile.ASNs) > 0 {
		// Standard provider — resolve ASNs to CIDRs and create a network alias.
		cidrs, err := resolveMultiASNPrefixes(ctx, h.store, profile.ASNs)
		if err != nil || len(cidrs) == 0 {
			// Non-fatal: fall back to "any" if ASN resolution fails.
			if err != nil {
				log.Printf("[EnableAppRoute] ASN resolution failed for %s: %v, falling back to port-only", profile.ID, err)
			}
		} else {
			_, _, aliasErr := ensureAlias(ctx, api, aliasName, cidrs)
			if aliasErr != nil {
				log.Printf("[EnableAppRoute] alias creation failed for %s: %v, falling back to port-only", profile.ID, aliasErr)
			} else {
				if err := applyAliases(ctx, api); err != nil {
					log.Printf("[EnableAppRoute] alias reconfigure failed: %v", err)
				}
				destNet = aliasName
				log.Printf("[EnableAppRoute] using IP-based routing for %s via alias %s (%d CIDRs)", profile.ID, aliasName, len(cidrs))
			}
		}
	}

	// Consolidate port rules into at most 2 filter rules (TCP + UDP).
	// Group ports by protocol: collect all TCP ports, all UDP ports. TCP/UDP goes into both.
	// Ports are merged into a single comma-separated string per protocol.
	var tcpPorts, udpPorts []string
	for _, pr := range profile.Rules {
		switch strings.ToUpper(pr.Protocol) {
		case "TCP":
			tcpPorts = append(tcpPorts, pr.Ports)
		case "UDP":
			udpPorts = append(udpPorts, pr.Ports)
		case "TCP/UDP":
			tcpPorts = append(tcpPorts, pr.Ports)
			udpPorts = append(udpPorts, pr.Ports)
		}
	}

	// Create at most 2 filter rules: one for TCP, one for UDP.
	type ruleSpec struct {
		proto    string
		portExpr string
		desc     string
	}
	var specs []ruleSpec
	if len(tcpPorts) > 0 {
		specs = append(specs, ruleSpec{
			proto:    "TCP",
			portExpr: strings.Join(tcpPorts, ","),
			desc:     "GATOR_APP_" + profile.ID + "_TCP",
		})
	}
	if len(udpPorts) > 0 {
		specs = append(specs, ruleSpec{
			proto:    "UDP",
			portExpr: strings.Join(udpPorts, ","),
			desc:     "GATOR_APP_" + profile.ID + "_UDP",
		})
	}

	// Use the VPN's configured source interfaces (comma-separated for multi-interface filter rules).
	sourceIfaces := vpnCfg.SourceInterfaces
	if len(sourceIfaces) == 0 {
		sourceIfaces = []string{"lan"} // Backward compat default.
	}
	ifaceCSV := strings.Join(sourceIfaces, ",")

	var createdUUIDs []string
	for _, spec := range specs {
		payload := map[string]any{
			"rule": map[string]any{
				"enabled":          "1",
				"action":           "pass",
				"quick":            "1",
				"interface":        ifaceCSV,
				"direction":        "in",
				"ipprotocol":       ipProto,
				"protocol":         spec.proto,
				"source_net":       "any",
				"source_not":       "0",
				"destination_net":  destNet,
				"destination_not":  "0",
				"destination_port": spec.portExpr,
				"gateway":          gwKey,
				"log":              "0",
				"description":      spec.desc,
			},
		}

		// Check if rule already exists by description.
		uuid := findRuleByDescription(ctx, api, spec.desc)
		if uuid != "" {
			// Update existing rule.
			_, err := api.Post(ctx, "/api/firewall/filter/set_rule/"+uuid, payload)
			if err == nil {
				createdUUIDs = append(createdUUIDs, uuid)
				continue
			}
		}

		// Create new rule.
		resp, err := api.Post(ctx, "/api/firewall/filter/add_rule", payload)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create rule for " + spec.desc + ": " + err.Error()})
			return
		}

		newUUID, err := extractUUID(resp)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "rule created but no UUID returned for " + spec.desc})
			return
		}
		createdUUIDs = append(createdUUIDs, newUUID)
	}

	// Move app rules above the catch-all policy rules so they match first.
	// OPNsense uses first-match-wins with quick rules.
	// Both the app rule and anchor are verified as Gator-owned before reordering.
	catchAllUUIDs := splitUUIDs(vpnCfg.OPNsenseFilterUUIDs)
	if len(catchAllUUIDs) > 0 && len(createdUUIDs) > 0 {
		for _, ruleUUID := range createdUUIDs {
			safeMoveFilterRuleBefore(ctx, api, ruleUUID, catchAllUUIDs[0])
		}
	}

	// Snapshot the current DB state before changes — needed for rollback.
	oldRoute, _ := h.store.GetAppRoute(ctx, vpnID, appID)

	// Apply firewall changes with pending confirmation (60s auto-rollback).
	revision, err := applyFirewallPending(ctx, api)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "rules created but failed to apply firewall: " + err.Error()})
		return
	}

	// Persist the mode change if we auto-switched.
	if modeChanged {
		vpnCfg.RoutingMode = routingMode
		if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
			// Non-fatal: the rules are applied, mode will be stale in DB.
			_ = err
		}
	}

	// Save to database.
	route := models.AppRoute{
		VPNConfigID:       vpnID,
		AppID:             appID,
		Enabled:           true,
		OPNsenseRuleUUIDs: strings.Join(createdUUIDs, ","),
	}
	if err := h.store.UpsertAppRoute(ctx, route); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rules applied but failed to persist state"})
		return
	}

	// Register for rollback if the user reverts or OPNsense auto-reverts.
	registerPendingChange(revision, vpnID, appID, oldRoute)

	resp := gin.H{
		"status":         "enabled",
		"app_id":         appID,
		"rule_count":     len(createdUUIDs),
		"rule_uuids":     createdUUIDs,
		"revision":       revision,
		"has_ip_routing": destNet != "any",
	}
	if modeChanged {
		resp["routing_mode"] = routingMode
	}
	c.JSON(http.StatusOK, resp)
}

// DisableAppRoute removes/disables OPNsense filter rules for a specific app.
func (h *AppRoutingHandler) DisableAppRoute(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}
	vpnID := vpnCfg.ID
	appID := c.Param("appId")

	route, err := h.store.GetAppRoute(ctx, vpnID, appID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read app route"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "app routing requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Delete OPNsense rules if we have stored UUIDs (verified as Gator-owned).
	var warnings []string
	var revision string
	if route != nil && route.OPNsenseRuleUUIDs != "" {
		for _, uuid := range strings.Split(route.OPNsenseRuleUUIDs, ",") {
			uuid = strings.TrimSpace(uuid)
			if uuid == "" {
				continue
			}
			deleted, err := safeDeleteFilterRule(ctx, api, uuid)
			if err != nil {
				warnings = append(warnings, "failed to delete rule "+uuid+": "+err.Error())
			} else if !deleted {
				warnings = append(warnings, "skipped non-Gator rule "+uuid)
			}
		}

		// Apply firewall changes with pending confirmation.
		rev, err := applyFirewallPending(ctx, api)
		if err != nil {
			warnings = append(warnings, "failed to apply firewall after rule deletion: "+err.Error())
		}
		revision = rev
	}

	// Clean up IP-based routing alias for this app (if it had ASNs or URL table).
	profile := h.resolveProfile(ctx, appID)
	if profile != nil && (len(profile.ASNs) > 0 || profile.URLTableHint != nil) {
		aliasName := aliasNameForApp(appID)
		if deleteAlias(ctx, api, aliasName) {
			log.Printf("[DisableAppRoute] cleaned up IP alias %s for %s", aliasName, appID)
		}
	}

	// Update database.
	dbRoute := models.AppRoute{
		VPNConfigID:       vpnID,
		AppID:             appID,
		Enabled:           false,
		OPNsenseRuleUUIDs: "",
	}
	if err := h.store.UpsertAppRoute(ctx, dbRoute); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rules removed but failed to persist state"})
		return
	}

	// Register for rollback if the user reverts or OPNsense auto-reverts.
	registerPendingChange(revision, vpnID, appID, route)

	resp := gin.H{"status": "disabled", "app_id": appID, "revision": revision}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	c.JSON(http.StatusOK, resp)
}

// findRuleByDescription searches OPNsense filter rules for one matching the exact description.
func findRuleByDescription(ctx context.Context, api *opnsenseAPIClient, description string) string {
	resp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{})
	if err != nil {
		return ""
	}
	for _, raw := range asSlice(resp["rows"]) {
		row := asMap(raw)
		if asString(row["description"]) == description {
			return asString(row["uuid"])
		}
	}
	return ""
}
