package models

type SimpleVPNConfig struct {
	ID                    int64    `json:"id"`
	InstanceID            int64    `json:"-"` // FK to firewall_instances
	Name                  string   `json:"name" binding:"required"`
	Protocol              string   `json:"protocol" binding:"required,oneof=wireguard"`
	IPVersion             string   `json:"ip_version"`   // "ipv4" or "ipv6", defaults to "ipv4"
	RoutingMode           string   `json:"routing_mode"` // "all", "selective", "bypass"; defaults to "all"
	LocalCIDR             string   `json:"local_cidr" binding:"required"`
	RemoteCIDR            string   `json:"remote_cidr" binding:"required"`
	Endpoint              string   `json:"endpoint" binding:"required"`
	DNS                   string   `json:"dns"`
	PrivateKey            string   `json:"private_key"`
	PeerPublicKey         string   `json:"peer_public_key"`
	PreSharedKey          string   `json:"pre_shared_key"`
	Enabled               bool     `json:"enabled"`
	SourceInterfaces      []string `json:"-"` // OPNsense interface identifiers (e.g. ["lan","opt2"]), stored as JSON
	OPNsensePeerUUID      string   `json:"-"`
	OPNsenseServerUUID    string   `json:"-"`
	OPNsenseGatewayUUID   string   `json:"-"`
	OPNsenseGatewayName   string   `json:"-"`
	OPNsenseSNATRuleUUIDs string   `json:"-"` // Comma-separated UUIDs (one SNAT per source interface)
	OPNsenseFilterUUIDs   string   `json:"-"` // Comma-separated UUIDs (one filter rule per source interface)
	OPNsenseWGInterface   string   `json:"-"`
	OPNsenseWGDevice      string   `json:"-"`
	LastAppliedAt         string   `json:"-"`
	RoutingApplied        bool     `json:"-"`
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
	GatewayApplied    bool     `json:"gateway_applied"`
	NATApplied        bool     `json:"nat_applied"`
	PolicyApplied     bool     `json:"policy_applied"`
	SourceInterfaces  []string `json:"source_interfaces,omitempty"`
	WGInterface       string   `json:"wg_interface,omitempty"`
	WGDevice          string   `json:"wg_device,omitempty"`
	InterfaceAssigned bool     `json:"interface_assigned"`
	GatewayName       string   `json:"gateway_name,omitempty"`
	LastAppliedAt     string   `json:"last_applied_at,omitempty"`
}

type SimpleVPNDetail struct {
	SimpleVPNStatus
	DNS string `json:"dns,omitempty"`
}
