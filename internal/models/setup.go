package models

type FirewallConfig struct {
	Type      string `json:"type" binding:"required,oneof=opnsense pfsense"`
	Host      string `json:"host" binding:"required"`
	APIKey    string `json:"api_key"`    // OPNsense only
	APISecret string `json:"api_secret"` // OPNsense only
	APIToken  string `json:"api_token"`  // pfSense only
	SkipTLS   bool   `json:"skip_tls"`
}

// FirewallInstance represents a saved firewall connection (host + credentials).
// Multiple instances can exist; one is active at a time.
type FirewallInstance struct {
	ID        int64  `json:"id"`
	Label     string `json:"label"` // User-friendly label (e.g. "Home OPNsense", "Lab pfSense")
	Type      string `json:"type"`  // "opnsense" or "pfsense"
	Host      string `json:"host"`  // Base URL
	APIKey    string `json:"-"`     // Hidden from JSON by default
	APISecret string `json:"-"`     // Hidden from JSON by default
	APIToken  string `json:"-"`     // Hidden from JSON by default
	SkipTLS   bool   `json:"skip_tls"`
	CreatedAt string `json:"created_at"`
}

// Config returns a FirewallConfig from this instance (for API client construction).
func (fi FirewallInstance) Config() FirewallConfig {
	return FirewallConfig{
		Type:      fi.Type,
		Host:      fi.Host,
		APIKey:    fi.APIKey,
		APISecret: fi.APISecret,
		APIToken:  fi.APIToken,
		SkipTLS:   fi.SkipTLS,
	}
}

type TestResult struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	Version  string `json:"version,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}
