package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DiscoveredVPN represents a VPN configuration found on OPNsense that can be imported.
type DiscoveredVPN struct {
	// Type: "vpn_client" (no listen port — outbound VPN) or "tunnel" (has listen port — site-to-site/server)
	Type string `json:"type"`

	// WireGuard
	ServerUUID string `json:"server_uuid"`
	ServerName string `json:"server_name"`
	PeerUUID   string `json:"peer_uuid"`
	PeerName   string `json:"peer_name"`
	LocalCIDR  string `json:"local_cidr"`  // Tunnel address from server
	RemoteCIDR string `json:"remote_cidr"` // Tunnel address from peer
	Endpoint   string `json:"endpoint"`    // server_address:server_port from peer
	PeerPubKey string `json:"peer_pubkey"` // Public key from peer
	HasPSK     bool   `json:"has_psk"`
	PrivateKey string `json:"-"` // Not sent to frontend
	PSK        string `json:"-"` // Not sent to frontend
	DNS        string `json:"dns"`
	ListenPort string `json:"listen_port"` // Non-empty = tunnel/server

	// Interface
	WGDevice  string `json:"wg_device"`  // e.g. "wg0"
	WGIface   string `json:"wg_iface"`   // e.g. "opt7"
	IfaceDesc string `json:"iface_desc"` // e.g. "MULLVAD"

	// Gateway
	GatewayUUID string `json:"gateway_uuid"`
	GatewayName string `json:"gateway_name"`
	GatewayIP   string `json:"gateway_ip"`

	// Filter rules (UUIDs of rules using this gateway)
	FilterUUIDs []string `json:"filter_uuids"`
	// SNAT rules (UUIDs of outbound NAT rules on the WG interface)
	SNATUUIDs []string `json:"snat_uuids"`

	// Source interfaces derived from filter rules
	SourceInterfaces []string `json:"source_interfaces"`
}

// DiscoverVPNs scans OPNsense for existing WireGuard VPN setups that can be imported.
// GET /api/opnsense/vpn/discover
func (h *GatewayHandler) DiscoverVPNs(c *gin.Context) {
	ctx := c.Request.Context()

	firewallCfg, err := h.store.GetFirewallConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read firewall setup"})
		return
	}
	if firewallCfg == nil || firewallCfg.Type != "opnsense" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VPN discovery requires OPNsense setup"})
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)

	discovered, err := discoverExistingVPNs(ctx, api)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "discovery failed: " + err.Error()})
		return
	}

	// Filter out already-managed VPNs and tunnels by peer UUID.
	managedPeers := make(map[string]bool)
	if vpns, _ := h.store.ListVPNConfigs(ctx); vpns != nil {
		for _, v := range vpns {
			if v.OPNsensePeerUUID != "" {
				managedPeers[v.OPNsensePeerUUID] = true
			}
		}
	}
	if tunnels, _ := h.store.ListSiteTunnels(ctx); tunnels != nil {
		for _, t := range tunnels {
			if t.OPNsensePeerUUID != "" {
				managedPeers[t.OPNsensePeerUUID] = true
			}
		}
	}

	var filtered []DiscoveredVPN
	for _, d := range discovered {
		if !managedPeers[d.PeerUUID] {
			filtered = append(filtered, d)
		}
	}

	c.JSON(http.StatusOK, gin.H{"vpns": filtered})
}

func discoverExistingVPNs(ctx context.Context, api *opnsenseAPIClient) ([]DiscoveredVPN, error) {
	// Step 1: List all WG servers (local instances).
	serverResp, err := api.Post(ctx, "/api/wireguard/server/search_server", map[string]any{})
	if err != nil {
		return nil, err
	}
	servers := asSlice(serverResp["rows"])
	log.Printf("[Discover] search_server returned %d servers", len(servers))
	if len(servers) == 0 {
		return nil, nil
	}

	// Step 2: Build interface map — device -> {identifier, description}.
	ifaceMap, err := buildInterfaceMap(ctx, api)
	if err != nil {
		log.Printf("[Discover] failed to build interface map: %v", err)
		// Non-fatal — continue without interface data.
	}

	// Step 3: Build gateway map — interface identifier -> gateway info.
	gwMap, err := buildGatewayMap(ctx, api)
	if err != nil {
		log.Printf("[Discover] failed to build gateway map: %v", err)
	}

	// Step 4: Build filter rule map — gateway name -> []filter rule info.
	filterMap, err := buildFilterRuleMap(ctx, api)
	if err != nil {
		log.Printf("[Discover] failed to build filter rule map: %v", err)
	}

	// Step 5: Build SNAT rule map — interface identifier -> []SNAT UUID.
	snatMap, err := buildSNATMap(ctx, api)
	if err != nil {
		log.Printf("[Discover] failed to build SNAT map: %v", err)
	}

	// Step 6: For each WG server, reconstruct the full picture.
	var discovered []DiscoveredVPN
	for i, raw := range servers {
		row := asMap(raw)
		serverUUID := asString(row["uuid"])
		serverName := asString(row["name"])
		if serverUUID == "" {
			continue
		}

		// Get full server details (includes private key, peers, tunnel address).
		serverDetail, err := api.Get(ctx, "/api/wireguard/server/get_server/"+serverUUID)
		if err != nil {
			log.Printf("[Discover] failed to get server %s: %v", serverUUID, err)
			continue
		}
		server := asMap(serverDetail["server"])

		localCIDR := extractSelectedValue(server["tunneladdress"])
		if localCIDR == "" {
			// Fallback: search_server row has tunneladdress as a plain string.
			localCIDR = asString(row["tunneladdress"])
		}
		privKey := asString(server["privkey"])
		dns := extractSelectedValue(server["dns"])
		if dns == "" {
			dns = asString(row["dns"])
		}

		// Listen port differentiates VPN clients from tunnels/servers.
		// A WG instance with a listen port is acting as a server endpoint.
		listenPort := asString(row["port"])
		if listenPort == "" {
			listenPort = asString(server["port"])
		}
		vpnType := "vpn_client"
		if listenPort != "" && listenPort != "0" {
			vpnType = "tunnel"
		}

		rawPeers := server["peers"]

		// Determine WG device name.
		// Try the "instance" field from search results first (e.g. "0" → "wg0").
		// Then try the "devices" field. Fall back to positional index.
		wgDevice := ""
		if inst := asString(row["instance"]); inst != "" {
			if inst[0] >= '0' && inst[0] <= '9' {
				wgDevice = "wg" + inst
			} else {
				wgDevice = inst
			}
		}
		if wgDevice == "" {
			if dev := asString(row["devices"]); dev != "" && strings.HasPrefix(dev, "wg") {
				wgDevice = dev
			}
		}
		if wgDevice == "" {
			// Positional fallback: the i-th server is wg{i}.
			wgDevice = fmt.Sprintf("wg%d", i)
		}
		// Extract peer UUIDs — handles CSV strings, selected-items maps, and slices.
		peerUUIDs := extractSelectedUUIDs(rawPeers)
		if len(peerUUIDs) == 0 {
			continue
		}

		// Split comma-separated tunnel addresses so we can match one per peer.
		localCIDRs := splitCSVValues(localCIDR)

		// Match interface (shared across all peers of this server).
		iface := ifaceMap[wgDevice]

		// Match gateway by interface identifier.
		var gw *gatewayInfo
		if iface.identifier != "" {
			gw = gwMap[iface.identifier]
		}

		// Match filter rules by gateway name (filter search results use display name).
		var filterUUIDs []string
		var sourceIfaces []string
		if gw != nil {
			filterEntries := filterMap[gw.name]
			for _, fe := range filterEntries {
				filterUUIDs = append(filterUUIDs, fe.uuid)
				for _, si := range fe.sourceInterfaces {
					sourceIfaces = appendUnique(sourceIfaces, si)
				}
			}
		}

		// Match SNAT rules by WG interface identifier.
		var snatUUIDs []string
		if iface.identifier != "" {
			snatUUIDs = snatMap[iface.identifier]
		}

		// Create one DiscoveredVPN entry per peer.
		for peerIdx, peerUUID := range peerUUIDs {
			peerDetail, err := api.Get(ctx, "/api/wireguard/client/get_client/"+peerUUID)
			if err != nil {
				log.Printf("[Discover] failed to get peer %s: %v", peerUUID, err)
				continue
			}
			peer := asMap(peerDetail["client"])
			peerName := asString(peer["name"])
			peerPubKey := asString(peer["pubkey"])
			psk := asString(peer["psk"])
			remoteCIDR := extractSelectedValue(peer["tunneladdress"])
			serverAddr := asString(peer["serveraddress"])
			serverPort := asString(peer["serverport"])

			endpoint := serverAddr
			if serverPort != "" && serverPort != "0" {
				endpoint = serverAddr + ":" + serverPort
			}

			// Match the correct local tunnel address for this peer.
			// If the server has multiple comma-separated addresses (one per peer subnet),
			// try to find the one in the same /24 as the peer's remote address.
			peerLocalCIDR := localCIDR // default: full comma-separated string
			if len(localCIDRs) > 1 {
				peerLocalCIDR = matchLocalCIDRForPeer(localCIDRs, remoteCIDR, endpoint)
				if peerLocalCIDR == "" && peerIdx < len(localCIDRs) {
					// Positional fallback.
					peerLocalCIDR = localCIDRs[peerIdx]
				}
				if peerLocalCIDR == "" {
					peerLocalCIDR = localCIDRs[0]
				}
			} else if len(localCIDRs) == 1 {
				peerLocalCIDR = localCIDRs[0]
			}

			d := DiscoveredVPN{
				Type:             vpnType,
				ServerUUID:       serverUUID,
				ServerName:       serverName,
				PeerUUID:         peerUUID,
				PeerName:         peerName,
				LocalCIDR:        peerLocalCIDR,
				RemoteCIDR:       remoteCIDR,
				Endpoint:         endpoint,
				PeerPubKey:       peerPubKey,
				HasPSK:           psk != "",
				PrivateKey:       privKey,
				PSK:              psk,
				DNS:              dns,
				ListenPort:       listenPort,
				WGDevice:         wgDevice,
				WGIface:          iface.identifier,
				IfaceDesc:        iface.description,
				GatewayUUID:      "",
				GatewayName:      "",
				GatewayIP:        "",
				FilterUUIDs:      filterUUIDs,
				SNATUUIDs:        snatUUIDs,
				SourceInterfaces: sourceIfaces,
			}
			if gw != nil {
				d.GatewayUUID = gw.uuid
				d.GatewayName = gw.name
				d.GatewayIP = gw.gateway
			}

			discovered = append(discovered, d)
		}
	}

	return discovered, nil
}

// matchLocalCIDRForPeer finds the local tunnel address (from the server's comma-separated list)
// that belongs to the same /24 subnet as the peer's remote address or endpoint.
// For example, if localCIDRs = ["10.200.200.2/24", "10.200.201.2/24"] and remoteCIDR = "10.200.201.1/32",
// it returns "10.200.201.2/24".
func matchLocalCIDRForPeer(localCIDRs []string, remoteCIDR, endpoint string) string {
	// Extract the /24 prefix from the remote CIDR (first 3 octets).
	remotePrefix := cidrPrefix24(remoteCIDR)
	if remotePrefix == "" {
		// If remoteCIDR is empty, try to match by endpoint IP — less reliable but worth trying.
		return ""
	}
	for _, lc := range localCIDRs {
		if cidrPrefix24(lc) == remotePrefix {
			return lc
		}
	}
	return ""
}

// cidrPrefix24 extracts the first 3 octets from a CIDR or IP string.
// "10.200.200.2/24" → "10.200.200", "10.200.201.1/32" → "10.200.201", "10.200.200.1" → "10.200.200".
func cidrPrefix24(s string) string {
	s = strings.TrimSpace(s)
	// Strip CIDR mask.
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 3 {
		return ""
	}
	return parts[0] + "." + parts[1] + "." + parts[2]
}

// --- Helper types and builders ---

type ifaceInfo struct {
	identifier  string // e.g. "opt7"
	description string // e.g. "MULLVAD"
}

type gatewayInfo struct {
	uuid    string
	name    string
	gateway string
}

type filterEntry struct {
	uuid             string
	sourceInterfaces []string
}

// extractSelectedUUIDs extracts UUIDs from an OPNsense field value.
// OPNsense detail endpoints return multi-select fields in one of these formats:
//  1. Comma-separated string: "uuid1,uuid2,uuid3"
//  2. Selected items map: {"uuid1":{"value":"Name","selected":1}, ...}
//  3. Slice of strings: ["uuid1","uuid2"]
//  4. Empty string or nil
func extractSelectedUUIDs(value any) []string {
	if value == nil {
		return nil
	}

	// Case 1: string — try CSV split.
	if s, ok := value.(string); ok {
		return splitCSVValues(s)
	}

	// Case 2: map — OPNsense selected items pattern.
	// Keys are UUIDs, values are objects with "selected" field.
	if m, ok := value.(map[string]any); ok {
		var uuids []string
		for uuid, detail := range m {
			uuid = strings.TrimSpace(uuid)
			if uuid == "" {
				continue
			}
			// Include if selected (or if there's no "selected" field — assume all are selected).
			if detailMap, ok := detail.(map[string]any); ok {
				sel := detailMap["selected"]
				// selected can be float64(1), bool(true), or string("1")
				if sel != nil {
					switch v := sel.(type) {
					case float64:
						if v != 1 {
							continue
						}
					case bool:
						if !v {
							continue
						}
					case string:
						if v != "1" {
							continue
						}
					}
				}
			}
			uuids = append(uuids, uuid)
		}
		return uuids
	}

	// Case 3: slice of strings.
	if arr, ok := value.([]any); ok {
		var uuids []string
		for _, item := range arr {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					uuids = append(uuids, s)
				}
			}
		}
		return uuids
	}

	return nil
}

// extractSelectedValues extracts the selected keys from an OPNsense field.
// Similar to extractSelectedUUIDs but returns the map keys (which are the actual values
// for fields like tunneladdress where keys are CIDRs). Falls back to CSV parsing for strings.
// If multiple values are selected, joins them with commas.
func extractSelectedValue(value any) string {
	if value == nil {
		return ""
	}
	// Plain string — return as-is.
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	// Selected-items map: keys are the values.
	uuids := extractSelectedUUIDs(value)
	if len(uuids) > 0 {
		return strings.Join(uuids, ",")
	}
	return ""
}

// buildInterfaceMap returns a map of device name -> interface info.
func buildInterfaceMap(ctx context.Context, api *opnsenseAPIClient) (map[string]ifaceInfo, error) {
	resp, err := api.Get(ctx, "/api/interfaces/overview/interfaces_info/0")
	if err != nil {
		return nil, err
	}

	result := map[string]ifaceInfo{}
	for _, raw := range asSlice(resp["rows"]) {
		iface := asMap(raw)
		device := asString(iface["device"])
		identifier := asString(iface["identifier"])
		description := asString(iface["description"])
		if device != "" {
			result[device] = ifaceInfo{
				identifier:  identifier,
				description: description,
			}
		}
	}
	return result, nil
}

// buildGatewayMap returns a map of interface identifier -> gateway info.
// Only returns gateways that point to WireGuard interfaces (wg*).
func buildGatewayMap(ctx context.Context, api *opnsenseAPIClient) (map[string]*gatewayInfo, error) {
	resp, err := api.Get(ctx, "/api/routing/settings/search_gateway")
	if err != nil {
		return nil, err
	}

	result := map[string]*gatewayInfo{}
	for _, raw := range asSlice(resp["rows"]) {
		row := asMap(raw)
		uuid := asString(row["uuid"])
		name := asString(row["name"])
		iface := asString(row["interface"])
		gw := asString(row["gateway"])
		if uuid != "" && iface != "" {
			result[iface] = &gatewayInfo{
				uuid:    uuid,
				name:    name,
				gateway: gw,
			}
		}
	}
	return result, nil
}

// buildFilterRuleMap returns a map of gateway UUID -> []filter rule info.
func buildFilterRuleMap(ctx context.Context, api *opnsenseAPIClient) (map[string][]filterEntry, error) {
	resp, err := api.Post(ctx, "/api/firewall/filter/searchRule", map[string]any{})
	if err != nil {
		return nil, err
	}

	result := map[string][]filterEntry{}
	for _, raw := range asSlice(resp["rows"]) {
		row := asMap(raw)
		uuid := asString(row["uuid"])
		gateway := asString(row["gateway"])
		if uuid == "" || gateway == "" || gateway == "*" {
			continue
		}

		// Parse source interfaces from the "interface" field (may be comma-separated).
		ifaceStr := asString(row["interface"])
		ifaces := strings.Split(ifaceStr, ",")
		var cleaned []string
		for _, i := range ifaces {
			i = strings.TrimSpace(i)
			if i != "" {
				cleaned = append(cleaned, i)
			}
		}

		// The gateway field in search results contains the gateway name, not UUID.
		// We need to match by name later, or look up the UUID.
		// For now, store by the gateway value as-is (it's the display name).
		result[gateway] = append(result[gateway], filterEntry{
			uuid:             uuid,
			sourceInterfaces: cleaned,
		})
	}
	return result, nil
}

// buildSNATMap returns a map of WG interface identifier -> []SNAT rule UUIDs.
func buildSNATMap(ctx context.Context, api *opnsenseAPIClient) (map[string][]string, error) {
	resp, err := api.Post(ctx, "/api/firewall/source_nat/searchRule", map[string]any{})
	if err != nil {
		return nil, err
	}

	result := map[string][]string{}
	for _, raw := range asSlice(resp["rows"]) {
		row := asMap(raw)
		uuid := asString(row["uuid"])
		iface := asString(row["interface"])
		if uuid != "" && iface != "" {
			result[iface] = append(result[iface], uuid)
		}
	}
	return result, nil
}
