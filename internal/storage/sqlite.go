package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/anothaDev/gator/internal/models"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite database: %w", err)
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ─── Instance Management ──────────────────────────────────────────

// CreateInstance creates a new firewall instance and returns its ID.
func (s *SQLiteStore) CreateInstance(ctx context.Context, inst models.FirewallInstance) (int64, error) {
	const query = `
		INSERT INTO firewall_instances (label, firewall_type, host, api_key, api_secret, api_token, skip_tls)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	skipTLS := 0
	if inst.SkipTLS {
		skipTLS = 1
	}
	result, err := s.db.ExecContext(ctx, query,
		inst.Label, inst.Type, inst.Host, inst.APIKey, inst.APISecret, inst.APIToken, skipTLS,
	)
	if err != nil {
		return 0, fmt.Errorf("create instance: %w", err)
	}
	return result.LastInsertId()
}

// UpdateInstance updates an existing firewall instance's connection details.
func (s *SQLiteStore) UpdateInstance(ctx context.Context, inst models.FirewallInstance) error {
	const query = `
		UPDATE firewall_instances SET
			label = ?, firewall_type = ?, host = ?,
			api_key = ?, api_secret = ?, api_token = ?, skip_tls = ?
		WHERE id = ?
	`
	skipTLS := 0
	if inst.SkipTLS {
		skipTLS = 1
	}
	_, err := s.db.ExecContext(ctx, query,
		inst.Label, inst.Type, inst.Host, inst.APIKey, inst.APISecret, inst.APIToken, skipTLS, inst.ID,
	)
	return err
}

// ListInstances returns all saved firewall instances.
func (s *SQLiteStore) ListInstances(ctx context.Context) ([]models.FirewallInstance, error) {
	const query = `SELECT id, label, firewall_type, host, api_key, api_secret, api_token, skip_tls, created_at FROM firewall_instances ORDER BY id`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []models.FirewallInstance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, *inst)
	}
	return instances, rows.Err()
}

// GetInstance returns a single instance by ID, or nil.
func (s *SQLiteStore) GetInstance(ctx context.Context, id int64) (*models.FirewallInstance, error) {
	const query = `SELECT id, label, firewall_type, host, api_key, api_secret, api_token, skip_tls, created_at FROM firewall_instances WHERE id = ?`
	inst, err := scanInstance(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// DeleteInstance removes a firewall instance and all its scoped data.
func (s *SQLiteStore) DeleteInstance(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete all scoped data in dependency order.
	// Errors are collected — any failure should abort the transaction.
	deletes := []string{
		`DELETE FROM app_routing_rules WHERE vpn_config_id IN (SELECT id FROM vpn_configs WHERE instance_id = ?)`,
		`DELETE FROM vpn_configs WHERE instance_id = ?`,
		`DELETE FROM site_tunnels WHERE instance_id = ?`,
		`DELETE FROM custom_app_profiles WHERE instance_id = ?`,
		`DELETE FROM cache WHERE key LIKE 'inst_' || ? || '_%'`,
		`DELETE FROM active_instance WHERE instance_id = ?`,
	}
	for _, q := range deletes {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return fmt.Errorf("cascade delete for instance %d: %w", id, err)
		}
	}

	// Delete the instance itself.
	if _, err := tx.ExecContext(ctx, `DELETE FROM firewall_instances WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete instance %d: %w", id, err)
	}

	return tx.Commit()
}

// SetActiveInstance sets the currently active instance.
func (s *SQLiteStore) SetActiveInstance(ctx context.Context, instanceID int64) error {
	const query = `
		INSERT INTO active_instance (id, instance_id)
		VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET instance_id = excluded.instance_id
	`
	_, err := s.db.ExecContext(ctx, query, instanceID)
	return err
}

// GetActiveInstanceID returns the active instance ID, or 0 if none.
func (s *SQLiteStore) GetActiveInstanceID(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT instance_id FROM active_instance WHERE id = 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// GetActiveInstance returns the active firewall instance, or nil if none set.
func (s *SQLiteStore) GetActiveInstance(ctx context.Context) (*models.FirewallInstance, error) {
	id, err := s.GetActiveInstanceID(ctx)
	if err != nil || id == 0 {
		return nil, err
	}
	return s.GetInstance(ctx, id)
}

// ─── Backward compat: GetFirewallConfig / SaveFirewallConfig ──────
// These wrap the instance system so existing handler interfaces still work.

// GetFirewallConfig returns the active instance as a FirewallConfig.
func (s *SQLiteStore) GetFirewallConfig(ctx context.Context) (*models.FirewallConfig, error) {
	inst, err := s.GetActiveInstance(ctx)
	if err != nil || inst == nil {
		return nil, err
	}
	cfg := inst.Config()
	return &cfg, nil
}

// SaveFirewallConfig creates or updates an instance from a FirewallConfig.
// If an instance with the same host exists, it updates it. Otherwise creates new.
// Sets it as the active instance.
func (s *SQLiteStore) SaveFirewallConfig(ctx context.Context, cfg models.FirewallConfig) error {
	// Check if an instance with this host already exists.
	var existingID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM firewall_instances WHERE host = ?`, cfg.Host,
	).Scan(&existingID)

	if err == nil && existingID > 0 {
		// Update existing instance.
		inst := models.FirewallInstance{
			ID:        existingID,
			Label:     labelFromHost(cfg.Host),
			Type:      cfg.Type,
			Host:      cfg.Host,
			APIKey:    cfg.APIKey,
			APISecret: cfg.APISecret,
			APIToken:  cfg.APIToken,
			SkipTLS:   cfg.SkipTLS,
		}
		if err := s.UpdateInstance(ctx, inst); err != nil {
			return err
		}
		return s.SetActiveInstance(ctx, existingID)
	}

	// Create new instance.
	inst := models.FirewallInstance{
		Label:     labelFromHost(cfg.Host),
		Type:      cfg.Type,
		Host:      cfg.Host,
		APIKey:    cfg.APIKey,
		APISecret: cfg.APISecret,
		APIToken:  cfg.APIToken,
		SkipTLS:   cfg.SkipTLS,
	}
	id, err := s.CreateInstance(ctx, inst)
	if err != nil {
		return err
	}
	return s.SetActiveInstance(ctx, id)
}

func scanInstance(scanner interface{ Scan(dest ...any) error }) (*models.FirewallInstance, error) {
	var inst models.FirewallInstance
	var skipTLS int
	err := scanner.Scan(
		&inst.ID, &inst.Label, &inst.Type, &inst.Host,
		&inst.APIKey, &inst.APISecret, &inst.APIToken,
		&skipTLS, &inst.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	inst.SkipTLS = skipTLS == 1
	return &inst, nil
}

// ─── VPN Configs (instance-scoped) ───────────────────────────────

// vpnSelectColumns is the list of columns read from vpn_configs (excluding id).
const vpnSelectColumns = `
	instance_id, name, protocol, ip_version, routing_mode, local_cidr, remote_cidr, endpoint, dns,
	private_key, peer_public_key, pre_shared_key,
	source_interfaces_json,
	opnsense_peer_uuid, opnsense_server_uuid,
	opnsense_gateway_uuid, opnsense_gateway_name,
	opnsense_snat_rule_uuids, opnsense_filter_uuids,
	opnsense_wg_interface, opnsense_wg_device, last_applied_at, enabled, routing_applied
`

func scanVPNConfig(scanner interface{ Scan(dest ...any) error }) (*models.SimpleVPNConfig, error) {
	var cfg models.SimpleVPNConfig
	var instanceID int64
	var enabled, routingApplied int
	var sourceInterfacesJSON string
	err := scanner.Scan(
		&cfg.ID,
		&instanceID,
		&cfg.Name, &cfg.Protocol, &cfg.IPVersion, &cfg.RoutingMode, &cfg.LocalCIDR, &cfg.RemoteCIDR,
		&cfg.Endpoint, &cfg.DNS, &cfg.PrivateKey, &cfg.PeerPublicKey,
		&cfg.PreSharedKey,
		&sourceInterfacesJSON,
		&cfg.OPNsensePeerUUID, &cfg.OPNsenseServerUUID,
		&cfg.OPNsenseGatewayUUID, &cfg.OPNsenseGatewayName,
		&cfg.OPNsenseSNATRuleUUIDs, &cfg.OPNsenseFilterUUIDs,
		&cfg.OPNsenseWGInterface, &cfg.OPNsenseWGDevice, &cfg.LastAppliedAt,
		&enabled, &routingApplied,
	)
	if err != nil {
		return nil, err
	}
	cfg.InstanceID = instanceID
	cfg.Enabled = enabled == 1
	cfg.RoutingApplied = routingApplied == 1
	if cfg.IPVersion == "" {
		cfg.IPVersion = "ipv4"
	}
	if cfg.RoutingMode == "" {
		cfg.RoutingMode = "all"
	}
	if sourceInterfacesJSON != "" && sourceInterfacesJSON != "[]" {
		_ = json.Unmarshal([]byte(sourceInterfacesJSON), &cfg.SourceInterfaces)
	}
	return &cfg, nil
}

func vpnWriteArgs(cfg models.SimpleVPNConfig) []any {
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	routingApplied := 0
	if cfg.RoutingApplied {
		routingApplied = 1
	}
	ipVersion := cfg.IPVersion
	if ipVersion == "" {
		ipVersion = "ipv4"
	}
	routingMode := cfg.RoutingMode
	if routingMode == "" {
		routingMode = "all"
	}
	sourceInterfacesJSON := "[]"
	if len(cfg.SourceInterfaces) > 0 {
		if data, err := json.Marshal(cfg.SourceInterfaces); err == nil {
			sourceInterfacesJSON = string(data)
		}
	}
	return []any{
		cfg.InstanceID,
		cfg.Name, cfg.Protocol, ipVersion, routingMode, cfg.LocalCIDR, cfg.RemoteCIDR,
		cfg.Endpoint, cfg.DNS, cfg.PrivateKey, cfg.PeerPublicKey,
		cfg.PreSharedKey,
		sourceInterfacesJSON,
		cfg.OPNsensePeerUUID, cfg.OPNsenseServerUUID,
		cfg.OPNsenseGatewayUUID, cfg.OPNsenseGatewayName,
		cfg.OPNsenseSNATRuleUUIDs, cfg.OPNsenseFilterUUIDs,
		cfg.OPNsenseWGInterface, cfg.OPNsenseWGDevice, cfg.LastAppliedAt,
		enabled, routingApplied,
	}
}

func (s *SQLiteStore) CreateVPNConfig(ctx context.Context, cfg models.SimpleVPNConfig) (int64, error) {
	// Ensure instance_id is set.
	if cfg.InstanceID == 0 {
		id, _ := s.GetActiveInstanceID(ctx)
		cfg.InstanceID = id
	}

	// Check for duplicate name within this instance.
	var existing int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_configs WHERE instance_id = ? AND name = ?`, cfg.InstanceID, cfg.Name,
	).Scan(&existing)
	if err != nil {
		return 0, fmt.Errorf("check duplicate vpn name: %w", err)
	}
	if existing > 0 {
		return 0, fmt.Errorf("a VPN profile named %q already exists", cfg.Name)
	}

	const query = `
		INSERT INTO vpn_configs (
			instance_id,
			name, protocol, ip_version, routing_mode, local_cidr, remote_cidr, endpoint, dns,
			private_key, peer_public_key, pre_shared_key,
			source_interfaces_json,
			opnsense_peer_uuid, opnsense_server_uuid,
			opnsense_gateway_uuid, opnsense_gateway_name,
			opnsense_snat_rule_uuids, opnsense_filter_uuids,
			opnsense_wg_interface, opnsense_wg_device, last_applied_at, enabled, routing_applied
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.ExecContext(ctx, query, vpnWriteArgs(cfg)...)
	if err != nil {
		return 0, fmt.Errorf("create vpn config: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) SaveSimpleVPNConfig(ctx context.Context, cfg models.SimpleVPNConfig) error {
	if cfg.ID == 0 {
		id, err := s.CreateVPNConfig(ctx, cfg)
		if err != nil {
			return err
		}
		cfg.ID = id
		return nil
	}

	// Safety: never write instance_id=0 on updates. Resolve from existing row or active instance.
	if cfg.InstanceID == 0 {
		var existingInstanceID int64
		_ = s.db.QueryRowContext(ctx, `SELECT instance_id FROM vpn_configs WHERE id = ?`, cfg.ID).Scan(&existingInstanceID)
		if existingInstanceID != 0 {
			cfg.InstanceID = existingInstanceID
		} else {
			cfg.InstanceID, _ = s.GetActiveInstanceID(ctx)
		}
	}

	const query = `
		UPDATE vpn_configs SET
			instance_id = ?,
			name = ?, protocol = ?, ip_version = ?, routing_mode = ?, local_cidr = ?, remote_cidr = ?,
			endpoint = ?, dns = ?, private_key = ?, peer_public_key = ?,
			pre_shared_key = ?,
			source_interfaces_json = ?,
			opnsense_peer_uuid = ?, opnsense_server_uuid = ?,
			opnsense_gateway_uuid = ?, opnsense_gateway_name = ?,
			opnsense_snat_rule_uuids = ?, opnsense_filter_uuids = ?,
			opnsense_wg_interface = ?, opnsense_wg_device = ?, last_applied_at = ?, enabled = ?, routing_applied = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	args := append(vpnWriteArgs(cfg), cfg.ID)
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("save vpn config: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetSimpleVPNConfig(ctx context.Context) (*models.SimpleVPNConfig, error) {
	// Backward compat: returns the first VPN config for the active instance.
	instanceID, _ := s.GetActiveInstanceID(ctx)
	if instanceID == 0 {
		return nil, nil
	}
	configs, err := s.ListVPNConfigs(ctx)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}
	return configs[0], nil
}

func (s *SQLiteStore) GetVPNConfigByID(ctx context.Context, id int64) (*models.SimpleVPNConfig, error) {
	query := `SELECT id, ` + vpnSelectColumns + ` FROM vpn_configs WHERE id = ?`
	cfg, err := scanVPNConfig(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get vpn config %d: %w", id, err)
	}
	return cfg, nil
}

// ListVPNConfigs returns VPN configs for the active instance.
func (s *SQLiteStore) ListVPNConfigs(ctx context.Context) ([]*models.SimpleVPNConfig, error) {
	instanceID, _ := s.GetActiveInstanceID(ctx)
	if instanceID == 0 {
		return nil, nil
	}
	return s.ListVPNConfigsForInstance(ctx, instanceID)
}

// ListVPNConfigsForInstance returns VPN configs for a specific instance.
func (s *SQLiteStore) ListVPNConfigsForInstance(ctx context.Context, instanceID int64) ([]*models.SimpleVPNConfig, error) {
	query := `SELECT id, ` + vpnSelectColumns + ` FROM vpn_configs WHERE instance_id = ? ORDER BY id`
	rows, err := s.db.QueryContext(ctx, query, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list vpn configs: %w", err)
	}
	defer rows.Close()

	var configs []*models.SimpleVPNConfig
	for rows.Next() {
		cfg, err := scanVPNConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("scan vpn config: %w", err)
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

func (s *SQLiteStore) DeleteVPNConfig(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM vpn_configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete vpn config %d: %w", id, err)
	}
	return nil
}

// ─── App routing rules ───────────────────────────────────────────

// ListAppRoutes returns all app routing rules for a VPN config.
func (s *SQLiteStore) ListAppRoutes(ctx context.Context, vpnConfigID int64) ([]*models.AppRoute, error) {
	const query = `SELECT id, vpn_config_id, app_id, enabled, opnsense_rule_uuids FROM app_routing_rules WHERE vpn_config_id = ? ORDER BY app_id`
	rows, err := s.db.QueryContext(ctx, query, vpnConfigID)
	if err != nil {
		return nil, fmt.Errorf("list app routes: %w", err)
	}
	defer rows.Close()

	var routes []*models.AppRoute
	for rows.Next() {
		var r models.AppRoute
		var enabled int
		if err := rows.Scan(&r.ID, &r.VPNConfigID, &r.AppID, &enabled, &r.OPNsenseRuleUUIDs); err != nil {
			return nil, fmt.Errorf("scan app route: %w", err)
		}
		r.Enabled = enabled == 1
		routes = append(routes, &r)
	}
	return routes, rows.Err()
}

// GetAppRoute returns a single app route by VPN config ID and app ID.
func (s *SQLiteStore) GetAppRoute(ctx context.Context, vpnConfigID int64, appID string) (*models.AppRoute, error) {
	const query = `SELECT id, vpn_config_id, app_id, enabled, opnsense_rule_uuids FROM app_routing_rules WHERE vpn_config_id = ? AND app_id = ?`
	var r models.AppRoute
	var enabled int
	err := s.db.QueryRowContext(ctx, query, vpnConfigID, appID).Scan(&r.ID, &r.VPNConfigID, &r.AppID, &enabled, &r.OPNsenseRuleUUIDs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app route: %w", err)
	}
	r.Enabled = enabled == 1
	return &r, nil
}

// UpsertAppRoute creates or updates an app routing rule.
func (s *SQLiteStore) UpsertAppRoute(ctx context.Context, r models.AppRoute) error {
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	const query = `
		INSERT INTO app_routing_rules (vpn_config_id, app_id, enabled, opnsense_rule_uuids, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(vpn_config_id, app_id) DO UPDATE SET
			enabled = excluded.enabled,
			opnsense_rule_uuids = excluded.opnsense_rule_uuids,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := s.db.ExecContext(ctx, query, r.VPNConfigID, r.AppID, enabled, r.OPNsenseRuleUUIDs)
	if err != nil {
		return fmt.Errorf("upsert app route: %w", err)
	}
	return nil
}

// DeleteAppRoutesForVPN removes all app routing rules for a VPN config.
func (s *SQLiteStore) DeleteAppRoutesForVPN(ctx context.Context, vpnConfigID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_routing_rules WHERE vpn_config_id = ?`, vpnConfigID)
	return err
}

// ListAppRoutesByAppID returns all app routing rules matching a given app_id across all VPNs.
func (s *SQLiteStore) ListAppRoutesByAppID(ctx context.Context, appID string) ([]*models.AppRoute, error) {
	const query = `SELECT id, vpn_config_id, app_id, enabled, opnsense_rule_uuids FROM app_routing_rules WHERE app_id = ?`
	rows, err := s.db.QueryContext(ctx, query, appID)
	if err != nil {
		return nil, fmt.Errorf("list app routes by app_id: %w", err)
	}
	defer rows.Close()

	var routes []*models.AppRoute
	for rows.Next() {
		var r models.AppRoute
		var enabled int
		if err := rows.Scan(&r.ID, &r.VPNConfigID, &r.AppID, &enabled, &r.OPNsenseRuleUUIDs); err != nil {
			return nil, fmt.Errorf("scan app route: %w", err)
		}
		r.Enabled = enabled == 1
		routes = append(routes, &r)
	}
	return routes, rows.Err()
}

// DeleteAppRoutesByAppID removes all app routing rules matching a given app_id.
func (s *SQLiteStore) DeleteAppRoutesByAppID(ctx context.Context, appID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_routing_rules WHERE app_id = ?`, appID)
	return err
}

// ─── Custom App Profiles (instance-scoped) ───────────────────────

// ListCustomAppProfiles returns user-defined app profiles for the active instance.
func (s *SQLiteStore) ListCustomAppProfiles(ctx context.Context) ([]models.AppProfile, error) {
	instanceID, _ := s.GetActiveInstanceID(ctx)
	if instanceID == 0 {
		return nil, nil
	}
	const query = `SELECT id, name, category, rules_json, note, asns_json FROM custom_app_profiles WHERE instance_id = ? ORDER BY name`
	return s.scanCustomProfiles(ctx, query, instanceID)
}

func (s *SQLiteStore) scanCustomProfiles(ctx context.Context, query string, args ...any) ([]models.AppProfile, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []models.AppProfile
	for rows.Next() {
		var p models.AppProfile
		var rulesJSON, asnsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &rulesJSON, &p.Note, &asnsJSON); err != nil {
			return nil, err
		}
		p.IsCustom = true
		p.Icon = "custom"
		if err := json.Unmarshal([]byte(rulesJSON), &p.Rules); err != nil {
			p.Rules = nil
		}
		if asnsJSON != "" {
			if err := json.Unmarshal([]byte(asnsJSON), &p.ASNs); err != nil {
				p.ASNs = nil
			}
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// GetCustomAppProfile returns a single custom profile by ID, scoped to the active instance.
func (s *SQLiteStore) GetCustomAppProfile(ctx context.Context, id string) (*models.AppProfile, error) {
	instanceID, _ := s.GetActiveInstanceID(ctx)
	if instanceID == 0 {
		return nil, nil
	}
	const query = `SELECT id, name, category, rules_json, note, asns_json FROM custom_app_profiles WHERE id = ? AND instance_id = ?`
	var p models.AppProfile
	var rulesJSON, asnsJSON string
	err := s.db.QueryRowContext(ctx, query, id, instanceID).Scan(&p.ID, &p.Name, &p.Category, &rulesJSON, &p.Note, &asnsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsCustom = true
	p.Icon = "custom"
	if err := json.Unmarshal([]byte(rulesJSON), &p.Rules); err != nil {
		p.Rules = nil
	}
	if asnsJSON != "" {
		if err := json.Unmarshal([]byte(asnsJSON), &p.ASNs); err != nil {
			p.ASNs = nil
		}
	}
	return &p, nil
}

// CreateCustomAppProfile inserts a new custom app profile scoped to the active instance.
func (s *SQLiteStore) CreateCustomAppProfile(ctx context.Context, p models.AppProfile) error {
	instanceID, _ := s.GetActiveInstanceID(ctx)
	rulesJSON, err := json.Marshal(p.Rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	asnsJSON := "[]"
	if len(p.ASNs) > 0 {
		if data, err := json.Marshal(p.ASNs); err == nil {
			asnsJSON = string(data)
		}
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO custom_app_profiles (id, instance_id, name, category, rules_json, note, asns_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, instanceID, p.Name, p.Category, string(rulesJSON), p.Note, asnsJSON,
	)
	return err
}

// DeleteCustomAppProfile removes a custom app profile by ID, scoped to the active instance.
func (s *SQLiteStore) DeleteCustomAppProfile(ctx context.Context, id string) error {
	instanceID, _ := s.GetActiveInstanceID(ctx)
	if instanceID == 0 {
		return fmt.Errorf("no active instance")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM custom_app_profiles WHERE id = ? AND instance_id = ?`, id, instanceID)
	return err
}

// ─── Cache ───────────────────────────────────────────────────────

// GetCache retrieves a cached value by key. Returns ("", nil) if not found.
func (s *SQLiteStore) GetCache(ctx context.Context, key string) (string, error) {
	const query = `SELECT value FROM cache WHERE key = ?`
	var value string
	err := s.db.QueryRowContext(ctx, query, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// ListCacheKeysByPrefix returns all cache keys matching a prefix, with their last update times.
func (s *SQLiteStore) ListCacheKeysByPrefix(ctx context.Context, prefix string) ([]CacheEntry, error) {
	const query = `SELECT key, updated_at FROM cache WHERE key LIKE ? || '%'`
	rows, err := s.db.QueryContext(ctx, query, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []CacheEntry
	for rows.Next() {
		var e CacheEntry
		if err := rows.Scan(&e.Key, &e.UpdatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CacheEntry represents a cache key and its update timestamp.
type CacheEntry struct {
	Key       string
	UpdatedAt string
}

// SetCache upserts a cached value.
func (s *SQLiteStore) SetCache(ctx context.Context, key, value string) error {
	const query = `
		INSERT INTO cache (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := s.db.ExecContext(ctx, query, key, value)
	return err
}

// ─── Site Tunnels ────────────────────────────────────────────────

const tunnelSelectColumns = `instance_id, name, description,
	remote_host, ssh_port, ssh_user, ssh_private_key, ssh_password,
	tunnel_subnet, firewall_ip, remote_ip, listen_port, keepalive,
	firewall_private_key, firewall_public_key, remote_private_key, remote_public_key,
	remote_wg_interface, opnsense_peer_uuid, opnsense_server_uuid,
	original_remote_host, original_ssh_port, ssh_phase,
	ssh_socket_was_active, ufw_was_active,
	deployed, status, created_at, updated_at`

func tunnelWriteArgs(t models.SiteTunnel) []any {
	deployed := 0
	if t.Deployed {
		deployed = 1
	}
	sshPhase := t.SSHPhase
	if sshPhase == "" {
		sshPhase = models.SSHPhasePublic
	}
	sshSocketWasActive := 0
	if t.SSHSocketWasActive {
		sshSocketWasActive = 1
	}
	ufwWasActive := 0
	if t.UFWWasActive {
		ufwWasActive = 1
	}
	return []any{
		t.InstanceID, t.Name, t.Description,
		t.RemoteHost, t.SSHPort, t.SSHUser, t.SSHPrivateKey, t.SSHPassword,
		t.TunnelSubnet, t.FirewallIP, t.RemoteIP, t.ListenPort, t.Keepalive,
		t.FirewallPrivateKey, t.FirewallPublicKey, t.RemotePrivateKey, t.RemotePublicKey,
		t.RemoteWGInterface, t.OPNsensePeerUUID, t.OPNsenseServerUUID,
		t.OriginalRemoteHost, t.OriginalSSHPort, sshPhase,
		sshSocketWasActive, ufwWasActive,
		deployed, t.Status,
	}
}

type tunnelScanner interface {
	Scan(dest ...any) error
}

func scanTunnel(row tunnelScanner) (*models.SiteTunnel, error) {
	var t models.SiteTunnel
	var deployed, sshSocketWasActive, ufwWasActive int
	err := row.Scan(
		&t.ID,
		&t.InstanceID, &t.Name, &t.Description,
		&t.RemoteHost, &t.SSHPort, &t.SSHUser, &t.SSHPrivateKey, &t.SSHPassword,
		&t.TunnelSubnet, &t.FirewallIP, &t.RemoteIP, &t.ListenPort, &t.Keepalive,
		&t.FirewallPrivateKey, &t.FirewallPublicKey, &t.RemotePrivateKey, &t.RemotePublicKey,
		&t.RemoteWGInterface, &t.OPNsensePeerUUID, &t.OPNsenseServerUUID,
		&t.OriginalRemoteHost, &t.OriginalSSHPort, &t.SSHPhase,
		&sshSocketWasActive, &ufwWasActive,
		&deployed, &t.Status, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.Deployed = deployed == 1
	t.SSHSocketWasActive = sshSocketWasActive == 1
	t.UFWWasActive = ufwWasActive == 1
	if t.SSHPhase == "" {
		t.SSHPhase = models.SSHPhasePublic
	}
	return &t, nil
}

// CreateSiteTunnel inserts a new site tunnel, scoped to the active instance.
func (s *SQLiteStore) CreateSiteTunnel(ctx context.Context, t models.SiteTunnel) (int64, error) {
	if t.InstanceID == 0 {
		t.InstanceID, _ = s.GetActiveInstanceID(ctx)
	}
	if t.SSHPort == 0 {
		t.SSHPort = 22
	}
	if t.SSHUser == "" {
		t.SSHUser = "root"
	}
	if t.ListenPort == 0 {
		t.ListenPort = 51820
	}
	if t.Keepalive == 0 {
		t.Keepalive = 25
	}
	if t.Status == "" {
		t.Status = "pending"
	}
	// Do NOT default RemoteWGInterface — deployStepConfigureRemote discovers
	// the correct interface via findAvailableWGInterface. Defaulting to "wg0"
	// would skip discovery and risk clobbering existing WG configs.

	const query = `
		INSERT INTO site_tunnels (
			instance_id, name, description,
			remote_host, ssh_port, ssh_user, ssh_private_key, ssh_password,
			tunnel_subnet, firewall_ip, remote_ip, listen_port, keepalive,
			firewall_private_key, firewall_public_key, remote_private_key, remote_public_key,
			remote_wg_interface, opnsense_peer_uuid, opnsense_server_uuid,
			original_remote_host, original_ssh_port, ssh_phase,
			ssh_socket_was_active, ufw_was_active,
			deployed, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.ExecContext(ctx, query, tunnelWriteArgs(t)...)
	if err != nil {
		return 0, fmt.Errorf("create site tunnel: %w", err)
	}
	return result.LastInsertId()
}

// SaveSiteTunnel updates an existing site tunnel.
func (s *SQLiteStore) SaveSiteTunnel(ctx context.Context, t models.SiteTunnel) error {
	if t.InstanceID == 0 {
		var existing int64
		_ = s.db.QueryRowContext(ctx, `SELECT instance_id FROM site_tunnels WHERE id = ?`, t.ID).Scan(&existing)
		if existing != 0 {
			t.InstanceID = existing
		} else {
			t.InstanceID, _ = s.GetActiveInstanceID(ctx)
		}
	}

	const query = `
		UPDATE site_tunnels SET
			instance_id = ?, name = ?, description = ?,
			remote_host = ?, ssh_port = ?, ssh_user = ?, ssh_private_key = ?, ssh_password = ?,
			tunnel_subnet = ?, firewall_ip = ?, remote_ip = ?, listen_port = ?, keepalive = ?,
			firewall_private_key = ?, firewall_public_key = ?, remote_private_key = ?, remote_public_key = ?,
			remote_wg_interface = ?, opnsense_peer_uuid = ?, opnsense_server_uuid = ?,
			original_remote_host = ?, original_ssh_port = ?, ssh_phase = ?,
			ssh_socket_was_active = ?, ufw_was_active = ?,
			deployed = ?, status = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	args := append(tunnelWriteArgs(t), t.ID)
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("save site tunnel: %w", err)
	}
	return nil
}

// GetSiteTunnel returns a site tunnel by ID, or nil if not found.
func (s *SQLiteStore) GetSiteTunnel(ctx context.Context, id int64) (*models.SiteTunnel, error) {
	query := `SELECT id, ` + tunnelSelectColumns + ` FROM site_tunnels WHERE id = ?`
	t, err := scanTunnel(s.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get site tunnel %d: %w", id, err)
	}
	return t, nil
}

// ListSiteTunnels returns all site tunnels for the active instance.
func (s *SQLiteStore) ListSiteTunnels(ctx context.Context) ([]*models.SiteTunnel, error) {
	instanceID, _ := s.GetActiveInstanceID(ctx)
	if instanceID == 0 {
		return nil, nil
	}
	query := `SELECT id, ` + tunnelSelectColumns + ` FROM site_tunnels WHERE instance_id = ? ORDER BY id`
	rows, err := s.db.QueryContext(ctx, query, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list site tunnels: %w", err)
	}
	defer rows.Close()

	var tunnels []*models.SiteTunnel
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan site tunnel: %w", err)
		}
		tunnels = append(tunnels, t)
	}
	return tunnels, rows.Err()
}

// DeleteSiteTunnel removes a site tunnel by ID.
func (s *SQLiteStore) DeleteSiteTunnel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM site_tunnels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete site tunnel %d: %w", id, err)
	}
	return nil
}

// NextTunnelSubnet returns the next available /24 subnet in the 10.200.x.0 range.
func (s *SQLiteStore) NextTunnelSubnet(ctx context.Context) (subnet, firewallIP, remoteIP string, err error) {
	instanceID, _ := s.GetActiveInstanceID(ctx)
	// Find which third octets are already in use.
	rows, err := s.db.QueryContext(ctx, `SELECT tunnel_subnet FROM site_tunnels WHERE instance_id = ?`, instanceID)
	if err != nil {
		return "", "", "", fmt.Errorf("query tunnel subnets: %w", err)
	}
	defer rows.Close()

	used := make(map[int]bool)
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		// Parse "10.200.X.0/24" → X
		var a, b, c, d, mask int
		if _, err := fmt.Sscanf(s, "%d.%d.%d.%d/%d", &a, &b, &c, &d, &mask); err == nil {
			if a == 10 && b == 200 {
				used[c] = true
			}
		}
	}

	// Find first unused octet starting at 200.
	for octet := 200; octet < 255; octet++ {
		if !used[octet] {
			return fmt.Sprintf("10.200.%d.0/24", octet),
				fmt.Sprintf("10.200.%d.2", octet),
				fmt.Sprintf("10.200.%d.1", octet),
				nil
		}
	}
	return "", "", "", fmt.Errorf("no available tunnel subnets in 10.200.x.0/24 range")
}

// ─── Schema migration ────────────────────────────────────────────

func (s *SQLiteStore) migrate(ctx context.Context) error {
	// Create new tables (safe — IF NOT EXISTS).
	const schema = `
		CREATE TABLE IF NOT EXISTS firewall_instances (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			label TEXT NOT NULL DEFAULT '',
			firewall_type TEXT NOT NULL,
			host TEXT NOT NULL,
			api_key TEXT NOT NULL DEFAULT '',
			api_secret TEXT NOT NULL DEFAULT '',
			api_token TEXT NOT NULL DEFAULT '',
			skip_tls INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS active_instance (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			instance_id INTEGER NOT NULL REFERENCES firewall_instances(id)
		);

		CREATE TABLE IF NOT EXISTS vpn_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			protocol TEXT NOT NULL,
			ip_version TEXT NOT NULL DEFAULT 'ipv4',
			local_cidr TEXT NOT NULL,
			remote_cidr TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			dns TEXT NOT NULL DEFAULT '',
			private_key TEXT NOT NULL DEFAULT '',
			peer_public_key TEXT NOT NULL DEFAULT '',
			pre_shared_key TEXT NOT NULL DEFAULT '',
			opnsense_peer_uuid TEXT NOT NULL DEFAULT '',
			opnsense_server_uuid TEXT NOT NULL DEFAULT '',
			opnsense_gateway_uuid TEXT NOT NULL DEFAULT '',
			opnsense_gateway_name TEXT NOT NULL DEFAULT '',
			opnsense_snat_rule_uuid TEXT NOT NULL DEFAULT '',
			opnsense_filter_uuid TEXT NOT NULL DEFAULT '',
			opnsense_wg_interface TEXT NOT NULL DEFAULT '',
			last_applied_at TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 0,
			routing_applied INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS cache (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS app_routing_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			vpn_config_id INTEGER NOT NULL,
			app_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			opnsense_rule_uuids TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(vpn_config_id, app_id)
		);

		CREATE TABLE IF NOT EXISTS custom_app_profiles (
			id TEXT PRIMARY KEY,
			instance_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT 'custom',
			rules_json TEXT NOT NULL DEFAULT '[]',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS site_tunnels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			instance_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			remote_host TEXT NOT NULL,
			ssh_port INTEGER NOT NULL DEFAULT 22,
			ssh_user TEXT NOT NULL DEFAULT 'root',
			ssh_private_key TEXT NOT NULL DEFAULT '',
			ssh_password TEXT NOT NULL DEFAULT '',
			tunnel_subnet TEXT NOT NULL DEFAULT '',
			firewall_ip TEXT NOT NULL DEFAULT '',
			remote_ip TEXT NOT NULL DEFAULT '',
			listen_port INTEGER NOT NULL DEFAULT 51820,
			keepalive INTEGER NOT NULL DEFAULT 25,
			firewall_private_key TEXT NOT NULL DEFAULT '',
			firewall_public_key TEXT NOT NULL DEFAULT '',
			remote_private_key TEXT NOT NULL DEFAULT '',
			remote_public_key TEXT NOT NULL DEFAULT '',
			remote_wg_interface TEXT NOT NULL DEFAULT 'wg0',
			opnsense_peer_uuid TEXT NOT NULL DEFAULT '',
			opnsense_server_uuid TEXT NOT NULL DEFAULT '',
			deployed INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`

	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create tables: %w", err)
	}

	// Migrate old single-row vpn_config (very old schema) into vpn_configs.
	if err := s.migrateOldVPNConfig(ctx); err != nil {
		return err
	}

	// Add columns that may be missing from earlier schema versions.
	s.addColumnSafe(ctx, "vpn_configs", "ip_version", "TEXT NOT NULL DEFAULT 'ipv4'")
	s.addColumnSafe(ctx, "vpn_configs", "opnsense_wg_device", "TEXT NOT NULL DEFAULT ''")
	s.addColumnSafe(ctx, "vpn_configs", "routing_mode", "TEXT NOT NULL DEFAULT 'all'")
	s.addColumnSafe(ctx, "vpn_configs", "source_interfaces_json", "TEXT NOT NULL DEFAULT '[]'")
	s.addColumnSafe(ctx, "vpn_configs", "opnsense_snat_rule_uuids", "TEXT NOT NULL DEFAULT ''")
	s.addColumnSafe(ctx, "vpn_configs", "opnsense_filter_uuids", "TEXT NOT NULL DEFAULT ''")
	s.addColumnSafe(ctx, "vpn_configs", "instance_id", "INTEGER NOT NULL DEFAULT 0")

	// Add instance_id to custom_app_profiles.
	s.addColumnSafe(ctx, "custom_app_profiles", "instance_id", "INTEGER NOT NULL DEFAULT 0")
	s.addColumnSafe(ctx, "custom_app_profiles", "asns_json", "TEXT NOT NULL DEFAULT '[]'")

	// Migrate old singular UUID columns to new plural ones (one-time copy).
	s.migrateColumnSafe(ctx, "vpn_configs", "opnsense_snat_rule_uuid", "opnsense_snat_rule_uuids")
	s.migrateColumnSafe(ctx, "vpn_configs", "opnsense_filter_uuid", "opnsense_filter_uuids")

	// Replace old global unique index with instance-scoped one.
	s.dropIndexSafe(ctx, "idx_vpn_configs_name")
	s.addIndexSafe(ctx, "vpn_configs", "idx_vpn_configs_instance_name", "instance_id, name", true)
	s.addIndexSafe(ctx, "firewall_instances", "idx_firewall_instances_host", "host", false)
	s.addIndexSafe(ctx, "app_routing_rules", "idx_app_routing_rules_app_id", "app_id", false)
	s.addIndexSafe(ctx, "custom_app_profiles", "idx_custom_app_profiles_instance_id", "instance_id", false)
	s.addIndexSafe(ctx, "custom_app_profiles", "idx_custom_app_profiles_instance_name", "instance_id, name", false)

	// Add SSH migration tracking columns to site_tunnels.
	s.addColumnSafe(ctx, "site_tunnels", "original_remote_host", "TEXT NOT NULL DEFAULT ''")
	s.addColumnSafe(ctx, "site_tunnels", "original_ssh_port", "INTEGER NOT NULL DEFAULT 0")
	// Legacy boolean columns — kept for backward compat, superseded by ssh_phase.
	s.addColumnSafe(ctx, "site_tunnels", "ssh_migrated", "INTEGER NOT NULL DEFAULT 0")
	s.addColumnSafe(ctx, "site_tunnels", "ssh_locked_down", "INTEGER NOT NULL DEFAULT 0")
	// New hardened SSH state columns.
	s.addColumnSafe(ctx, "site_tunnels", "ssh_phase", "TEXT NOT NULL DEFAULT 'public'")
	s.addColumnSafe(ctx, "site_tunnels", "ssh_socket_was_active", "INTEGER NOT NULL DEFAULT 0")
	s.addColumnSafe(ctx, "site_tunnels", "ufw_was_active", "INTEGER NOT NULL DEFAULT 0")
	// Migrate legacy booleans → ssh_phase for existing rows.
	s.db.ExecContext(ctx, `UPDATE site_tunnels SET ssh_phase = 'tunnel_only_verified' WHERE ssh_locked_down = 1 AND ssh_phase = 'public'`)
	s.db.ExecContext(ctx, `UPDATE site_tunnels SET ssh_phase = 'dual_listen_verified' WHERE ssh_migrated = 1 AND ssh_locked_down = 0 AND ssh_phase = 'public'`)

	// Tunnel name uniqueness per instance.
	s.addIndexSafe(ctx, "site_tunnels", "idx_site_tunnels_instance_name", "instance_id, name", true)

	// Migrate setup_config data into firewall_instances (if setup_config exists).
	s.migrateSetupConfigToInstances(ctx)

	// Assign orphan vpn_configs/custom_app_profiles to the active instance.
	s.assignOrphansToActiveInstance(ctx)

	return nil
}

// migrateSetupConfigToInstances migrates the old single-row setup_config into firewall_instances.
func (s *SQLiteStore) migrateSetupConfigToInstances(ctx context.Context) {
	// Check if old table exists.
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='setup_config'`,
	).Scan(&count)
	if err != nil || count == 0 {
		return
	}

	// Read old config.
	var fwType, host, apiKey, apiSecret, apiToken string
	var skipTLS int
	err = s.db.QueryRowContext(ctx,
		`SELECT firewall_type, host, api_key, api_secret, api_token, skip_tls FROM setup_config WHERE id = 1`,
	).Scan(&fwType, &host, &apiKey, &apiSecret, &apiToken, &skipTLS)
	if err != nil {
		// No data or error — just drop.
		_, _ = s.db.ExecContext(ctx, `DROP TABLE IF EXISTS setup_config`)
		return
	}

	// Check if we already have an instance with this host.
	var existingID int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM firewall_instances WHERE host = ?`, host).Scan(&existingID)
	if err == nil && existingID > 0 {
		// Already migrated. Just ensure it's active.
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO active_instance (id, instance_id) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET instance_id = excluded.instance_id`,
			existingID,
		)
		_, _ = s.db.ExecContext(ctx, `DROP TABLE IF EXISTS setup_config`)
		return
	}

	// Create new instance.
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO firewall_instances (label, firewall_type, host, api_key, api_secret, api_token, skip_tls) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		labelFromHost(host), fwType, host, apiKey, apiSecret, apiToken, skipTLS,
	)
	if err != nil {
		return
	}
	newID, _ := result.LastInsertId()
	if newID > 0 {
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO active_instance (id, instance_id) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET instance_id = excluded.instance_id`,
			newID,
		)
	}

	_, _ = s.db.ExecContext(ctx, `DROP TABLE IF EXISTS setup_config`)
}

// assignOrphansToActiveInstance assigns vpn_configs and custom_app_profiles with instance_id=0 to the active instance.
func (s *SQLiteStore) assignOrphansToActiveInstance(ctx context.Context) {
	var activeID int64
	err := s.db.QueryRowContext(ctx, `SELECT instance_id FROM active_instance WHERE id = 1`).Scan(&activeID)
	if err != nil || activeID == 0 {
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE vpn_configs SET instance_id = ? WHERE instance_id = 0`, activeID)
	_, _ = s.db.ExecContext(ctx, `UPDATE custom_app_profiles SET instance_id = ? WHERE instance_id = 0`, activeID)
}

// addColumnSafe adds a column if it doesn't already exist. Silently ignores "duplicate column" errors.
func (s *SQLiteStore) addColumnSafe(ctx context.Context, table, column, def string) {
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, def)
	_, _ = s.db.ExecContext(ctx, query)
}

// migrateColumnSafe copies data from an old column to a new column.
func (s *SQLiteStore) migrateColumnSafe(ctx context.Context, table, oldColumn, newColumn string) {
	query := fmt.Sprintf(
		`UPDATE %s SET %s = %s WHERE %s != '' AND (%s = '' OR %s IS NULL)`,
		table, newColumn, oldColumn, oldColumn, newColumn, newColumn,
	)
	_, _ = s.db.ExecContext(ctx, query)
}

// addIndexSafe creates an index if it doesn't already exist.
func (s *SQLiteStore) addIndexSafe(ctx context.Context, table, indexName, column string, unique bool) {
	keyword := "INDEX"
	if unique {
		keyword = "UNIQUE INDEX"
	}
	query := fmt.Sprintf("CREATE %s IF NOT EXISTS %s ON %s (%s)", keyword, indexName, table, column)
	_, _ = s.db.ExecContext(ctx, query)
}

// dropIndexSafe drops an index if it exists.
func (s *SQLiteStore) dropIndexSafe(ctx context.Context, indexName string) {
	_, _ = s.db.ExecContext(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName))
}

// migrateOldVPNConfig copies data from the old vpn_config table (single row) to vpn_configs.
func (s *SQLiteStore) migrateOldVPNConfig(ctx context.Context) error {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='vpn_config'`,
	).Scan(&count)
	if err != nil || count == 0 {
		return nil
	}

	var rowCount int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vpn_config`).Scan(&rowCount)
	if err != nil || rowCount == 0 {
		_, _ = s.db.ExecContext(ctx, `DROP TABLE IF EXISTS vpn_config`)
		return nil
	}

	var newCount int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vpn_configs`).Scan(&newCount)
	if newCount > 0 {
		_, _ = s.db.ExecContext(ctx, `DROP TABLE IF EXISTS vpn_config`)
		return nil
	}

	colMap := map[string]bool{}
	colRows, err := s.db.QueryContext(ctx, `PRAGMA table_info(vpn_config)`)
	if err != nil {
		return nil
	}
	defer colRows.Close()
	for colRows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := colRows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			continue
		}
		colMap[name] = true
	}

	col := func(name, fallback string) string {
		if colMap[name] {
			return name
		}
		return fallback + " AS " + name
	}

	selectQuery := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s FROM vpn_config WHERE id = 1`,
		col("name", "''"), col("protocol", "''"),
		col("local_cidr", "''"), col("remote_cidr", "''"),
		col("endpoint", "''"), col("dns", "''"),
		col("private_key", "''"), col("peer_public_key", "''"),
		col("pre_shared_key", "''"),
		col("opnsense_peer_uuid", "''"), col("opnsense_server_uuid", "''"),
		col("opnsense_gateway_uuid", "''"), col("opnsense_gateway_name", "''"),
		col("opnsense_snat_rule_uuid", "''"), col("opnsense_filter_uuid", "''"),
		col("opnsense_wg_interface", "''"), col("last_applied_at", "''"),
		col("enabled", "0"), col("routing_applied", "0"),
	)

	var cfg models.SimpleVPNConfig
	var enabled, routingApplied int
	err = s.db.QueryRowContext(ctx, selectQuery).Scan(
		&cfg.Name, &cfg.Protocol, &cfg.LocalCIDR, &cfg.RemoteCIDR,
		&cfg.Endpoint, &cfg.DNS, &cfg.PrivateKey, &cfg.PeerPublicKey,
		&cfg.PreSharedKey, &cfg.OPNsensePeerUUID, &cfg.OPNsenseServerUUID,
		&cfg.OPNsenseGatewayUUID, &cfg.OPNsenseGatewayName,
		&cfg.OPNsenseSNATRuleUUIDs, &cfg.OPNsenseFilterUUIDs,
		&cfg.OPNsenseWGInterface, &cfg.LastAppliedAt,
		&enabled, &routingApplied,
	)
	if err != nil {
		_, _ = s.db.ExecContext(ctx, `DROP TABLE IF EXISTS vpn_config`)
		return nil
	}

	cfg.Enabled = enabled == 1
	cfg.RoutingApplied = routingApplied == 1

	if cfg.Name != "" {
		if _, err := s.CreateVPNConfig(ctx, cfg); err != nil {
			return fmt.Errorf("migrate vpn_config to vpn_configs: %w", err)
		}
	}

	_, _ = s.db.ExecContext(ctx, `DROP TABLE IF EXISTS vpn_config`)
	return nil
}

// labelFromHost generates a label from a host URL.
func labelFromHost(host string) string {
	// Strip protocol prefix for a cleaner label.
	label := host
	for _, prefix := range []string{"https://", "http://"} {
		if len(label) > len(prefix) && label[:len(prefix)] == prefix {
			label = label[len(prefix):]
			break
		}
	}
	return label
}
