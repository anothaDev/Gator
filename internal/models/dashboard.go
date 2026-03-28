package models

type OPNsenseOverview struct {
	Connected    bool   `json:"connected"`
	Error        string `json:"error,omitempty"`
	ErrorDetail  string `json:"error_detail,omitempty"`
	Host         string `json:"host,omitempty"`
	FirewallType string `json:"firewall_type,omitempty"`
	Name         string `json:"name,omitempty"`
	Version      string `json:"version,omitempty"`
	Updates      string `json:"updates,omitempty"`
	Uptime       string `json:"uptime,omitempty"`
	DateTime     string `json:"datetime,omitempty"`
	LoadAvg      string `json:"load_avg,omitempty"`
	CPUCount     int    `json:"cpu_count,omitempty"`
	CPUUsage     int    `json:"cpu_usage"`
	Temperature  []struct {
		Device string `json:"device"`
		TempC  string `json:"temp_c"`
	} `json:"temperature,omitempty"`
	Memory struct {
		UsedMB  int `json:"used_mb"`
		TotalMB int `json:"total_mb"`
	} `json:"memory"`
	Disk struct {
		Mountpoint string `json:"mountpoint,omitempty"`
		UsedPct    int    `json:"used_pct"`
		Size       string `json:"size,omitempty"`
		Used       string `json:"used,omitempty"`
	} `json:"disk"`
	Gateways struct {
		Total   int `json:"total"`
		Online  int `json:"online"`
		Offline int `json:"offline"`
	} `json:"gateways"`
	WireGuard struct {
		Interfaces int `json:"interfaces"`
		Peers      int `json:"peers"`
		Online     int `json:"online"`
	} `json:"wireguard"`
	VPN struct {
		Configured     bool   `json:"configured"`
		Applied        bool   `json:"applied"`
		RoutingApplied bool   `json:"routing_applied"`
		Name           string `json:"name,omitempty"`
		LastAppliedAt  string `json:"last_applied_at,omitempty"`
	} `json:"vpn"`
	Tunnels struct {
		Total    int `json:"total"`
		Deployed int `json:"deployed"`
		Errors   int `json:"errors"`
	} `json:"tunnels"`
	VPNCount int `json:"vpn_count"`
}
