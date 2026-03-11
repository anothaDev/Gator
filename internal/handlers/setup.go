package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/models"
)

type SetupHandler struct {
	store SetupConfigStore
}

type SetupConfigStore interface {
	SaveFirewallConfig(ctx context.Context, cfg models.FirewallConfig) error
	GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error)
	ListInstances(ctx context.Context) ([]models.FirewallInstance, error)
	GetInstance(ctx context.Context, id int64) (*models.FirewallInstance, error)
	GetActiveInstanceID(ctx context.Context) (int64, error)
	GetActiveInstance(ctx context.Context) (*models.FirewallInstance, error)
	SetActiveInstance(ctx context.Context, instanceID int64) error
	DeleteInstance(ctx context.Context, id int64) error
}

func NewSetupHandler(store SetupConfigStore) *SetupHandler {
	return &SetupHandler{store: store}
}

func (h *SetupHandler) TestConnection(c *gin.Context) {
	var cfg models.FirewallConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, models.TestResult{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	if err := validateConfigForType(cfg); err != nil {
		c.JSON(http.StatusBadRequest, models.TestResult{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	result := testFirewall(cfg)
	c.JSON(http.StatusOK, result)
}

func (h *SetupHandler) TestOPNsenseConnection(c *gin.Context) {
	var req struct {
		Host      string `json:"host" binding:"required"`
		APIKey    string `json:"api_key" binding:"required"`
		APISecret string `json:"api_secret" binding:"required"`
		SkipTLS   bool   `json:"skip_tls"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.TestResult{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	result := testFirewall(models.FirewallConfig{
		Type:      "opnsense",
		Host:      req.Host,
		APIKey:    req.APIKey,
		APISecret: req.APISecret,
		SkipTLS:   req.SkipTLS,
	})
	c.JSON(http.StatusOK, result)
}

func (h *SetupHandler) TestPfSenseConnection(c *gin.Context) {
	var req struct {
		Host     string `json:"host" binding:"required"`
		APIToken string `json:"api_token" binding:"required"`
		SkipTLS  bool   `json:"skip_tls"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.TestResult{
			Success: false,
			Message: "Invalid request: " + err.Error(),
		})
		return
	}

	result := testFirewall(models.FirewallConfig{
		Type:     "pfsense",
		Host:     req.Host,
		APIToken: req.APIToken,
		SkipTLS:  req.SkipTLS,
	})
	c.JSON(http.StatusOK, result)
}

func (h *SetupHandler) SaveConfig(c *gin.Context) {
	var cfg models.FirewallConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validateConfigForType(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	normalizedHost, err := normalizeHost(cfg.Host)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg.Host = normalizedHost

	if err := h.store.SaveFirewallConfig(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save setup config"})
		return
	}

	// Trigger initial config backup in the background for OPNsense.
	if cfg.Type == "opnsense" {
		go createInitialBackup(cfg)
	}

	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

func (h *SetupHandler) GetStatus(c *gin.Context) {
	ctx := c.Request.Context()

	inst, err := h.store.GetActiveInstance(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read setup config"})
		return
	}

	if inst == nil {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"configured":     true,
		"type":           inst.Type,
		"host":           inst.Host,
		"skip_tls":       inst.SkipTLS,
		"instance_id":    inst.ID,
		"instance_label": inst.Label,
	})
}

// ListInstances returns all saved firewall instances.
func (h *SetupHandler) ListInstances(c *gin.Context) {
	instances, err := h.store.ListInstances(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list instances"})
		return
	}
	if instances == nil {
		instances = []models.FirewallInstance{}
	}

	activeID, _ := h.store.GetActiveInstanceID(c.Request.Context())

	// Build response with masked credentials.
	type instanceResp struct {
		ID        int64  `json:"id"`
		Label     string `json:"label"`
		Type      string `json:"type"`
		Host      string `json:"host"`
		SkipTLS   bool   `json:"skip_tls"`
		Active    bool   `json:"active"`
		CreatedAt string `json:"created_at"`
	}

	resp := make([]instanceResp, 0, len(instances))
	for _, inst := range instances {
		resp = append(resp, instanceResp{
			ID:        inst.ID,
			Label:     inst.Label,
			Type:      inst.Type,
			Host:      inst.Host,
			SkipTLS:   inst.SkipTLS,
			Active:    inst.ID == activeID,
			CreatedAt: inst.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"instances": resp})
}

// SwitchInstance sets a different instance as active.
func (h *SetupHandler) SwitchInstance(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance ID"})
		return
	}

	// Verify the instance exists before switching.
	inst, err := h.store.GetInstance(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up instance"})
		return
	}
	if inst == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	if err := h.store.SetActiveInstance(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to switch instance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "switched", "instance_id": id})
}

// DeleteInstanceHandler deletes a firewall instance and all its data.
func (h *SetupHandler) DeleteInstanceHandler(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance ID"})
		return
	}

	// Don't allow deleting the active instance.
	activeID, _ := h.store.GetActiveInstanceID(c.Request.Context())
	if id == activeID {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot delete the active instance — switch to another first"})
		return
	}

	if err := h.store.DeleteInstance(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete instance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "instance_id": id})
}

func validateConfigForType(cfg models.FirewallConfig) error {
	switch cfg.Type {
	case "opnsense":
		if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.APISecret) == "" {
			return fmt.Errorf("opnsense requires api_key and api_secret")
		}
	case "pfsense":
		if strings.TrimSpace(cfg.APIToken) == "" {
			return fmt.Errorf("pfsense requires api_token")
		}
	default:
		return fmt.Errorf("unknown firewall type: %s", cfg.Type)
	}

	return nil
}

func testFirewall(cfg models.FirewallConfig) models.TestResult {
	host, err := normalizeHost(cfg.Host)
	if err != nil {
		return models.TestResult{
			Success: false,
			Message: err.Error(),
		}
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.SkipTLS,
			},
		},
	}

	switch cfg.Type {
	case "opnsense":
		return testOPNsense(client, host, cfg.APIKey, cfg.APISecret)
	case "pfsense":
		return testPfSense(client, host, cfg.APIToken)
	default:
		return models.TestResult{
			Success: false,
			Message: "Unknown firewall type: " + cfg.Type,
		}
	}
}

func normalizeHost(rawHost string) (string, error) {
	host := strings.TrimSpace(rawHost)
	host = strings.TrimRight(host, "/")
	if host == "" {
		return "", fmt.Errorf("host is required")
	}

	if !strings.Contains(host, "://") {
		host = "https://" + host
	}

	parsed, err := url.Parse(host)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid host. use an IP/hostname or URL, e.g. 10.0.0.2 or https://10.0.0.2")
	}

	return strings.TrimRight(parsed.String(), "/"), nil
}

func testOPNsense(client *http.Client, host, key, secret string) models.TestResult {
	url := host + "/api/core/firmware/status"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return models.TestResult{
			Success: false,
			Message: "Failed to create request: " + err.Error(),
		}
	}
	req.SetBasicAuth(key, secret)

	resp, err := client.Do(req)
	if err != nil {
		return models.TestResult{
			Success: false,
			Message: connectionErrorMessage(err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return models.TestResult{
			Success: false,
			Message: "Authentication failed. Check your API key and secret.",
		}
	}

	if resp.StatusCode != http.StatusOK {
		return models.TestResult{
			Success: false,
			Message: fmt.Sprintf("Unexpected status code: %d", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.TestResult{
			Success: true,
			Message: "Connected successfully, but could not read firmware info.",
		}
	}

	var firmware map[string]interface{}
	if err := json.Unmarshal(body, &firmware); err == nil {
		version, _ := firmware["product_version"].(string)
		hostname, _ := firmware["product_name"].(string)
		return models.TestResult{
			Success:  true,
			Message:  "Connected to OPNsense successfully.",
			Version:  version,
			Hostname: hostname,
		}
	}

	return models.TestResult{
		Success: true,
		Message: "Connected to OPNsense successfully.",
	}
}

func testPfSense(client *http.Client, host, token string) models.TestResult {
	url := host + "/api/v2/system/info"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return models.TestResult{
			Success: false,
			Message: "Failed to create request: " + err.Error(),
		}
	}
	req.Header.Set("X-API-Key", token)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return models.TestResult{
			Success: false,
			Message: connectionErrorMessage(err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return models.TestResult{
			Success: false,
			Message: "Authentication failed. Check your API token.",
		}
	}

	if resp.StatusCode != http.StatusOK {
		return models.TestResult{
			Success: false,
			Message: fmt.Sprintf("Unexpected status code: %d. Make sure the pfSense REST API package is installed and reachable at /api/v2.", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.TestResult{
			Success: true,
			Message: "Connected successfully, but could not read system info.",
		}
	}

	var info struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &info); err == nil {
		version := stringValue(info.Data["version"])
		if version == "" {
			version = stringValue(info.Data["pfsense_version"])
		}
		hostname := stringValue(info.Data["hostname"])
		return models.TestResult{
			Success:  true,
			Message:  "Connected to pfSense successfully.",
			Version:  version,
			Hostname: hostname,
		}
	}

	return models.TestResult{
		Success: true,
		Message: "Connected to pfSense successfully.",
	}
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func connectionErrorMessage(err error) string {
	msg := err.Error()

	switch {
	case strings.Contains(msg, "x509: cannot validate certificate for") &&
		strings.Contains(msg, "doesn't contain any IP SANs"):
		return "TLS verification failed: certificate does not include this IP address. Use the firewall hostname from the certificate, install a certificate with an IP SAN, or enable 'Skip TLS verification'."
	case strings.Contains(msg, "x509: certificate signed by unknown authority"):
		return "TLS verification failed: certificate is not trusted (self-signed or unknown CA). Import the CA certificate or enable 'Skip TLS verification'."
	case strings.Contains(msg, "tls: failed to verify certificate"):
		return "TLS verification failed. Check the certificate chain and hostname, or enable 'Skip TLS verification'."
	default:
		return "Connection failed: " + msg
	}
}
