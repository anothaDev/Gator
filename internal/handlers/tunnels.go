package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/anothaDev/gator/internal/models"
	"github.com/anothaDev/gator/internal/sshclient"
	"golang.org/x/crypto/curve25519"
)

// TunnelStore defines the storage interface for site tunnels.
type TunnelStore interface {
	CreateSiteTunnel(ctx context.Context, t models.SiteTunnel) (int64, error)
	SaveSiteTunnel(ctx context.Context, t models.SiteTunnel) error
	GetSiteTunnel(ctx context.Context, id int64) (*models.SiteTunnel, error)
	ListSiteTunnels(ctx context.Context) ([]*models.SiteTunnel, error)
	DeleteSiteTunnel(ctx context.Context, id int64) error
	NextTunnelSubnet(ctx context.Context) (subnet, firewallIP, remoteIP string, err error)
	GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error)
	GetActiveInstanceID(ctx context.Context) (int64, error)
}

// TunnelHandler handles all site tunnel API endpoints.
type TunnelHandler struct {
	store TunnelStore
}

// NewTunnelHandler creates a TunnelHandler.
func NewTunnelHandler(store TunnelStore) *TunnelHandler {
	return &TunnelHandler{store: store}
}

// ─── CRUD ────────────────────────────────────────────────────────

// ListTunnels returns all tunnels for the active instance.
func (h *TunnelHandler) ListTunnels(c *gin.Context) {
	tunnels, err := h.store.ListSiteTunnels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	statuses := make([]models.SiteTunnelStatus, 0, len(tunnels))
	for _, t := range tunnels {
		statuses = append(statuses, tunnelStatusFromModel(t))
	}
	c.JSON(http.StatusOK, gin.H{"tunnels": statuses})
}

// GetTunnel returns a single tunnel by ID.
func (h *TunnelHandler) GetTunnel(c *gin.Context) {
	t, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, tunnelDetailFromModel(t))
}

// CreateTunnel creates a new tunnel record (does not deploy).
func (h *TunnelHandler) CreateTunnel(c *gin.Context) {
	var req models.SiteTunnel
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Auto-assign subnet if not provided.
	if req.TunnelSubnet == "" {
		subnet, fwIP, remoteIP, err := h.store.NextTunnelSubnet(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign subnet: " + err.Error()})
			return
		}
		req.TunnelSubnet = subnet
		req.FirewallIP = fwIP
		req.RemoteIP = remoteIP
	}

	// Defaults.
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}
	if req.ListenPort == 0 {
		req.ListenPort = 51820
	}
	if req.Keepalive == 0 {
		req.Keepalive = 25
	}
	// Don't default RemoteWGInterface here — let deployStepConfigureRemote
	// discover the next available interface on the remote via findAvailableWGInterface.
	// Defaulting to "wg0" would clobber existing WG configs on the remote.
	req.Status = "pending"

	// Validate fields that will be used in shell commands / OPNsense API.
	if err := validateTunnelFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := h.store.CreateSiteTunnel(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	t, _ := h.store.GetSiteTunnel(ctx, id)
	c.JSON(http.StatusCreated, tunnelDetailFromModel(t))
}

// SaveTunnel updates an existing tunnel's configuration.
func (h *TunnelHandler) SaveTunnel(c *gin.Context) {
	existing, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}

	var req models.SiteTunnel
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Preserve ALL fields that shouldn't be overwritten by the request.
	// The frontend edit form only sends name/description/ssh fields;
	// everything else must be carried forward from the existing record.
	req.ID = existing.ID
	req.InstanceID = existing.InstanceID
	req.FirewallPrivateKey = existing.FirewallPrivateKey
	req.FirewallPublicKey = existing.FirewallPublicKey
	req.RemotePrivateKey = existing.RemotePrivateKey
	req.RemotePublicKey = existing.RemotePublicKey
	req.OPNsensePeerUUID = existing.OPNsensePeerUUID
	req.OPNsenseServerUUID = existing.OPNsenseServerUUID
	req.Deployed = existing.Deployed
	req.Status = existing.Status
	req.RemoteWGInterface = existing.RemoteWGInterface
	req.OriginalRemoteHost = existing.OriginalRemoteHost
	req.OriginalSSHPort = existing.OriginalSSHPort
	req.SSHPhase = existing.SSHPhase
	req.SSHSocketWasActive = existing.SSHSocketWasActive
	req.UFWWasActive = existing.UFWWasActive

	// Preserve tunnel addressing if not sent (frontend edit form doesn't include these).
	if req.TunnelSubnet == "" {
		req.TunnelSubnet = existing.TunnelSubnet
	}
	if req.FirewallIP == "" {
		req.FirewallIP = existing.FirewallIP
	}
	if req.RemoteIP == "" {
		req.RemoteIP = existing.RemoteIP
	}
	if req.ListenPort == 0 {
		req.ListenPort = existing.ListenPort
	}
	if req.Keepalive == 0 {
		req.Keepalive = existing.Keepalive
	}

	// Preserve SSH credentials if not sent (frontend strips them).
	if req.SSHPrivateKey == "" {
		req.SSHPrivateKey = existing.SSHPrivateKey
	}
	if req.SSHPassword == "" {
		req.SSHPassword = existing.SSHPassword
	}

	// Validate fields that will be used in shell commands.
	if err := validateTunnelFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.SaveSiteTunnel(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	t, _ := h.store.GetSiteTunnel(c.Request.Context(), req.ID)
	c.JSON(http.StatusOK, tunnelDetailFromModel(t))
}

// DeleteTunnel removes a tunnel. If deployed, tears down both sides first.
func (h *TunnelHandler) DeleteTunnel(c *gin.Context) {
	t, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	var warnings []string

	if t.Deployed {
		warnings = h.teardownTunnel(ctx, t)
	} else if t.RemoteWGInterface != "" {
		// The deploy partially succeeded (remote was configured) but never
		// fully completed. Clean up just the remote WG interface.
		w := h.cleanupRemoteWGInterface(ctx, t)
		warnings = append(warnings, w...)
	}

	if err := h.store.DeleteSiteTunnel(ctx, t.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "warnings": warnings})
}

// NextSubnet returns the next available tunnel subnet.
func (h *TunnelHandler) NextSubnet(c *gin.Context) {
	subnet, fwIP, remoteIP, err := h.store.NextTunnelSubnet(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tunnel_subnet": subnet,
		"firewall_ip":   fwIP,
		"remote_ip":     remoteIP,
	})
}

// ─── SSH Test ────────────────────────────────────────────────────

// TestSSH tests SSH connectivity to a remote host.
func (h *TunnelHandler) TestSSH(c *gin.Context) {
	var req struct {
		Host       string `json:"host" binding:"required"`
		Port       int    `json:"port"`
		User       string `json:"user"`
		PrivateKey string `json:"private_key"`
		Password   string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client, err := sshclient.Connect(sshclient.Config{
		Host:       req.Host,
		Port:       req.Port,
		User:       req.User,
		PrivateKey: req.PrivateKey,
		Password:   req.Password,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer client.Close()

	info, err := client.TestConnection(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "info": info})
}

// ─── Import from OPNsense ────────────────────────────────────────

// ImportTunnel imports an existing tunnel discovered on OPNsense into Gator's DB.
// The user provides SSH creds; Gator cross-checks the remote WG config.
func (h *TunnelHandler) ImportTunnel(c *gin.Context) {
	var req struct {
		// From discovery
		Name       string `json:"name" binding:"required"`
		ServerUUID string `json:"server_uuid" binding:"required"`
		PeerUUID   string `json:"peer_uuid" binding:"required"`
		LocalCIDR  string `json:"local_cidr" binding:"required"` // Firewall tunnel address
		Endpoint   string `json:"endpoint" binding:"required"`   // remote_host:port
		ListenPort int    `json:"listen_port"`
		PeerPubKey string `json:"peer_pubkey"`

		// SSH credentials for the remote side
		SSHHost       string `json:"ssh_host"`
		SSHPort       int    `json:"ssh_port"`
		SSHUser       string `json:"ssh_user"`
		SSHPrivateKey string `json:"ssh_private_key"`
		SSHPassword   string `json:"ssh_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Parse the endpoint to extract remote host.
	remoteHost := req.SSHHost
	if remoteHost == "" {
		// Derive from the WG endpoint (e.g. "203.0.113.10:51820" → "203.0.113.10").
		remoteHost = req.Endpoint
		if idx := strings.LastIndex(remoteHost, ":"); idx != -1 {
			remoteHost = remoteHost[:idx]
		}
	}

	// Parse firewall tunnel IP from LocalCIDR (e.g. "10.200.200.2/24" → "10.200.200.2").
	firewallIP := req.LocalCIDR
	if idx := strings.Index(firewallIP, "/"); idx != -1 {
		firewallIP = firewallIP[:idx]
	}

	// Infer the subnet and remote IP from the firewall IP.
	// e.g. firewallIP = "10.200.200.2" → subnet = "10.200.200.0/24"
	tunnelSubnet := ""
	remoteIP := ""
	parts := strings.Split(firewallIP, ".")
	if len(parts) == 4 {
		tunnelSubnet = fmt.Sprintf("%s.%s.%s.0/24", parts[0], parts[1], parts[2])
		// Guess remote IP: if firewall is .2, remote is .1 (common convention).
		if parts[3] == "2" {
			remoteIP = fmt.Sprintf("%s.%s.%s.1", parts[0], parts[1], parts[2])
		} else if parts[3] == "1" {
			remoteIP = fmt.Sprintf("%s.%s.%s.2", parts[0], parts[1], parts[2])
		}
	}

	listenPort := req.ListenPort
	if listenPort == 0 {
		listenPort = 51820
	}

	// Fetch the private key from OPNsense (not sent to frontend by discovery).
	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "firewall config not available"})
		return
	}
	api := newOPNsenseAPIClient(*firewallCfg)

	fwPrivateKey := ""
	serverDetail, err := api.Get(ctx, "/api/wireguard/server/get_server/"+req.ServerUUID)
	if err == nil {
		server := asMap(serverDetail["server"])
		fwPrivateKey = asString(server["privkey"])
	}

	// Build the tunnel record.
	tunnel := models.SiteTunnel{
		Name:               req.Name,
		RemoteHost:         remoteHost,
		SSHPort:            req.SSHPort,
		SSHUser:            req.SSHUser,
		SSHPrivateKey:      req.SSHPrivateKey,
		SSHPassword:        req.SSHPassword,
		TunnelSubnet:       tunnelSubnet,
		FirewallIP:         firewallIP,
		RemoteIP:           remoteIP,
		ListenPort:         listenPort,
		Keepalive:          25,
		FirewallPrivateKey: fwPrivateKey,
		FirewallPublicKey:  "", // Will compute if we have the private key
		RemotePublicKey:    req.PeerPubKey,
		OPNsensePeerUUID:   req.PeerUUID,
		OPNsenseServerUUID: req.ServerUUID,
		Deployed:           true,
		Status:             "deployed",
	}

	// Compute firewall public key from private key.
	if fwPrivateKey != "" {
		if pub, err := publicKeyFromPrivate(fwPrivateKey); err == nil {
			tunnel.FirewallPublicKey = pub
		}
	}

	// Defaults.
	if tunnel.SSHPort == 0 {
		tunnel.SSHPort = 22
	}
	if tunnel.SSHUser == "" {
		tunnel.SSHUser = "root"
	}

	// ─── SSH cross-check (optional but valuable) ─────────────
	var crossCheck map[string]any
	if tunnel.SSHPrivateKey != "" || tunnel.SSHPassword != "" {
		crossCheck = h.crossCheckRemote(ctx, &tunnel)
	}

	// Resolve instance ID.
	instanceID, _ := h.store.GetActiveInstanceID(ctx)
	tunnel.InstanceID = instanceID

	id, err := h.store.CreateSiteTunnel(ctx, tunnel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save tunnel: " + err.Error()})
		return
	}

	saved, _ := h.store.GetSiteTunnel(ctx, id)
	result := gin.H{
		"status": "imported",
		"tunnel": tunnelDetailFromModel(saved),
	}
	if crossCheck != nil {
		result["cross_check"] = crossCheck
	}
	c.JSON(http.StatusCreated, result)
}

// DiscoverTunnels returns only tunnel-type entries from OPNsense discovery.
// GET /api/tunnels/discover
func (h *TunnelHandler) DiscoverTunnels(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OPNsense setup required"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)
	discovered, err := discoverExistingVPNs(ctx, api)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "discovery failed: " + err.Error()})
		return
	}

	// Filter for tunnel type only + exclude already-imported ones.
	// Filter by PeerUUID (not ServerUUID) so that multiple peers on the same
	// WG server instance are each independently discoverable/importable.
	existing, _ := h.store.ListSiteTunnels(ctx)
	importedPeers := make(map[string]bool)
	for _, t := range existing {
		if t.OPNsensePeerUUID != "" {
			importedPeers[t.OPNsensePeerUUID] = true
		}
	}

	var tunnels []DiscoveredVPN
	for _, d := range discovered {
		if d.Type == "tunnel" && !importedPeers[d.PeerUUID] {
			tunnels = append(tunnels, d)
		}
	}

	c.JSON(http.StatusOK, gin.H{"tunnels": tunnels})
}

// crossCheckRemote connects via SSH and reads the remote WG config to verify consistency.
func (h *TunnelHandler) crossCheckRemote(ctx context.Context, t *models.SiteTunnel) map[string]any {
	result := map[string]any{}

	client, err := h.connectSSH(t)
	if err != nil {
		result["ssh_ok"] = false
		result["ssh_error"] = err.Error()
		return result
	}
	defer client.Close()
	result["ssh_ok"] = true

	// Get system info.
	info, _ := client.TestConnection(ctx)
	if info != nil {
		result["hostname"] = info["hostname"]
		result["os"] = info["os"]
	}

	// Check 1: Is WireGuard package installed?
	_, exitCode, _ := client.RunQuiet(ctx, "which wg")
	result["wg_installed"] = exitCode == 0

	// Check 2: Are there WireGuard config files?
	output, err := client.Run(ctx, "ls /etc/wireguard/*.conf 2>/dev/null")
	confFiles := []string{}
	if err == nil && strings.TrimSpace(output) != "" {
		confFiles = strings.Split(strings.TrimSpace(output), "\n")
	}
	result["wg_configs"] = confFiles
	result["wg_configured"] = len(confFiles) > 0

	if len(confFiles) == 0 {
		return result
	}

	// Read each config and look for our firewall's public key.
	var matchedIface string
	var matchedConfig string
	for _, confPath := range confFiles {
		confPath = strings.TrimSpace(confPath)
		if confPath == "" {
			continue
		}
		content, err := client.Run(ctx, fmt.Sprintf("cat %q", confPath))
		if err != nil {
			continue
		}

		// Check if this config has our firewall's public key as a peer.
		if t.FirewallPublicKey != "" && strings.Contains(content, t.FirewallPublicKey) {
			// Extract the interface name from the path.
			// "/etc/wireguard/wg0.conf" → "wg0"
			iface := confPath
			iface = strings.TrimPrefix(iface, "/etc/wireguard/")
			iface = strings.TrimSuffix(iface, ".conf")
			matchedIface = iface
			matchedConfig = content
			break
		}
	}

	if matchedIface != "" {
		result["matched_interface"] = matchedIface
		result["config_matches_firewall"] = true
		t.RemoteWGInterface = matchedIface

		// Parse the remote config for cross-check details.
		remoteIP := parseINIField(matchedConfig, "Address")
		if remoteIP != "" {
			// Strip CIDR mask for comparison.
			if idx := strings.Index(remoteIP, "/"); idx != -1 {
				remoteIP = remoteIP[:idx]
			}
			if t.RemoteIP == "" {
				t.RemoteIP = remoteIP
			}
			result["remote_address"] = remoteIP
			result["address_matches"] = (remoteIP == t.RemoteIP)
		}

		// Extract the remote's private key to derive its public key for verification.
		remotePrivKey := parseINIField(matchedConfig, "PrivateKey")
		if remotePrivKey != "" {
			t.RemotePrivateKey = remotePrivKey
			if pub, err := publicKeyFromPrivate(remotePrivKey); err == nil {
				if t.RemotePublicKey != "" {
					result["pubkey_matches"] = (pub == t.RemotePublicKey)
				} else {
					t.RemotePublicKey = pub
				}
			}
		}

		// Check the WG interface status.
		wgShow, err := client.Run(ctx, fmt.Sprintf("wg show %s", matchedIface))
		if err == nil && strings.Contains(wgShow, "latest handshake") {
			result["handshake_active"] = true
		}
	} else if t.FirewallPublicKey == "" {
		result["config_matches_firewall"] = "unknown (no firewall public key to match)"
	} else {
		result["config_matches_firewall"] = false
		result["note"] = "No remote WG config contains the firewall's public key"
	}

	return result
}

// publicKeyFromPrivate derives a WireGuard public key from a base64-encoded private key.
func publicKeyFromPrivate(privKeyB64 string) (string, error) {
	privBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privKeyB64))
	if err != nil {
		return "", err
	}
	if len(privBytes) != 32 {
		return "", fmt.Errorf("invalid key length: %d", len(privBytes))
	}
	pubBytes, err := curve25519.X25519(privBytes, curve25519.Basepoint)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pubBytes), nil
}

// parseINIField extracts a value from an INI-style WireGuard config.
// e.g. parseINIField(conf, "Address") → "10.200.200.1/24"
func parseINIField(config, field string) string {
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, field) {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// ─── Deploy ──────────────────────────────────────────────────────

// DeployTunnel orchestrates the full tunnel setup on both sides.
// Called step-by-step from the frontend deploy modal.
func (h *TunnelHandler) DeployStep(c *gin.Context) {
	t, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}

	var req struct {
		Step string `json:"step" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	switch req.Step {
	case "generate-keys":
		h.deployStepGenerateKeys(c, ctx, t)
	case "configure-remote":
		h.deployStepConfigureRemote(c, ctx, t)
	case "configure-firewall":
		h.deployStepConfigureFirewall(c, ctx, t)
	case "migrate-ssh":
		h.deployStepMigrateSSH(c, ctx, t)
	case "verify":
		h.deployStepVerify(c, ctx, t)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown step: " + req.Step})
	}
}

func (h *TunnelHandler) deployStepGenerateKeys(c *gin.Context, ctx context.Context, t *models.SiteTunnel) {
	fwPub := t.FirewallPublicKey

	// Only generate firewall keys if we don't already have them (fresh tunnel).
	// Imported tunnels already have firewall keys from OPNsense.
	if t.FirewallPrivateKey == "" {
		fwPriv, pub, err := generateWireGuardKeyPair()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "generate firewall keys: " + err.Error()})
			return
		}
		t.FirewallPrivateKey = fwPriv
		t.FirewallPublicKey = pub
		fwPub = pub
	}

	// Always generate remote keys (fresh or imported — the remote needs a keypair).
	remPriv, remPub, err := generateWireGuardKeyPair()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate remote keys: " + err.Error()})
		return
	}
	t.RemotePrivateKey = remPriv
	t.RemotePublicKey = remPub

	if err := h.store.SaveSiteTunnel(ctx, *t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save keys: " + err.Error()})
		return
	}

	// NOTE: We do NOT update the OPNsense peer's pubkey here. The pubkey update
	// is deferred to deployStepConfigureRemote — AFTER the remote VPS has been
	// configured with the new keys. This avoids a chicken-and-egg problem: if
	// SSH access is via the tunnel IP (after migration), updating the peer's
	// pubkey here would break the tunnel before configure-remote can reach the
	// VPS to install the new keys.

	c.JSON(http.StatusOK, gin.H{
		"status":              "ok",
		"firewall_public_key": fwPub,
		"remote_public_key":   remPub,
	})
}

func (h *TunnelHandler) deployStepConfigureRemote(c *gin.Context, ctx context.Context, t *models.SiteTunnel) {
	if t.RemotePrivateKey == "" || t.FirewallPublicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keys not generated — run generate-keys step first"})
		return
	}

	client, err := h.connectSSH(t)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "ssh connect: " + err.Error()})
		return
	}
	defer client.Close()

	// 1. Install WireGuard if not present.
	installed, err := h.ensureWireGuardInstalled(ctx, client)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "install wireguard: " + err.Error()})
		return
	}

	// 2. Determine WG interface name.
	// If the tunnel already has a known interface (from a previous deploy/import), reuse it.
	// Otherwise find the next available one.
	wgIface := t.RemoteWGInterface
	if wgIface == "" {
		wgIface, err = h.findAvailableWGInterface(ctx, client)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "detect wg interface: " + err.Error()})
			return
		}
	}
	t.RemoteWGInterface = wgIface

	// 3. Build the WireGuard config.
	// Detect LAN subnets from OPNsense so the remote can route to the home network.
	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load firewall config"})
		return
	}
	api := newOPNsenseAPIClient(*firewallCfg)
	lanSubnets := getLANSubnets(ctx, api)

	conf := buildRemoteWGConfig(t, lanSubnets)
	confPath := fmt.Sprintf("/etc/wireguard/%s.conf", wgIface)

	// 4. Write config file.
	if err := client.WriteFile(ctx, confPath, conf, "0600"); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "write wg config: " + err.Error()})
		return
	}

	// 5. Enable and apply WireGuard config.
	if _, err := client.Run(ctx, fmt.Sprintf("systemctl enable wg-quick@%s 2>/dev/null", wgIface)); err != nil {
		log.Printf("[configureRemote] systemctl enable failed (non-fatal): %v", err)
	}

	// Check if the WG interface is already up. If so, we need a careful
	// coordination dance because SSH may be going through the tunnel:
	//   1. Schedule `wg syncconf` on the remote with a delay (background task)
	//   2. Return from SSH while the tunnel still works
	//   3. Update OPNsense peer pubkey via LAN API (fast, not through tunnel)
	//   4. Remote's delayed task fires — applies new key
	//   5. Both sides have new key, handshake succeeds, tunnel recovers
	//
	// If the interface is down, just use `wg-quick up` (first-time setup).
	ifaceUp, _ := client.Run(ctx, fmt.Sprintf("ip link show %s 2>/dev/null", wgIface))
	ifaceExists := strings.Contains(ifaceUp, "state UP") || strings.Contains(ifaceUp, "state UNKNOWN")

	if ifaceExists {
		log.Printf("[configureRemote] %s already up, scheduling deferred wg syncconf", wgIface)

		// Schedule the key swap to run in 3 seconds as a detached background task.
		// The sleep gives us time to close SSH and update OPNsense first.
		syncCmd := fmt.Sprintf(
			"nohup sh -c 'sleep 3 && wg-quick strip %s | wg syncconf %s /dev/stdin' >/dev/null 2>&1 &",
			confPath, wgIface,
		)
		if _, err := client.Run(ctx, syncCmd); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "schedule wg syncconf: " + err.Error()})
			return
		}

		// 6. Allow WireGuard port through firewall (if ufw is active).
		h.openWGPort(ctx, client, t.ListenPort)

		// Close the SSH session (defer client.Close() handles this) so we're
		// not holding a connection through the tunnel during the key swap.
		client.Close()

		// 7. Update OPNsense peer pubkey NOW — before the remote applies the
		// new key in ~3 seconds. This is an API call to the LAN, not tunnel.
		if t.OPNsensePeerUUID != "" {
			h.updateOPNsensePeerPubkey(ctx, api, t)
		}
	} else {
		// Interface doesn't exist — bring it up fresh (first-time setup).
		log.Printf("[configureRemote] %s not up, using wg-quick up", wgIface)
		if _, err := client.Run(ctx, fmt.Sprintf("wg-quick up %s", wgIface)); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "start wg interface: " + err.Error()})
			return
		}

		// 6. Allow WireGuard port through firewall (if ufw is active).
		h.openWGPort(ctx, client, t.ListenPort)

		// 7. Update OPNsense peer pubkey after remote is running.
		if t.OPNsensePeerUUID != "" {
			h.updateOPNsensePeerPubkey(ctx, api, t)
		}
	}

	// Save the interface name.
	if err := h.store.SaveSiteTunnel(ctx, *t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save tunnel: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":              "ok",
		"wg_interface":        wgIface,
		"wireguard_installed": installed,
	})
}

func (h *TunnelHandler) deployStepConfigureFirewall(c *gin.Context, ctx context.Context, t *models.SiteTunnel) {
	if t.FirewallPrivateKey == "" || t.RemotePublicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keys not generated"})
		return
	}

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "firewall config not available or not OPNsense"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)
	tunnelName := "GATOR_s2s_" + sanitizeName(t.Name)

	// 1. Create WireGuard peer (the remote VPS).
	// tunneladdress = the remote's tunnel IP (AllowedIPs for this peer).
	// Some OPNsense versions require this to be non-empty.
	peerTunnelAddr := t.RemoteIP + "/32"
	peerPayload := map[string]any{
		"client": map[string]any{
			"enabled":       "1",
			"name":          tunnelName + "_peer",
			"pubkey":        t.RemotePublicKey,
			"serveraddress": t.RemoteHost,
			"serverport":    fmt.Sprintf("%d", t.ListenPort),
			"tunneladdress": peerTunnelAddr,
			"keepalive":     fmt.Sprintf("%d", t.Keepalive),
		},
	}

	peerResp, err := api.Post(ctx, "/api/wireguard/client/add_client", peerPayload)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "create WG peer: " + err.Error()})
		return
	}
	peerUUID, _ := extractUUID(peerResp)
	if peerUUID == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "create WG peer: no UUID returned"})
		return
	}
	t.OPNsensePeerUUID = peerUUID

	// 2. Create WireGuard local instance (server).
	// AllowedIPs for the peer: the remote tunnel IP + any remote subnets.
	allowedIPs := t.RemoteIP + "/32"

	serverPayload := map[string]any{
		"server": map[string]any{
			"enabled":       "1",
			"name":          tunnelName,
			"privkey":       t.FirewallPrivateKey,
			"port":          "", // No listen port — firewall initiates
			"tunneladdress": t.FirewallIP + "/24",
			"peers":         peerUUID,
			"disableroutes": "1",
			"gateway":       t.RemoteIP,
			"dns":           "",
		},
	}
	_ = allowedIPs // used in peer's tunneladdress if needed

	serverResp, err := api.Post(ctx, "/api/wireguard/server/add_server", serverPayload)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "create WG instance: " + err.Error()})
		return
	}
	serverUUID, _ := extractUUID(serverResp)
	if serverUUID == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "create WG instance: no UUID returned"})
		return
	}
	t.OPNsenseServerUUID = serverUUID

	// 3. Reconfigure WireGuard service.
	if _, err := api.Post(ctx, "/api/wireguard/service/reconfigure", map[string]any{}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "reconfigure wireguard: " + err.Error()})
		return
	}

	// 4. Ensure WAN rule allows inbound UDP on the WG listen port.
	// Without this, WireGuard handshake responses from remote peers can't reach
	// the firewall when the remote initiates (or for any non-stateful return path).
	h.ensureWANWireGuardRule(ctx, api, t.ListenPort)

	// 5. Kill stale pf states for this peer's remote IP. This prevents poisoned
	// inbound states from WAN rules with reply-to from hijacking WG traffic.
	h.killStaleWGStates(ctx, api, t)

	// 6. Mark as deployed.
	t.Deployed = true
	t.Status = "deployed"
	if err := h.store.SaveSiteTunnel(ctx, *t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save tunnel: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"peer_uuid":   peerUUID,
		"server_uuid": serverUUID,
	})
}

func (h *TunnelHandler) deployStepVerify(c *gin.Context, ctx context.Context, t *models.SiteTunnel) {
	if !t.Deployed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tunnel not deployed"})
		return
	}

	result := map[string]any{"status": "ok"}
	var issues []string

	// Check remote side via SSH.
	client, err := h.connectSSH(t)
	if err != nil {
		issues = append(issues, "ssh connect failed: "+err.Error())
	} else {
		defer client.Close()

		// Check WireGuard interface is up.
		wgShow, err := client.Run(ctx, fmt.Sprintf("wg show %s", t.RemoteWGInterface))
		if err != nil {
			issues = append(issues, "wg show failed: "+err.Error())
		} else {
			result["remote_wg_show"] = wgShow
			if strings.Contains(wgShow, "latest handshake") {
				result["remote_handshake"] = true
			}
		}

		// Ping firewall tunnel IP from remote.
		pingOut, _ := client.Run(ctx, fmt.Sprintf("ping -c 2 -W 3 %s", t.FirewallIP))
		result["remote_ping"] = pingOut
		if strings.Contains(pingOut, " 0% packet loss") || strings.Contains(pingOut, "2 received") {
			result["remote_ping_ok"] = true
		}
	}

	// Check OPNsense side — ping remote tunnel IP.
	firewallCfg, _ := h.store.GetFirewallConfig(ctx)
	if firewallCfg != nil && firewallCfg.Type == "opnsense" {
		api := newOPNsenseAPIClient(*firewallCfg)
		// OPNsense doesn't have a direct ping API easily accessible, but we can check WG status.
		wgStatus, err := api.Get(ctx, "/api/wireguard/service/show")
		if err == nil {
			result["firewall_wg_status"] = wgStatus
		}
	}

	if len(issues) > 0 {
		result["issues"] = issues
	}

	c.JSON(http.StatusOK, result)
}

// ─── Migrate SSH ─────────────────────────────────────────────────
//
// The "safe" step: makes sshd ALSO listen on the tunnel IP while keeping the
// public listener (0.0.0.0:22) as a safety net. After this, Gator's SSH
// connection switches to the tunnel IP so future commands travel inside WG.
//
// Hardened flow:
//  1. Preflight — verify sshd, systemctl, ss exist
//  2. Record original state (ssh.socket, host/port)
//  3. Read existing sshd drop-in (if any) for backup
//  4. Schedule dead man's switch (auto-reverts in 2 min if we don't cancel)
//  5. Write new config + disable ssh.socket if active
//  6. Validate with sshd -t — restore backup if invalid
//  7. Restart sshd
//  8. Verify via NEW SSH connection from Gator to tunnel IP (proof command)
//  9. Persist DB switch + mark phase = dual_listen_verified
//  10. Cancel rollback timer
//
// If anything fails after step 4, the rollback timer will fire and restore
// the previous config automatically. Gator crash = safe.
func (h *TunnelHandler) deployStepMigrateSSH(c *gin.Context, ctx context.Context, t *models.SiteTunnel) {
	if t.RemoteIP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no remote tunnel IP — deploy WireGuard first"})
		return
	}

	client, err := h.connectSSH(t)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "ssh connect: " + err.Error()})
		return
	}
	defer client.Close()

	// 1. Preflight.
	if missing := sshdPreflight(ctx, client, false); len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "remote missing required tools: " + strings.Join(missing, ", "),
			"missing": missing,
		})
		return
	}

	// 2. Record original state before any mutation.
	oldHost := t.RemoteHost
	oldPort := t.SSHPort
	_, socketCode, _ := client.RunQuiet(ctx, "systemctl is-active ssh.socket")
	socketWasActive := socketCode == 0

	// Mark pending — if Gator crashes after this, we know we were mid-flight.
	if !t.SSHIsMigrated() {
		t.OriginalRemoteHost = oldHost
		t.OriginalSSHPort = oldPort
		t.SSHSocketWasActive = socketWasActive
	}
	t.SSHPhase = models.SSHPhaseDualListenPending
	_ = h.store.SaveSiteTunnel(ctx, *t)

	sshPort := 22
	tunnelIP := t.RemoteIP

	// 3. Read existing drop-in config for backup (may not exist yet).
	prevConfig, _ := client.Run(ctx, "cat /etc/ssh/sshd_config.d/gator-tunnel.conf 2>/dev/null || true")
	if strings.TrimSpace(prevConfig) == "" {
		prevConfig = ""
	}

	// 4. Schedule dead man's switch — auto-reverts in 2 minutes.
	if err := scheduleSSHRollback(ctx, client, prevConfig, socketWasActive); err != nil {
		// Non-fatal — log it but continue. Better to attempt the migration
		// without a rollback than to block entirely.
		// (systemd-run may not be available on all hosts)
	}

	// 5. Write new sshd drop-in: keep 0.0.0.0:22 (safety net) + add tunnel:22.
	dropIn := fmt.Sprintf(
		"# Managed by Gator — SSH access via WireGuard tunnel\n"+
			"ListenAddress 0.0.0.0:22\n"+
			"ListenAddress %s:%d\n",
		tunnelIP, sshPort,
	)

	client.Run(ctx, "mkdir -p /etc/ssh/sshd_config.d")

	// Ensure sshd_config includes the drop-in directory.
	output, _ := client.Run(ctx, "grep -c 'Include.*/etc/ssh/sshd_config.d' /etc/ssh/sshd_config 2>/dev/null")
	if strings.TrimSpace(output) == "0" || strings.TrimSpace(output) == "" {
		client.Run(ctx, `sed -i '1i Include /etc/ssh/sshd_config.d/*.conf' /etc/ssh/sshd_config`)
	}

	if err := client.WriteFile(ctx, "/etc/ssh/sshd_config.d/gator-tunnel.conf", dropIn, "0644"); err != nil {
		t.SSHPhase = models.SSHPhaseError
		_ = h.store.SaveSiteTunnel(ctx, *t)
		c.JSON(http.StatusBadGateway, gin.H{"error": "write sshd drop-in: " + err.Error()})
		return
	}

	// Disable ssh.socket if active (it overrides ListenAddress directives).
	if socketWasActive {
		client.Run(ctx, "systemctl disable --now ssh.socket 2>/dev/null")
		client.Run(ctx, "systemctl enable ssh.service 2>/dev/null")
	}

	// 6+7. Validate config and restart. Restores backup on validation failure.
	if err := safeRestartSSHD(ctx, client, prevConfig); err != nil {
		t.SSHPhase = models.SSHPhaseError
		_ = h.store.SaveSiteTunnel(ctx, *t)
		cancelSSHRollback(ctx, client)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// Brief pause for sshd to fully bind.
	client.Run(ctx, "sleep 1")

	// 8. Real verification — open a NEW SSH connection from Gator to tunnel:22.
	if err := h.verifySSHViaTunnel(t); err != nil {
		// Listener may be up but not reachable from Gator. Don't persist.
		// Rollback timer is still armed and will revert in ~2 min.
		t.SSHPhase = models.SSHPhaseError
		_ = h.store.SaveSiteTunnel(ctx, *t)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":    "tunnel SSH verification failed: " + err.Error(),
			"rollback": "armed — will auto-revert in ~2 minutes if not fixed",
		})
		return
	}

	// 9. All good — persist the switch.
	t.RemoteHost = tunnelIP
	t.SSHPort = sshPort
	t.SSHPhase = models.SSHPhaseDualListenVerified
	if err := h.store.SaveSiteTunnel(ctx, *t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save tunnel: " + err.Error()})
		return
	}

	// 10. Cancel rollback — we're committed.
	cancelSSHRollback(ctx, client)

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"old_ssh": fmt.Sprintf("%s:%d", oldHost, oldPort),
		"new_ssh": fmt.Sprintf("%s:%d", tunnelIP, sshPort),
	})
}

// ─── Status ──────────────────────────────────────────────────────

// TunnelStatus fetches live status from both sides.
func (h *TunnelHandler) TunnelStatus(c *gin.Context) {
	t, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}

	status := tunnelStatusFromModel(t)

	if !t.Deployed {
		c.JSON(http.StatusOK, status)
		return
	}

	// Query remote via SSH for live WG data.
	client, err := h.connectSSH(t)
	if err != nil {
		status.Status = "unreachable"
		c.JSON(http.StatusOK, status)
		return
	}
	defer client.Close()

	ctx := c.Request.Context()
	wgShow, err := client.Run(ctx, fmt.Sprintf("wg show %s", t.RemoteWGInterface))
	if err == nil {
		status.Handshake = parseWGField(wgShow, "latest handshake")
		status.TransferRx = parseWGField(wgShow, "transfer")
		if txLine := parseWGField(wgShow, "transfer"); txLine != "" {
			parts := strings.SplitN(txLine, ",", 2)
			if len(parts) == 2 {
				status.TransferRx = strings.TrimSpace(parts[0])
				status.TransferTx = strings.TrimSpace(parts[1])
			} else {
				status.TransferRx = txLine
			}
		}
		status.RemoteReachable = true
	}

	c.JSON(http.StatusOK, status)
}

// ─── Teardown ────────────────────────────────────────────────────

// TeardownTunnel removes tunnel configuration from both sides without deleting the DB record.
func (h *TunnelHandler) TeardownTunnel(c *gin.Context) {
	t, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}

	if !t.Deployed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tunnel not deployed"})
		return
	}

	warnings := h.teardownTunnel(c.Request.Context(), t)

	t.Deployed = false
	t.Status = "pending"
	t.OPNsensePeerUUID = ""
	t.OPNsenseServerUUID = ""
	t.FirewallPrivateKey = ""
	t.FirewallPublicKey = ""
	t.RemotePrivateKey = ""
	t.RemotePublicKey = ""
	_ = h.store.SaveSiteTunnel(c.Request.Context(), *t)

	c.JSON(http.StatusOK, gin.H{"status": "torn_down", "warnings": warnings})
}

// RestartTunnel restarts WireGuard on both sides.
func (h *TunnelHandler) RestartTunnel(c *gin.Context) {
	t, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}

	if !t.Deployed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tunnel not deployed"})
		return
	}

	ctx := c.Request.Context()
	var warnings []string

	// Restart remote side.
	client, err := h.connectSSH(t)
	if err != nil {
		warnings = append(warnings, "ssh connect failed: "+err.Error())
	} else {
		defer client.Close()
		if _, err := client.Run(ctx, fmt.Sprintf("systemctl restart wg-quick@%s", t.RemoteWGInterface)); err != nil {
			warnings = append(warnings, "remote restart failed: "+err.Error())
		}
	}

	// Restart OPNsense WireGuard service.
	firewallCfg, _ := h.store.GetFirewallConfig(ctx)
	if firewallCfg != nil && firewallCfg.Type == "opnsense" {
		api := newOPNsenseAPIClient(*firewallCfg)
		if _, err := api.Post(ctx, "/api/wireguard/service/reconfigure", map[string]any{}); err != nil {
			warnings = append(warnings, "firewall reconfigure failed: "+err.Error())
		}
		// Kill stale pf states so the tunnel comes up cleanly.
		h.killStaleWGStates(ctx, api, t)
	}

	c.JSON(http.StatusOK, gin.H{"status": "restarted", "warnings": warnings})
}

// ─── Cross-check ─────────────────────────────────────────────────

// CrossCheck runs the remote cross-check on an existing tunnel without modifying it.
func (h *TunnelHandler) CrossCheck(c *gin.Context) {
	t, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}

	if t.SSHPrivateKey == "" && t.SSHPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no SSH credentials — edit the tunnel to add SSH key or password"})
		return
	}

	// Run cross-check on a copy so we don't mutate the stored tunnel.
	check := *t
	result := h.crossCheckRemote(c.Request.Context(), &check)
	c.JSON(http.StatusOK, gin.H{"cross_check": result})
}

// ─── SSH Lockdown ────────────────────────────────────────────────
//
// The "no going back via public IP" step. Removes the 0.0.0.0:22 safety net
// that Migrate SSH left in place, so sshd ONLY listens on the tunnel IP.
// Also sets up ufw to deny port 22 from the internet entirely.
//
// After this, the only way in is through the WireGuard tunnel. If the tunnel
// dies and you can't fix it remotely, you'll need the Hetzner/provider console.
//
// Hardened flow:
//  1. Require ssh_phase = dual_listen_verified
//  2. Preflight (including ufw)
//  3. Record ufw state, schedule rollback
//  4. Write tunnel-only sshd config, validate with sshd -t
//  5. Apply ufw rules (WG port first!)
//  6. Restart sshd
//  7. Verify via NEW SSH connection through tunnel
//  8. Persist phase + cancel rollback
func (h *TunnelHandler) LockdownSSH(c *gin.Context) {
	t, ok := h.getTunnelByParam(c)
	if !ok {
		return
	}

	if !t.Deployed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tunnel not deployed — deploy first"})
		return
	}
	if t.SSHPhase != models.SSHPhaseDualListenVerified {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "SSH must be migrated and verified before locking down",
			"ssh_phase": t.SSHPhase,
		})
		return
	}

	ctx := c.Request.Context()
	client, err := h.connectSSH(t)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "ssh connect: " + err.Error()})
		return
	}
	defer client.Close()

	// 2. Preflight — need ufw for this step.
	if missing := sshdPreflight(ctx, client, true); len(missing) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "remote missing required tools: " + strings.Join(missing, ", "),
			"missing": missing,
		})
		return
	}

	// 3. Record ufw state + read current drop-in for rollback.
	_, ufwCode, _ := client.RunQuiet(ctx, "ufw status | grep -q 'Status: active'")
	t.UFWWasActive = ufwCode == 0

	prevConfig, _ := client.Run(ctx, "cat /etc/ssh/sshd_config.d/gator-tunnel.conf 2>/dev/null || true")
	if strings.TrimSpace(prevConfig) == "" {
		prevConfig = ""
	}

	// Mark pending.
	t.SSHPhase = models.SSHPhaseTunnelOnlyPending
	_ = h.store.SaveSiteTunnel(ctx, *t)

	// Schedule dead man's switch.
	_ = scheduleSSHRollback(ctx, client, prevConfig, t.SSHSocketWasActive)

	// 4. Write tunnel-only sshd config.
	sshPort := t.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	lockedDropIn := fmt.Sprintf(
		"# Managed by Gator — SSH locked to WireGuard tunnel only\n"+
			"ListenAddress %s:%d\n",
		t.RemoteIP, sshPort,
	)

	var results []string
	if err := client.WriteFile(ctx, "/etc/ssh/sshd_config.d/gator-tunnel.conf", lockedDropIn, "0644"); err != nil {
		t.SSHPhase = models.SSHPhaseError
		_ = h.store.SaveSiteTunnel(ctx, *t)
		cancelSSHRollback(ctx, client)
		c.JSON(http.StatusBadGateway, gin.H{"error": "write sshd drop-in: " + err.Error()})
		return
	}

	// 5. Apply ufw rules BEFORE restart — order matters:
	//   a. Allow WG port (so tunnel survives ufw enable)
	//   b. Allow SSH from tunnel subnet
	//   c. Deny public SSH
	//   d. Enable ufw
	tunnelSubnet := t.TunnelSubnet
	cmds := []string{
		fmt.Sprintf("ufw allow %d/udp comment 'Gator WireGuard tunnel'", t.ListenPort),
		fmt.Sprintf("ufw allow from %s to any port 22 proto tcp comment 'Gator tunnel SSH'", tunnelSubnet),
		"ufw deny 22/tcp",
		"ufw --force enable",
	}
	for _, cmd := range cmds {
		out, err := client.Run(ctx, cmd)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: FAILED — %s", cmd, err.Error()))
		} else {
			results = append(results, fmt.Sprintf("%s: %s", cmd, out))
		}
	}

	// 6. Validate and restart sshd.
	if err := safeRestartSSHD(ctx, client, prevConfig); err != nil {
		t.SSHPhase = models.SSHPhaseError
		_ = h.store.SaveSiteTunnel(ctx, *t)
		results = append(results, "sshd restart: FAILED — "+err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "results": results})
		return
	}
	results = append(results, "sshd restarted (tunnel-only listener)")

	client.Run(ctx, "sleep 1")

	// 7. Verify via new tunnel connection.
	if err := h.verifySSHViaTunnel(t); err != nil {
		t.SSHPhase = models.SSHPhaseError
		_ = h.store.SaveSiteTunnel(ctx, *t)
		results = append(results, "tunnel SSH verification: FAILED — "+err.Error())
		c.JSON(http.StatusBadGateway, gin.H{
			"error":    "lockdown verification failed: " + err.Error(),
			"rollback": "armed — will auto-revert in ~2 minutes",
			"results":  results,
		})
		return
	}

	// 8. All verified — persist and cancel rollback.
	t.SSHPhase = models.SSHPhaseTunnelOnlyVerified
	if err := h.store.SaveSiteTunnel(ctx, *t); err != nil {
		results = append(results, "save ssh_phase: "+err.Error())
	}
	cancelSSHRollback(ctx, client)

	c.JSON(http.StatusOK, gin.H{"status": "locked_down", "results": results})
}

// ─── Helpers ─────────────────────────────────────────────────────

func (h *TunnelHandler) getTunnelByParam(c *gin.Context) (*models.SiteTunnel, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tunnel id"})
		return nil, false
	}
	t, err := h.store.GetSiteTunnel(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	if t == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return nil, false
	}
	// Verify the tunnel belongs to the active instance (prevent cross-instance access).
	activeID, _ := h.store.GetActiveInstanceID(c.Request.Context())
	if activeID != 0 && t.InstanceID != activeID {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return nil, false
	}
	return t, true
}

func (h *TunnelHandler) connectSSH(t *models.SiteTunnel) (*sshclient.Client, error) {
	return sshclient.Connect(sshclient.Config{
		Host:       t.RemoteHost,
		Port:       t.SSHPort,
		User:       t.SSHUser,
		PrivateKey: t.SSHPrivateKey,
		Password:   t.SSHPassword,
	})
}

// ─── SSH Hardening Helpers ───────────────────────────────────────

// sshdPreflight checks that the remote has the minimum tools needed for SSH
// mutation. Returns a list of missing capabilities. Empty list = good to go.
func sshdPreflight(ctx context.Context, client *sshclient.Client, needUFW bool) []string {
	var missing []string
	checks := []struct {
		cmd  string
		name string
	}{
		{"which sshd", "sshd"},
		{"sshd -t 2>&1 || true", "sshd -t (config validation)"}, // just testing it runs
		{"which systemctl", "systemctl"},
		{"which ss", "ss"},
	}
	if needUFW {
		checks = append(checks, struct {
			cmd  string
			name string
		}{"which ufw", "ufw"})
	}
	for _, c := range checks {
		_, code, _ := client.RunQuiet(ctx, c.cmd)
		if code != 0 {
			missing = append(missing, c.name)
		}
	}
	return missing
}

// validateSSHConfig runs `sshd -t` to check the config is valid before we
// restart. Returns nil if valid, error with sshd's output if not.
func validateSSHConfig(ctx context.Context, client *sshclient.Client) error {
	out, code, err := client.RunQuiet(ctx, "sshd -t 2>&1")
	if err != nil {
		return fmt.Errorf("sshd -t failed to run: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("sshd config invalid: %s", out)
	}
	return nil
}

// safeRestartSSHD validates the sshd config, then restarts. If validation fails,
// restores the backup config (if provided) and returns an error without restarting.
// backupContent can be empty if there's nothing to restore (first-time write).
func safeRestartSSHD(ctx context.Context, client *sshclient.Client, backupContent string) error {
	if err := validateSSHConfig(ctx, client); err != nil {
		// Config is bad — restore backup if we have one.
		if backupContent != "" {
			client.WriteFile(ctx, "/etc/ssh/sshd_config.d/gator-tunnel.conf", backupContent, "0644")
		} else {
			// No backup means this was the first write — just remove the bad config.
			client.Run(ctx, "rm -f /etc/ssh/sshd_config.d/gator-tunnel.conf")
		}
		return fmt.Errorf("sshd config validation failed, restored previous config: %w", err)
	}
	_, err := client.Run(ctx, "systemctl restart sshd 2>/dev/null || systemctl restart ssh 2>/dev/null")
	return err
}

const sshRollbackUnit = "gator-ssh-rollback"

// scheduleSSHRollback sets up a dead man's switch on the remote. If we don't
// cancel it within 2 minutes, it restores the previous sshd config and restarts.
// This protects against Gator crashing mid-mutation or losing connectivity.
func scheduleSSHRollback(ctx context.Context, client *sshclient.Client, prevConfig string, socketWasActive bool) error {
	// Build the rollback script inline. It:
	//   1. Restores the previous sshd drop-in (or removes it if there was none)
	//   2. Restores ssh.socket if it was active before
	//   3. Restarts sshd
	//   4. Writes a marker so we know rollback fired
	var restoreCmd string
	if prevConfig == "" {
		restoreCmd = "rm -f /etc/ssh/sshd_config.d/gator-tunnel.conf"
	} else {
		// Escape single quotes in the config for the shell heredoc.
		escaped := strings.ReplaceAll(prevConfig, "'", "'\\''")
		restoreCmd = fmt.Sprintf("cat > /etc/ssh/sshd_config.d/gator-tunnel.conf << 'GATOR_ROLLBACK'\n%s\nGATOR_ROLLBACK", escaped)
	}

	socketRestore := ""
	if socketWasActive {
		socketRestore = "systemctl enable --now ssh.socket 2>/dev/null; "
	}

	script := fmt.Sprintf(
		"%s; %ssystemctl restart sshd 2>/dev/null || systemctl restart ssh 2>/dev/null; echo 'gator ssh rollback fired' > /tmp/gator-ssh-rollback.log",
		restoreCmd, socketRestore,
	)

	// Use systemd-run with a 2-minute timer. The --unit name lets us cancel it later.
	cmd := fmt.Sprintf(
		`systemd-run --on-active=120s --unit=%s --description="Gator SSH rollback" /bin/bash -c '%s'`,
		sshRollbackUnit, strings.ReplaceAll(script, "'", "'\\''"),
	)
	_, err := client.Run(ctx, cmd)
	return err
}

// cancelSSHRollback stops the dead man's switch after successful verification.
func cancelSSHRollback(ctx context.Context, client *sshclient.Client) {
	// Stop both the timer and the service (in case it's queued).
	client.Run(ctx, fmt.Sprintf("systemctl stop %s.timer 2>/dev/null; systemctl stop %s.service 2>/dev/null", sshRollbackUnit, sshRollbackUnit))
}

// verifySSHViaTunnel opens a brand-new SSH connection to the tunnel IP and
// runs a proof command. This is the real test — not just checking listeners,
// but proving TCP reachable + auth works + shell executes.
func (h *TunnelHandler) verifySSHViaTunnel(t *models.SiteTunnel) error {
	tunnelClient, err := sshclient.Connect(sshclient.Config{
		Host:       t.RemoteIP,
		Port:       22,
		User:       t.SSHUser,
		PrivateKey: t.SSHPrivateKey,
		Password:   t.SSHPassword,
	})
	if err != nil {
		return fmt.Errorf("new SSH connection to %s:22 failed: %w", t.RemoteIP, err)
	}
	defer tunnelClient.Close()

	out, err := tunnelClient.Run(context.Background(), "echo gator-verify && hostname")
	if err != nil {
		return fmt.Errorf("proof command failed on %s: %w", t.RemoteIP, err)
	}
	if !strings.Contains(out, "gator-verify") {
		return fmt.Errorf("proof command returned unexpected output: %s", out)
	}
	return nil
}

// cleanupRemoteWGInterface removes a WireGuard interface from the remote server
// when a tunnel was partially deployed (remote configured, but full deploy never completed).
// This prevents stale interfaces from blocking future deploys.
func (h *TunnelHandler) cleanupRemoteWGInterface(ctx context.Context, t *models.SiteTunnel) []string {
	var warnings []string
	client, err := h.connectSSH(t)
	if err != nil {
		warnings = append(warnings, "ssh connect for cleanup: "+err.Error())
		return warnings
	}
	defer client.Close()

	iface := t.RemoteWGInterface
	if _, err := client.Run(ctx, fmt.Sprintf("systemctl stop wg-quick@%s 2>/dev/null; systemctl disable wg-quick@%s 2>/dev/null", iface, iface)); err != nil {
		warnings = append(warnings, "stop remote WG: "+err.Error())
	}
	client.Run(ctx, fmt.Sprintf("wg-quick down %s 2>/dev/null", iface))
	confPath := fmt.Sprintf("/etc/wireguard/%s.conf", iface)
	if _, err := client.Run(ctx, fmt.Sprintf("rm -f %s", confPath)); err != nil {
		warnings = append(warnings, "remove remote config: "+err.Error())
	}

	// Close WG port in ufw (best-effort).
	h.closeWGPort(ctx, client, t.ListenPort)

	return warnings
}

func (h *TunnelHandler) teardownTunnel(ctx context.Context, t *models.SiteTunnel) []string {
	var warnings []string

	// 1. Tear down remote side via SSH.
	// ORDER IS CRITICAL: revert SSH before tearing down WG, otherwise we lose
	// access if SSH was migrated through the tunnel.
	client, err := h.connectSSH(t)
	if err != nil {
		warnings = append(warnings, "ssh connect for teardown: "+err.Error())
	} else {
		defer client.Close()

		// 1a. Undo SSH lockdown (ufw deny rules) — must happen first so port 22
		// becomes accessible from the internet again before we tear down the tunnel.
		if t.SSHIsLockedDown() {
			w := h.teardownSSHLockdown(ctx, client, t)
			warnings = append(warnings, w...)
		}

		// 1b. Revert SSH migration (remove sshd drop-in, restart sshd).
		// After this, SSH listens on the default port 22 on all interfaces again.
		if t.SSHIsMigrated() {
			w := h.teardownSSHMigration(ctx, client, t)
			warnings = append(warnings, w...)
		}

		// 1c. Close WG port in ufw (if active).
		h.closeWGPort(ctx, client, t.ListenPort)

		// 1d. Bring down WG interface and remove config.
		iface := t.RemoteWGInterface
		if _, err := client.Run(ctx, fmt.Sprintf("systemctl stop wg-quick@%s 2>/dev/null; systemctl disable wg-quick@%s 2>/dev/null", iface, iface)); err != nil {
			warnings = append(warnings, "stop remote WG: "+err.Error())
		}
		// Also bring down via wg-quick in case systemd unit doesn't exist.
		client.Run(ctx, fmt.Sprintf("wg-quick down %s 2>/dev/null", iface))
		confPath := fmt.Sprintf("/etc/wireguard/%s.conf", iface)
		if _, err := client.Run(ctx, fmt.Sprintf("rm -f %s", confPath)); err != nil {
			warnings = append(warnings, "remove remote config: "+err.Error())
		}
	}

	// 1e. Restore original SSH host/port in the DB so Gator can reach the remote
	// again after teardown (via public IP on port 22).
	if t.SSHIsMigrated() && t.OriginalRemoteHost != "" {
		t.RemoteHost = t.OriginalRemoteHost
		t.SSHPort = t.OriginalSSHPort
		if t.SSHPort == 0 {
			t.SSHPort = 22
		}
		t.SSHPhase = models.SSHPhasePublic
		t.OriginalRemoteHost = ""
		t.OriginalSSHPort = 0
		t.SSHSocketWasActive = false
		t.UFWWasActive = false
		if err := h.store.SaveSiteTunnel(ctx, *t); err != nil {
			warnings = append(warnings, "restore original SSH host: "+err.Error())
		}
	} else if t.SSHPhase != models.SSHPhasePublic {
		t.SSHPhase = models.SSHPhasePublic
		if err := h.store.SaveSiteTunnel(ctx, *t); err != nil {
			warnings = append(warnings, "reset ssh_phase: "+err.Error())
		}
	}

	// 2. Tear down OPNsense side.
	firewallCfg, _ := h.store.GetFirewallConfig(ctx)
	if firewallCfg != nil && firewallCfg.Type == "opnsense" {
		api := newOPNsenseAPIClient(*firewallCfg)
		needsReconfigure := false

		// Handle the WG server instance — only delete if this is the last peer.
		// If the server has other peers, just unlink this peer and its tunnel address.
		if t.OPNsenseServerUUID != "" {
			deleted, w := h.teardownServerOrUnlinkPeer(ctx, api, t)
			warnings = append(warnings, w...)
			needsReconfigure = true
			if deleted {
				// Server was deleted entirely — peer was removed with it or separately.
				if t.OPNsensePeerUUID != "" {
					if _, err := api.Post(ctx, "/api/wireguard/client/del_client/"+t.OPNsensePeerUUID, map[string]any{}); err != nil {
						warnings = append(warnings, "delete WG peer: "+err.Error())
					}
				}
			} else {
				// Server was kept (has other peers). Delete only this peer.
				if t.OPNsensePeerUUID != "" {
					if _, err := api.Post(ctx, "/api/wireguard/client/del_client/"+t.OPNsensePeerUUID, map[string]any{}); err != nil {
						warnings = append(warnings, "delete WG peer: "+err.Error())
					}
				}
			}
		} else if t.OPNsensePeerUUID != "" {
			// No server UUID recorded — just delete the peer.
			if _, err := api.Post(ctx, "/api/wireguard/client/del_client/"+t.OPNsensePeerUUID, map[string]any{}); err != nil {
				warnings = append(warnings, "delete WG peer: "+err.Error())
			}
			needsReconfigure = true
		}

		// Reconfigure WireGuard.
		if needsReconfigure {
			if _, err := api.Post(ctx, "/api/wireguard/service/reconfigure", map[string]any{}); err != nil {
				warnings = append(warnings, "reconfigure wireguard: "+err.Error())
			}
		}

		// Remove the WAN WireGuard allow rule for this tunnel's listen port.
		if w := h.removeWANWireGuardRule(ctx, api, t.ListenPort); w != "" {
			warnings = append(warnings, w)
		}
	}

	return warnings
}

// teardownServerOrUnlinkPeer handles the WG server instance during teardown.
// If the server has other peers besides the one being torn down, it removes only
// this peer from the server's peer list and this tunnel's address from the server's
// tunnel addresses. Returns (true, warnings) if the server was deleted entirely,
// or (false, warnings) if the server was kept with the remaining peers.
func (h *TunnelHandler) teardownServerOrUnlinkPeer(
	ctx context.Context,
	api *opnsenseAPIClient,
	t *models.SiteTunnel,
) (deleted bool, warnings []string) {
	// Fetch current server state.
	resp, err := api.Get(ctx, "/api/wireguard/server/get_server/"+t.OPNsenseServerUUID)
	if err != nil {
		// Server might already be gone — treat as deleted.
		warnings = append(warnings, "get WG server: "+err.Error())
		return true, warnings
	}

	server := asMap(resp["server"])
	currentPeers := extractSelectedUUIDs(server["peers"])
	currentAddrs := splitCSVValues(extractSelectedValue(server["tunneladdress"]))

	// Filter out this tunnel's peer UUID.
	var remainingPeers []string
	for _, p := range currentPeers {
		if p != t.OPNsensePeerUUID {
			remainingPeers = append(remainingPeers, p)
		}
	}

	if len(remainingPeers) == 0 {
		// This was the last peer — delete the entire server instance.
		if _, err := api.Post(ctx, "/api/wireguard/server/del_server/"+t.OPNsenseServerUUID, map[string]any{}); err != nil {
			warnings = append(warnings, "delete WG server: "+err.Error())
		}
		return true, warnings
	}

	// Server has other peers — update it instead of deleting.
	// Remove this tunnel's address from the server's tunnel addresses.
	var remainingAddrs []string
	tunnelPrefix := cidrPrefix24(t.FirewallIP)
	for _, addr := range currentAddrs {
		if tunnelPrefix != "" && cidrPrefix24(addr) == tunnelPrefix {
			continue // This address belongs to the tunnel being torn down.
		}
		remainingAddrs = append(remainingAddrs, addr)
	}

	// Build the update payload with remaining peers and addresses.
	updatePayload := map[string]any{
		"server": map[string]any{
			"peers":         strings.Join(remainingPeers, ","),
			"tunneladdress": strings.Join(remainingAddrs, ","),
		},
	}

	if _, err := api.Post(ctx, "/api/wireguard/server/set_server/"+t.OPNsenseServerUUID, updatePayload); err != nil {
		warnings = append(warnings, "update WG server (unlink peer): "+err.Error())
	}

	return false, warnings
}

func (h *TunnelHandler) ensureWireGuardInstalled(ctx context.Context, client *sshclient.Client) (bool, error) {
	// Check if wg command exists.
	_, exitCode, err := client.RunQuiet(ctx, "which wg")
	if err != nil {
		return false, err
	}
	if exitCode == 0 {
		return false, nil // already installed
	}

	// Detect package manager and install.
	if _, code, _ := client.RunQuiet(ctx, "which apt-get"); code == 0 {
		if _, err := client.Run(ctx, "apt-get update -qq && apt-get install -y -qq wireguard"); err != nil {
			return false, fmt.Errorf("apt install wireguard: %w", err)
		}
		return true, nil
	}
	if _, code, _ := client.RunQuiet(ctx, "which dnf"); code == 0 {
		if _, err := client.Run(ctx, "dnf install -y wireguard-tools"); err != nil {
			return false, fmt.Errorf("dnf install wireguard: %w", err)
		}
		return true, nil
	}
	if _, code, _ := client.RunQuiet(ctx, "which yum"); code == 0 {
		if _, err := client.Run(ctx, "yum install -y wireguard-tools"); err != nil {
			return false, fmt.Errorf("yum install wireguard: %w", err)
		}
		return true, nil
	}

	return false, fmt.Errorf("no supported package manager found (apt/dnf/yum)")
}

func (h *TunnelHandler) findAvailableWGInterface(ctx context.Context, client *sshclient.Client) (string, error) {
	for i := 0; i < 10; i++ {
		iface := fmt.Sprintf("wg%d", i)
		exists, err := client.FileExists(ctx, fmt.Sprintf("/etc/wireguard/%s.conf", iface))
		if err != nil {
			return "", err
		}
		if !exists {
			return iface, nil
		}
	}
	return "", fmt.Errorf("no available wg interface (wg0-wg9 all taken)")
}

// openWGPort whitelists the WireGuard UDP port in ufw. Only runs if ufw is
// already active — on fresh boxes where ufw is inactive, this is a no-op.
// That's fine during deploy, but Lock SSH must add the rule unconditionally
// before calling `ufw --force enable` (see LockdownSSH).
func (h *TunnelHandler) openWGPort(ctx context.Context, client *sshclient.Client, port int) {
	_, code, _ := client.RunQuiet(ctx, "ufw status | grep -q 'Status: active'")
	if code != 0 {
		return
	}
	client.Run(ctx, fmt.Sprintf("ufw allow %d/udp comment 'Gator WireGuard tunnel'", port))
}

// closeWGPort removes the WG UDP port from ufw during tunnel cleanup.
func (h *TunnelHandler) closeWGPort(ctx context.Context, client *sshclient.Client, port int) {
	_, code, _ := client.RunQuiet(ctx, "ufw status | grep -q 'Status: active'")
	if code != 0 {
		return
	}
	client.Run(ctx, fmt.Sprintf("ufw delete allow %d/udp 2>/dev/null", port))
}

// teardownSSHLockdown undoes LockdownSSH: removes the ufw deny + tunnel-only
// allow rules so port 22 is open from the internet again. Called during full
// tunnel teardown. Does NOT remove the WG port rule — closeWGPort handles that.
func (h *TunnelHandler) teardownSSHLockdown(ctx context.Context, client *sshclient.Client, t *models.SiteTunnel) []string {
	var warnings []string

	_, exitCode, _ := client.RunQuiet(ctx, "which ufw")
	if exitCode != 0 {
		return warnings
	}

	// Remove deny first so SSH is reachable again immediately.
	if _, err := client.Run(ctx, "ufw delete deny 22/tcp 2>/dev/null"); err != nil {
		warnings = append(warnings, "remove ufw deny 22: "+err.Error())
	}
	if t.TunnelSubnet != "" {
		cmd := fmt.Sprintf("ufw delete allow from %s to any port 22 proto tcp 2>/dev/null", t.TunnelSubnet)
		if _, err := client.Run(ctx, cmd); err != nil {
			warnings = append(warnings, "remove ufw tunnel SSH allow: "+err.Error())
		}
	}

	// If ufw was inactive before lockdown enabled it, disable it again.
	// Don't leave a firewall running that the user didn't configure.
	if !t.UFWWasActive {
		if _, err := client.Run(ctx, "ufw --force disable 2>/dev/null"); err != nil {
			warnings = append(warnings, "disable ufw (was inactive before): "+err.Error())
		}
	}

	return warnings
}

// teardownSSHMigration undoes deployStepMigrateSSH: removes the sshd drop-in
// so sshd falls back to its default config (all interfaces, port 22). Also
// re-enables ssh.socket on Ubuntu 22.04+ since we disabled it during migration
// (without it, sshd won't start on reboot).
func (h *TunnelHandler) teardownSSHMigration(ctx context.Context, client *sshclient.Client, t *models.SiteTunnel) []string {
	var warnings []string

	if _, err := client.Run(ctx, "rm -f /etc/ssh/sshd_config.d/gator-tunnel.conf"); err != nil {
		warnings = append(warnings, "remove sshd drop-in: "+err.Error())
	}

	// Restore ssh.socket ONLY if it was active before we disabled it.
	// Don't blindly re-enable it on hosts that never had it.
	if t.SSHSocketWasActive {
		client.Run(ctx, "systemctl enable --now ssh.socket 2>/dev/null")
		client.Run(ctx, "systemctl enable ssh.service 2>/dev/null")
	}

	if _, err := client.Run(ctx, "systemctl restart sshd 2>/dev/null || systemctl restart ssh 2>/dev/null"); err != nil {
		warnings = append(warnings, "restart sshd after revert: "+err.Error())
	}

	return warnings
}

// getLANSubnets queries OPNsense to discover LAN-type subnets that should be
// routable through site-to-site tunnels (e.g. "192.168.1.0/24").
func getLANSubnets(ctx context.Context, api *opnsenseAPIClient) []string {
	resp, err := api.Get(ctx, "/api/interfaces/overview/interfaces_info/0")
	if err != nil {
		return nil
	}
	var subnets []string
	seen := map[string]bool{}
	for _, raw := range asSlice(resp["rows"]) {
		iface := asMap(raw)
		identifier := asString(iface["identifier"])
		device := asString(iface["device"])

		// Skip WAN, WireGuard, loopback, and unassigned interfaces.
		if identifier == "wan" || identifier == "lo0" || identifier == "" {
			continue
		}
		if strings.HasPrefix(device, "wg") {
			continue
		}

		addr4 := asString(iface["addr4"])
		if addr4 == "" || !strings.Contains(addr4, "/") {
			continue
		}
		// Convert host address to network CIDR (e.g. "192.168.1.1/24" → "192.168.1.0/24").
		_, ipNet, err := net.ParseCIDR(addr4)
		if err != nil {
			continue
		}
		subnet := ipNet.String()
		if !seen[subnet] {
			seen[subnet] = true
			subnets = append(subnets, subnet)
		}
	}
	return subnets
}

// ensureWANWireGuardRule creates a WAN inbound rule for UDP on the given port
// if one doesn't already exist, and ensures it sits ABOVE any WAN pass rules
// that have a gateway set (e.g. WAN_PPPOE). This is critical because WAN rules
// with gateways inject reply-to/route-to into pf, which can create poisoned
// states that hijack WireGuard traffic and route handshake responses back out
// the WAN instead of delivering them to the WG module.
func (h *TunnelHandler) ensureWANWireGuardRule(ctx context.Context, api *opnsenseAPIClient, port int) {
	portStr := fmt.Sprintf("%d", port)
	desc := "Gator: Allow WireGuard (site-to-site tunnels)"

	// Fetch all rules to find our rule and any WAN gateway rules.
	searchResp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{
		"current": 1, "rowCount": -1,
	})
	if err != nil {
		log.Printf("[ensureWANWireGuardRule] search_rule failed: %v", err)
		return
	}

	var ourRuleUUID string
	var firstWANGatewayRuleUUID string
	var ourPosition, gatewayPosition int

	for i, raw := range asSlice(searchResp["rows"]) {
		r := asMap(raw)
		uuid := asString(r["uuid"])
		iface := asString(r["interface"])
		action := asString(r["action"])

		// Check if this is our WG rule.
		if iface == "wan" && action == "pass" &&
			strings.EqualFold(asString(r["protocol"]), "UDP") &&
			asString(r["destination_port"]) == portStr {
			ourRuleUUID = uuid
			ourPosition = i
		}

		// Track the first WAN pass rule with a gateway (the problematic kind).
		if firstWANGatewayRuleUUID == "" && iface == "wan" && action == "pass" {
			gw := asString(r["gateway"])
			if gw != "" && gw != "default" {
				firstWANGatewayRuleUUID = uuid
				gatewayPosition = i
			}
		}
	}

	// Create the rule if it doesn't exist.
	if ourRuleUUID == "" {
		resp, err := api.Post(ctx, "/api/firewall/filter/add_rule", map[string]any{
			"rule": map[string]any{
				"enabled":          "1",
				"action":           "pass",
				"quick":            "1",
				"interface":        "wan",
				"direction":        "in",
				"ipprotocol":       "inet",
				"protocol":         "UDP",
				"source_net":       "any",
				"destination_net":  "(self)",
				"destination_port": portStr,
				"description":      desc,
			},
		})
		if err != nil {
			log.Printf("[ensureWANWireGuardRule] add_rule failed: %v", err)
			return
		}
		ourRuleUUID, _ = extractUUID(resp)
		if ourRuleUUID == "" {
			log.Printf("[ensureWANWireGuardRule] add_rule returned no UUID")
			return
		}
		log.Printf("[ensureWANWireGuardRule] created rule %s", ourRuleUUID)

		// New rules go to the bottom, so we definitely need to move it up.
		ourPosition = len(asSlice(searchResp["rows"])) // After all existing rules.
	}

	// Move our rule above the first WAN gateway rule if it's currently below it.
	if firstWANGatewayRuleUUID != "" && ourPosition > gatewayPosition {
		log.Printf("[ensureWANWireGuardRule] moving rule %s above WAN gateway rule %s (pos %d -> before %d)",
			ourRuleUUID, firstWANGatewayRuleUUID, ourPosition, gatewayPosition)
		_, err := api.Post(ctx, "/api/firewall/filter/move_rule_before/"+ourRuleUUID+"/"+firstWANGatewayRuleUUID, map[string]any{})
		if err != nil {
			log.Printf("[ensureWANWireGuardRule] move_rule_before failed: %v", err)
		}
	}

	// Apply the ruleset.
	api.Post(ctx, "/api/firewall/filter/apply", map[string]any{})
	log.Printf("[ensureWANWireGuardRule] applied ruleset")
}

// removeWANWireGuardRule finds and deletes the WAN WireGuard allow rule for a
// specific port. Returns a warning string if deletion fails, or "" on success.
func (h *TunnelHandler) removeWANWireGuardRule(ctx context.Context, api *opnsenseAPIClient, port int) string {
	portStr := fmt.Sprintf("%d", port)

	searchResp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{
		"current": 1, "rowCount": -1,
	})
	if err != nil {
		return "search rules for WAN WG rule: " + err.Error()
	}

	for _, raw := range asSlice(searchResp["rows"]) {
		r := asMap(raw)
		iface := asString(r["interface"])
		action := asString(r["action"])
		proto := asString(r["protocol"])
		dport := asString(r["destination_port"])
		desc := asString(r["description"])

		if iface == "wan" && action == "pass" &&
			strings.EqualFold(proto, "UDP") &&
			dport == portStr &&
			strings.Contains(desc, "Gator") {
			uuid := asString(r["uuid"])
			if _, err := api.Post(ctx, "/api/firewall/filter/del_rule/"+uuid, map[string]any{}); err != nil {
				return "delete WAN WG rule: " + err.Error()
			}
			api.Post(ctx, "/api/firewall/filter/apply", map[string]any{})
			log.Printf("[removeWANWireGuardRule] deleted rule %s (port %s)", uuid, portStr)
			return ""
		}
	}

	// Rule not found — fine, might have been manually deleted.
	return ""
}

// updateOPNsensePeerPubkey updates the OPNsense peer's public key, re-links
// the peer to its server (set_client can unlink it), reconfigures WireGuard,
// and kills stale pf states.
func (h *TunnelHandler) updateOPNsensePeerPubkey(ctx context.Context, api *opnsenseAPIClient, t *models.SiteTunnel) {
	// 1. Update the peer's pubkey.
	_, err := api.Post(ctx, "/api/wireguard/client/set_client/"+t.OPNsensePeerUUID, map[string]any{
		"client": map[string]any{
			"pubkey": t.RemotePublicKey,
		},
	})
	if err != nil {
		log.Printf("[updateOPNsensePeerPubkey] set_client failed: %v", err)
		return
	}

	// 2. Re-link the peer to the server. OPNsense's set_client can reset the
	// server's peer selection (sets selected=0), breaking the tunnel.
	if t.OPNsenseServerUUID != "" {
		// Read current server config to preserve other peers.
		srvResp, err := api.Get(ctx, "/api/wireguard/server/get_server/"+t.OPNsenseServerUUID)
		if err == nil {
			srvData := asMap(srvResp["server"])
			peers := asMap(srvData["peers"])
			var selected []string
			for uuid, raw := range peers {
				info := asMap(raw)
				// Keep all previously selected peers + ensure ours is included.
				if asString(info["selected"]) == "1" || uuid == t.OPNsensePeerUUID {
					selected = append(selected, uuid)
				}
			}
			if len(selected) > 0 {
				_, err = api.Post(ctx, "/api/wireguard/server/set_server/"+t.OPNsenseServerUUID, map[string]any{
					"server": map[string]any{
						"peers": strings.Join(selected, ","),
					},
				})
				if err != nil {
					log.Printf("[updateOPNsensePeerPubkey] re-link peers failed: %v", err)
				}
			}
		}
	}

	// 3. Reconfigure WireGuard to pick up the new pubkey.
	api.Post(ctx, "/api/wireguard/service/reconfigure", map[string]any{})

	// 4. Kill stale pf states for this peer.
	h.killStaleWGStates(ctx, api, t)

	log.Printf("[updateOPNsensePeerPubkey] updated peer %s pubkey and re-linked to server %s",
		t.OPNsensePeerUUID, t.OPNsenseServerUUID)
}

// killStaleWGStates kills pf states for a tunnel's remote WireGuard peer.
// It looks up the peer's public IP from OPNsense (serveraddress) and kills
// any matching pf states. This prevents poisoned inbound states (created by
// WAN rules with reply-to) from hijacking WireGuard handshake traffic.
// After killing, OPNsense's next keepalive creates a clean outbound state.
func (h *TunnelHandler) killStaleWGStates(ctx context.Context, api *opnsenseAPIClient, t *models.SiteTunnel) {
	if t.OPNsensePeerUUID == "" {
		return
	}

	// Get the peer's public IP from OPNsense.
	peerResp, err := api.Get(ctx, "/api/wireguard/client/get_client/"+t.OPNsensePeerUUID)
	if err != nil {
		log.Printf("[killStaleWGStates] get_client %s failed: %v", t.OPNsensePeerUUID, err)
		return
	}
	peerData := asMap(peerResp["client"])
	remoteIP := asString(peerData["serveraddress"])
	if remoteIP == "" {
		log.Printf("[killStaleWGStates] peer %s has no serveraddress, skipping", t.OPNsensePeerUUID)
		return
	}

	resp, err := api.Post(ctx, "/api/diagnostics/firewall/kill_states", map[string]any{
		"filter": remoteIP,
	})
	if err != nil {
		log.Printf("[killStaleWGStates] kill_states for %s failed: %v", remoteIP, err)
		return
	}
	dropped := asString(asMap(resp)["dropped_states"])
	log.Printf("[killStaleWGStates] killed states for %s (peer %s): dropped=%s", remoteIP, t.Name, dropped)
}

func buildRemoteWGConfig(t *models.SiteTunnel, lanSubnets []string) string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	b.WriteString(fmt.Sprintf("PrivateKey = %s\n", t.RemotePrivateKey))
	b.WriteString(fmt.Sprintf("Address = %s/24\n", t.RemoteIP))
	b.WriteString(fmt.Sprintf("ListenPort = %d\n", t.ListenPort))
	b.WriteString("\n[Peer]\n")
	b.WriteString(fmt.Sprintf("PublicKey = %s\n", t.FirewallPublicKey))

	// AllowedIPs: firewall's tunnel IP + LAN subnets (so remote can reach home network).
	allowedIPs := []string{t.FirewallIP + "/32"}
	allowedIPs = append(allowedIPs, lanSubnets...)
	b.WriteString(fmt.Sprintf("AllowedIPs = %s\n", strings.Join(allowedIPs, ", ")))

	// No Endpoint — OPNsense initiates with keepalive from its side.
	// Setting Endpoint here would cause the remote to send unsolicited UDP to
	// the WAN, requiring an explicit inbound WAN firewall rule. Letting OPNsense
	// initiate uses stateful tracking cleanly (same pattern as Helsinki tunnel).

	// No PersistentKeepalive — OPNsense handles keepalive from its side.

	return b.String()
}

func tunnelStatusFromModel(t *models.SiteTunnel) models.SiteTunnelStatus {
	return models.SiteTunnelStatus{
		ID:                t.ID,
		Name:              t.Name,
		Description:       t.Description,
		RemoteHost:        t.RemoteHost,
		TunnelSubnet:      t.TunnelSubnet,
		FirewallIP:        t.FirewallIP,
		RemoteIP:          t.RemoteIP,
		ListenPort:        t.ListenPort,
		RemoteWGInterface: t.RemoteWGInterface,
		Deployed:          t.Deployed,
		Status:            t.Status,
		CreatedAt:         t.CreatedAt,
	}
}

func tunnelDetailFromModel(t *models.SiteTunnel) models.SiteTunnelDetail {
	return models.SiteTunnelDetail{
		SiteTunnelStatus:  tunnelStatusFromModel(t),
		SSHPort:           t.SSHPort,
		SSHUser:           t.SSHUser,
		HasSSHKey:         t.SSHPrivateKey != "",
		HasSSHPassword:    t.SSHPassword != "",
		FirewallPublicKey: t.FirewallPublicKey,
		RemotePublicKey:   t.RemotePublicKey,
		Keepalive:         t.Keepalive,
	}
}

func parseWGField(wgShow, field string) string {
	for _, line := range strings.Split(wgShow, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, field+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, field+":"))
		}
	}
	return ""
}

// generateWireGuardKeyPair generates a Curve25519 keypair for WireGuard.
func generateWireGuardKeyPair() (privateKey, publicKey string, err error) {
	var privBytes [32]byte
	if _, err := rand.Read(privBytes[:]); err != nil {
		return "", "", fmt.Errorf("generate random key: %w", err)
	}
	// Clamp the private key per Curve25519 convention.
	privBytes[0] &= 248
	privBytes[31] &= 127
	privBytes[31] |= 64

	pubBytes, err := curve25519.X25519(privBytes[:], curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("compute public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(privBytes[:]),
		base64.StdEncoding.EncodeToString(pubBytes),
		nil
}

// sanitizeName converts a human-readable name to a safe identifier.
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// ─── Input Validation ────────────────────────────────────────────
//
// These validators ensure user/DB values are safe before use in SSH commands
// and OPNsense API calls. All shell-interpolated values MUST pass through
// one of these before reaching fmt.Sprintf + client.Run.

// validWGInterface checks that a WireGuard interface name matches wg0-wg99.
var reWGInterface = regexp.MustCompile(`^wg[0-9]{1,2}$`)

func validWGInterface(iface string) bool {
	return reWGInterface.MatchString(iface)
}

// validIP checks that a string is a valid IPv4 or IPv6 address.
func validIP(s string) bool {
	return net.ParseIP(strings.TrimSpace(s)) != nil
}

// validCIDR checks that a string is a valid CIDR notation (e.g. "10.200.200.0/24").
func validCIDR(s string) bool {
	_, _, err := net.ParseCIDR(strings.TrimSpace(s))
	return err == nil
}

// validPort checks that a port number is in the valid range.
func validPort(port int) bool {
	return port > 0 && port <= 65535
}

// validateTunnelFields checks all tunnel fields that will be used in shell
// commands or OPNsense API calls. Returns an error describing the first invalid field.
func validateTunnelFields(t *models.SiteTunnel) error {
	if t.RemoteWGInterface != "" && !validWGInterface(t.RemoteWGInterface) {
		return fmt.Errorf("invalid WG interface name %q (must match wgN)", t.RemoteWGInterface)
	}
	if t.FirewallIP != "" && !validIP(t.FirewallIP) {
		return fmt.Errorf("invalid firewall IP %q", t.FirewallIP)
	}
	if t.RemoteIP != "" && !validIP(t.RemoteIP) {
		return fmt.Errorf("invalid remote IP %q", t.RemoteIP)
	}
	if t.TunnelSubnet != "" && !validCIDR(t.TunnelSubnet) {
		return fmt.Errorf("invalid tunnel subnet %q", t.TunnelSubnet)
	}
	if t.ListenPort != 0 && !validPort(t.ListenPort) {
		return fmt.Errorf("invalid listen port %d", t.ListenPort)
	}
	if t.Keepalive < 0 || t.Keepalive > 600 {
		return fmt.Errorf("invalid keepalive %d (0-600)", t.Keepalive)
	}
	return nil
}
