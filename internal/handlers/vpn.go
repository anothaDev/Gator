package handlers

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/anothaDev/gator/internal/models"
)

// splitUUIDs splits a comma-separated UUID string into individual trimmed, non-empty UUIDs.
func splitUUIDs(csv string) []string {
	if csv == "" {
		return nil
	}
	var result []string
	for _, s := range strings.Split(csv, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

type VPNConfigStore interface {
	CreateVPNConfig(ctx context.Context, cfg models.SimpleVPNConfig) (int64, error)
	SaveSimpleVPNConfig(ctx context.Context, cfg models.SimpleVPNConfig) error
	GetSimpleVPNConfig(ctx context.Context) (*models.SimpleVPNConfig, error)
	GetVPNConfigByID(ctx context.Context, id int64) (*models.SimpleVPNConfig, error)
	ListVPNConfigs(ctx context.Context) ([]*models.SimpleVPNConfig, error)
	DeleteVPNConfig(ctx context.Context, id int64) error
	GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error)
	GetActiveInstanceID(ctx context.Context) (int64, error)
	// App routing (needed for VPN deactivation/deletion cleanup).
	ListAppRoutes(ctx context.Context, vpnConfigID int64) ([]*models.AppRoute, error)
	UpsertAppRoute(ctx context.Context, r models.AppRoute) error
	DeleteAppRoutesForVPN(ctx context.Context, vpnConfigID int64) error
}

type VPNHandler struct {
	store      VPNConfigStore
	reconciler *Reconciler
}

func NewVPNHandler(store VPNConfigStore) *VPNHandler {
	return &VPNHandler{store: store}
}

// SetReconciler injects the reconciler after construction (avoids init-order issues).
func (h *VPNHandler) SetReconciler(r *Reconciler) { h.reconciler = r }

// asyncReconcile triggers a non-blocking reconciler refresh for the active instance.
func (h *VPNHandler) asyncReconcile(ctx context.Context) {
	if h.reconciler == nil {
		return
	}
	instanceID, _ := h.store.GetActiveInstanceID(ctx)
	if instanceID != 0 {
		h.reconciler.RefreshAsync(instanceID)
	}
}

func vpnStatusFromConfig(cfg *models.SimpleVPNConfig) models.SimpleVPNStatus {
	routingMode := cfg.RoutingMode
	if routingMode == "" {
		routingMode = "all"
	}
	ownership := cfg.OwnershipStatus
	if ownership == "" {
		ownership = models.OwnershipLocalOnly
	}

	// Applied is true when OPNsense bindings (may) exist: verified, pending, or
	// drifted. Drifted means partial mismatch — resources are still present.
	// Only needs_reimport means everything is gone.
	applied := models.IsOwnershipManaged(ownership)

	// Routing, gateway, NAT, policy are only meaningful when managed.
	routingApplied := applied && cfg.RoutingApplied
	gatewayApplied := applied && cfg.OPNsenseGatewayUUID != ""
	natApplied := applied && cfg.OPNsenseSNATRuleUUIDs != ""
	policyApplied := applied && cfg.OPNsenseFilterUUIDs != ""

	return models.SimpleVPNStatus{
		ID:                cfg.ID,
		Name:              cfg.Name,
		Protocol:          cfg.Protocol,
		IPVersion:         cfg.IPVersion,
		RoutingMode:       routingMode,
		LocalCIDR:         cfg.LocalCIDR,
		RemoteCIDR:        cfg.RemoteCIDR,
		Endpoint:          cfg.Endpoint,
		Enabled:           cfg.Enabled,
		HasPrivateKey:     cfg.PrivateKey != "",
		HasPeerPublicKey:  cfg.PeerPublicKey != "",
		HasPreSharedKey:   cfg.PreSharedKey != "",
		Applied:           applied,
		RoutingApplied:    routingApplied,
		GatewayOnline:     applied && cfg.GatewayOnline,
		GatewayApplied:    gatewayApplied,
		NATApplied:        natApplied,
		PolicyApplied:     policyApplied,
		SourceInterfaces:  cfg.SourceInterfaces,
		WGInterface:       cfg.OPNsenseWGInterface,
		WGDevice:          cfg.OPNsenseWGDevice,
		InterfaceAssigned: cfg.OPNsenseWGInterface != "" && cfg.OPNsenseWGInterface != cfg.OPNsenseWGDevice,
		GatewayName:       cfg.OPNsenseGatewayName,
		LastAppliedAt:     cfg.LastAppliedAt,
		OwnershipStatus:   ownership,
		DriftReason:       cfg.DriftReason,
		LastVerifiedAt:    cfg.LastVerifiedAt,
	}
}

func vpnDetailFromConfig(cfg *models.SimpleVPNConfig) models.SimpleVPNDetail {
	return models.SimpleVPNDetail{
		SimpleVPNStatus: vpnStatusFromConfig(cfg),
		DNS:             cfg.DNS,
	}
}

// ImportFromOPNsense adopts an existing WireGuard VPN setup from OPNsense into Gator's database.
// The request body contains the data from the discovery scan.
func (h *VPNHandler) ImportFromOPNsense(c *gin.Context) {
	var req struct {
		Name             string   `json:"name" binding:"required"`
		ServerUUID       string   `json:"server_uuid" binding:"required"`
		PeerUUID         string   `json:"peer_uuid" binding:"required"`
		LocalCIDR        string   `json:"local_cidr" binding:"required"`
		RemoteCIDR       string   `json:"remote_cidr" binding:"required"`
		Endpoint         string   `json:"endpoint" binding:"required"`
		PeerPubKey       string   `json:"peer_pubkey"`
		DNS              string   `json:"dns"`
		WGIface          string   `json:"wg_iface"`
		WGDevice         string   `json:"wg_device"`
		GatewayUUID      string   `json:"gateway_uuid"`
		GatewayName      string   `json:"gateway_name"`
		FilterUUIDs      []string `json:"filter_uuids"`
		SNATUUIDs        []string `json:"snat_uuids"`
		SourceInterfaces []string `json:"source_interfaces"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Fetch the private key and PSK from OPNsense (we don't send them to the frontend).
	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	api := newOPNsenseAPIClient(*firewallCfg)

	var privateKey, psk string
	serverResp, err := api.Get(ctx, "/api/wireguard/server/get_server/"+req.ServerUUID)
	if err == nil {
		server := asMap(serverResp["server"])
		privateKey = asString(server["privkey"])
	}
	peerResp, err := api.Get(ctx, "/api/wireguard/client/get_client/"+req.PeerUUID)
	if err == nil {
		peer := asMap(peerResp["client"])
		psk = asString(peer["psk"])
		if req.PeerPubKey == "" {
			req.PeerPubKey = asString(peer["pubkey"])
		}
	}

	// Resolve instance ID explicitly — CreateVPNConfig also does this,
	// but we want to fail early if no active instance is set.
	instanceID, _ := h.store.GetActiveInstanceID(ctx)
	if instanceID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active firewall instance — configure one first"})
		return
	}

	cfg := models.SimpleVPNConfig{
		InstanceID:            instanceID,
		Name:                  strings.TrimSpace(req.Name),
		Protocol:              "wireguard",
		IPVersion:             "ipv4",
		RoutingMode:           "all",
		LocalCIDR:             strings.TrimSpace(req.LocalCIDR),
		RemoteCIDR:            strings.TrimSpace(req.RemoteCIDR),
		Endpoint:              strings.TrimSpace(req.Endpoint),
		DNS:                   strings.TrimSpace(req.DNS),
		PrivateKey:            privateKey,
		PeerPublicKey:         strings.TrimSpace(req.PeerPubKey),
		PreSharedKey:          psk,
		Enabled:               true,
		RoutingApplied:        req.GatewayUUID != "",
		SourceInterfaces:      req.SourceInterfaces,
		OPNsensePeerUUID:      req.PeerUUID,
		OPNsenseServerUUID:    req.ServerUUID,
		OPNsenseGatewayUUID:   req.GatewayUUID,
		OPNsenseGatewayName:   req.GatewayName,
		OPNsenseSNATRuleUUIDs: strings.Join(req.SNATUUIDs, ","),
		OPNsenseFilterUUIDs:   strings.Join(req.FilterUUIDs, ","),
		OPNsenseWGInterface:   req.WGIface,
		OPNsenseWGDevice:      req.WGDevice,
		LastAppliedAt:         time.Now().UTC().Format(time.RFC3339),
		OwnershipStatus:       models.OwnershipManagedPending,
	}

	id, err := h.store.CreateVPNConfig(ctx, cfg)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to import VPN config: " + err.Error()})
		return
	}

	h.asyncReconcile(ctx)
	c.JSON(http.StatusCreated, gin.H{"status": "imported", "id": id})
}

// ReadoptVPN re-links a drifted or needs-reimport VPN profile to a discovered
// OPNsense resource. Updates the profile's OPNsense UUIDs and re-fetches secrets.
func (h *VPNHandler) ReadoptVPN(c *gin.Context) {
	existing, ok := h.getVPNByParam(c)
	if !ok {
		return
	}

	// Only allow re-adoption for drifted or needs-reimport profiles.
	if existing.OwnershipStatus != models.OwnershipManagedDrifted &&
		existing.OwnershipStatus != models.OwnershipNeedsReimport {
		c.JSON(http.StatusConflict, gin.H{"error": "VPN is " + existing.OwnershipStatus + " — re-adopt is only for drifted or needs-reimport profiles"})
		return
	}

	var req struct {
		ServerUUID       string   `json:"server_uuid" binding:"required"`
		PeerUUID         string   `json:"peer_uuid" binding:"required"`
		LocalCIDR        string   `json:"local_cidr"`
		RemoteCIDR       string   `json:"remote_cidr"`
		Endpoint         string   `json:"endpoint"`
		PeerPubKey       string   `json:"peer_pubkey"`
		DNS              string   `json:"dns"`
		WGIface          string   `json:"wg_iface"`
		WGDevice         string   `json:"wg_device"`
		GatewayUUID      string   `json:"gateway_uuid"`
		GatewayName      string   `json:"gateway_name"`
		FilterUUIDs      []string `json:"filter_uuids"`
		SNATUUIDs        []string `json:"snat_uuids"`
		SourceInterfaces []string `json:"source_interfaces"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Re-fetch secrets from OPNsense.
	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	api := newOPNsenseAPIClient(*firewallCfg)

	serverResp, err := api.Get(ctx, "/api/wireguard/server/get_server/"+req.ServerUUID)
	if err == nil {
		server := asMap(serverResp["server"])
		if pk := asString(server["privkey"]); pk != "" {
			existing.PrivateKey = pk
		}
	}
	peerResp, err := api.Get(ctx, "/api/wireguard/client/get_client/"+req.PeerUUID)
	if err == nil {
		peer := asMap(peerResp["client"])
		if psk := asString(peer["psk"]); psk != "" {
			existing.PreSharedKey = psk
		}
		if req.PeerPubKey == "" {
			req.PeerPubKey = asString(peer["pubkey"])
		}
	}

	// Update OPNsense bindings.
	existing.OPNsensePeerUUID = req.PeerUUID
	existing.OPNsenseServerUUID = req.ServerUUID
	existing.OPNsenseGatewayUUID = req.GatewayUUID
	existing.OPNsenseGatewayName = req.GatewayName
	existing.OPNsenseWGInterface = req.WGIface
	existing.OPNsenseWGDevice = req.WGDevice
	// Always overwrite rule/interface bindings — stale UUIDs from the old
	// resource must not survive readopt (they'd cause immediate drift).
	existing.OPNsenseFilterUUIDs = strings.Join(req.FilterUUIDs, ",")
	existing.OPNsenseSNATRuleUUIDs = strings.Join(req.SNATUUIDs, ",")
	existing.SourceInterfaces = req.SourceInterfaces

	// Update config fields if provided (discovery may have newer values).
	if req.PeerPubKey != "" {
		existing.PeerPublicKey = req.PeerPubKey
	}
	if req.LocalCIDR != "" {
		existing.LocalCIDR = strings.TrimSpace(req.LocalCIDR)
	}
	if req.RemoteCIDR != "" {
		existing.RemoteCIDR = strings.TrimSpace(req.RemoteCIDR)
	}
	if req.Endpoint != "" {
		existing.Endpoint = strings.TrimSpace(req.Endpoint)
	}
	if req.DNS != "" {
		existing.DNS = strings.TrimSpace(req.DNS)
	}

	// Mark as pending verification — the reconciler will promote to managed_verified.
	now := time.Now().UTC().Format(time.RFC3339)
	existing.OwnershipStatus = models.OwnershipManagedPending
	existing.LastAppliedAt = now
	existing.DriftReason = ""
	existing.RoutingApplied = req.GatewayUUID != ""

	if err := h.store.SaveSimpleVPNConfig(ctx, *existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save re-adopted VPN config"})
		return
	}

	h.asyncReconcile(ctx)
	c.JSON(http.StatusOK, gin.H{"status": "readopted", "id": existing.ID})
}

// ListConfigs returns all VPN configurations.
func (h *VPNHandler) ListConfigs(c *gin.Context) {
	configs, err := h.store.ListVPNConfigs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list vpn configs"})
		return
	}

	statuses := make([]models.SimpleVPNStatus, 0, len(configs))
	for _, cfg := range configs {
		statuses = append(statuses, vpnStatusFromConfig(cfg))
	}

	c.JSON(http.StatusOK, gin.H{"vpns": statuses})
}

// GetConfig returns a single VPN config by ID.
func (h *VPNHandler) GetConfig(c *gin.Context) {
	cfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, vpnDetailFromConfig(cfg))
}

func trimVPNFields(cfg *models.SimpleVPNConfig) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Protocol = strings.TrimSpace(cfg.Protocol)
	cfg.IPVersion = strings.TrimSpace(cfg.IPVersion)
	if cfg.IPVersion == "" {
		cfg.IPVersion = "ipv4"
	}
	cfg.LocalCIDR = strings.TrimSpace(cfg.LocalCIDR)
	cfg.RemoteCIDR = strings.TrimSpace(cfg.RemoteCIDR)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.DNS = strings.TrimSpace(cfg.DNS)
	cfg.PrivateKey = strings.TrimSpace(cfg.PrivateKey)
	cfg.PeerPublicKey = strings.TrimSpace(cfg.PeerPublicKey)
	cfg.PreSharedKey = strings.TrimSpace(cfg.PreSharedKey)
}

// CreateConfig creates a new VPN configuration.
func (h *VPNHandler) CreateConfig(c *gin.Context) {
	var cfg models.SimpleVPNConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	trimVPNFields(&cfg)
	cfg.ID = 0 // Force new.

	if err := validateSimpleVPNConfig(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := h.store.CreateVPNConfig(c.Request.Context(), cfg)
	if err != nil {
		// Surface duplicate-name errors as 409 Conflict with the actual message.
		if strings.Contains(err.Error(), "already exists") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create vpn config"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "created", "id": id})
}

// SaveConfig updates an existing VPN configuration by ID.
func (h *VPNHandler) SaveConfig(c *gin.Context) {
	existing, ok := h.getVPNByParam(c)
	if !ok {
		return
	}

	var cfg models.SimpleVPNConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	trimVPNFields(&cfg)

	// Preserve secrets if not resent.
	if cfg.PrivateKey == "" {
		cfg.PrivateKey = existing.PrivateKey
	}
	if cfg.PeerPublicKey == "" {
		cfg.PeerPublicKey = existing.PeerPublicKey
	}
	if cfg.PreSharedKey == "" {
		cfg.PreSharedKey = existing.PreSharedKey
	}

	// Preserve internal state (not sent by frontend).
	cfg.ID = existing.ID
	cfg.InstanceID = existing.InstanceID
	cfg.OPNsensePeerUUID = existing.OPNsensePeerUUID
	cfg.OPNsenseServerUUID = existing.OPNsenseServerUUID
	cfg.OPNsenseGatewayUUID = existing.OPNsenseGatewayUUID
	cfg.OPNsenseGatewayName = existing.OPNsenseGatewayName
	cfg.SourceInterfaces = existing.SourceInterfaces
	cfg.OPNsenseSNATRuleUUIDs = existing.OPNsenseSNATRuleUUIDs
	cfg.OPNsenseFilterUUIDs = existing.OPNsenseFilterUUIDs
	cfg.OPNsenseWGInterface = existing.OPNsenseWGInterface
	cfg.OPNsenseWGDevice = existing.OPNsenseWGDevice
	cfg.LastAppliedAt = existing.LastAppliedAt
	cfg.RoutingApplied = existing.RoutingApplied
	cfg.OwnershipStatus = existing.OwnershipStatus
	cfg.LastVerifiedAt = existing.LastVerifiedAt
	cfg.DriftReason = existing.DriftReason

	if err := validateSimpleVPNConfig(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.SaveSimpleVPNConfig(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save vpn config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "saved", "id": existing.ID})
}

// DeleteConfig deletes a VPN configuration by ID, cleaning up OPNsense resources first.
func (h *VPNHandler) DeleteConfig(c *gin.Context) {
	ctx := c.Request.Context()

	cfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}
	id := cfg.ID

	// Attempt OPNsense cascade cleanup. Collect warnings for non-fatal failures.
	var warnings []string
	hasRemoteResources := cfg.OPNsenseFilterUUIDs != "" || cfg.OPNsenseSNATRuleUUIDs != "" ||
		cfg.OPNsenseGatewayUUID != "" || cfg.OPNsenseServerUUID != "" || cfg.OPNsensePeerUUID != ""

	if hasRemoteResources {
		firewallCfg, err := h.store.GetFirewallConfig(ctx)
		if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
			warnings = append(warnings, "could not load firewall config — OPNsense resources were not cleaned up")
		} else {
			api := newOPNsenseAPIClient(*firewallCfg)
			needFirewallApply := false

			// 1. Delete filter rules (verified as Gator-owned before deleting).
			for _, uuid := range splitUUIDs(cfg.OPNsenseFilterUUIDs) {
				deleted, err := safeDeleteFilterRule(ctx, api, uuid)
				if err != nil {
					warnings = append(warnings, "failed to delete filter rule "+uuid+": "+err.Error())
				} else if deleted {
					needFirewallApply = true
				}
			}

			// 2. Delete SNAT rules (verified as Gator-owned before deleting).
			for _, uuid := range splitUUIDs(cfg.OPNsenseSNATRuleUUIDs) {
				deleted, err := safeDeleteSNATRule(ctx, api, uuid)
				if err != nil {
					warnings = append(warnings, "failed to delete NAT rule "+uuid+": "+err.Error())
				} else if deleted {
					needFirewallApply = true
				}
			}

			// 3. Apply firewall changes if any rules were removed.
			if needFirewallApply {
				if err := applyFirewallWithRollback(ctx, api); err != nil {
					warnings = append(warnings, "failed to apply firewall after rule deletion: "+err.Error())
				}
			}

			// 4. Delete gateway.
			if cfg.OPNsenseGatewayUUID != "" {
				resp, err := api.Post(ctx, "/api/routing/settings/del_gateway/"+cfg.OPNsenseGatewayUUID, map[string]any{})
				if err != nil {
					warnings = append(warnings, "failed to delete gateway: "+err.Error())
				} else if err := expectOPNsenseResult(resp, "deleted", "ok", ""); err != nil {
					warnings = append(warnings, "gateway delete: "+err.Error())
				} else {
					if _, err := api.Post(ctx, "/api/routing/settings/reconfigure", map[string]any{}); err != nil {
						warnings = append(warnings, "failed to reconfigure routing after gateway deletion: "+err.Error())
					}
				}
			}

			// 5. Delete WireGuard local instance.
			if cfg.OPNsenseServerUUID != "" {
				resp, err := api.Post(ctx, "/api/wireguard/server/del_server/"+cfg.OPNsenseServerUUID, map[string]any{})
				if err != nil {
					warnings = append(warnings, "failed to delete WG local instance: "+err.Error())
				} else if err := expectOPNsenseResult(resp, "deleted", "ok", ""); err != nil {
					warnings = append(warnings, "WG local instance delete: "+err.Error())
				}
			}

			// 6. Delete WireGuard peer.
			if cfg.OPNsensePeerUUID != "" {
				resp, err := api.Post(ctx, "/api/wireguard/client/del_client/"+cfg.OPNsensePeerUUID, map[string]any{})
				if err != nil {
					warnings = append(warnings, "failed to delete WG peer: "+err.Error())
				} else if err := expectOPNsenseResult(resp, "deleted", "ok", ""); err != nil {
					warnings = append(warnings, "WG peer delete: "+err.Error())
				}
			}

			// 7. Reconfigure WireGuard service.
			if cfg.OPNsenseServerUUID != "" || cfg.OPNsensePeerUUID != "" {
				if _, err := api.Post(ctx, "/api/wireguard/service/reconfigure", map[string]any{}); err != nil {
					warnings = append(warnings, "failed to reconfigure WireGuard: "+err.Error())
				}
			}
		}
	}

	// Clean up app-specific routing rules from OPNsense.
	if hasRemoteResources {
		firewallCfg, err := h.store.GetFirewallConfig(ctx)
		if err == nil && firewallCfg != nil && firewallCfg.Type == "opnsense" {
			api := newOPNsenseAPIClient(*firewallCfg)
			appWarnings := h.cleanupAppRoutingRules(ctx, api, id)
			warnings = append(warnings, appWarnings...)
		}
	}
	// Delete app routing DB rows.
	if err := h.store.DeleteAppRoutesForVPN(ctx, id); err != nil {
		warnings = append(warnings, "delete app routes: "+err.Error())
	}

	// Always delete the local config, even if remote cleanup had issues.
	if err := h.store.DeleteVPNConfig(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete vpn config"})
		return
	}

	h.asyncReconcile(ctx)
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "warnings": warnings})
}

// getVPNByParam reads the :id URL parameter and loads the VPN config.
func (h *VPNHandler) getVPNByParam(c *gin.Context) (*models.SimpleVPNConfig, bool) {
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
	// Verify the VPN belongs to the active instance (prevent cross-instance access).
	activeID, _ := h.store.GetActiveInstanceID(c.Request.Context())
	if activeID != 0 && cfg.InstanceID != activeID {
		c.JSON(http.StatusNotFound, gin.H{"error": "vpn config not found"})
		return nil, false
	}
	return cfg, true
}

func (h *VPNHandler) ApplyToOPNsense(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "firewall setup not found. complete setup first"})
		return
	}
	if firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "apply-to-opnsense requires an OPNsense setup"})
		return
	}

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}

	if err := validateSimpleVPNConfig(*vpnCfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if vpnCfg.Protocol != "wireguard" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only wireguard apply is supported right now"})
		return
	}
	if strings.TrimSpace(vpnCfg.PrivateKey) == "" || strings.TrimSpace(vpnCfg.PeerPublicKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wireguard private/public keys are required. import a .conf file first"})
		return
	}

	host, port, err := splitEndpoint(vpnCfg.Endpoint)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	apiClient := newOPNsenseAPIClient(*firewallCfg)

	baseName := sanitizeOPNsenseName(vpnCfg.Name, "gator_wg")
	peerName := sanitizeOPNsenseName(baseName+"_peer", "gator_wg_peer")
	serverName := sanitizeOPNsenseName(baseName+"_local", "gator_wg_local")

	peerPayload := map[string]any{
		"client": map[string]any{
			"enabled":       "1",
			"name":          peerName,
			"pubkey":        vpnCfg.PeerPublicKey,
			"tunneladdress": vpnCfg.RemoteCIDR,
			"serveraddress": host,
			"serverport":    port,
			"keepalive":     "25",
		},
	}
	if vpnCfg.PreSharedKey != "" {
		peerPayload["client"].(map[string]any)["psk"] = vpnCfg.PreSharedKey
	}

	peerUUID, peerCreated, err := ensureWireGuardPeer(
		ctx,
		apiClient,
		peerPayload,
		vpnCfg.PeerPublicKey,
		vpnCfg.OPNsensePeerUUID,
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to apply wireguard peer: " + err.Error()})
		return
	}

	serverPayload := map[string]any{
		"server": map[string]any{
			"enabled":       "1",
			"name":          serverName,
			"privkey":       vpnCfg.PrivateKey,
			"tunneladdress": vpnCfg.LocalCIDR,
			"peers":         peerUUID,
			"disableroutes": "1",
		},
	}
	if vpnCfg.DNS != "" {
		serverPayload["server"].(map[string]any)["dns"] = vpnCfg.DNS
	}
	if gateway := deriveTunnelGateway(vpnCfg.LocalCIDR); gateway != "" {
		serverPayload["server"].(map[string]any)["gateway"] = gateway
	}
	serverPayload["server"].(map[string]any)["peers"] = peerUUID

	// Check if another deployed VPN already owns a WG server we should share.
	var reuseServerUUID string
	allVPNs, listErr := h.store.ListVPNConfigs(ctx)
	if listErr == nil {
		for _, other := range allVPNs {
			if other.ID != vpnCfg.ID && other.OPNsenseServerUUID != "" {
				reuseServerUUID = other.OPNsenseServerUUID
				if other.OPNsenseWGInterface != "" {
					vpnCfg.OPNsenseWGInterface = other.OPNsenseWGInterface
					vpnCfg.OPNsenseWGDevice = other.OPNsenseWGDevice
				}
				break
			}
		}
	}

	var serverUUID string
	var serverCreated bool

	if reuseServerUUID != "" {
		// Another VPN owns the WG server — don't touch it during deploy.
		// Just store its UUID. Full reconfiguration happens on ActivateVPN.
		serverUUID = reuseServerUUID
		serverCreated = false
		log.Printf("[Deploy] reusing existing WG server %s from another VPN", reuseServerUUID)
	} else {
		// First VPN to deploy — create the server normally.
		serverUUID, serverCreated, err = ensureWireGuardServer(
			ctx,
			apiClient,
			serverPayload,
			serverName,
			vpnCfg.PrivateKey,
			vpnCfg.OPNsenseServerUUID,
			peerUUID,
		)
		if err != nil {
			if peerCreated {
				_, _ = apiClient.Post(ctx, "/api/wireguard/client/del_client/"+peerUUID, map[string]any{})
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to apply wireguard local instance: " + err.Error()})
			return
		}

		generalResp, err := apiClient.Post(ctx, "/api/wireguard/general/set", map[string]any{
			"general": map[string]any{"enabled": "1"},
		})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to enable wireguard: " + err.Error()})
			return
		}
		if err := expectOPNsenseResult(generalResp, "saved", "ok"); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "wireguard enable failed: " + err.Error()})
			return
		}

		reconfigResp, err := apiClient.Post(ctx, "/api/wireguard/service/reconfigure", map[string]any{})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reconfigure wireguard: " + err.Error()})
			return
		}
		if err := expectOPNsenseResult(reconfigResp, "saved", "ok"); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "wireguard reconfigure failed: " + err.Error()})
			return
		}

		_, _ = apiClient.Post(ctx, "/api/wireguard/service/start", map[string]any{})
	}

	vpnCfg.OPNsensePeerUUID = peerUUID
	vpnCfg.OPNsenseServerUUID = serverUUID
	vpnCfg.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	vpnCfg.OwnershipStatus = models.OwnershipManagedPending
	vpnCfg.DriftReason = ""
	if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "wireguard applied but failed to persist status"})
		return
	}

	applyStatus := "applied"
	applyMessage := "WireGuard config created and applied on OPNsense."
	if !peerCreated && !serverCreated {
		applyStatus = "already_applied"
		applyMessage = "VPN is already applied on OPNsense. Existing WireGuard entries were reused."
	} else if !peerCreated || !serverCreated {
		applyStatus = "updated_existing"
		applyMessage = "VPN applied by updating existing WireGuard entries on OPNsense."
	}

	h.asyncReconcile(ctx)
	c.JSON(http.StatusOK, gin.H{
		"status":          applyStatus,
		"message":         applyMessage,
		"peer_uuid":       peerUUID,
		"server_uuid":     serverUUID,
		"peer_created":    peerCreated,
		"server_created":  serverCreated,
		"last_applied_at": vpnCfg.LastAppliedAt,
	})
}

// routingPreamble loads and validates the firewall config, VPN config (by :id), and WireGuard interface.
// Returns (vpnCfg, apiClient, wgInterface, wgDevice, baseName, ok).
// If ok is false, an error response has already been sent.
func (h *VPNHandler) routingPreamble(c *gin.Context) (
	*models.SimpleVPNConfig,
	*opnsenseAPIClient,
	string, // wgIface (logical name, e.g. "opt1")
	string, // wgDevice (kernel device, e.g. "wg0")
	string, // baseName
	bool,
) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return nil, nil, "", "", "", false
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "routing apply requires an OPNsense setup"})
		return nil, nil, "", "", "", false
	}

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return nil, nil, "", "", "", false
	}
	if vpnCfg.OPNsenseServerUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wireguard tunnel is not applied yet. apply the VPN tunnel first"})
		return nil, nil, "", "", "", false
	}

	apiClient := newOPNsenseAPIClient(*firewallCfg)

	wgIface := vpnCfg.OPNsenseWGInterface
	wgDevice := vpnCfg.OPNsenseWGDevice
	if wgIface == "" {
		iface, device, err := discoverWGInterface(ctx, apiClient)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to discover wireguard interface: " + err.Error()})
			return nil, nil, "", "", "", false
		}
		if iface == "" && device == "" {
			c.JSON(http.StatusBadGateway, gin.H{"error": "no wireguard interface found on OPNsense. ensure the WG tunnel is applied and active"})
			return nil, nil, "", "", "", false
		}
		if iface == "" && device != "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf(
					"WireGuard device %s exists but is not assigned as an OPNsense interface. "+
						"Go to Interfaces > Assignments in OPNsense, assign %s, enable it, then retry.",
					device, device,
				),
			})
			return nil, nil, "", "", "", false
		}
		wgIface = iface
		wgDevice = device
	}

	baseName := sanitizeOPNsenseName(vpnCfg.Name, "gator_wg")

	return vpnCfg, apiClient, wgIface, wgDevice, baseName, true
}

// opnsenseIPProtocol converts our "ipv4"/"ipv6" to OPNsense's "inet"/"inet6".
func opnsenseIPProtocol(ipVersion string) string {
	if ipVersion == "ipv6" {
		return "inet6"
	}
	return "inet"
}

// resolveGatewayName returns the gateway name for this VPN, either from the stored config or derived from the base name.
func resolveGatewayName(vpnCfg *models.SimpleVPNConfig, baseName string) string {
	if vpnCfg.OPNsenseGatewayName != "" {
		return vpnCfg.OPNsenseGatewayName
	}
	return sanitizeGatewayName(baseName + "_gw")
}

// ApplyGatewayToOPNsense discovers the WG interface, creates a gateway, and reconfigures routing.
func (h *VPNHandler) ApplyGatewayToOPNsense(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, apiClient, wgIface, wgDevice, baseName, ok := h.routingPreamble(c)
	if !ok {
		return
	}

	gwName := resolveGatewayName(vpnCfg, baseName)

	gatewayIP := deriveTunnelGateway(vpnCfg.LocalCIDR)
	if gatewayIP == "" {
		prefix, err := parsePrefix(vpnCfg.LocalCIDR)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid local_cidr: " + err.Error()})
			return
		}
		gatewayIP = prefix
	}

	monitorIP := deriveMonitorIP(vpnCfg.DNS, vpnCfg.IPVersion)
	gwUUID, gwCreated, err := ensureGateway(ctx, apiClient, gwName, wgIface, gatewayIP, opnsenseIPProtocol(vpnCfg.IPVersion), monitorIP, vpnCfg.OPNsenseGatewayUUID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create VPN gateway: " + err.Error()})
		return
	}

	if _, err := apiClient.Post(ctx, "/api/routing/settings/reconfigure", map[string]any{}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reconfigure routing: " + err.Error()})
		return
	}

	vpnCfg.OPNsenseGatewayUUID = gwUUID
	vpnCfg.OPNsenseGatewayName = gwName
	vpnCfg.OPNsenseWGInterface = wgIface
	vpnCfg.OPNsenseWGDevice = wgDevice
	vpnCfg.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	vpnCfg.OwnershipStatus = models.OwnershipManagedPending
	vpnCfg.DriftReason = ""

	if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gateway applied but failed to persist status"})
		return
	}

	status := "created"
	msg := "VPN gateway created and routing reconfigured."
	if !gwCreated {
		status = "updated"
		msg = "Existing VPN gateway updated and routing reconfigured."
	}

	h.asyncReconcile(ctx)
	c.JSON(http.StatusOK, gin.H{
		"status":       status,
		"message":      msg,
		"gateway_uuid": gwUUID,
		"gateway_name": gwName,
		"wg_interface": wgIface,
	})
}

// ApplyNATToOPNsense creates or updates outbound NAT rules for VPN traffic.
// Creates one SNAT rule per source interface.
// OPNsense's automatic outbound NAT only generates rules for the WAN interface,
// NOT for WireGuard/OpenVPN tunnel interfaces. Manual SNAT rules are always needed
// for VPN tunnels regardless of the outbound NAT mode.
// The mode is also switched to hybrid if currently on automatic, since manual rules
// are ignored in pure automatic mode.
func (h *VPNHandler) ApplyNATToOPNsense(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, apiClient, wgIface, wgDevice, baseName, ok := h.routingPreamble(c)
	if !ok {
		return
	}

	if vpnCfg.OPNsenseGatewayUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway is not applied yet. apply the gateway first"})
		return
	}

	sourceIfaces := vpnCfg.SourceInterfaces
	if len(sourceIfaces) == 0 {
		sourceIfaces = []string{"lan"} // Backward compat default.
	}

	// Build a map of existing SNAT UUIDs by description for reuse.
	existingUUIDs := splitUUIDs(vpnCfg.OPNsenseSNATRuleUUIDs)

	var snatUUIDs []string
	anyCreated := false
	for i, srcIface := range sourceIfaces {
		snatDesc := "GATOR_NAT_" + strings.ToUpper(strings.ReplaceAll(baseName, " ", "_")) + "_" + strings.ToUpper(srcIface)
		preferredUUID := ""
		if i < len(existingUUIDs) {
			preferredUUID = existingUUIDs[i]
		}
		uuid, created, err := ensureSNATRule(ctx, apiClient, wgIface, srcIface, opnsenseIPProtocol(vpnCfg.IPVersion), snatDesc, preferredUUID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create outbound NAT rule for " + srcIface + ": " + err.Error()})
			return
		}
		snatUUIDs = append(snatUUIDs, uuid)
		if created {
			anyCreated = true
		}
	}

	// Delete any leftover SNAT rules from removed interfaces (verified as Gator-owned).
	for i := len(sourceIfaces); i < len(existingUUIDs); i++ {
		_, _ = safeDeleteSNATRule(ctx, apiClient, existingUUIDs[i])
	}

	// Apply firewall changes so the NAT rules take effect.
	if err := applyFirewallWithRollback(ctx, apiClient); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NAT rules saved but failed to apply firewall: " + err.Error()})
		return
	}

	vpnCfg.OPNsenseSNATRuleUUIDs = strings.Join(snatUUIDs, ",")
	vpnCfg.OPNsenseWGInterface = wgIface
	vpnCfg.OPNsenseWGDevice = wgDevice
	vpnCfg.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	vpnCfg.OwnershipStatus = models.OwnershipManagedPending
	vpnCfg.DriftReason = ""

	if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "NAT rules applied but failed to persist status"})
		return
	}

	status := "created"
	msg := fmt.Sprintf("Outbound NAT rules created for %d interface(s) and firewall applied.", len(snatUUIDs))
	if !anyCreated {
		status = "updated"
		msg = fmt.Sprintf("Existing outbound NAT rules updated for %d interface(s) and firewall applied.", len(snatUUIDs))
	}

	h.asyncReconcile(ctx)
	c.JSON(http.StatusOK, gin.H{
		"status":     status,
		"message":    msg,
		"snat_uuids": snatUUIDs,
	})
}

// ApplyPolicyRuleToOPNsense creates or updates the firewall policy routing rule.
func (h *VPNHandler) ApplyPolicyRuleToOPNsense(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, apiClient, _, _, baseName, ok := h.routingPreamble(c)
	if !ok {
		return
	}

	if vpnCfg.OPNsenseGatewayUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway is not applied yet. apply the gateway first"})
		return
	}

	gwName := resolveGatewayName(vpnCfg, baseName)

	sourceIfaces := vpnCfg.SourceInterfaces
	if len(sourceIfaces) == 0 {
		sourceIfaces = []string{"lan"} // Backward compat default.
	}

	// Use comma-separated interfaces in the filter rule (OPNsense InterfaceField supports multiple).
	ifaceCSV := strings.Join(sourceIfaces, ",")

	filterDesc := "GATOR_VPN_" + strings.ToUpper(strings.ReplaceAll(baseName, " ", "_"))
	existingFilterUUIDs := splitUUIDs(vpnCfg.OPNsenseFilterUUIDs)
	preferredFilterUUID := ""
	if len(existingFilterUUIDs) > 0 {
		preferredFilterUUID = existingFilterUUIDs[0]
	}
	filterUUID, filterCreated, err := ensureFilterRule(ctx, apiClient, ifaceCSV, "any", gwName, opnsenseIPProtocol(vpnCfg.IPVersion), filterDesc, preferredFilterUUID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create policy routing rule: " + err.Error()})
		return
	}

	// Auto-adopt: find non-Gator rules on the same gateway+interfaces and absorb their fields.
	adopted := autoAdoptStaleRules(ctx, apiClient, filterUUID, gwName, sourceIfaces)

	// Delete any leftover filter rules from old single-interface approach (verified as Gator-owned).
	for i := 1; i < len(existingFilterUUIDs); i++ {
		_, _ = safeDeleteFilterRule(ctx, apiClient, existingFilterUUIDs[i])
	}

	// Deactivate other VPNs' filter rules so only this one routes traffic.
	deactivateWarnings := h.deactivateOtherVPNs(ctx, apiClient, vpnCfg.ID)

	if err := applyFirewallWithRollback(ctx, apiClient); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "policy rule saved but failed to apply firewall: " + err.Error()})
		return
	}

	vpnCfg.OPNsenseFilterUUIDs = filterUUID
	vpnCfg.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	vpnCfg.RoutingApplied = true
	vpnCfg.OwnershipStatus = models.OwnershipManagedPending
	vpnCfg.DriftReason = ""

	if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "policy rule applied but failed to persist status"})
		return
	}

	status := "created"
	msg := "Policy routing rule created and firewall applied."
	if !filterCreated {
		status = "updated"
		msg = "Existing policy routing rule updated and firewall applied."
	}

	h.asyncReconcile(ctx)
	c.JSON(http.StatusOK, gin.H{
		"status":        status,
		"message":       msg,
		"filter_uuid":   filterUUID,
		"warnings":      deactivateWarnings,
		"adopted_rules": adopted,
	})
}

// cleanupAppRoutingRules deletes all app-specific OPNsense filter rules for a VPN
// and clears the app routing state in the DB. Called during VPN deactivation and deletion.
func (h *VPNHandler) cleanupAppRoutingRules(ctx context.Context, api *opnsenseAPIClient, vpnID int64) []string {
	var warnings []string

	routes, err := h.store.ListAppRoutes(ctx, vpnID)
	if err != nil {
		warnings = append(warnings, "list app routes: "+err.Error())
		return warnings
	}

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
			deleted, err := safeDeleteFilterRule(ctx, api, uuid)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("delete app rule %s for %s: %s", uuid, route.AppID, err.Error()))
			} else if deleted {
				needApply = true
			}
		}
		// Clear this route's OPNsense state in the DB.
		route.Enabled = false
		route.OPNsenseRuleUUIDs = ""
		if err := h.store.UpsertAppRoute(ctx, *route); err != nil {
			warnings = append(warnings, fmt.Sprintf("clear app route %s: %s", route.AppID, err.Error()))
		}
	}

	if needApply {
		if err := applyFirewallWithRollback(ctx, api); err != nil {
			warnings = append(warnings, "apply firewall after app rule cleanup: "+err.Error())
		}
	}

	return warnings
}

// deactivateOtherVPNs disables the filter rules of all VPNs except excludeID.
// Returns warnings for any non-fatal failures.
func (h *VPNHandler) deactivateOtherVPNs(ctx context.Context, api *opnsenseAPIClient, excludeID int64) []string {
	configs, err := h.store.ListVPNConfigs(ctx)
	if err != nil {
		return []string{"failed to list VPN configs: " + err.Error()}
	}

	var warnings []string
	for _, cfg := range configs {
		if cfg.ID == excludeID || !cfg.RoutingApplied || cfg.OPNsenseFilterUUIDs == "" {
			continue
		}

		// Switch all filter rules for this VPN to default gateway (verified as Gator-owned).
		for _, uuid := range splitUUIDs(cfg.OPNsenseFilterUUIDs) {
			if err := safeSetFilterRuleGateway(ctx, api, uuid, ""); err != nil {
				warnings = append(warnings, fmt.Sprintf("failed to deactivate %s filter rule %s: %s", cfg.Name, uuid, err.Error()))
			}
		}

		cfg.RoutingApplied = false
		if err := h.store.SaveSimpleVPNConfig(ctx, *cfg); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to update %s status", cfg.Name))
		}
	}

	return warnings
}

// ActivateVPN enables this VPN's filter rule and disables all others.
func (h *VPNHandler) ActivateVPN(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}

	// Verify OPNsense state is current before mutating.
	if h.reconciler != nil {
		instanceID, _ := h.store.GetActiveInstanceID(ctx)
		if err := h.reconciler.EnsureFresh(ctx, instanceID, 5*time.Second); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "cannot verify OPNsense state: " + err.Error()})
			return
		}
		// Re-read after reconcile may have updated ownership.
		vpnCfg, ok = h.getVPNByParam(c)
		if !ok {
			return
		}
		if vpnCfg.OwnershipStatus != models.OwnershipManagedVerified &&
			vpnCfg.OwnershipStatus != models.OwnershipManagedPending &&
			vpnCfg.OwnershipStatus != "" {
			c.JSON(http.StatusConflict, gin.H{"error": "VPN is " + vpnCfg.OwnershipStatus + " — re-deploy or re-import required"})
			return
		}
	}

	// For imported VPNs with no Gator-managed filter rules (legacy setup),
	// just toggle the status flag — we don't own the rules.
	if vpnCfg.OPNsenseFilterUUIDs == "" {
		if vpnCfg.OPNsenseGatewayUUID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "VPN is not fully deployed. Deploy it first."})
			return
		}
		vpnCfg.RoutingApplied = true
		vpnCfg.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
		vpnCfg.OwnershipStatus = models.OwnershipManagedPending
		vpnCfg.DriftReason = ""
		if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist status"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "activated", "warnings": []string{"No Gator-managed filter rules — status updated only."}})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// Deactivate other VPNs.
	warnings := h.deactivateOtherVPNs(ctx, api, vpnCfg.ID)

	// Reconfigure the shared WG server to this VPN's keys/tunnel/peer.
	if vpnCfg.OPNsenseServerUUID != "" && vpnCfg.OPNsensePeerUUID != "" {
		serverPayload := map[string]any{
			"server": map[string]any{
				"enabled":       "1",
				"name":          sanitizeOPNsenseName(vpnCfg.Name+"_local", "gator_wg_local"),
				"privkey":       vpnCfg.PrivateKey,
				"tunneladdress": vpnCfg.LocalCIDR,
				"peers":         vpnCfg.OPNsensePeerUUID,
				"disableroutes": "1",
			},
		}
		if vpnCfg.DNS != "" {
			serverPayload["server"].(map[string]any)["dns"] = vpnCfg.DNS
		}
		if gw := deriveTunnelGateway(vpnCfg.LocalCIDR); gw != "" {
			serverPayload["server"].(map[string]any)["gateway"] = gw
		}
		_, err := api.Post(ctx, "/api/wireguard/server/set_server/"+vpnCfg.OPNsenseServerUUID, serverPayload)
		if err != nil {
			warnings = append(warnings, "failed to reconfigure WG server: "+err.Error())
		}
		if _, err := api.Post(ctx, "/api/wireguard/service/reconfigure", map[string]any{}); err != nil {
			warnings = append(warnings, "failed to reconfigure wireguard service: "+err.Error())
		}

		// Update the gateway IP for the new tunnel address.
		if vpnCfg.OPNsenseGatewayUUID != "" {
			gatewayIP := deriveTunnelGateway(vpnCfg.LocalCIDR)
			if gatewayIP != "" {
				monitorIP := deriveMonitorIP(vpnCfg.DNS, vpnCfg.IPVersion)
				_, _, gwErr := ensureGateway(ctx, api, vpnCfg.OPNsenseGatewayName, vpnCfg.OPNsenseWGInterface, gatewayIP, opnsenseIPProtocol(vpnCfg.IPVersion), monitorIP, vpnCfg.OPNsenseGatewayUUID)
				if gwErr != nil {
					warnings = append(warnings, "failed to update gateway IP: "+gwErr.Error())
				}
				if _, err := api.Post(ctx, "/api/routing/settings/reconfigure", map[string]any{}); err != nil {
					warnings = append(warnings, "failed to reconfigure routing: "+err.Error())
				}
			}
		}
	}

	// Switch this VPN's filter rules back to the VPN gateway.
	gwName := vpnCfg.OPNsenseGatewayName
	if gwName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VPN has no gateway name — redeploy to fix"})
		return
	}
	gwKey := resolveGatewayKey(ctx, api, gwName)

	for _, uuid := range splitUUIDs(vpnCfg.OPNsenseFilterUUIDs) {
		if err := safeSetFilterRuleGateway(ctx, api, uuid, gwKey); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to update filter rule gateway: " + err.Error()})
			return
		}
	}

	// Apply firewall.
	if err := applyFirewallWithRollback(ctx, api); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to apply firewall: " + err.Error()})
		return
	}

	vpnCfg.RoutingApplied = true
	vpnCfg.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	vpnCfg.OwnershipStatus = models.OwnershipManagedPending
	vpnCfg.DriftReason = ""
	if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "activated but failed to persist status"})
		return
	}

	h.asyncReconcile(ctx)
	c.JSON(http.StatusOK, gin.H{"status": "activated", "warnings": warnings})
}

// DeactivateVPN disables this VPN's filter rule (traffic returns to default gateway).
func (h *VPNHandler) DeactivateVPN(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}

	// Verify OPNsense state is current before mutating.
	if h.reconciler != nil {
		instanceID, _ := h.store.GetActiveInstanceID(ctx)
		if err := h.reconciler.EnsureFresh(ctx, instanceID, 5*time.Second); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "cannot verify OPNsense state: " + err.Error()})
			return
		}
		vpnCfg, ok = h.getVPNByParam(c)
		if !ok {
			return
		}
		if vpnCfg.OwnershipStatus != models.OwnershipManagedVerified &&
			vpnCfg.OwnershipStatus != models.OwnershipManagedPending &&
			vpnCfg.OwnershipStatus != "" {
			c.JSON(http.StatusConflict, gin.H{"error": "VPN is " + vpnCfg.OwnershipStatus + " — re-deploy or re-import required"})
			return
		}
	}

	// For imported VPNs with no Gator-managed filter rules (legacy setup),
	// just toggle the status flag.
	if vpnCfg.OPNsenseFilterUUIDs == "" {
		vpnCfg.RoutingApplied = false
		vpnCfg.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
		vpnCfg.OwnershipStatus = models.OwnershipManagedPending
		vpnCfg.DriftReason = ""
		if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist status"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "deactivated"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	// 1. Clean up app-specific routing rules (GATOR_APP_* rules on OPNsense).
	// Must happen before switching the main filter rule, so the firewall apply
	// covers both the app rule deletions and the gateway switch in one pass.
	appWarnings := h.cleanupAppRoutingRules(ctx, api, vpnCfg.ID)

	// 2. Switch this VPN's filter rules to default gateway instead of disabling them.
	// Disabling would break traffic when the rule has a non-default destination
	// (e.g. !RFC1918_Networks) — there'd be no pass rule left for internet traffic.
	for _, uuid := range splitUUIDs(vpnCfg.OPNsenseFilterUUIDs) {
		if err := safeSetFilterRuleGateway(ctx, api, uuid, ""); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to update filter rule gateway: " + err.Error()})
			return
		}
	}

	// 3. Apply firewall.
	if err := applyFirewallWithRollback(ctx, api); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to apply firewall: " + err.Error()})
		return
	}

	vpnCfg.RoutingApplied = false
	vpnCfg.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	vpnCfg.OwnershipStatus = models.OwnershipManagedPending
	vpnCfg.DriftReason = ""
	if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deactivated but failed to persist status"})
		return
	}

	resp := gin.H{"status": "deactivated"}
	if len(appWarnings) > 0 {
		resp["app_route_warnings"] = appWarnings
	}
	h.asyncReconcile(ctx)
	c.JSON(http.StatusOK, resp)
}

// SetSourceInterfaces updates which OPNsense interfaces this VPN routes traffic from.
func (h *VPNHandler) SetSourceInterfaces(c *gin.Context) {
	ctx := c.Request.Context()

	vpnCfg, ok := h.getVPNByParam(c)
	if !ok {
		return
	}

	var body struct {
		Interfaces []string `json:"interfaces"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(body.Interfaces) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one interface is required"})
		return
	}

	vpnCfg.SourceInterfaces = body.Interfaces
	if err := h.store.SaveSimpleVPNConfig(ctx, *vpnCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save source interfaces"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated", "source_interfaces": body.Interfaces})
}

func sanitizeGatewayName(input string) string {
	// OPNsense gateway names: a-zA-Z0-9_ only, max 32 chars.
	var b strings.Builder
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else if r == '-' || r == '.' || r == ' ' {
			b.WriteRune('_')
		}
	}
	name := b.String()
	if name == "" {
		name = "gator_vpn_gw"
	}
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

func parsePrefix(cidr string) (string, error) {
	for _, entry := range strings.Split(cidr, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			continue
		}
		return prefix.Addr().String(), nil
	}
	return "", fmt.Errorf("no valid CIDR in %q", cidr)
}

func validateCIDRList(value string) bool {
	for _, cidr := range strings.Split(value, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return false
		}
	}
	return true
}

func validateSimpleVPNConfig(cfg models.SimpleVPNConfig) error {
	if !validateCIDRList(cfg.LocalCIDR) {
		return errInvalidField("local_cidr", "must be a valid CIDR (example: 10.73.211.155/32)")
	}

	if !validateCIDRList(cfg.RemoteCIDR) {
		return errInvalidField("remote_cidr", "must be a valid CIDR (example: 0.0.0.0/0)")
	}

	if cfg.Endpoint == "" {
		return errInvalidField("endpoint", "is required (example: 198.51.100.1:51820)")
	}

	if cfg.PreSharedKey != "" && len(cfg.PreSharedKey) < 8 {
		return errInvalidField("pre_shared_key", "must be at least 8 characters")
	}

	return nil
}

func expectOPNsenseResult(payload map[string]any, allowed ...string) error {
	result, _ := payload["result"].(string)
	if result == "" {
		return nil
	}

	for _, allowedValue := range allowed {
		if result == allowedValue {
			return nil
		}
	}

	if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
		return fmt.Errorf("%s", message)
	}
	if validations, ok := payload["validations"]; ok {
		return fmt.Errorf("validation error: %v", validations)
	}

	return fmt.Errorf("result=%s", result)
}

func extractUUID(payload map[string]any) (string, error) {
	if uuid, ok := payload["uuid"].(string); ok && strings.TrimSpace(uuid) != "" {
		return uuid, nil
	}
	return "", fmt.Errorf("missing uuid")
}

func splitEndpoint(endpoint string) (string, string, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "", "", fmt.Errorf("endpoint is required")
	}

	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		if strings.Count(trimmed, ":") == 1 {
			parts := strings.SplitN(trimmed, ":", 2)
			host = strings.TrimSpace(parts[0])
			port = strings.TrimSpace(parts[1])
		} else {
			return "", "", fmt.Errorf("endpoint must be host:port")
		}
	}

	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if host == "" || port == "" {
		return "", "", fmt.Errorf("endpoint must be host:port")
	}

	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return "", "", fmt.Errorf("endpoint port must be between 1 and 65535")
	}

	return host, port, nil
}

func sanitizeOPNsenseName(input, fallback string) string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		raw = fallback
	}

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}

		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}

	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = fallback
	}
	if len(name) > 64 {
		name = name[:64]
	}

	return name
}

func deriveTunnelGateway(localCIDR string) string {
	for _, cidr := range strings.Split(localCIDR, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		addr := prefix.Addr()
		if !addr.Is4() {
			continue
		}

		raw := addr.As4()
		value := uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
		if value == 0 {
			continue
		}
		value--

		gateway := [4]byte{
			byte(value >> 24),
			byte(value >> 16),
			byte(value >> 8),
			byte(value),
		}

		return netip.AddrFrom4(gateway).String()
	}
	return ""
}

// deriveMonitorIP picks a tunnel-reachable IP for gateway health monitoring.
// It uses the VPN's DNS server (which is inside the tunnel) as the monitor target.
// Falls back to empty string if no suitable IP is found.
func deriveMonitorIP(dns string, ipVersion string) string {
	wantV4 := ipVersion != "ipv6"
	for _, entry := range strings.Split(dns, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			continue
		}
		if wantV4 && addr.Is4() {
			return addr.String()
		}
		if !wantV4 && addr.Is6() {
			return addr.String()
		}
	}
	return ""
}

func ensureWireGuardPeer(
	ctx context.Context,
	apiClient *opnsenseAPIClient,
	peerPayload map[string]any,
	peerPublicKey string,
	preferredUUID string,
) (string, bool, error) {
	if preferredUUID != "" {
		if exists, _ := wireGuardPeerExists(ctx, apiClient, preferredUUID); exists {
			resp, err := apiClient.Post(ctx, "/api/wireguard/client/set_client/"+preferredUUID, peerPayload)
			if err == nil {
				if err := expectOPNsenseResult(resp, "saved", "ok"); err == nil {
					return preferredUUID, false, nil
				}
			}
		}
	}

	if existingUUID, ok := findPeerByPublicKey(ctx, apiClient, peerPublicKey); ok {
		resp, err := apiClient.Post(ctx, "/api/wireguard/client/set_client/"+existingUUID, peerPayload)
		if err != nil {
			return "", false, err
		}
		if err := expectOPNsenseResult(resp, "saved", "ok"); err != nil {
			return "", false, err
		}
		return existingUUID, false, nil
	}

	resp, err := apiClient.Post(ctx, "/api/wireguard/client/add_client", peerPayload)
	if err != nil {
		return "", false, err
	}
	if err := expectOPNsenseResult(resp, "saved", "ok"); err != nil {
		if isUniquePublicKeyError(err) {
			if existingUUID, ok := findPeerByPublicKey(ctx, apiClient, peerPublicKey); ok {
				setResp, setErr := apiClient.Post(ctx, "/api/wireguard/client/set_client/"+existingUUID, peerPayload)
				if setErr != nil {
					return "", false, setErr
				}
				if setErr := expectOPNsenseResult(setResp, "saved", "ok"); setErr != nil {
					return "", false, setErr
				}
				return existingUUID, false, nil
			}
		}
		return "", false, err
	}

	uuid, err := extractUUID(resp)
	if err != nil {
		return "", false, err
	}

	return uuid, true, nil
}

func ensureWireGuardServer(
	ctx context.Context,
	apiClient *opnsenseAPIClient,
	serverPayload map[string]any,
	serverName string,
	privateKey string,
	preferredUUID string,
	peerUUID string,
) (string, bool, error) {
	if preferredUUID != "" {
		if exists, _ := wireGuardServerExists(ctx, apiClient, preferredUUID); exists {
			mergePeerIntoServerPayload(ctx, apiClient, preferredUUID, serverPayload, peerUUID)
			resp, err := apiClient.Post(ctx, "/api/wireguard/server/set_server/"+preferredUUID, serverPayload)
			if err == nil {
				if err := expectOPNsenseResult(resp, "saved", "ok"); err == nil {
					return preferredUUID, false, nil
				}
			}
		}
	}

	if existingUUID, ok := findServerByNameOrPrivateKey(ctx, apiClient, serverName, privateKey); ok {
		mergePeerIntoServerPayload(ctx, apiClient, existingUUID, serverPayload, peerUUID)
		resp, err := apiClient.Post(ctx, "/api/wireguard/server/set_server/"+existingUUID, serverPayload)
		if err != nil {
			return "", false, err
		}
		if err := expectOPNsenseResult(resp, "saved", "ok"); err != nil {
			return "", false, err
		}
		return existingUUID, false, nil
	}

	resp, err := apiClient.Post(ctx, "/api/wireguard/server/add_server", serverPayload)
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

func wireGuardPeerExists(ctx context.Context, apiClient *opnsenseAPIClient, uuid string) (bool, error) {
	resp, err := apiClient.Get(ctx, "/api/wireguard/client/get_client/"+uuid)
	if err != nil {
		return false, err
	}
	peer := asMap(resp["client"])
	return len(peer) > 0, nil
}

func wireGuardServerExists(ctx context.Context, apiClient *opnsenseAPIClient, uuid string) (bool, error) {
	resp, err := apiClient.Get(ctx, "/api/wireguard/server/get_server/"+uuid)
	if err != nil {
		return false, err
	}
	server := asMap(resp["server"])
	return len(server) > 0, nil
}

func findPeerByPublicKey(ctx context.Context, apiClient *opnsenseAPIClient, pubkey string) (string, bool) {
	if strings.TrimSpace(pubkey) == "" {
		return "", false
	}

	resp, err := apiClient.Post(ctx, "/api/wireguard/client/search_client", map[string]any{})
	if err != nil {
		return "", false
	}

	for _, rowRaw := range asSlice(resp["rows"]) {
		row := asMap(rowRaw)
		if asString(row["pubkey"]) == pubkey {
			uuid := asString(row["uuid"])
			if uuid != "" {
				return uuid, true
			}
		}
	}

	return "", false
}

func findServerByNameOrPrivateKey(
	ctx context.Context,
	apiClient *opnsenseAPIClient,
	name string,
	privateKey string,
) (string, bool) {
	resp, err := apiClient.Post(ctx, "/api/wireguard/server/search_server", map[string]any{})
	if err != nil {
		return "", false
	}

	for _, rowRaw := range asSlice(resp["rows"]) {
		row := asMap(rowRaw)
		uuid := asString(row["uuid"])
		if uuid == "" {
			continue
		}
		if name != "" && asString(row["name"]) == name {
			return uuid, true
		}
		if privateKey != "" && asString(row["privkey"]) == privateKey {
			return uuid, true
		}
	}

	return "", false
}

func mergePeerIntoServerPayload(
	ctx context.Context,
	apiClient *opnsenseAPIClient,
	serverUUID string,
	serverPayload map[string]any,
	peerUUID string,
) {
	currentPeers := []string{peerUUID}

	resp, err := apiClient.Get(ctx, "/api/wireguard/server/get_server/"+serverUUID)
	if err == nil {
		server := asMap(resp["server"])
		currentPeers = appendUnique(currentPeers, splitCSVValues(asString(server["peers"]))...)
	}

	payloadServer := asMap(serverPayload["server"])
	payloadServer["peers"] = strings.Join(currentPeers, ",")
	serverPayload["server"] = payloadServer
}

func splitCSVValues(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func appendUnique(items []string, candidates ...string) []string {
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		items = append(items, candidate)
	}

	return items
}

func isUniquePublicKeyError(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "public keys should be unique") ||
		(strings.Contains(text, "pubkey") && strings.Contains(text, "unique"))
}

func errInvalidField(field, msg string) error {
	return &validationError{field: field, message: msg}
}

type validationError struct {
	field   string
	message string
}

func (e *validationError) Error() string {
	return e.field + " " + e.message
}
