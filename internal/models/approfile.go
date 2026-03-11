package models

// PortRule describes a single protocol + port(s) combo for an app.
type PortRule struct {
	Protocol string `json:"protocol"` // "TCP", "UDP", "TCP/UDP"
	Ports    string `json:"ports"`    // single, range ("6881-6889"), or comma-separated
}

// URLTableHint provides metadata for services that need a URL Table (JSON) alias
// instead of a plain network alias (because their IP ranges are too large for static lists).
// The user downloads the file from DownloadURL and uploads it to Gator.
type URLTableHint struct {
	DownloadURL string `json:"download_url"` // Where the user downloads the JSON file
	JQFilter    string `json:"jq_filter"`    // jq path expression for OPNsense
	Description string `json:"description"`  // Human-readable explanation
	Filename    string `json:"filename"`     // Stored filename in data/ip-ranges/
}

// AppProfile is a predefined or user-defined app/service with known ports.
type AppProfile struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Icon         string        `json:"icon"`
	Category     string        `json:"category"`
	Rules        []PortRule    `json:"rules"`
	ASNs         []int         `json:"asns,omitempty"`           // ASN numbers for IP-based routing
	URLTableHint *URLTableHint `json:"url_table_hint,omitempty"` // For large providers needing URL table aliases
	Note         string        `json:"note,omitempty"`
	IsCustom     bool          `json:"is_custom,omitempty"`
}

// AppPreset is a predefined collection of app routing settings.
type AppPreset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	VPNOn       []string `json:"vpn_on,omitempty"`
	VPNOff      []string `json:"vpn_off,omitempty"`
}

// AppRoute tracks per-VPN per-app routing state in the database.
type AppRoute struct {
	ID                int64  `json:"id"`
	VPNConfigID       int64  `json:"vpn_config_id"`
	AppID             string `json:"app_id"`
	Enabled           bool   `json:"enabled"`
	OPNsenseRuleUUIDs string `json:"opnsense_rule_uuids"` // comma-separated UUIDs
}

// AppRouteStatus is the API response for a single app's routing state.
type AppRouteStatus struct {
	AppID   string `json:"app_id"`
	Enabled bool   `json:"enabled"`
	Applied bool   `json:"applied"` // true if OPNsense rules exist
}
