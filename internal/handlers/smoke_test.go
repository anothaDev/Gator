package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/anothaDev/gator/internal/models"
	"github.com/anothaDev/gator/internal/storage"
)

func TestSwitchInstanceSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{Type: "pfsense", Host: "https://fw-a.local", APIToken: "token-a"}); err != nil {
		t.Fatalf("save first instance: %v", err)
	}
	firstID, err := store.GetActiveInstanceID(ctx)
	if err != nil {
		t.Fatalf("get first active instance: %v", err)
	}

	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{Type: "pfsense", Host: "https://fw-b.local", APIToken: "token-b"}); err != nil {
		t.Fatalf("save second instance: %v", err)
	}
	secondID, err := store.GetActiveInstanceID(ctx)
	if err != nil {
		t.Fatalf("get second active instance: %v", err)
	}
	if firstID == 0 || secondID == 0 || firstID == secondID {
		t.Fatalf("expected two distinct instance IDs, got first=%d second=%d", firstID, secondID)
	}

	router := gin.New()
	setupHandler := NewSetupHandler(store)
	router.POST("/api/instances/:id/activate", setupHandler.SwitchInstance)
	router.GET("/api/setup/status", setupHandler.GetStatus)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/instances/%d/activate", firstID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("switch instance returned %d: %s", rec.Code, rec.Body.String())
	}

	var switchResp struct {
		Status     string `json:"status"`
		InstanceID int64  `json:"instance_id"`
	}
	decodeJSON(t, rec.Body.Bytes(), &switchResp)
	if switchResp.Status != "switched" || switchResp.InstanceID != firstID {
		t.Fatalf("unexpected switch response: %+v", switchResp)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status returned %d: %s", rec.Code, rec.Body.String())
	}

	var statusResp struct {
		Configured   bool   `json:"configured"`
		InstanceID   int64  `json:"instance_id"`
		Host         string `json:"host"`
		FirewallType string `json:"type"`
	}
	decodeJSON(t, rec.Body.Bytes(), &statusResp)
	if !statusResp.Configured || statusResp.InstanceID != firstID {
		t.Fatalf("unexpected status response: %+v", statusResp)
	}
	if statusResp.Host != "https://fw-a.local" || statusResp.FirewallType != "pfsense" {
		t.Fatalf("unexpected active instance details: %+v", statusResp)
	}

	activeID, err := store.GetActiveInstanceID(ctx)
	if err != nil {
		t.Fatalf("read active instance after switch: %v", err)
	}
	if activeID != firstID {
		t.Fatalf("active instance not updated, got %d want %d", activeID, firstID)
	}
}

func TestImportFromOPNsenseSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/wireguard/server/get_server/server-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"server": map[string]any{"privkey": "priv-from-opnsense"}})
		case "/api/wireguard/client/get_client/peer-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"client": map[string]any{"psk": "psk-secret-123", "pubkey": "peer-pub-from-opnsense"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	if err := store.SaveFirewallConfig(context.Background(), models.FirewallConfig{
		Type:      "opnsense",
		Host:      api.URL,
		APIKey:    "key",
		APISecret: "secret",
	}); err != nil {
		t.Fatalf("save firewall config: %v", err)
	}

	router := gin.New()
	vpnHandler := NewVPNHandler(store)
	router.POST("/api/opnsense/vpn/import", vpnHandler.ImportFromOPNsense)

	body := map[string]any{
		"name":              "Imported VPN",
		"server_uuid":       "server-1",
		"peer_uuid":         "peer-1",
		"local_cidr":        "10.8.0.2/32",
		"remote_cidr":       "0.0.0.0/0",
		"endpoint":          "vpn.example.com:51820",
		"gateway_uuid":      "gw-1",
		"gateway_name":      "GW_IMPORTED",
		"filter_uuids":      []string{"rule-a", "rule-b"},
		"snat_uuids":        []string{"nat-a"},
		"source_interfaces": []string{"lan", "opt1"},
		"wg_iface":          "opt7",
		"wg_device":         "wg0",
	}
	rec := performJSONRequest(t, router, http.MethodPost, "/api/opnsense/vpn/import", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import returned %d: %s", rec.Code, rec.Body.String())
	}

	var importResp struct {
		Status string `json:"status"`
		ID     int64  `json:"id"`
	}
	decodeJSON(t, rec.Body.Bytes(), &importResp)
	if importResp.Status != "imported" || importResp.ID == 0 {
		t.Fatalf("unexpected import response: %+v", importResp)
	}

	cfg, err := store.GetVPNConfigByID(context.Background(), importResp.ID)
	if err != nil {
		t.Fatalf("read imported config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected imported config to be saved")
	}
	if cfg.Name != "Imported VPN" || cfg.PrivateKey != "priv-from-opnsense" || cfg.PeerPublicKey != "peer-pub-from-opnsense" {
		t.Fatalf("unexpected imported config: %+v", cfg)
	}
	if cfg.PreSharedKey != "psk-secret-123" || cfg.OPNsensePeerUUID != "peer-1" || cfg.OPNsenseServerUUID != "server-1" {
		t.Fatalf("expected imported secrets and UUIDs to persist: %+v", cfg)
	}
	if cfg.OPNsenseGatewayUUID != "gw-1" || cfg.OPNsenseGatewayName != "GW_IMPORTED" || cfg.OPNsenseWGDevice != "wg0" || cfg.OPNsenseWGInterface != "opt7" {
		t.Fatalf("expected imported gateway/interface metadata to persist: %+v", cfg)
	}
	if got := fmt.Sprintf("%v", cfg.SourceInterfaces); got != "[lan opt1]" {
		t.Fatalf("unexpected source interfaces: %v", cfg.SourceInterfaces)
	}
}

func TestApplyToOPNsenseSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/wireguard/client/search_client":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{}})
		case "/api/wireguard/client/add_client":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "saved", "uuid": "peer-created"})
		case "/api/wireguard/server/search_server":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{}})
		case "/api/wireguard/server/add_server":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "saved", "uuid": "server-created"})
		case "/api/wireguard/general/set":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "saved"})
		case "/api/wireguard/service/reconfigure":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "ok"})
		case "/api/wireguard/service/start":
			writeJSON(t, w, http.StatusOK, map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	ctx := context.Background()
	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{
		Type:      "opnsense",
		Host:      api.URL,
		APIKey:    "key",
		APISecret: "secret",
	}); err != nil {
		t.Fatalf("save firewall config: %v", err)
	}

	vpnID, err := store.CreateVPNConfig(ctx, models.SimpleVPNConfig{
		Name:          "Deploy Me",
		Protocol:      "wireguard",
		LocalCIDR:     "10.64.0.2/32",
		RemoteCIDR:    "0.0.0.0/0",
		Endpoint:      "vpn.example.com:51820",
		PrivateKey:    "private-key-123",
		PeerPublicKey: "peer-public-key-123",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create vpn config: %v", err)
	}

	router := gin.New()
	vpnHandler := NewVPNHandler(store)
	router.POST("/api/opnsense/vpn/:id/apply", vpnHandler.ApplyToOPNsense)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/opnsense/vpn/%d/apply", vpnID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply returned %d: %s", rec.Code, rec.Body.String())
	}

	var applyResp struct {
		Status        string `json:"status"`
		PeerUUID      string `json:"peer_uuid"`
		ServerUUID    string `json:"server_uuid"`
		PeerCreated   bool   `json:"peer_created"`
		ServerCreated bool   `json:"server_created"`
	}
	decodeJSON(t, rec.Body.Bytes(), &applyResp)
	if applyResp.Status != "applied" || applyResp.PeerUUID != "peer-created" || applyResp.ServerUUID != "server-created" {
		t.Fatalf("unexpected apply response: %+v", applyResp)
	}
	if !applyResp.PeerCreated || !applyResp.ServerCreated {
		t.Fatalf("expected created flags in apply response: %+v", applyResp)
	}

	cfg, err := store.GetVPNConfigByID(ctx, vpnID)
	if err != nil {
		t.Fatalf("read applied config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected applied config to exist")
	}
	if cfg.OPNsensePeerUUID != "peer-created" || cfg.OPNsenseServerUUID != "server-created" {
		t.Fatalf("expected persisted OPNsense UUIDs, got %+v", cfg)
	}
	if cfg.LastAppliedAt == "" {
		t.Fatal("expected LastAppliedAt to be set after apply")
	}
}

func TestAppRouteEnableDisableSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/firewall/filter/get_rule":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"rule": map[string]any{
					"gateway": map[string]any{
						"GW_TEST": "GW_TEST",
					},
				},
			})
		case "/api/firewall/filter/search_rule":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{}})
		case "/api/firewall/filter/add_rule":
			writeJSON(t, w, http.StatusOK, map[string]any{"uuid": "rule-ssh", "result": "saved"})
		case "/api/firewall/filter/savepoint":
			writeJSON(t, w, http.StatusOK, map[string]any{"revision": "rev-route"})
		case "/api/firewall/filter/apply/rev-route":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		case "/api/firewall/filter/get_rule/rule-ssh":
			writeJSON(t, w, http.StatusOK, map[string]any{"rule": map[string]any{"description": "GATOR_APP_SSH_TCP"}})
		case "/api/firewall/filter/del_rule/rule-ssh":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "deleted"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	ctx := context.Background()
	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{
		Type:      "opnsense",
		Host:      api.URL,
		APIKey:    "key",
		APISecret: "secret",
	}); err != nil {
		t.Fatalf("save firewall config: %v", err)
	}

	vpnID, err := store.CreateVPNConfig(ctx, models.SimpleVPNConfig{
		Name:                "Route Me",
		Protocol:            "wireguard",
		RoutingMode:         "selective",
		LocalCIDR:           "10.64.0.2/32",
		RemoteCIDR:          "0.0.0.0/0",
		Endpoint:            "vpn.example.com:51820",
		PrivateKey:          "private-key-123",
		PeerPublicKey:       "peer-public-key-123",
		Enabled:             true,
		OPNsenseGatewayName: "GW_TEST",
	})
	if err != nil {
		t.Fatalf("create vpn config: %v", err)
	}

	router := gin.New()
	appRoutingHandler := NewAppRoutingHandler(store)
	router.POST("/api/opnsense/vpn/:id/app-routes/:appId/enable", appRoutingHandler.EnableAppRoute)
	router.POST("/api/opnsense/vpn/:id/app-routes/:appId/disable", appRoutingHandler.DisableAppRoute)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/opnsense/vpn/%d/app-routes/ssh/enable", vpnID), bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable app route returned %d: %s", rec.Code, rec.Body.String())
	}

	var enableResp struct {
		Status    string   `json:"status"`
		AppID     string   `json:"app_id"`
		RuleUUIDs []string `json:"rule_uuids"`
		Revision  string   `json:"revision"`
	}
	decodeJSON(t, rec.Body.Bytes(), &enableResp)
	if enableResp.Status != "enabled" || enableResp.AppID != "ssh" || len(enableResp.RuleUUIDs) != 1 || enableResp.RuleUUIDs[0] != "rule-ssh" {
		t.Fatalf("unexpected enable response: %+v", enableResp)
	}

	route, err := store.GetAppRoute(ctx, vpnID, "ssh")
	if err != nil {
		t.Fatalf("read stored app route after enable: %v", err)
	}
	if route == nil || !route.Enabled || route.OPNsenseRuleUUIDs != "rule-ssh" {
		t.Fatalf("unexpected route after enable: %+v", route)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/opnsense/vpn/%d/app-routes/ssh/disable", vpnID), bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable app route returned %d: %s", rec.Code, rec.Body.String())
	}

	var disableResp struct {
		Status   string `json:"status"`
		AppID    string `json:"app_id"`
		Revision string `json:"revision"`
	}
	decodeJSON(t, rec.Body.Bytes(), &disableResp)
	if disableResp.Status != "disabled" || disableResp.AppID != "ssh" {
		t.Fatalf("unexpected disable response: %+v", disableResp)
	}

	route, err = store.GetAppRoute(ctx, vpnID, "ssh")
	if err != nil {
		t.Fatalf("read stored app route after disable: %v", err)
	}
	if route == nil || route.Enabled || route.OPNsenseRuleUUIDs != "" {
		t.Fatalf("unexpected route after disable: %+v", route)
	}
}

func TestInstanceScopedAccessGuardsSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{Type: "pfsense", Host: "https://fw-a.local", APIToken: "token-a"}); err != nil {
		t.Fatalf("save first instance: %v", err)
	}
	firstID, err := store.GetActiveInstanceID(ctx)
	if err != nil {
		t.Fatalf("get first active instance: %v", err)
	}
	vpnID, err := store.CreateVPNConfig(ctx, models.SimpleVPNConfig{
		Name:          "Scoped VPN",
		Protocol:      "wireguard",
		LocalCIDR:     "10.64.0.2/32",
		RemoteCIDR:    "0.0.0.0/0",
		Endpoint:      "vpn.example.com:51820",
		PrivateKey:    "private-key-123",
		PeerPublicKey: "peer-public-key-123",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create vpn config: %v", err)
	}

	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{Type: "pfsense", Host: "https://fw-b.local", APIToken: "token-b"}); err != nil {
		t.Fatalf("save second instance: %v", err)
	}
	secondID, err := store.GetActiveInstanceID(ctx)
	if err != nil {
		t.Fatalf("get second active instance: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("expected distinct active instance after second save")
	}

	router := gin.New()
	vpnHandler := NewVPNHandler(store)
	appRoutingHandler := NewAppRoutingHandler(store)
	router.GET("/api/vpn/configs/:id", vpnHandler.GetConfig)
	router.GET("/api/opnsense/vpn/:id/app-routes", appRoutingHandler.ListAppRoutes)

	for _, path := range []string{
		fmt.Sprintf("/api/vpn/configs/%d", vpnID),
		fmt.Sprintf("/api/opnsense/vpn/%d/app-routes", vpnID),
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for cross-instance path %s, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestDeleteConfigCleanupSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	var mu sync.Mutex
	hits := map[string]int{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case "/api/firewall/filter/get_rule/filter-main", "/api/firewall/filter/get_rule/app-rule-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"rule": map[string]any{"description": "GATOR_RULE"}})
		case "/api/firewall/filter/del_rule/filter-main", "/api/firewall/filter/del_rule/app-rule-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "deleted"})
		case "/api/firewall/source_nat/get_rule/snat-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"rule": map[string]any{"description": "GATOR_SNAT"}})
		case "/api/firewall/source_nat/del_rule/snat-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "deleted"})
		case "/api/firewall/filter/savepoint":
			writeJSON(t, w, http.StatusOK, map[string]any{"revision": "rev-delete"})
		case "/api/firewall/filter/apply/rev-delete":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		case "/api/firewall/filter/cancel_rollback/rev-delete":
			writeJSON(t, w, http.StatusOK, map[string]any{})
		case "/api/routing/settings/del_gateway/gw-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "deleted"})
		case "/api/routing/settings/reconfigure":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		case "/api/wireguard/server/del_server/server-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "deleted"})
		case "/api/wireguard/client/del_client/peer-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "deleted"})
		case "/api/wireguard/service/reconfigure":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	ctx := context.Background()
	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{
		Type:      "opnsense",
		Host:      api.URL,
		APIKey:    "key",
		APISecret: "secret",
	}); err != nil {
		t.Fatalf("save firewall config: %v", err)
	}

	vpnID, err := store.CreateVPNConfig(ctx, models.SimpleVPNConfig{
		Name:                  "Delete Me",
		Protocol:              "wireguard",
		LocalCIDR:             "10.64.0.2/32",
		RemoteCIDR:            "0.0.0.0/0",
		Endpoint:              "vpn.example.com:51820",
		PrivateKey:            "private-key-123",
		PeerPublicKey:         "peer-public-key-123",
		Enabled:               true,
		OPNsenseFilterUUIDs:   "filter-main",
		OPNsenseSNATRuleUUIDs: "snat-1",
		OPNsenseGatewayUUID:   "gw-1",
		OPNsenseServerUUID:    "server-1",
		OPNsensePeerUUID:      "peer-1",
	})
	if err != nil {
		t.Fatalf("create vpn config: %v", err)
	}
	if err := store.UpsertAppRoute(ctx, models.AppRoute{
		VPNConfigID:       vpnID,
		AppID:             "ssh",
		Enabled:           true,
		OPNsenseRuleUUIDs: "app-rule-1",
	}); err != nil {
		t.Fatalf("seed app route: %v", err)
	}

	router := gin.New()
	vpnHandler := NewVPNHandler(store)
	router.DELETE("/api/vpn/configs/:id", vpnHandler.DeleteConfig)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/vpn/configs/%d", vpnID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete returned %d: %s", rec.Code, rec.Body.String())
	}

	var deleteResp struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
	}
	decodeJSON(t, rec.Body.Bytes(), &deleteResp)
	if deleteResp.Status != "deleted" || len(deleteResp.Warnings) != 0 {
		t.Fatalf("unexpected delete response: %+v", deleteResp)
	}

	cfg, err := store.GetVPNConfigByID(ctx, vpnID)
	if err != nil {
		t.Fatalf("read config after delete: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected VPN to be deleted, got %+v", cfg)
	}
	route, err := store.GetAppRoute(ctx, vpnID, "ssh")
	if err != nil {
		t.Fatalf("read app route after delete: %v", err)
	}
	if route != nil {
		t.Fatalf("expected app routes to be deleted, got %+v", route)
	}

	for _, path := range []string{
		"/api/firewall/filter/del_rule/filter-main",
		"/api/firewall/filter/del_rule/app-rule-1",
		"/api/firewall/source_nat/del_rule/snat-1",
		"/api/routing/settings/del_gateway/gw-1",
		"/api/wireguard/server/del_server/server-1",
		"/api/wireguard/client/del_client/peer-1",
	} {
		mu.Lock()
		count := hits[path]
		mu.Unlock()
		if count == 0 {
			t.Fatalf("expected cleanup endpoint %s to be called", path)
		}
	}
}

func TestPendingFirewallConfirmAndRevertSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetPendingRevisions()
	t.Cleanup(resetPendingRevisions)

	store := newTestStore(t)
	var mu sync.Mutex
	hits := map[string]int{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case "/api/firewall/filter/cancel_rollback/rev-confirm":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		case "/api/firewall/filter/revert/rev-revert":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	ctx := context.Background()
	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{
		Type:      "opnsense",
		Host:      api.URL,
		APIKey:    "key",
		APISecret: "secret",
	}); err != nil {
		t.Fatalf("save firewall config: %v", err)
	}

	vpnID, err := store.CreateVPNConfig(ctx, models.SimpleVPNConfig{
		Name:          "Pending VPN",
		Protocol:      "wireguard",
		LocalCIDR:     "10.64.0.2/32",
		RemoteCIDR:    "0.0.0.0/0",
		Endpoint:      "vpn.example.com:51820",
		PrivateKey:    "private-key-123",
		PeerPublicKey: "peer-public-key-123",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create vpn config: %v", err)
	}

	if err := store.UpsertAppRoute(ctx, models.AppRoute{
		VPNConfigID:       vpnID,
		AppID:             "ssh",
		Enabled:           true,
		OPNsenseRuleUUIDs: "new-rule",
	}); err != nil {
		t.Fatalf("seed changed app route: %v", err)
	}

	registerPendingChange("rev-confirm", vpnID, "ssh", &models.AppRoute{
		VPNConfigID:       vpnID,
		AppID:             "ssh",
		Enabled:           false,
		OPNsenseRuleUUIDs: "",
	})
	registerPendingChange("rev-revert", vpnID, "ssh", &models.AppRoute{
		VPNConfigID:       vpnID,
		AppID:             "ssh",
		Enabled:           false,
		OPNsenseRuleUUIDs: "",
	})

	router := gin.New()
	gatewayHandler := NewGatewayHandler(store)
	router.GET("/api/opnsense/firewall/pending", gatewayHandler.PendingFirewall)
	router.POST("/api/opnsense/firewall/confirm", gatewayHandler.ConfirmFirewall)
	router.POST("/api/opnsense/firewall/revert", gatewayHandler.RevertFirewall)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/opnsense/firewall/pending", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pending returned %d: %s", rec.Code, rec.Body.String())
	}

	var pendingResp struct {
		Pending  bool   `json:"pending"`
		Revision string `json:"revision"`
	}
	decodeJSON(t, rec.Body.Bytes(), &pendingResp)
	if !pendingResp.Pending || pendingResp.Revision == "" {
		t.Fatalf("unexpected pending response: %+v", pendingResp)
	}

	rec = performJSONRequest(t, router, http.MethodPost, "/api/opnsense/firewall/confirm", map[string]any{"revision": "rev-confirm"})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm returned %d: %s", rec.Code, rec.Body.String())
	}
	if _, info := getPendingRevision(); info == nil {
		// ok, still something pending (the revert case)
	} else {
		// no-op to satisfy flow readability
	}

	mu.Lock()
	confirmCalls := hits["/api/firewall/filter/cancel_rollback/rev-confirm"]
	mu.Unlock()
	if confirmCalls == 0 {
		t.Fatal("expected confirm rollback endpoint to be called")
	}

	rec = performJSONRequest(t, router, http.MethodPost, "/api/opnsense/firewall/revert", map[string]any{"revision": "rev-revert"})
	if rec.Code != http.StatusOK {
		t.Fatalf("revert returned %d: %s", rec.Code, rec.Body.String())
	}

	route, err := store.GetAppRoute(ctx, vpnID, "ssh")
	if err != nil {
		t.Fatalf("read app route after revert: %v", err)
	}
	if route == nil || route.Enabled || route.OPNsenseRuleUUIDs != "" {
		t.Fatalf("expected revert to restore old route state, got %+v", route)
	}

	mu.Lock()
	revertCalls := hits["/api/firewall/filter/revert/rev-revert"]
	mu.Unlock()
	if revertCalls == 0 {
		t.Fatal("expected revert endpoint to be called")
	}

	clearPendingRevision("rev-confirm")
	clearPendingRevision("rev-revert")
}

func TestCustomProfileCreateDeleteCleanupSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	var mu sync.Mutex
	hits := map[string]int{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case "/api/firewall/filter/get_rule/custom-rule-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"rule": map[string]any{"description": "GATOR_APP_CUSTOM_TEST_APP_TCP"}})
		case "/api/firewall/filter/del_rule/custom-rule-1":
			writeJSON(t, w, http.StatusOK, map[string]any{"result": "deleted"})
		case "/api/firewall/filter/savepoint":
			writeJSON(t, w, http.StatusOK, map[string]any{"revision": "rev-custom-delete"})
		case "/api/firewall/filter/apply/rev-custom-delete":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		case "/api/firewall/filter/cancel_rollback/rev-custom-delete":
			writeJSON(t, w, http.StatusOK, map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	ctx := context.Background()
	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{
		Type:      "opnsense",
		Host:      api.URL,
		APIKey:    "key",
		APISecret: "secret",
	}); err != nil {
		t.Fatalf("save firewall config: %v", err)
	}

	vpnID, err := store.CreateVPNConfig(ctx, models.SimpleVPNConfig{
		Name:          "Profile VPN",
		Protocol:      "wireguard",
		LocalCIDR:     "10.64.0.2/32",
		RemoteCIDR:    "0.0.0.0/0",
		Endpoint:      "vpn.example.com:51820",
		PrivateKey:    "private-key-123",
		PeerPublicKey: "peer-public-key-123",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create vpn config: %v", err)
	}

	router := gin.New()
	appRoutingHandler := NewAppRoutingHandler(store)
	router.POST("/api/app-profiles", appRoutingHandler.CreateCustomProfile)
	router.DELETE("/api/app-profiles/:profileId", appRoutingHandler.DeleteCustomProfile)

	createBody := map[string]any{
		"name":     "Test App",
		"category": "custom",
		"rules": []map[string]any{{
			"protocol": "TCP",
			"ports":    "12345",
		}},
		"note": "smoke test profile",
	}
	rec := performJSONRequest(t, router, http.MethodPost, "/api/app-profiles", createBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("create custom profile returned %d: %s", rec.Code, rec.Body.String())
	}

	var createResp struct {
		Status  string            `json:"status"`
		Profile models.AppProfile `json:"profile"`
	}
	decodeJSON(t, rec.Body.Bytes(), &createResp)
	if createResp.Status != "created" || createResp.Profile.ID != "custom_test_app" {
		t.Fatalf("unexpected create profile response: %+v", createResp)
	}

	if err := store.UpsertAppRoute(ctx, models.AppRoute{
		VPNConfigID:       vpnID,
		AppID:             createResp.Profile.ID,
		Enabled:           true,
		OPNsenseRuleUUIDs: "custom-rule-1",
	}); err != nil {
		t.Fatalf("seed custom app route: %v", err)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/app-profiles/custom_test_app", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete custom profile returned %d: %s", rec.Code, rec.Body.String())
	}

	var deleteResp struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
	}
	decodeJSON(t, rec.Body.Bytes(), &deleteResp)
	if deleteResp.Status != "deleted" || len(deleteResp.Warnings) != 0 {
		t.Fatalf("unexpected delete custom profile response: %+v", deleteResp)
	}

	profile, err := store.GetCustomAppProfile(ctx, "custom_test_app")
	if err != nil {
		t.Fatalf("read custom profile after delete: %v", err)
	}
	if profile != nil {
		t.Fatalf("expected custom profile to be deleted, got %+v", profile)
	}
	routes, err := store.ListAppRoutesByAppID(ctx, "custom_test_app")
	if err != nil {
		t.Fatalf("read app routes after delete: %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("expected custom app routes to be deleted, got %+v", routes)
	}

	mu.Lock()
	ruleDeleteCalls := hits["/api/firewall/filter/del_rule/custom-rule-1"]
	mu.Unlock()
	if ruleDeleteCalls == 0 {
		t.Fatal("expected custom profile cleanup rule delete to be called")
	}
}

func TestSetupLifecycleAndConnectionSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)

	pfsenseAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/system/info":
			writeJSON(t, w, http.StatusOK, map[string]any{"data": map[string]any{"hostname": "pf-host", "version": "2.7.2"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer pfsenseAPI.Close()

	opnsenseAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/core/firmware/status":
			writeJSON(t, w, http.StatusOK, map[string]any{"product_name": "OPNsense", "product_version": "24.7"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer opnsenseAPI.Close()

	router := gin.New()
	setupHandler := NewSetupHandler(store)
	router.POST("/api/pfsense/test-connection", setupHandler.TestPfSenseConnection)
	router.POST("/api/opnsense/test-connection", setupHandler.TestOPNsenseConnection)
	router.POST("/api/setup/save", setupHandler.SaveConfig)
	router.GET("/api/setup/status", setupHandler.GetStatus)
	router.GET("/api/instances", setupHandler.ListInstances)
	router.DELETE("/api/instances/:id", setupHandler.DeleteInstanceHandler)

	for _, tc := range []struct {
		path string
		body map[string]any
		want string
	}{
		{path: "/api/pfsense/test-connection", body: map[string]any{"host": pfsenseAPI.URL, "api_token": "pf-token"}, want: "Connected to pfSense successfully."},
		{path: "/api/opnsense/test-connection", body: map[string]any{"host": opnsenseAPI.URL, "api_key": "key", "api_secret": "secret"}, want: "Connected to OPNsense successfully."},
	} {
		rec := performJSONRequest(t, router, http.MethodPost, tc.path, tc.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("test connection %s returned %d: %s", tc.path, rec.Code, rec.Body.String())
		}
		var resp struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		decodeJSON(t, rec.Body.Bytes(), &resp)
		if !resp.Success || resp.Message != tc.want {
			t.Fatalf("unexpected connection response for %s: %+v", tc.path, resp)
		}
	}

	for _, body := range []map[string]any{
		{"type": "pfsense", "host": pfsenseAPI.URL, "api_token": "pf-token"},
		{"type": "opnsense", "host": opnsenseAPI.URL, "api_key": "key", "api_secret": "secret"},
	} {
		rec := performJSONRequest(t, router, http.MethodPost, "/api/setup/save", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("save config returned %d: %s", rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list instances returned %d: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Instances []struct {
			ID     int64  `json:"id"`
			Type   string `json:"type"`
			Active bool   `json:"active"`
		} `json:"instances"`
	}
	decodeJSON(t, rec.Body.Bytes(), &listResp)
	if len(listResp.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %+v", listResp)
	}
	var deleteID int64
	for _, inst := range listResp.Instances {
		if inst.Type == "pfsense" {
			deleteID = inst.ID
		}
	}
	if deleteID == 0 {
		t.Fatalf("expected pfsense instance in list: %+v", listResp)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status returned %d: %s", rec.Code, rec.Body.String())
	}
	var statusResp struct {
		Configured bool   `json:"configured"`
		Type       string `json:"type"`
	}
	decodeJSON(t, rec.Body.Bytes(), &statusResp)
	if !statusResp.Configured || statusResp.Type != "opnsense" {
		t.Fatalf("unexpected setup status: %+v", statusResp)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/instances/%d", deleteID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete instance returned %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayOverviewInventoryMigrationAndBackupSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	withTempWorkingDir(t)

	configXML := []byte("<opnsense><nat><outbound><mode>hybrid</mode></outbound></nat><padding>xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx</padding></opnsense>")
	legacyCSV := []byte("id,rule\n1,legacy-a\n2,legacy-b\n")

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/diagnostics/system/system_time":
			writeJSON(t, w, http.StatusOK, map[string]any{"uptime": "5 days", "datetime": "2026-03-10 10:00:00", "loadavg": "0.10 0.20 0.30"})
		case "/api/diagnostics/system/system_resources":
			writeJSON(t, w, http.StatusOK, map[string]any{"memory": map[string]any{"total_frmt": "4096", "used_frmt": "1024"}})
		case "/api/diagnostics/system/system_disk":
			writeJSON(t, w, http.StatusOK, map[string]any{"devices": []any{map[string]any{"mountpoint": "/", "used_pct": "42%"}}})
		case "/api/routes/gateway/status":
			writeJSON(t, w, http.StatusOK, map[string]any{"items": []any{map[string]any{"name": "GW_TEST", "status_translated": "Online"}}})
		case "/api/wireguard/service/show":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"type": "interface"}, map[string]any{"type": "peer", "peer-status": "online"}}})
		case "/api/routing/settings/search_gateway":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"uuid": "gw-1", "name": "GW_TEST", "interface": "opt1", "gateway": "10.64.0.1", "ipprotocol": "inet", "disabled": "0", "defaultgw": "0", "descr": "gateway"}}})
		case "/api/interfaces/overview/interfaces_info/0":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"identifier": "lan", "device": "em0", "description": "LAN", "enabled": "1", "type": "static", "addr4": "192.168.1.1", "macaddr": "aa:bb"}, map[string]any{"identifier": "opt1", "device": "wg0", "description": "WG", "enabled": "1", "type": "wireguard", "addr4": "10.64.0.2"}}})
		case "/api/firewall/alias/search_item":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"uuid": "alias-1", "name": "GATOR_TEST", "type": "network", "content": "10.0.0.0/8", "description": "Gator alias", "enabled": "1"}}})
		case "/api/firewall/source_nat/search_rule":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"uuid": "nat-1", "interface": "wan", "source_net": "lan", "destination_net": "any", "protocol": "any", "target": "wanip", "description": "GATOR_SNAT", "enabled": "1"}}})
		case "/api/firewall/filter/search_rule", "/api/firewall/filter/searchRule":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"uuid": "rule-1", "action": "pass", "quick": "1", "interface": "lan", "direction": "in", "ipprotocol": "inet", "protocol": "tcp", "source_net": "any", "destination_net": "any", "destination_port": "22", "gateway": "GW_TEST", "description": "GATOR_RULE", "enabled": "1"}}})
		case "/api/core/backup/download/this":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(configXML)
		case "/api/firewall/migration/download_rules":
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write(legacyCSV)
		case "/api/firewall/filter/upload_rules":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		case "/api/firewall/filter/savepoint":
			writeJSON(t, w, http.StatusOK, map[string]any{"revision": "rev-migration"})
		case "/api/firewall/filter/apply/rev-migration", "/api/firewall/filter/apply":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		case "/api/firewall/filter/cancel_rollback/rev-migration":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		case "/api/firewall/migration/flush":
			writeJSON(t, w, http.StatusOK, map[string]any{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	ctx := context.Background()
	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{Type: "opnsense", Host: api.URL, APIKey: "key", APISecret: "secret"}); err != nil {
		t.Fatalf("save firewall config: %v", err)
	}
	if err := store.SetCache(ctx, "firmware_name", "OPNsense"); err != nil {
		t.Fatalf("set cache: %v", err)
	}
	if err := store.SetCache(ctx, "firmware_version", "24.7"); err != nil {
		t.Fatalf("set cache: %v", err)
	}
	if _, err := store.CreateVPNConfig(ctx, models.SimpleVPNConfig{Name: "Overview VPN", Protocol: "wireguard", LocalCIDR: "10.64.0.2/32", RemoteCIDR: "0.0.0.0/0", Endpoint: "vpn.example.com:51820", Enabled: true}); err != nil {
		t.Fatalf("seed vpn config: %v", err)
	}

	router := gin.New()
	opnsenseHandler := NewOPNsenseHandler(store)
	gatewayHandler := NewGatewayHandler(store)
	router.GET("/api/opnsense/overview", opnsenseHandler.Overview)
	router.GET("/api/opnsense/gateways", gatewayHandler.ListGateways)
	router.GET("/api/opnsense/interfaces", gatewayHandler.ListInterfaces)
	router.GET("/api/opnsense/interfaces/selectable", gatewayHandler.ListSelectableInterfaces)
	router.GET("/api/opnsense/aliases", gatewayHandler.ListAliases)
	router.GET("/api/opnsense/nat-rules", gatewayHandler.ListNATRules)
	router.GET("/api/opnsense/rules", gatewayHandler.ListFilterRules)
	router.GET("/api/opnsense/nat/mode", gatewayHandler.GetNATMode)
	router.GET("/api/opnsense/migration/status", gatewayHandler.MigrationStatus)
	router.GET("/api/opnsense/migration/download", gatewayHandler.MigrationDownload)
	router.POST("/api/opnsense/migration/upload", gatewayHandler.MigrationUpload)
	router.POST("/api/opnsense/migration/apply", gatewayHandler.MigrationApply)
	router.POST("/api/opnsense/migration/confirm", gatewayHandler.MigrationConfirm)
	router.POST("/api/opnsense/migration/flush", gatewayHandler.MigrationFlush)
	router.GET("/api/opnsense/backups", gatewayHandler.ListBackups)
	router.POST("/api/opnsense/backups", gatewayHandler.CreateBackup)
	router.GET("/api/opnsense/backups/:filename", gatewayHandler.DownloadBackup)
	router.DELETE("/api/opnsense/backups/:filename", gatewayHandler.DeleteBackup)

	for _, path := range []string{"/api/opnsense/overview", "/api/opnsense/gateways", "/api/opnsense/interfaces", "/api/opnsense/interfaces/selectable", "/api/opnsense/aliases", "/api/opnsense/nat-rules", "/api/opnsense/rules", "/api/opnsense/nat/mode", "/api/opnsense/migration/status", "/api/opnsense/migration/download"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d: %s", path, rec.Code, rec.Body.String())
		}
	}

	rec := performJSONRequest(t, router, http.MethodPost, "/api/opnsense/migration/upload", map[string]any{"csv": string(legacyCSV)})
	if rec.Code != http.StatusOK {
		t.Fatalf("migration upload returned %d: %s", rec.Code, rec.Body.String())
	}
	rec = performJSONRequest(t, router, http.MethodPost, "/api/opnsense/migration/apply", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("migration apply returned %d: %s", rec.Code, rec.Body.String())
	}
	rec = performJSONRequest(t, router, http.MethodPost, "/api/opnsense/migration/confirm", map[string]any{"revision": "rev-migration"})
	if rec.Code != http.StatusOK {
		t.Fatalf("migration confirm returned %d: %s", rec.Code, rec.Body.String())
	}
	rec = performJSONRequest(t, router, http.MethodPost, "/api/opnsense/migration/flush", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("migration flush returned %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/opnsense/backups", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("backup create returned %d: %s", rec.Code, rec.Body.String())
	}
	var backupResp struct {
		Filename string `json:"filename"`
	}
	decodeJSON(t, rec.Body.Bytes(), &backupResp)
	if backupResp.Filename == "" {
		t.Fatalf("expected backup filename in response: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/opnsense/backups", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("backup list returned %d: %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/opnsense/backups/"+backupResp.Filename, nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("backup download returned %d len=%d", rec.Code, rec.Body.Len())
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/opnsense/backups/"+backupResp.Filename, nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("backup delete returned %d: %s", rec.Code, rec.Body.String())
	}
}

func TestIPRangesSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	withTempWorkingDir(t)

	router := gin.New()
	ipHandler := NewIPRangesHandler(store)
	router.POST("/api/ip-ranges/upload", ipHandler.Upload)
	router.GET("/api/ip-ranges", ipHandler.List)
	router.GET("/api/ip-ranges/serve/:filename", ipHandler.Serve)
	router.DELETE("/api/ip-ranges/:filename", ipHandler.Delete)

	rec := performMultipartUpload(t, router, "/api/ip-ranges/upload", "file", "ranges.json", []byte(`{"prefixes":["1.1.1.0/24"]}`), map[string]string{"filename": "custom-ranges.json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("ip range upload returned %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ip-ranges", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ip range list returned %d: %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/ip-ranges/serve/custom-ranges.json", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("prefixes")) {
		t.Fatalf("ip range serve returned %d: %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/ip-ranges/custom-ranges.json", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ip range delete returned %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTunnelCrudDiscoverAndImportSmoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/wireguard/server/search_server":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"uuid": "server-tunnel", "name": "Tunnel Server", "port": "51820", "instance": "0"}}})
		case "/api/interfaces/overview/interfaces_info/0":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"identifier": "opt1", "device": "wg0", "description": "WG Tunnel", "enabled": "1"}}})
		case "/api/routing/settings/search_gateway":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"uuid": "gw-tunnel", "name": "GW_TUNNEL", "interface": "opt1", "gateway": "10.200.200.1"}}})
		case "/api/firewall/filter/search_rule", "/api/firewall/filter/searchRule":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"uuid": "filter-tunnel", "description": "Legacy tunnel", "gateway": "GW_TUNNEL", "interface": "lan"}}})
		case "/api/firewall/source_nat/search_rule", "/api/firewall/source_nat/searchRule":
			writeJSON(t, w, http.StatusOK, map[string]any{"rows": []any{map[string]any{"uuid": "snat-tunnel", "interface": "opt1", "description": "Tunnel SNAT"}}})
		case "/api/wireguard/server/get_server/server-tunnel":
			writeJSON(t, w, http.StatusOK, map[string]any{"server": map[string]any{"tunneladdress": "10.200.200.2/24", "privkey": "priv-key", "peers": "peer-tunnel"}})
		case "/api/wireguard/client/get_client/peer-tunnel":
			writeJSON(t, w, http.StatusOK, map[string]any{"client": map[string]any{"name": "peer-tunnel", "pubkey": "peer-pub", "tunneladdress": "10.200.200.1/24", "serveraddress": "198.51.100.10", "serverport": "51820"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	ctx := context.Background()
	if err := store.SaveFirewallConfig(ctx, models.FirewallConfig{Type: "opnsense", Host: api.URL, APIKey: "key", APISecret: "secret"}); err != nil {
		t.Fatalf("save firewall config: %v", err)
	}

	router := gin.New()
	tunnelHandler := NewTunnelHandler(store)
	router.GET("/api/tunnels", tunnelHandler.ListTunnels)
	router.POST("/api/tunnels", tunnelHandler.CreateTunnel)
	router.GET("/api/tunnels/:id", tunnelHandler.GetTunnel)
	router.PUT("/api/tunnels/:id", tunnelHandler.SaveTunnel)
	router.DELETE("/api/tunnels/:id", tunnelHandler.DeleteTunnel)
	router.GET("/api/tunnels/next-subnet", tunnelHandler.NextSubnet)
	router.GET("/api/tunnels/discover", tunnelHandler.DiscoverTunnels)
	router.POST("/api/tunnels/import", tunnelHandler.ImportTunnel)

	rec := performJSONRequest(t, router, http.MethodPost, "/api/tunnels", map[string]any{"name": "Site A", "remote_host": "198.51.100.20"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tunnel returned %d: %s", rec.Code, rec.Body.String())
	}
	var created models.SiteTunnelDetail
	decodeJSON(t, rec.Body.Bytes(), &created)
	if created.ID == 0 || created.TunnelSubnet == "" || created.RemoteHost != "198.51.100.20" {
		t.Fatalf("unexpected created tunnel: %+v", created)
	}

	for _, path := range []string{fmt.Sprintf("/api/tunnels/%d", created.ID), "/api/tunnels", "/api/tunnels/next-subnet", "/api/tunnels/discover"} {
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d: %s", path, rec.Code, rec.Body.String())
		}
	}

	rec = performJSONRequest(t, router, http.MethodPut, fmt.Sprintf("/api/tunnels/%d", created.ID), map[string]any{"name": "Site A Updated", "remote_host": "198.51.100.30"})
	if rec.Code != http.StatusOK {
		t.Fatalf("save tunnel returned %d: %s", rec.Code, rec.Body.String())
	}

	rec = performJSONRequest(t, router, http.MethodPost, "/api/tunnels/import", map[string]any{"name": "Imported Tunnel", "server_uuid": "server-tunnel", "peer_uuid": "peer-tunnel", "local_cidr": "10.200.200.2/24", "endpoint": "198.51.100.10:51820", "peer_pubkey": "peer-pub"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("import tunnel returned %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/tunnels/%d", created.ID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete tunnel returned %d: %s", rec.Code, rec.Body.String())
	}
}

func newTestStore(t *testing.T) *storage.SQLiteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gator-test.db")
	store, err := storage.NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func performJSONRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func performMultipartUpload(t *testing.T, router http.Handler, path, fieldName, fileName string, data []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, raw []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode JSON %s: %v", string(raw), err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("write JSON response: %v", err)
	}
}

func resetPendingRevisions() {
	pendingRevisionsMu.Lock()
	defer pendingRevisionsMu.Unlock()
	pendingRevisions = make(map[string]*pendingRevisionInfo)
}

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
}
