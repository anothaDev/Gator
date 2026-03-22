package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/anothaDev/gator/internal/models"
)

// ReconcilerStore defines the storage interface needed by the reconciler.
type ReconcilerStore interface {
	GetActiveInstanceID(ctx context.Context) (int64, error)
	GetFirewallConfigForInstance(ctx context.Context, instanceID int64) (*models.FirewallConfig, error)
	ListVPNConfigsForInstance(ctx context.Context, instanceID int64) ([]*models.SimpleVPNConfig, error)
	ListSiteTunnelsForInstance(ctx context.Context, instanceID int64) ([]*models.SiteTunnel, error)
	SaveSimpleVPNConfig(ctx context.Context, cfg models.SimpleVPNConfig) error
	SaveSiteTunnel(ctx context.Context, t models.SiteTunnel) error
}

// OPNsenseSnapshot holds the result of bulk API queries against a single
// OPNsense instance. Used to verify whether stored UUIDs still exist.
type OPNsenseSnapshot struct {
	Servers       map[string]bool // WG server UUID → exists
	Peers         map[string]bool // WG peer UUID → exists
	Gateways      map[string]bool // Gateway UUID → exists
	GatewayStatus map[string]bool // Gateway name → online
	FilterRules   map[string]bool // Filter rule UUID → exists
	SNATRules     map[string]bool // SNAT rule UUID → exists
	FetchedAt     time.Time
	Err           error // non-nil if the fetch failed
	InstanceID    int64
}

// Reconciler periodically verifies local DB state against live OPNsense
// and updates ownership_status on VPN configs and site tunnels.
type Reconciler struct {
	store     ReconcilerStore
	mu        sync.RWMutex
	snapshots map[int64]*OPNsenseSnapshot
	interval  time.Duration
	stopCh    chan struct{}
	stopped   chan struct{}

	// instanceMu serializes reconciliation per instance.
	// Key: instanceID (int64), Value: *sync.Mutex.
	instanceMu sync.Map
}

// NewReconciler creates a reconciler with the given store and polling interval.
func NewReconciler(store ReconcilerStore, interval time.Duration) *Reconciler {
	return &Reconciler{
		store:     store,
		snapshots: make(map[int64]*OPNsenseSnapshot),
		interval:  interval,
		stopCh:    make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

// Start launches the background reconciliation loop. The initial reconcile
// runs immediately but does NOT block — the server starts accepting requests
// while reconciliation proceeds in the background.
func (r *Reconciler) Start() {
	go func() {
		defer close(r.stopped)

		// Initial reconcile (best-effort).
		ctx := context.Background()
		r.reconcileOnce(ctx)

		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.reconcileOnce(ctx)
			case <-r.stopCh:
				return
			}
		}
	}()
}

// Stop halts the background loop and waits for it to finish.
func (r *Reconciler) Stop() {
	close(r.stopCh)
	<-r.stopped
}

// Refresh forces an immediate reconciliation for the given instance.
// Blocks until complete. Safe to call from request handlers.
func (r *Reconciler) Refresh(ctx context.Context, instanceID int64) {
	r.serializedReconcile(ctx, instanceID)
}

// RefreshAsync triggers a reconciliation in the background without blocking.
// If a reconcile is already running for this instance, the request is dropped
// (the in-flight run will produce a fresh-enough snapshot).
func (r *Reconciler) RefreshAsync(instanceID int64) {
	mu := r.instanceMutex(instanceID)
	go func() {
		if !mu.TryLock() {
			return // another reconcile is in-flight; skip
		}
		defer mu.Unlock()
		r.reconcileInstance(context.Background(), instanceID)
	}()
}

// Snapshot returns the most recent snapshot for an instance, or nil if none.
func (r *Reconciler) Snapshot(instanceID int64) *OPNsenseSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshots[instanceID]
}

// EnsureFresh refreshes the snapshot if it is older than maxAge.
// Returns an error only if the refresh was needed and OPNsense was unreachable.
// Serialized: if another reconcile is in-flight for this instance, this call
// blocks until it completes, then re-checks freshness (the in-flight run may
// have already satisfied the maxAge requirement).
func (r *Reconciler) EnsureFresh(ctx context.Context, instanceID int64, maxAge time.Duration) error {
	snap := r.Snapshot(instanceID)
	if snap != nil && time.Since(snap.FetchedAt) < maxAge {
		return snap.Err
	}
	r.serializedReconcile(ctx, instanceID)
	snap = r.Snapshot(instanceID)
	if snap == nil {
		return nil
	}
	return snap.Err
}

// instanceMutex returns the per-instance mutex, creating one if needed.
func (r *Reconciler) instanceMutex(instanceID int64) *sync.Mutex {
	val, _ := r.instanceMu.LoadOrStore(instanceID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// serializedReconcile acquires the per-instance lock, then runs reconciliation.
// If another goroutine already holds the lock (i.e. a reconcile is in-flight),
// this call blocks until it finishes, then re-runs only if the snapshot is
// still stale. This prevents overlapping runs from racing on DB writes.
func (r *Reconciler) serializedReconcile(ctx context.Context, instanceID int64) {
	mu := r.instanceMutex(instanceID)
	mu.Lock()
	defer mu.Unlock()
	r.reconcileInstance(ctx, instanceID)
}

// reconcileOnce runs reconciliation for the currently active instance.
func (r *Reconciler) reconcileOnce(ctx context.Context) {
	instanceID, _ := r.store.GetActiveInstanceID(ctx)
	if instanceID == 0 {
		return
	}
	r.serializedReconcile(ctx, instanceID)
}

// reconcileInstance fetches a fresh snapshot from OPNsense and updates
// ownership_status on all VPN configs and tunnels for the given instance.
// CALLER MUST HOLD the per-instance mutex (via serializedReconcile or
// RefreshAsync's TryLock).
func (r *Reconciler) reconcileInstance(ctx context.Context, instanceID int64) {
	firewallCfg, err := r.store.GetFirewallConfigForInstance(ctx, instanceID)
	if err != nil || firewallCfg == nil || firewallCfg.Type != "opnsense" {
		return
	}

	api := newOPNsenseAPIClient(*firewallCfg)
	snap := r.fetchSnapshot(ctx, api, instanceID)

	r.mu.Lock()
	r.snapshots[instanceID] = snap
	r.mu.Unlock()

	// If the fetch failed, don't mutate ownership status.
	if snap.Err != nil {
		log.Printf("[reconciler] instance %d: snapshot fetch failed: %v", instanceID, snap.Err)
		return
	}

	r.reconcileVPNConfigs(ctx, snap)
	r.reconcileTunnels(ctx, snap)
}

// fetchSnapshot makes bulk API calls to OPNsense and builds a snapshot.
// On any failure, returns a snapshot with Err set.
func (r *Reconciler) fetchSnapshot(ctx context.Context, api *opnsenseAPIClient, instanceID int64) *OPNsenseSnapshot {
	snap := &OPNsenseSnapshot{
		FetchedAt:  time.Now(),
		InstanceID: instanceID,
	}

	// WG servers.
	serverResp, err := api.Post(ctx, "/api/wireguard/server/search_server", map[string]any{})
	if err != nil {
		snap.Err = err
		return snap
	}
	snap.Servers = uuidSetFromRows(serverResp, "rows")

	// WG peers.
	peerResp, err := api.Post(ctx, "/api/wireguard/client/search_client", map[string]any{})
	if err != nil {
		snap.Err = err
		return snap
	}
	snap.Peers = uuidSetFromRows(peerResp, "rows")

	// Gateways (config list — UUID existence check).
	gwResp, err := api.Get(ctx, "/api/routing/settings/search_gateway")
	if err != nil {
		snap.Err = err
		return snap
	}
	snap.Gateways = uuidSetFromRows(gwResp, "rows")

	// Gateway runtime status (online/offline).
	statusResp, err := api.Get(ctx, "/api/routes/gateway/status")
	if err != nil {
		snap.Err = err
		return snap
	}
	snap.GatewayStatus = make(map[string]bool)
	for _, raw := range asSlice(statusResp["items"]) {
		item := asMap(raw)
		name := asString(item["name"])
		if name == "" {
			continue
		}
		status := strings.ToLower(asString(item["status_translated"]))
		if status == "" {
			status = strings.ToLower(asString(item["status"]))
		}
		snap.GatewayStatus[name] = strings.Contains(status, "online")
	}

	// Filter rules (automation).
	filterResp, err := api.Post(ctx, "/api/firewall/filter/search_rule", map[string]any{})
	if err != nil {
		snap.Err = err
		return snap
	}
	snap.FilterRules = uuidSetFromRows(filterResp, "rows")

	// SNAT (outbound NAT) rules.
	snatResp, err := api.Post(ctx, "/api/firewall/source_nat/search_rule", map[string]any{})
	if err != nil {
		snap.Err = err
		return snap
	}
	snap.SNATRules = uuidSetFromRows(snatResp, "rows")

	return snap
}

// reconcileVPNConfigs checks each VPN config against the snapshot and updates ownership.
func (r *Reconciler) reconcileVPNConfigs(ctx context.Context, snap *OPNsenseSnapshot) {
	configs, err := r.store.ListVPNConfigsForInstance(ctx, snap.InstanceID)
	if err != nil {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, cfg := range configs {
		if cfg.OwnershipStatus == models.OwnershipLocalOnly {
			continue
		}

		// No stored UUIDs at all — nothing to verify.
		if cfg.OPNsenseServerUUID == "" && cfg.OPNsensePeerUUID == "" && cfg.OPNsenseGatewayUUID == "" {
			continue
		}

		// Check each primary binding against the snapshot.
		serverOK := cfg.OPNsenseServerUUID == "" || snap.Servers[cfg.OPNsenseServerUUID]
		peerOK := cfg.OPNsensePeerUUID == "" || snap.Peers[cfg.OPNsensePeerUUID]
		gatewayOK := cfg.OPNsenseGatewayUUID == "" || snap.Gateways[cfg.OPNsenseGatewayUUID]

		// Check filter/SNAT rule bindings (secondary — drift but never reimport).
		filterUUIDs := splitUUIDs(cfg.OPNsenseFilterUUIDs)
		snatUUIDs := splitUUIDs(cfg.OPNsenseSNATRuleUUIDs)
		missingFilters := 0
		for _, uuid := range filterUUIDs {
			if !snap.FilterRules[uuid] {
				missingFilters++
			}
		}
		missingSNATs := 0
		for _, uuid := range snatUUIDs {
			if !snap.SNATRules[uuid] {
				missingSNATs++
			}
		}
		rulesOK := missingFilters == 0 && missingSNATs == 0

		// Build drift reasons.
		var reasons []string
		if cfg.OPNsenseServerUUID != "" && !snap.Servers[cfg.OPNsenseServerUUID] {
			reasons = append(reasons, "WG server not found")
		}
		if cfg.OPNsensePeerUUID != "" && !snap.Peers[cfg.OPNsensePeerUUID] {
			reasons = append(reasons, "WG peer not found")
		}
		if cfg.OPNsenseGatewayUUID != "" && !snap.Gateways[cfg.OPNsenseGatewayUUID] {
			reasons = append(reasons, "gateway not found")
		}
		if missingFilters > 0 {
			reasons = append(reasons, fmt.Sprintf("%d/%d filter rule(s) missing", missingFilters, len(filterUUIDs)))
		}
		if missingSNATs > 0 {
			reasons = append(reasons, fmt.Sprintf("%d/%d NAT rule(s) missing", missingSNATs, len(snatUUIDs)))
		}

		// Determine new ownership status.
		// Primary bindings (server, peer, gateway) drive the main status.
		// Rule drift alone degrades to managed_drifted, never needs_reimport.
		//
		// needs_reimport fires when ALL populated primary bindings are gone
		// (unpopulated ones don't count — a VPN that never had a gateway still
		// needs reimport if server+peer are both deleted).
		primaryTotal := 0
		primaryMissing := 0
		if cfg.OPNsenseServerUUID != "" {
			primaryTotal++
			if !serverOK {
				primaryMissing++
			}
		}
		if cfg.OPNsensePeerUUID != "" {
			primaryTotal++
			if !peerOK {
				primaryMissing++
			}
		}
		if cfg.OPNsenseGatewayUUID != "" {
			primaryTotal++
			if !gatewayOK {
				primaryMissing++
			}
		}

		var newStatus string
		switch {
		case serverOK && peerOK && gatewayOK && rulesOK:
			newStatus = models.OwnershipManagedVerified
		case primaryTotal > 0 && primaryMissing == primaryTotal:
			// All populated primary bindings are gone.
			newStatus = models.OwnershipNeedsReimport
		default:
			newStatus = models.OwnershipManagedDrifted
		}

		// Update gateway_online from runtime dpinger status (health only, not config).
		if cfg.OPNsenseGatewayName != "" {
			gwOnline, found := snap.GatewayStatus[cfg.OPNsenseGatewayName]
			if found {
				cfg.GatewayOnline = gwOnline
			}
		}

		// Only persist if ownership or drift state changed, or the timestamp
		// is stale enough to warrant a refresh (every 5 minutes max).
		changed := cfg.OwnershipStatus != newStatus ||
			cfg.DriftReason != strings.Join(reasons, "; ")
		timestampStale := cfg.LastVerifiedAt == "" || isTimestampOlderThan(cfg.LastVerifiedAt, 5*time.Minute)

		if !changed && !timestampStale {
			continue
		}
		if !changed {
			// Only refresh the timestamp, no log.
			cfg.LastVerifiedAt = now
			_ = r.store.SaveSimpleVPNConfig(ctx, *cfg)
			continue
		}

		oldStatus := cfg.OwnershipStatus
		cfg.OwnershipStatus = newStatus
		cfg.DriftReason = strings.Join(reasons, "; ")
		cfg.LastVerifiedAt = now

		if newStatus == models.OwnershipNeedsReimport {
			cfg.RoutingApplied = false
		}

		log.Printf("[reconciler] VPN %q: %s -> %s%s",
			cfg.Name, oldStatus, newStatus,
			func() string {
				if len(reasons) > 0 {
					return " (" + strings.Join(reasons, "; ") + ")"
				}
				return ""
			}())

		_ = r.store.SaveSimpleVPNConfig(ctx, *cfg)
	}
}

// reconcileTunnels checks each tunnel against the snapshot and updates ownership.
func (r *Reconciler) reconcileTunnels(ctx context.Context, snap *OPNsenseSnapshot) {
	tunnels, err := r.store.ListSiteTunnelsForInstance(ctx, snap.InstanceID)
	if err != nil {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, t := range tunnels {
		if t.OwnershipStatus == models.OwnershipLocalOnly {
			continue
		}

		if t.OPNsenseServerUUID == "" && t.OPNsensePeerUUID == "" {
			continue
		}

		serverOK := t.OPNsenseServerUUID == "" || snap.Servers[t.OPNsenseServerUUID]
		peerOK := t.OPNsensePeerUUID == "" || snap.Peers[t.OPNsensePeerUUID]

		var reasons []string
		if t.OPNsenseServerUUID != "" && !snap.Servers[t.OPNsenseServerUUID] {
			reasons = append(reasons, "WG server not found")
		}
		if t.OPNsensePeerUUID != "" && !snap.Peers[t.OPNsensePeerUUID] {
			reasons = append(reasons, "WG peer not found")
		}

		primaryTotal := 0
		primaryMissing := 0
		if t.OPNsenseServerUUID != "" {
			primaryTotal++
			if !serverOK {
				primaryMissing++
			}
		}
		if t.OPNsensePeerUUID != "" {
			primaryTotal++
			if !peerOK {
				primaryMissing++
			}
		}

		var newStatus string
		switch {
		case serverOK && peerOK:
			newStatus = models.OwnershipManagedVerified
		case primaryTotal > 0 && primaryMissing == primaryTotal:
			newStatus = models.OwnershipNeedsReimport
		default:
			newStatus = models.OwnershipManagedDrifted
		}

		changed := t.OwnershipStatus != newStatus ||
			t.DriftReason != strings.Join(reasons, "; ")
		timestampStale := t.LastVerifiedAt == "" || isTimestampOlderThan(t.LastVerifiedAt, 5*time.Minute)

		if !changed && !timestampStale {
			continue
		}
		if !changed {
			t.LastVerifiedAt = now
			_ = r.store.SaveSiteTunnel(ctx, *t)
			continue
		}

		oldStatus := t.OwnershipStatus
		t.OwnershipStatus = newStatus
		t.DriftReason = strings.Join(reasons, "; ")
		t.LastVerifiedAt = now

		if newStatus == models.OwnershipNeedsReimport {
			t.Deployed = false
			t.Status = "pending"
		}

		log.Printf("[reconciler] tunnel %q: %s -> %s%s",
			t.Name, oldStatus, newStatus,
			func() string {
				if len(reasons) > 0 {
					return " (" + strings.Join(reasons, "; ") + ")"
				}
				return ""
			}())

		_ = r.store.SaveSiteTunnel(ctx, *t)
	}
}

// isTimestampOlderThan checks if an RFC3339 timestamp is older than maxAge.
// Returns true if the timestamp cannot be parsed (treat missing/invalid as stale).
func isTimestampOlderThan(ts string, maxAge time.Duration) bool {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return true
	}
	return time.Since(t) > maxAge
}

// uuidSetFromRows extracts a set of UUIDs from an OPNsense search response.
func uuidSetFromRows(resp map[string]any, rowsKey string) map[string]bool {
	rows := asSlice(resp[rowsKey])
	uuids := make(map[string]bool, len(rows))
	for _, rowRaw := range rows {
		row := asMap(rowRaw)
		uuid := asString(row["uuid"])
		if uuid != "" {
			uuids[uuid] = true
		}
	}
	return uuids
}
