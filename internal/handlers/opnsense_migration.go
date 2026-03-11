package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// MigrationStatus returns the current state of legacy vs MVC rules.
// GET /api/opnsense/migration/status
func (h *GatewayHandler) MigrationStatus(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Count MVC rules.
	mvcResp, err := api.Post(ctx, "/api/firewall/filter/searchRule", map[string]any{})
	var mvcCount int
	if err == nil {
		mvcCount = len(asSlice(mvcResp["rows"]))
	}

	// Try downloading legacy rules CSV to count them.
	legacyCSV, legacyErr := api.GetRaw(ctx, "/api/firewall/migration/download_rules")
	var legacyCount int
	var legacyAvailable bool
	if legacyErr == nil && len(legacyCSV) > 0 {
		legacyAvailable = true
		// Count non-empty lines (minus header).
		lines := strings.Split(strings.TrimSpace(string(legacyCSV)), "\n")
		if len(lines) > 1 {
			legacyCount = len(lines) - 1 // Subtract header row
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"legacy_available": legacyAvailable,
		"legacy_count":     legacyCount,
		"mvc_count":        mvcCount,
		"legacy_error":     errString(legacyErr),
	})
}

// MigrationDownload downloads the legacy rules as CSV and returns them.
// GET /api/opnsense/migration/download
func (h *GatewayHandler) MigrationDownload(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	csv, err := api.GetRaw(ctx, "/api/firewall/migration/download_rules")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to download legacy rules: " + err.Error()})
		return
	}

	if len(csv) == 0 {
		c.JSON(http.StatusOK, gin.H{"csv": "", "count": 0})
		return
	}

	lines := strings.Split(strings.TrimSpace(string(csv)), "\n")
	count := 0
	if len(lines) > 1 {
		count = len(lines) - 1
	}

	c.JSON(http.StatusOK, gin.H{
		"csv":   string(csv),
		"count": count,
	})
}

// MigrationUpload uploads legacy rules CSV to the new MVC filter system.
// POST /api/opnsense/migration/upload
func (h *GatewayHandler) MigrationUpload(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		CSV string `json:"csv"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.CSV == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CSV data required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Upload the CSV to the MVC filter system.
	// OPNsense's upload_rules expects the CSV as a JSON payload with a "payload" field.
	uploadPayload := map[string]any{
		"payload": req.CSV,
	}
	resp, err := api.Post(ctx, "/api/firewall/filter/upload_rules", uploadPayload)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to upload rules: " + err.Error()})
		return
	}

	// Check response for validation errors.
	status := asString(resp["status"])
	if status == "error" || status == "failed" {
		msg := asString(resp["message"])
		if msg == "" {
			// Try to extract validation errors.
			if validations, ok := resp["validations"]; ok {
				b, _ := json.Marshal(validations)
				msg = string(b)
			}
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "upload validation failed: " + msg, "details": resp})
		return
	}

	log.Printf("[Migration] rules uploaded successfully: %v", resp)

	c.JSON(http.StatusOK, gin.H{
		"status":  "uploaded",
		"details": resp,
	})
}

// MigrationApply applies the firewall after uploading rules, using savepoint for safety.
// POST /api/opnsense/migration/apply
func (h *GatewayHandler) MigrationApply(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Create a savepoint for rollback safety.
	spResp, err := api.Post(ctx, "/api/firewall/filter/savepoint", map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create savepoint: " + err.Error()})
		return
	}
	revision := asString(spResp["revision"])
	if revision == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "savepoint returned no revision"})
		return
	}

	log.Printf("[Migration] savepoint created: %s", revision)

	// Apply the firewall rules.
	_, err = api.Post(ctx, "/api/firewall/filter/apply/"+revision, map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to apply firewall: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "applied",
		"revision": revision,
		"message":  fmt.Sprintf("Firewall applied with savepoint %s. Will auto-rollback in 60s if not confirmed.", revision),
	})
}

// MigrationConfirm cancels the rollback timer after a successful apply.
// POST /api/opnsense/migration/confirm
func (h *GatewayHandler) MigrationConfirm(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		Revision string `json:"revision"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Revision == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "revision required"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	_, err = api.Post(ctx, "/api/firewall/filter/cancel_rollback/"+req.Revision, map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to confirm: " + err.Error()})
		return
	}

	log.Printf("[Migration] rollback cancelled for revision %s — migration confirmed", req.Revision)

	c.JSON(http.StatusOK, gin.H{"status": "confirmed"})
}

// MigrationFlush removes all legacy rules from OPNsense.
// POST /api/opnsense/migration/flush
func (h *GatewayHandler) MigrationFlush(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	resp, err := api.Post(ctx, "/api/firewall/migration/flush", map[string]any{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to remove legacy rules: " + err.Error()})
		return
	}

	log.Printf("[Migration] legacy rules flushed: %v", resp)

	// Apply to activate the changes.
	if _, err := api.Post(ctx, "/api/firewall/filter/apply", map[string]any{}); err != nil {
		log.Printf("[Migration] warning: flush succeeded but apply failed: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "flushed",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
