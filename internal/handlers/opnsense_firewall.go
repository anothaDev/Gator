package handlers

import (
	"context"
	"encoding/xml"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anothaDev/gator/internal/models"
	"github.com/gin-gonic/gin"
)

// ─── Pending Revision Tracker ──────────────────────────────────
//
// When EnableAppRoute / DisableAppRoute apply firewall changes with
// applyFirewallPending(), the changes are "pending" until the user
// confirms or the 60-second OPNsense timer auto-reverts.
//
// This tracker records what DB state existed before each pending change
// so that on revert (explicit or auto), we can undo the DB writes.

// pendingAppRouteChange records the pre-change state of a single app route.
type pendingAppRouteChange struct {
	VpnID    int64
	AppID    string
	OldRoute *models.AppRoute // nil if the route didn't exist before
}

// pendingRevisionInfo holds all changes associated with a single revision.
type pendingRevisionInfo struct {
	Changes   []pendingAppRouteChange
	CreatedAt time.Time
}

var (
	pendingRevisions   = make(map[string]*pendingRevisionInfo) // revision -> info
	pendingRevisionsMu sync.Mutex
)

// registerPendingChange records a pre-change snapshot for a revision.
// If the revision is empty (no savepoint support), this is a no-op.
func registerPendingChange(revision string, vpnID int64, appID string, oldRoute *models.AppRoute) {
	if revision == "" {
		return
	}
	pendingRevisionsMu.Lock()
	defer pendingRevisionsMu.Unlock()
	info, ok := pendingRevisions[revision]
	if !ok {
		info = &pendingRevisionInfo{CreatedAt: time.Now()}
		pendingRevisions[revision] = info
	}
	info.Changes = append(info.Changes, pendingAppRouteChange{
		VpnID:    vpnID,
		AppID:    appID,
		OldRoute: oldRoute,
	})
}

// clearPendingRevision removes a revision from the tracker (on confirm).
func clearPendingRevision(revision string) {
	if revision == "" {
		return
	}
	pendingRevisionsMu.Lock()
	defer pendingRevisionsMu.Unlock()
	delete(pendingRevisions, revision)
}

// popPendingRevision removes and returns the pending info for a revision.
func popPendingRevision(revision string) *pendingRevisionInfo {
	if revision == "" {
		return nil
	}
	pendingRevisionsMu.Lock()
	defer pendingRevisionsMu.Unlock()
	info := pendingRevisions[revision]
	delete(pendingRevisions, revision)
	return info
}

// getPendingRevision returns the pending info without removing it.
func getPendingRevision() (string, *pendingRevisionInfo) {
	pendingRevisionsMu.Lock()
	defer pendingRevisionsMu.Unlock()
	for rev, info := range pendingRevisions {
		return rev, info
	}
	return "", nil
}

// appRouteWriter is the minimal interface for undoing app route DB changes.
type appRouteWriter interface {
	UpsertAppRoute(ctx context.Context, r models.AppRoute) error
}

// revertPendingDBChanges restores DB state for all changes in a pending revision.
func revertPendingDBChanges(ctx context.Context, store appRouteWriter, info *pendingRevisionInfo) {
	for _, ch := range info.Changes {
		if ch.OldRoute != nil {
			// Restore the previous state.
			if err := store.UpsertAppRoute(ctx, *ch.OldRoute); err != nil {
				log.Printf("[RevertPending] failed to restore app route %s for vpn %d: %v", ch.AppID, ch.VpnID, err)
			}
		} else {
			// Route didn't exist before — set it back to disabled/empty.
			if err := store.UpsertAppRoute(ctx, models.AppRoute{
				VPNConfigID:       ch.VpnID,
				AppID:             ch.AppID,
				Enabled:           false,
				OPNsenseRuleUUIDs: "",
			}); err != nil {
				log.Printf("[RevertPending] failed to clear app route %s for vpn %d: %v", ch.AppID, ch.VpnID, err)
			}
		}
	}
}

// StartPendingRevisionCleanup starts a background goroutine that expires
// stale pending revisions (auto-reverted by OPNsense after 60s).
// The store is needed to undo DB changes.
// StartPendingRevisionCleanup runs a background goroutine that cleans up
// expired pending revisions. The returned stop function signals it to exit.
func StartPendingRevisionCleanup(store appRouteWriter) (stop func()) {
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
			case <-stopCh:
				return
			}

			pendingRevisionsMu.Lock()
			var expired []string
			for rev, info := range pendingRevisions {
				if time.Since(info.CreatedAt) > 70*time.Second {
					expired = append(expired, rev)
				}
			}
			// Pop expired outside the loop to avoid map mutation issues.
			expiredInfos := make(map[string]*pendingRevisionInfo, len(expired))
			for _, rev := range expired {
				expiredInfos[rev] = pendingRevisions[rev]
				delete(pendingRevisions, rev)
			}
			pendingRevisionsMu.Unlock()

			// Undo DB state for expired revisions (OPNsense auto-reverted).
			if len(expiredInfos) > 0 {
				ctx := context.Background()
				for rev, info := range expiredInfos {
					log.Printf("[PendingCleanup] revision %s expired (OPNsense auto-reverted), undoing %d DB changes", rev, len(info.Changes))
					revertPendingDBChanges(ctx, store, info)
				}
			}
		}
	}()
	return func() { close(stopCh) }
}

// isGatorDescription returns true if a rule description indicates it's managed by Gator.
func isGatorDescription(description string) bool {
	return strings.HasPrefix(description, "GATOR_") || strings.HasPrefix(description, "Gator ")
}

// verifyGatorFilterRule fetches a filter rule by UUID and returns true if it belongs to Gator.
// Returns (isGator, error). On network/API error, returns (false, err).
// If the rule doesn't exist (deleted externally), returns (false, nil).
func verifyGatorFilterRule(ctx context.Context, api *opnsenseAPIClient, uuid string) (bool, error) {
	resp, err := api.Get(ctx, "/api/firewall/filter/get_rule/"+uuid)
	if err != nil {
		return false, err
	}
	ruleData := asMap(resp["rule"])
	if len(ruleData) == 0 {
		return false, nil // Rule doesn't exist.
	}
	description := asString(ruleData["description"])
	return isGatorDescription(description), nil
}

// safeToggleFilterRule toggles a filter rule only if it belongs to Gator.
// Returns nil on success (including when the rule is not a Gator rule — skipped silently).
func safeToggleFilterRule(ctx context.Context, api *opnsenseAPIClient, uuid string, enabled string) error {
	ok, err := verifyGatorFilterRule(ctx, api, uuid)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("[safeToggle] skipping non-Gator filter rule %s", uuid)
		return nil
	}
	_, err = api.Post(ctx, "/api/firewall/filter/toggle_rule/"+uuid+"/"+enabled, map[string]any{})
	return err
}

// safeSetFilterRuleGateway changes the gateway on a Gator filter rule.
// Used instead of toggle for activate/deactivate — disabling a rule with a
// non-default destination (e.g. !RFC1918_Networks) would block traffic entirely
// since there'd be no pass rule left. Switching the gateway to "" (default)
// keeps the rule passing traffic but through WAN instead of VPN.
func safeSetFilterRuleGateway(ctx context.Context, api *opnsenseAPIClient, uuid string, gatewayKey string) error {
	resp, err := api.Get(ctx, "/api/firewall/filter/get_rule/"+uuid)
	if err != nil {
		return err
	}
	ruleData := asMap(resp["rule"])
	if len(ruleData) == 0 {
		return nil // Rule doesn't exist.
	}
	description := extractSelectedValue(ruleData["description"])
	if !isGatorDescription(description) {
		log.Printf("[safeSetGateway] skipping non-Gator filter rule %s", uuid)
		return nil
	}

	setResp, err := api.Post(ctx, "/api/firewall/filter/set_rule/"+uuid, map[string]any{
		"rule": map[string]any{
			"gateway": gatewayKey,
		},
	})
	if err != nil {
		return err
	}
	return expectOPNsenseResult(setResp, "saved", "ok")
}

// safeDeleteFilterRule deletes a filter rule only if it belongs to Gator.
// Returns (deleted bool, error). deleted=false means skipped (not Gator) or doesn't exist.
func safeDeleteFilterRule(ctx context.Context, api *opnsenseAPIClient, uuid string) (bool, error) {
	ok, err := verifyGatorFilterRule(ctx, api, uuid)
	if err != nil {
		return false, err
	}
	if !ok {
		log.Printf("[safeDelete] skipping non-Gator filter rule %s", uuid)
		return false, nil
	}
	resp, err := api.Post(ctx, "/api/firewall/filter/del_rule/"+uuid, map[string]any{})
	if err != nil {
		return false, err
	}
	if err := expectOPNsenseResult(resp, "deleted", "ok", ""); err != nil {
		return false, err
	}
	return true, nil
}

// safeDeleteSNATRule deletes a SNAT rule only if its description indicates Gator ownership.
func safeDeleteSNATRule(ctx context.Context, api *opnsenseAPIClient, uuid string) (bool, error) {
	resp, err := api.Get(ctx, "/api/firewall/source_nat/get_rule/"+uuid)
	if err != nil {
		return false, err
	}
	ruleData := asMap(resp["rule"])
	if len(ruleData) == 0 {
		return false, nil // Doesn't exist.
	}
	description := asString(ruleData["description"])
	if !isGatorDescription(description) {
		log.Printf("[safeDelete] skipping non-Gator SNAT rule %s (desc: %q)", uuid, description)
		return false, nil
	}
	delResp, err := api.Post(ctx, "/api/firewall/source_nat/del_rule/"+uuid, map[string]any{})
	if err != nil {
		return false, err
	}
	if err := expectOPNsenseResult(delResp, "deleted", "ok", ""); err != nil {
		return false, err
	}
	return true, nil
}

// safeMoveFilterRuleBefore moves a rule only if both the rule and the anchor belong to Gator.
func safeMoveFilterRuleBefore(ctx context.Context, api *opnsenseAPIClient, ruleUUID, anchorUUID string) {
	ruleOK, _ := verifyGatorFilterRule(ctx, api, ruleUUID)
	anchorOK, _ := verifyGatorFilterRule(ctx, api, anchorUUID)
	if !ruleOK || !anchorOK {
		log.Printf("[safeMove] skipping move — rule %s (gator=%v) before anchor %s (gator=%v)", ruleUUID, ruleOK, anchorUUID, anchorOK)
		return
	}
	_, _ = api.Post(ctx, "/api/firewall/filter/move_rule_before/"+ruleUUID+"/"+anchorUUID, map[string]any{})
}

// PendingFirewall returns the current pending revision (if any) so the
// frontend can resume its confirmation countdown after navigation.
func (h *GatewayHandler) PendingFirewall(c *gin.Context) {
	rev, info := getPendingRevision()
	if rev == "" || info == nil {
		c.JSON(http.StatusOK, gin.H{"pending": false})
		return
	}
	elapsed := time.Since(info.CreatedAt).Seconds()
	remaining := 60.0 - elapsed
	if remaining < 0 {
		remaining = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"pending":           true,
		"revision":          rev,
		"remaining_seconds": int(remaining),
	})
}

// ConfirmFirewall cancels the auto-rollback for a pending revision.
func (h *GatewayHandler) ConfirmFirewall(c *gin.Context) {
	ctx := c.Request.Context()

	var body struct {
		Revision string `json:"revision"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Revision == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "revision is required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)
	if err := confirmFirewall(ctx, api, body.Revision); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to confirm: " + err.Error()})
		return
	}

	// DB state is correct — just clear the pending tracker.
	clearPendingRevision(body.Revision)

	c.JSON(http.StatusOK, gin.H{"status": "confirmed", "revision": body.Revision})
}

// RevertFirewall explicitly reverts a pending firewall revision.
func (h *GatewayHandler) RevertFirewall(c *gin.Context) {
	ctx := c.Request.Context()

	var body struct {
		Revision string `json:"revision"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Revision == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "revision is required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)
	if err := revertFirewall(ctx, api, body.Revision); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to revert: " + err.Error()})
		return
	}

	// Undo any DB changes that were made for this revision.
	if info := popPendingRevision(body.Revision); info != nil {
		log.Printf("[RevertFirewall] undoing %d pending app route changes for revision %s", len(info.Changes), body.Revision)
		revertPendingDBChanges(ctx, h.store, info)
	}

	c.JSON(http.StatusOK, gin.H{"status": "reverted", "revision": body.Revision})
}

// DeleteFilterRule deletes a single automation filter rule from OPNsense.
// Only Gator-managed rules (description starts with "GATOR_" or "Gator ") can be deleted.
func (h *GatewayHandler) DeleteFilterRule(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule deletion requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Verify and delete only if Gator-owned.
	deleted, err := safeDeleteFilterRule(ctx, api, uuid)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete rule: " + err.Error()})
		return
	}
	if !deleted {
		c.JSON(http.StatusForbidden, gin.H{"error": "only Gator-managed rules can be deleted from this interface"})
		return
	}

	// Apply firewall changes.
	if err := applyFirewallWithRollback(ctx, api); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "deleted", "warning": "deleted but failed to apply firewall: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ListAliases returns all firewall aliases from OPNsense.
// Read-only — no mutations.
func (h *GatewayHandler) ListAliases(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias listing requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Post(ctx, "/api/firewall/alias/search_item", map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list aliases: " + err.Error()})
		return
	}

	rows := asSlice(resp["rows"])

	aliases := make([]gin.H, 0, len(rows))
	for _, raw := range rows {
		row := asMap(raw)
		if len(row) == 0 {
			continue
		}

		name := asString(row["name"])
		description := asString(row["description"])
		isGator := strings.HasPrefix(name, "GATOR_") || strings.Contains(description, "Gator")

		enabled := asString(row["enabled"])
		if enabled == "" {
			enabled = "1"
		}

		aliases = append(aliases, gin.H{
			"uuid":        asString(row["uuid"]),
			"enabled":     enabled,
			"name":        name,
			"type":        asString(row["type"]),
			"content":     asString(row["content"]),
			"description": description,
			"is_gator":    isGator,
		})
	}

	c.JSON(http.StatusOK, gin.H{"aliases": aliases})
}

// ListNATRules returns all outbound NAT (source NAT) rules from OPNsense.
// Read-only — no mutations.
func (h *GatewayHandler) ListNATRules(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NAT listing requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Post(ctx, "/api/firewall/source_nat/search_rule", map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list NAT rules: " + err.Error()})
		return
	}

	rows := asSlice(resp["rows"])

	rules := make([]gin.H, 0, len(rows))
	for _, raw := range rows {
		row := asMap(raw)
		if len(row) == 0 {
			continue
		}

		description := asString(row["description"])
		isGator := strings.HasPrefix(description, "GATOR_") || strings.HasPrefix(description, "Gator ")

		enabled := asString(row["enabled"])
		if enabled == "" {
			enabled = "1"
		}

		rules = append(rules, gin.H{
			"uuid":        asString(row["uuid"]),
			"enabled":     enabled,
			"interface":   asString(row["interface"]),
			"source_net":  asString(row["source_net"]),
			"destination": asString(row["destination_net"]),
			"protocol":    asString(row["protocol"]),
			"target":      asString(row["target"]),
			"description": description,
			"is_gator":    isGator,
		})
	}

	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// ListFilterRules returns all automation filter rules from OPNsense.
// Read-only — no mutations.
func (h *GatewayHandler) ListFilterRules(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rules listing requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list filter rules: " + err.Error()})
		return
	}

	rows := asSlice(resp["rows"])

	rules := make([]gin.H, 0, len(rows))
	for _, raw := range rows {
		row := asMap(raw)
		if len(row) == 0 {
			continue
		}

		description := asString(row["description"])
		isGator := strings.HasPrefix(description, "GATOR_") || strings.HasPrefix(description, "Gator ")

		enabled := asString(row["enabled"])
		if enabled == "" {
			enabled = "1"
		}

		rules = append(rules, gin.H{
			"uuid":             asString(row["uuid"]),
			"enabled":          enabled,
			"action":           asString(row["action"]),
			"quick":            asString(row["quick"]),
			"interface":        asString(row["interface"]),
			"direction":        asString(row["direction"]),
			"ipprotocol":       asString(row["ipprotocol"]),
			"protocol":         asString(row["protocol"]),
			"source_net":       asString(row["source_net"]),
			"source_not":       asString(row["source_not"]),
			"destination_net":  asString(row["destination_net"]),
			"destination_not":  asString(row["destination_not"]),
			"destination_port": asString(row["destination_port"]),
			"gateway":          asString(row["gateway"]),
			"description":      description,
			"is_gator":         isGator,
		})
	}

	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// DetectConflicts scans existing filter rules and reports potential conflicts with Gator deployment.
// POST body: { "interfaces": ["lan", "opt2"] }
// Returns: { "conflicts": [...], "has_gateway_rules": bool, "recommendation": "selective" | "all" }
func (h *GatewayHandler) DetectConflicts(c *gin.Context) {
	ctx := c.Request.Context()

	var body struct {
		Interfaces []string `json:"interfaces"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Interfaces) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "interfaces list required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list filter rules: " + err.Error()})
		return
	}

	// Build a set of selected interfaces for fast lookup.
	ifaceSet := make(map[string]bool, len(body.Interfaces))
	for _, iface := range body.Interfaces {
		ifaceSet[iface] = true
	}

	type conflict struct {
		UUID        string `json:"uuid"`
		Interface   string `json:"interface"`
		Source      string `json:"source"`
		Destination string `json:"destination"`
		Gateway     string `json:"gateway"`
		Description string `json:"description"`
	}

	var conflicts []conflict
	hasGatewayRules := false

	for _, raw := range asSlice(resp["rows"]) {
		row := asMap(raw)
		description := asString(row["description"])

		// Skip Gator-managed rules.
		if isGatorDescription(description) {
			continue
		}

		gateway := asString(row["gateway"])
		if gateway == "" || gateway == "default" {
			continue // No custom gateway — not a VPN/policy route conflict.
		}

		ruleIface := asString(row["interface"])
		// Check if this rule's interface overlaps with any selected source interfaces.
		overlaps := false
		for _, part := range strings.Split(ruleIface, ",") {
			part = strings.TrimSpace(part)
			if ifaceSet[part] {
				overlaps = true
				break
			}
		}
		if !overlaps {
			continue
		}

		hasGatewayRules = true
		conflicts = append(conflicts, conflict{
			UUID:        asString(row["uuid"]),
			Interface:   ruleIface,
			Source:      asString(row["source_net"]),
			Destination: asString(row["destination_net"]),
			Gateway:     gateway,
			Description: description,
		})
	}

	recommendation := "all"
	if hasGatewayRules {
		recommendation = "selective"
	}

	c.JSON(http.StatusOK, gin.H{
		"conflicts":         conflicts,
		"has_gateway_rules": hasGatewayRules,
		"recommendation":    recommendation,
	})
}

// --- Outbound NAT Mode Detection ---

// opnsenseConfigXML is the minimal subset of config.xml we need to parse.
type opnsenseConfigXML struct {
	XMLName xml.Name `xml:"opnsense"`
	NAT     struct {
		Outbound struct {
			Mode string `xml:"mode"`
		} `xml:"outbound"`
	} `xml:"nat"`
}

// GetNATMode downloads the OPNsense config.xml and extracts the outbound NAT mode.
// Returns one of: "automatic", "hybrid", "advanced" (manual), "disabled", or "unknown".
func (h *GatewayHandler) GetNATMode(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Use a longer timeout — config.xml can be large and slow to download.
	dlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	raw, err := api.GetRaw(dlCtx, "/api/core/backup/download/this")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to download config: " + err.Error()})
		return
	}

	var cfg opnsenseConfigXML
	if err := xml.Unmarshal(raw, &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse config.xml: " + err.Error()})
		return
	}

	mode := cfg.NAT.Outbound.Mode
	if mode == "" {
		// OPNsense defaults to "automatic" when the tag is missing.
		mode = "automatic"
	}

	// Describe what Gator needs.
	ok := mode == "hybrid" || mode == "advanced"
	message := ""
	if !ok {
		switch mode {
		case "automatic":
			message = "Outbound NAT is set to Automatic. Manual SNAT rules (required for VPN tunnels) are ignored in this mode. Switch to Hybrid in OPNsense: Firewall > NAT > Outbound."
		case "disabled":
			message = "Outbound NAT is disabled. VPN routing requires NAT. Switch to Hybrid in OPNsense: Firewall > NAT > Outbound."
		default:
			message = "Unknown outbound NAT mode. Verify settings in OPNsense: Firewall > NAT > Outbound."
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"mode":       mode,
		"compatible": ok,
		"message":    message,
	})
}

// --- Post-deploy stale rule cleanup ---

// FindStaleRules finds non-Gator filter rules that use the same gateway as a Gator VPN deployment.
// These are typically migrated legacy rules that are now redundant.
// POST body: { "gateway_name": "MULLVAD_GW", "interfaces": ["opt5"] }
func (h *GatewayHandler) FindStaleRules(c *gin.Context) {
	ctx := c.Request.Context()

	var body struct {
		GatewayName string   `json:"gateway_name"`
		Interfaces  []string `json:"interfaces"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.GatewayName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway_name required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to list filter rules: " + err.Error()})
		return
	}

	ifaceSet := make(map[string]bool, len(body.Interfaces))
	for _, iface := range body.Interfaces {
		ifaceSet[iface] = true
	}

	type staleRule struct {
		UUID            string `json:"uuid"`
		Interface       string `json:"interface"`
		Action          string `json:"action"`
		Quick           string `json:"quick"`
		Direction       string `json:"direction"`
		IPProtocol      string `json:"ipprotocol"`
		Protocol        string `json:"protocol"`
		Source          string `json:"source"`
		Destination     string `json:"destination"`
		DestinationPort string `json:"destination_port"`
		Gateway         string `json:"gateway"`
		Description     string `json:"description"`
		Enabled         bool   `json:"enabled"`
	}

	var stale []staleRule
	for _, raw := range asSlice(resp["rows"]) {
		row := asMap(raw)
		description := asString(row["description"])

		// Skip Gator-managed rules — those are ours.
		if isGatorDescription(description) {
			continue
		}

		gateway := asString(row["gateway"])
		if gateway != body.GatewayName {
			continue
		}

		// Check interface overlap.
		ruleIface := asString(row["interface"])
		overlaps := len(body.Interfaces) == 0 // If no interfaces specified, match all.
		for _, part := range strings.Split(ruleIface, ",") {
			if ifaceSet[strings.TrimSpace(part)] {
				overlaps = true
				break
			}
		}
		if !overlaps {
			continue
		}

		stale = append(stale, staleRule{
			UUID:            asString(row["uuid"]),
			Interface:       ruleIface,
			Action:          asString(row["action"]),
			Quick:           asString(row["quick"]),
			Direction:       asString(row["direction"]),
			IPProtocol:      asString(row["ipprotocol"]),
			Protocol:        asString(row["protocol"]),
			Source:          asString(row["source_net"]),
			Destination:     asString(row["destination_net"]),
			DestinationPort: asString(row["destination_port"]),
			Gateway:         gateway,
			Description:     description,
			Enabled:         asString(row["enabled"]) == "1",
		})
	}

	c.JSON(http.StatusOK, gin.H{"stale_rules": stale})
}

// DeleteNonGatorRule deletes a specific filter rule that is NOT owned by Gator.
// This is used for post-deploy cleanup of migrated legacy rules.
// The caller explicitly confirms they want to remove this rule.
func (h *GatewayHandler) DeleteNonGatorRule(c *gin.Context) {
	ctx := c.Request.Context()
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UUID required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Verify the rule exists and is NOT Gator-owned (safety: don't accidentally delete our own rules via this path).
	ruleResp, err := api.Get(ctx, "/api/firewall/filter/get_rule/"+uuid)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch rule: " + err.Error()})
		return
	}
	rule := asMap(ruleResp["rule"])
	description := asString(rule["description"])
	if isGatorDescription(description) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refusing to delete a Gator-managed rule via this endpoint"})
		return
	}

	// Delete the rule.
	_, err = api.Post(ctx, "/api/firewall/filter/del_rule/"+uuid, map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete rule: " + err.Error()})
		return
	}

	// Apply firewall.
	if err := applyFirewallWithRollback(ctx, api); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "deleted but failed to apply firewall: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "uuid": uuid})
}

// AdoptStaleRule reads all fields from a non-Gator "stale" rule, patches them
// into an existing Gator filter rule (preserving Gator's description, enabled
// state, and gateway), then deletes the stale rule and applies the firewall.
//
// This lets users migrate hand-crafted routing logic (e.g. destination aliases
// like RFC1918_Networks, protocol restrictions, port filters, inversions) into
// the Gator-managed rule without losing any detail.
//
// POST body: { "stale_uuid": "...", "gator_uuid": "..." }
func (h *GatewayHandler) AdoptStaleRule(c *gin.Context) {
	ctx := c.Request.Context()

	var body struct {
		StaleUUID string `json:"stale_uuid" binding:"required"`
		GatorUUID string `json:"gator_uuid" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stale_uuid and gator_uuid required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// 1. Verify the Gator rule exists and IS Gator-owned.
	gatorResp, err := api.Get(ctx, "/api/firewall/filter/get_rule/"+body.GatorUUID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch Gator rule: " + err.Error()})
		return
	}
	gatorRule := asMap(gatorResp["rule"])
	if len(gatorRule) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gator rule not found"})
		return
	}
	if !isGatorDescription(extractSelectedValue(gatorRule["description"])) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target rule is not Gator-managed"})
		return
	}

	// 2. Fetch the stale rule's full detail via get_rule.
	staleResp, err := api.Get(ctx, "/api/firewall/filter/get_rule/"+body.StaleUUID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch stale rule: " + err.Error()})
		return
	}
	staleRule := asMap(staleResp["rule"])
	if len(staleRule) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "stale rule not found"})
		return
	}
	if isGatorDescription(extractSelectedValue(staleRule["description"])) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refusing to adopt a Gator-managed rule"})
		return
	}

	// 3. Extract all routing-relevant fields from the stale rule.
	//    get_rule returns selected-items maps for multi-select fields,
	//    so we use extractSelectedValue which handles both plain strings and maps.
	//
	//    We preserve the Gator rule's: description, gateway, enabled, interface.
	//    Everything else comes from the stale rule.
	adoptedFields := map[string]any{
		"action":           extractSelectedValue(staleRule["action"]),
		"quick":            extractSelectedValue(staleRule["quick"]),
		"direction":        extractSelectedValue(staleRule["direction"]),
		"ipprotocol":       extractSelectedValue(staleRule["ipprotocol"]),
		"protocol":         extractSelectedValue(staleRule["protocol"]),
		"source_net":       extractSelectedValue(staleRule["source_net"]),
		"source_not":       extractSelectedValue(staleRule["source_not"]),
		"source_port":      extractSelectedValue(staleRule["source_port"]),
		"destination_net":  extractSelectedValue(staleRule["destination_net"]),
		"destination_not":  extractSelectedValue(staleRule["destination_not"]),
		"destination_port": extractSelectedValue(staleRule["destination_port"]),
		"log":              extractSelectedValue(staleRule["log"]),
	}

	// Preserve Gator-owned fields from the existing Gator rule.
	adoptedFields["enabled"] = extractSelectedValue(gatorRule["enabled"])
	adoptedFields["description"] = extractSelectedValue(gatorRule["description"])
	adoptedFields["gateway"] = extractSelectedValue(gatorRule["gateway"])
	adoptedFields["interface"] = extractSelectedValue(gatorRule["interface"])

	// 4. Patch the Gator rule.
	setResp, err := api.Post(ctx, "/api/firewall/filter/set_rule/"+body.GatorUUID, map[string]any{
		"rule": adoptedFields,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to update Gator rule: " + err.Error()})
		return
	}
	if err := expectOPNsenseResult(setResp, "saved", "ok"); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "OPNsense rejected the rule update: " + err.Error()})
		return
	}

	// 5. Delete the stale rule.
	_, err = api.Post(ctx, "/api/firewall/filter/del_rule/"+body.StaleUUID, map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Gator rule updated but failed to delete stale rule: " + err.Error()})
		return
	}

	// 6. Apply firewall.
	if err := applyFirewallWithRollback(ctx, api); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "rules updated but failed to apply firewall: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "adopted",
		"stale_uuid": body.StaleUUID,
		"gator_uuid": body.GatorUUID,
		"adopted_fields": gin.H{
			"source_net":       adoptedFields["source_net"],
			"source_not":       adoptedFields["source_not"],
			"destination_net":  adoptedFields["destination_net"],
			"destination_not":  adoptedFields["destination_not"],
			"protocol":         adoptedFields["protocol"],
			"destination_port": adoptedFields["destination_port"],
		},
	})
}

// adoptedRuleInfo describes a stale rule that was auto-adopted during deployment.
type adoptedRuleInfo struct {
	UUID        string `json:"uuid"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Protocol    string `json:"protocol"`
}

// autoAdoptStaleRules scans for non-Gator filter rules on the same gateway and
// interfaces, reads their full details, patches those fields into the Gator rule,
// then deletes the stale rules.
//
// It adopts the FIRST matching stale rule's fields (source, destination, protocol,
// inversions, ports, etc.) into the Gator rule. Additional stale rules on the
// same gateway are deleted — the assumption is that the first match carries the
// user's intended routing logic. Returns info about what was adopted.
//
// This is best-effort: failures are logged but do not block deployment.
func autoAdoptStaleRules(
	ctx context.Context,
	api *opnsenseAPIClient,
	gatorFilterUUID string,
	gatewayName string,
	sourceIfaces []string,
) []adoptedRuleInfo {
	if gatorFilterUUID == "" || gatewayName == "" {
		return nil
	}

	// Search all filter rules.
	searchResp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{})
	if err != nil {
		log.Printf("[autoAdopt] failed to search filter rules: %v", err)
		return nil
	}

	ifaceSet := make(map[string]bool, len(sourceIfaces))
	for _, iface := range sourceIfaces {
		ifaceSet[iface] = true
	}

	// Find stale rule UUIDs (non-Gator, same gateway, overlapping interfaces).
	var staleUUIDs []string
	for _, raw := range asSlice(searchResp["rows"]) {
		row := asMap(raw)
		uuid := asString(row["uuid"])
		if uuid == "" || uuid == gatorFilterUUID {
			continue
		}
		description := asString(row["description"])
		if isGatorDescription(description) {
			continue
		}
		if asString(row["gateway"]) != gatewayName {
			continue
		}
		ruleIface := asString(row["interface"])
		overlaps := false
		for _, part := range strings.Split(ruleIface, ",") {
			if ifaceSet[strings.TrimSpace(part)] {
				overlaps = true
				break
			}
		}
		if !overlaps {
			continue
		}
		staleUUIDs = append(staleUUIDs, uuid)
	}

	if len(staleUUIDs) == 0 {
		return nil
	}

	log.Printf("[autoAdopt] found %d stale rule(s) on gateway %s, adopting first into %s", len(staleUUIDs), gatewayName, gatorFilterUUID)

	var adopted []adoptedRuleInfo
	firstAdopted := false

	for _, staleUUID := range staleUUIDs {
		// Fetch full detail for the stale rule.
		detailResp, err := api.Get(ctx, "/api/firewall/filter/get_rule/"+staleUUID)
		if err != nil {
			log.Printf("[autoAdopt] failed to fetch stale rule %s: %v", staleUUID, err)
			continue
		}
		staleDetail := asMap(detailResp["rule"])
		if len(staleDetail) == 0 {
			continue
		}

		// get_rule returns selected-items maps for multi-select fields,
		// so extractSelectedValue handles both plain strings and maps.
		info := adoptedRuleInfo{
			UUID:        staleUUID,
			Description: extractSelectedValue(staleDetail["description"]),
			Source:      extractSelectedValue(staleDetail["source_net"]),
			Destination: extractSelectedValue(staleDetail["destination_net"]),
			Protocol:    extractSelectedValue(staleDetail["protocol"]),
		}

		// Adopt the first stale rule's fields into the Gator rule.
		if !firstAdopted {
			// Read the Gator rule to preserve its owned fields.
			gatorResp, err := api.Get(ctx, "/api/firewall/filter/get_rule/"+gatorFilterUUID)
			if err != nil {
				log.Printf("[autoAdopt] failed to fetch Gator rule %s: %v", gatorFilterUUID, err)
				break
			}
			gatorRule := asMap(gatorResp["rule"])
			if len(gatorRule) == 0 {
				break
			}

			// Build patch: take routing fields from stale, keep Gator-owned fields.
			// Use extractSelectedValue for all fields — get_rule returns selected-items
			// maps for fields like source_net, destination_net, protocol, etc.
			patch := map[string]any{
				// From stale rule:
				"action":           extractSelectedValue(staleDetail["action"]),
				"quick":            extractSelectedValue(staleDetail["quick"]),
				"direction":        extractSelectedValue(staleDetail["direction"]),
				"ipprotocol":       extractSelectedValue(staleDetail["ipprotocol"]),
				"protocol":         extractSelectedValue(staleDetail["protocol"]),
				"source_net":       extractSelectedValue(staleDetail["source_net"]),
				"source_not":       extractSelectedValue(staleDetail["source_not"]),
				"source_port":      extractSelectedValue(staleDetail["source_port"]),
				"destination_net":  extractSelectedValue(staleDetail["destination_net"]),
				"destination_not":  extractSelectedValue(staleDetail["destination_not"]),
				"destination_port": extractSelectedValue(staleDetail["destination_port"]),
				"log":              extractSelectedValue(staleDetail["log"]),
				// Preserved from Gator rule:
				"enabled":     extractSelectedValue(gatorRule["enabled"]),
				"description": extractSelectedValue(gatorRule["description"]),
				"gateway":     extractSelectedValue(gatorRule["gateway"]),
				"interface":   extractSelectedValue(gatorRule["interface"]),
			}

			setResp, err := api.Post(ctx, "/api/firewall/filter/set_rule/"+gatorFilterUUID, map[string]any{"rule": patch})
			if err != nil {
				log.Printf("[autoAdopt] failed to patch Gator rule with stale fields: %v", err)
			} else if err := expectOPNsenseResult(setResp, "saved", "ok"); err != nil {
				log.Printf("[autoAdopt] OPNsense rejected Gator rule patch: %v", err)
			} else {
				firstAdopted = true
				log.Printf("[autoAdopt] adopted fields from %s into Gator rule %s (dest=%s, src=%s, proto=%s)",
					staleUUID, gatorFilterUUID, info.Destination, info.Source, info.Protocol)
			}
		}

		// Delete the stale rule.
		_, err = api.Post(ctx, "/api/firewall/filter/del_rule/"+staleUUID, map[string]any{})
		if err != nil {
			log.Printf("[autoAdopt] failed to delete stale rule %s: %v", staleUUID, err)
		} else {
			log.Printf("[autoAdopt] deleted stale rule %s", staleUUID)
		}

		adopted = append(adopted, info)
	}

	return adopted
}
