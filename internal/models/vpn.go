package models

// Ownership status constants for VPN configs and site tunnels.
// These track whether a local DB row is verified against live OPNsense state.
const (
	OwnershipLocalOnly       = "local_only"       // Saved locally, never deployed
	OwnershipManagedPending  = "managed_pending"  // Bindings written by handler, awaiting reconciler verification
	OwnershipManagedVerified = "managed_verified" // Deployed, live-verified against fresh snapshot
	OwnershipManagedDrifted  = "managed_drifted"  // Was managed, fresh snapshot found partial mismatch
	OwnershipNeedsReimport   = "needs_reimport"   // Was managed, fresh snapshot found all bindings gone
)

// IsOwnershipManaged returns true if the status indicates the resource has
// OPNsense bindings (pending, verified, or drifted). Use this instead of
// comparing directly to OwnershipManagedVerified when you need "is this
// resource deployed?" rather than "is this resource live-verified?".
func IsOwnershipManaged(status string) bool {
	return status == OwnershipManagedPending ||
		status == OwnershipManagedVerified ||
		status == OwnershipManagedDrifted
}

type SimpleVPNConfig struct {
	ID               int64    `json:"id"`
	InstanceID       int64    `json:"-"` // FK to firewall_instances
	Name             string   `json:"name" binding:"required"`
	Protocol         string   `json:"protocol" binding:"required,oneof=wireguard"`
	IPVersion        string   `json:"ip_version"`   // "ipv4" or "ipv6", defaults to "ipv4"
	RoutingMode      string   `json:"routing_mode"` // "all", "selective", "bypass"; defaults to "all"
	LocalCIDR        string   `json:"local_cidr" binding:"required"`
	RemoteCIDR       string   `json:"remote_cidr" binding:"required"`
	Endpoint         string   `json:"endpoint" binding:"required"`
	DNS              string   `json:"dns"`
	PrivateKey       string   `json:"private_key"`
	PeerPublicKey    string   `json:"peer_public_key"`
	PreSharedKey     string   `json:"pre_shared_key"`
	Enabled          bool     `json:"enabled"`
	SourceInterfaces []string `json:"-"` // OPNsense interface identifiers (e.g. ["lan","opt2"]), stored as JSON

	// Last known OPNsense bindings — hints for reconciliation, NOT proof of live state.
	OPNsensePeerUUID      string `json:"-"`
	OPNsenseServerUUID    string `json:"-"`
	OPNsenseGatewayUUID   string `json:"-"`
	OPNsenseGatewayName   string `json:"-"`
	OPNsenseSNATRuleUUIDs string `json:"-"` // Comma-separated UUIDs (one SNAT per source interface)
	OPNsenseFilterUUIDs   string `json:"-"` // Comma-separated UUIDs (one filter rule per source interface)
	OPNsenseWGInterface   string `json:"-"`
	OPNsenseWGDevice      string `json:"-"`
	LastAppliedAt         string `json:"-"`
	RoutingApplied        bool   `json:"-"` // Config state: filter rules point at this VPN's gateway (set by activate/deactivate handlers only)
	GatewayOnline         bool   `json:"-"` // Runtime state: gateway is currently reachable (set by reconciler from dpinger status)

	// Ownership / reconciliation state.
	OwnershipStatus string `json:"-"` // One of Ownership* constants
	LastVerifiedAt  string `json:"-"` // RFC3339 timestamp of last successful live verification
	DriftReason     string `json:"-"` // Human-readable reason when drifted (e.g. "peer not found")
}

type SimpleVPNStatus struct {
	ID                int64    `json:"id"`
	Name              string   `json:"name"`
	Protocol          string   `json:"protocol"`
	IPVersion         string   `json:"ip_version"`
	RoutingMode       string   `json:"routing_mode"`
	LocalCIDR         string   `json:"local_cidr,omitempty"`
	RemoteCIDR        string   `json:"remote_cidr,omitempty"`
	Endpoint          string   `json:"endpoint,omitempty"`
	Enabled           bool     `json:"enabled"`
	HasPrivateKey     bool     `json:"has_private_key"`
	HasPeerPublicKey  bool     `json:"has_peer_public_key"`
	HasPreSharedKey   bool     `json:"has_pre_shared_key"`
	Applied           bool     `json:"applied"`
	RoutingApplied    bool     `json:"routing_applied"`
	GatewayOnline     bool     `json:"gateway_online"`
	GatewayApplied    bool     `json:"gateway_applied"`
	NATApplied        bool     `json:"nat_applied"`
	PolicyApplied     bool     `json:"policy_applied"`
	SourceInterfaces  []string `json:"source_interfaces,omitempty"`
	WGInterface       string   `json:"wg_interface,omitempty"`
	WGDevice          string   `json:"wg_device,omitempty"`
	InterfaceAssigned bool     `json:"interface_assigned"`
	GatewayName       string   `json:"gateway_name,omitempty"`
	LastAppliedAt     string   `json:"last_applied_at,omitempty"`

	// Ownership / reconciliation fields.
	OwnershipStatus string `json:"ownership_status"`
	DriftReason     string `json:"drift_reason,omitempty"`
	LastVerifiedAt  string `json:"last_verified_at,omitempty"`
}

type SimpleVPNDetail struct {
	SimpleVPNStatus
	DNS string `json:"dns,omitempty"`
}
