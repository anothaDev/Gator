package handlers

import "github.com/raul/gator/internal/models"

// builtinAppProfiles is the static list of predefined app/service port mappings.
var builtinAppProfiles = []models.AppProfile{
	// ── Gaming ──────────────────────────────────────────────────────────────
	{
		ID: "steam", Name: "Steam", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "27015-27050"},
			{Protocol: "UDP", Ports: "27000-27100"},
			{Protocol: "UDP", Ports: "4380"},
			{Protocol: "TCP", Ports: "443"},
		},
		Note: "Includes Steam store (443) — may overlap with HTTPS browsing",
	},
	{
		ID: "cs2", Name: "Counter-Strike 2", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "UDP", Ports: "27015-27050"},
			{Protocol: "TCP", Ports: "27015-27050"},
		},
	},
	{
		ID: "valorant", Name: "Valorant", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
			{Protocol: "UDP", Ports: "7000-8000"},
		},
	},
	{
		ID: "league_of_legends", Name: "League of Legends", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "2099,5222-5223,8393-8400"},
			{Protocol: "UDP", Ports: "5000-5500"},
		},
	},
	{
		ID: "minecraft", Name: "Minecraft", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "25565"},
		},
	},
	{
		ID: "fortnite", Name: "Fortnite", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443,5222,5795-5847"},
			{Protocol: "UDP", Ports: "5222,5795-5847,9000-9100"},
		},
	},
	{
		ID: "apex_legends", Name: "Apex Legends", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "1024-1124,3216,9960-9969,18000,18060,18120,27900,28910,29900"},
			{Protocol: "UDP", Ports: "1024-1124,18000,29900,37000-40000"},
		},
	},
	{
		ID: "overwatch", Name: "Overwatch 2", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "1119,3724,6113"},
			{Protocol: "UDP", Ports: "3478-3479,5060,5062,6250,12000-64000"},
		},
	},
	{
		ID: "warzone", Name: "Call of Duty / Warzone", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "3074,27014-27050"},
			{Protocol: "UDP", Ports: "3074,3478-3480,27000-27031"},
		},
	},
	{
		ID: "dota2", Name: "Dota 2", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "27015-27050"},
			{Protocol: "UDP", Ports: "27015-27050"},
		},
	},
	{
		ID: "rocket_league", Name: "Rocket League", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
			{Protocol: "UDP", Ports: "7000-9000"},
		},
	},
	{
		ID: "gta_online", Name: "GTA Online", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "6672,61455-61458"},
			{Protocol: "UDP", Ports: "6672,61455-61458"},
		},
	},
	{
		ID: "rust", Name: "Rust", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "28015-28016"},
			{Protocol: "UDP", Ports: "28015-28016"},
		},
	},
	{
		ID: "arma3", Name: "Arma 3 / DayZ", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "2302-2306"},
			{Protocol: "UDP", Ports: "2302-2306"},
		},
	},
	{
		ID: "escape_tarkov", Name: "Escape from Tarkov", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443,17000-17100"},
			{Protocol: "UDP", Ports: "17000-17100"},
		},
	},
	{
		ID: "world_of_warcraft", Name: "World of Warcraft", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "1119,3724"},
			{Protocol: "UDP", Ports: "3724"},
		},
	},
	{
		ID: "palworld", Name: "Palworld", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "UDP", Ports: "8211"},
		},
	},
	{
		ID: "helldivers2", Name: "Helldivers 2", Icon: "gamepad", Category: "gaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
			{Protocol: "UDP", Ports: "27015-27100"},
		},
	},

	// ── Streaming ───────────────────────────────────────────────────────────
	{
		ID: "netflix", Name: "Netflix", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
		},
		ASNs: []int{2906},
	},
	{
		ID: "youtube", Name: "YouTube", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
			{Protocol: "UDP", Ports: "443"},
		},
		ASNs: []int{15169, 36040, 36384, 36385}, // Google, YouTube-specific
	},
	{
		ID: "twitch", Name: "Twitch", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443,1935"},
		},
		ASNs: []int{46489},
	},
	{
		ID: "spotify", Name: "Spotify", Icon: "music", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443,4070"},
		},
		ASNs: []int{8403, 43650},
	},
	{
		ID: "disney_plus", Name: "Disney+", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
		},
		ASNs: []int{11251, 7575}, // Disney Streaming, Walt Disney
	},
	{
		ID: "hbo_max", Name: "Max (HBO)", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
		},
		ASNs: []int{7381, 20057}, // Warner Bros Discovery
	},
	{
		ID: "amazon_prime", Name: "Prime Video", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
		},
		ASNs: []int{16509, 14618}, // Amazon AWS
		URLTableHint: &models.URLTableHint{
			DownloadURL: "https://ip-ranges.amazonaws.com/ip-ranges.json",
			JQFilter:    `.prefixes[] | select(.service=="CLOUDFRONT" or .service=="AMAZON") | .ip_prefix`,
			Description: "Amazon publishes their IP ranges as a JSON file. Download it and upload to Gator for precise Prime Video routing.",
			Filename:    "amazon_ip_ranges.json",
		},
		Note: "Uses Amazon IP ranges — requires uploaded ip-ranges.json for IP-based routing",
	},
	{
		ID: "apple_tv", Name: "Apple TV+", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
		},
		ASNs: []int{714, 6185}, // Apple
	},
	{
		ID: "plex", Name: "Plex", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "32400"},
		},
	},
	{
		ID: "emby_jellyfin", Name: "Emby / Jellyfin", Icon: "tv", Category: "streaming",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "8096,8920"},
		},
	},

	// ── Communication ───────────────────────────────────────────────────────
	{
		ID: "discord", Name: "Discord", Icon: "message-circle", Category: "communication",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
			{Protocol: "UDP", Ports: "50000-65535"},
		},
		ASNs: []int{49544},
		Note: "Voice uses high UDP ports",
	},
	{
		ID: "teamspeak", Name: "TeamSpeak", Icon: "headphones", Category: "communication",
		Rules: []models.PortRule{
			{Protocol: "UDP", Ports: "9987"},
			{Protocol: "TCP", Ports: "30033"},
			{Protocol: "TCP", Ports: "10011"},
		},
	},
	{
		ID: "mumble", Name: "Mumble", Icon: "headphones", Category: "communication",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "64738"},
			{Protocol: "UDP", Ports: "64738"},
		},
	},
	{
		ID: "zoom", Name: "Zoom", Icon: "video", Category: "communication",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443,8801-8802"},
			{Protocol: "UDP", Ports: "3478-3479,8801-8810"},
		},
		ASNs: []int{30103},
	},
	{
		ID: "ms_teams", Name: "Microsoft Teams", Icon: "video", Category: "communication",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
			{Protocol: "UDP", Ports: "3478-3481,50000-50059"},
		},
		ASNs: []int{8075}, // Microsoft
	},
	{
		ID: "whatsapp", Name: "WhatsApp", Icon: "message-circle", Category: "communication",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443,5222"},
		},
	},
	{
		ID: "signal", Name: "Signal", Icon: "message-circle", Category: "communication",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
		},
		Note: "HTTPS only — best paired with IP-based routing for precision",
	},
	{
		ID: "slack", Name: "Slack", Icon: "message-circle", Category: "communication",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
		},
		Note: "HTTPS only — best paired with IP-based routing for precision",
	},

	// ── File Sharing ────────────────────────────────────────────────────────
	{
		ID: "torrents", Name: "BitTorrent", Icon: "download", Category: "file_sharing",
		Rules: []models.PortRule{
			{Protocol: "TCP/UDP", Ports: "6881-6889"},
			{Protocol: "UDP", Ports: "6969"},
			{Protocol: "TCP", Ports: "51413"},
		},
	},
	{
		ID: "usenet", Name: "Usenet (NNTP)", Icon: "download", Category: "file_sharing",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "119,443,563"},
		},
	},
	{
		ID: "ftp", Name: "FTP / SFTP", Icon: "upload", Category: "file_sharing",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "20-22"},
		},
	},
	{
		ID: "syncthing", Name: "Syncthing", Icon: "upload", Category: "file_sharing",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "22000"},
			{Protocol: "UDP", Ports: "22000,21027"},
		},
	},
	{
		ID: "smb", Name: "SMB / CIFS", Icon: "upload", Category: "file_sharing",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "445"},
		},
	},

	// ── Browsing ────────────────────────────────────────────────────────────
	{
		ID: "http_browsing", Name: "Web Browsing", Icon: "globe", Category: "browsing",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "80,443"},
		},
	},
	{
		ID: "quic", Name: "QUIC (HTTP/3)", Icon: "globe", Category: "browsing",
		Rules: []models.PortRule{
			{Protocol: "UDP", Ports: "443"},
		},
		Note: "Used by Chrome, YouTube, Google services",
	},
	{
		ID: "dns", Name: "DNS", Icon: "globe", Category: "browsing",
		Rules: []models.PortRule{
			{Protocol: "TCP/UDP", Ports: "53"},
		},
	},
	{
		ID: "doh", Name: "DNS over HTTPS", Icon: "globe", Category: "browsing",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "443"},
		},
		Note: "Same as HTTPS — only meaningful with IP-based routing to DoH resolvers",
	},

	// ── Tools & Remote Access ───────────────────────────────────────────────
	{
		ID: "ssh", Name: "SSH", Icon: "terminal", Category: "remote_access",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "22"},
		},
	},
	{
		ID: "rdp", Name: "Remote Desktop (RDP)", Icon: "terminal", Category: "remote_access",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "3389"},
			{Protocol: "UDP", Ports: "3389"},
		},
	},
	{
		ID: "vnc", Name: "VNC", Icon: "terminal", Category: "remote_access",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "5900-5901"},
		},
	},
	{
		ID: "wireguard", Name: "WireGuard", Icon: "terminal", Category: "remote_access",
		Rules: []models.PortRule{
			{Protocol: "UDP", Ports: "51820"},
		},
		Note: "Nested VPN or external WG tunnels",
	},
	{
		ID: "openvpn", Name: "OpenVPN", Icon: "terminal", Category: "remote_access",
		Rules: []models.PortRule{
			{Protocol: "UDP", Ports: "1194"},
			{Protocol: "TCP", Ports: "443"},
		},
	},

	// ── Home & IoT ──────────────────────────────────────────────────────────
	{
		ID: "homeassistant", Name: "Home Assistant", Icon: "home", Category: "home_iot",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "8123"},
		},
	},
	{
		ID: "mqtt", Name: "MQTT", Icon: "home", Category: "home_iot",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "1883,8883"},
		},
	},
	{
		ID: "sonos", Name: "Sonos", Icon: "home", Category: "home_iot",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "1400,1443,4444"},
			{Protocol: "UDP", Ports: "1900-1901,6969"},
		},
	},

	// ── Mail ────────────────────────────────────────────────────────────────
	{
		ID: "email", Name: "Email (IMAP/SMTP)", Icon: "mail", Category: "mail",
		Rules: []models.PortRule{
			{Protocol: "TCP", Ports: "25,465,587,993,995"},
		},
	},
}

// builtinAppPresets is the static list of preset routing profiles.
var builtinAppPresets = []models.AppPreset{
	{
		ID: "privacy_mode", Name: "Privacy Mode",
		Description: "Route everything through VPN",
	},
	{
		ID: "gaming_mode", Name: "Gaming Mode",
		Description: "Direct connection for games, VPN for browsing and torrents",
		VPNOn:       []string{"http_browsing", "torrents"},
		VPNOff:      []string{"cs2", "steam", "valorant", "league_of_legends", "fortnite", "apex_legends", "overwatch", "warzone", "dota2", "rocket_league", "minecraft", "rust", "gta_online"},
	},
	{
		ID: "streaming_mode", Name: "Streaming Mode",
		Description: "Bypass VPN for streaming services to avoid geo-blocks and buffering",
		VPNOff:      []string{"netflix", "youtube", "twitch", "spotify", "disney_plus", "hbo_max", "amazon_prime", "apple_tv", "plex"},
	},
	{
		ID: "torrents_only", Name: "Torrents Only",
		Description: "Only route torrent and usenet traffic through VPN",
		VPNOn:       []string{"torrents", "usenet"},
	},
}

// appProfileByID returns a profile by ID, or nil if not found.
func appProfileByID(id string) *models.AppProfile {
	for i := range builtinAppProfiles {
		if builtinAppProfiles[i].ID == id {
			return &builtinAppProfiles[i]
		}
	}
	return nil
}
