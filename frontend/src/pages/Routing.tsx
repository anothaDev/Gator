import { createSignal, createResource, createMemo, For, Show, onMount, onCleanup } from "solid-js";
import { apiGet, apiPost, apiDelete } from "../lib/api";
import CustomProfileModal from "./routing/CustomProfileModal";
import IpRangeUploadModal from "./routing/IpRangeUploadModal";
import AppCard from "./routing/AppCard";
import RoutingToolbar from "./routing/RoutingToolbar";
import EmptyState from "../components/EmptyState";
import Card from "../components/Card";
import Button from "../components/Button";

type PortRule = {
  protocol: string;
  ports: string;
};

type URLTableHint = {
  download_url: string;
  jq_filter: string;
  description: string;
  filename: string;
};

type AppProfile = {
  id: string;
  name: string;
  icon: string;
  category: string;
  rules: PortRule[];
  asns?: number[];
  url_table_hint?: URLTableHint;
  note?: string;
  is_custom?: boolean;
};

type AppPreset = {
  id: string;
  name: string;
  description: string;
  vpn_on?: string[];
  vpn_off?: string[];
};

type AppRouteStatus = {
  app_id: string;
  enabled: boolean;
  applied: boolean;
};

type VPNOption = {
  id: number;
  name: string;
  routing_applied: boolean;
  gateway_applied: boolean;
};

async function fetchProfiles(): Promise<{ apps: AppProfile[]; presets: AppPreset[] }> {
  const { ok, data } = await apiGet<{ apps: AppProfile[]; presets: AppPreset[] }>("/api/app-profiles");
  if (!ok) throw new Error("Failed to load app profiles");
  return data;
}

async function fetchVPNList(): Promise<VPNOption[]> {
  const { ok, data } = await apiGet<{ vpns: any[] }>("/api/vpn/configs");
  if (!ok) throw new Error("Failed to load VPN configs");
  return (data.vpns ?? [])
    .filter((v: any) => v.gateway_applied)
    .map((v: any) => ({
      id: v.id,
      name: v.name,
      routing_applied: v.routing_applied,
      gateway_applied: v.gateway_applied,
    }));
}

type RoutingMode = "all" | "selective" | "bypass";

async function fetchAppRoutes(vpnId: number): Promise<{ routes: AppRouteStatus[]; mode: RoutingMode }> {
  const { ok, data } = await apiGet<{ routes: AppRouteStatus[]; routing_mode: RoutingMode }>(`/api/opnsense/vpn/${vpnId}/app-routes`);
  if (!ok) throw new Error("Failed to load app routes");
  return { routes: data.routes ?? [], mode: data.routing_mode ?? "all" };
}

const categoryOrder = ["gaming", "streaming", "communication", "file_sharing", "browsing", "remote_access", "home_iot", "mail", "custom"];
const categoryLabels: Record<string, string> = {
  gaming: "Gaming",
  streaming: "Streaming",
  communication: "Communication",
  file_sharing: "File Sharing",
  browsing: "Browsing",
  remote_access: "Remote Access",
  home_iot: "Home & IoT",
  mail: "Mail",
  custom: "Custom",
};

function groupByCategory(apps: AppProfile[]): { category: string; label: string; apps: AppProfile[] }[] {
  const map = new Map<string, AppProfile[]>();
  for (const app of apps) {
    const cat = app.category || "other";
    if (!map.has(cat)) map.set(cat, []);
    map.get(cat)!.push(app);
  }
  const sorted = [...map.entries()].sort(([a], [b]) => {
    const ia = categoryOrder.indexOf(a);
    const ib = categoryOrder.indexOf(b);
    return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib);
  });
  return sorted.map(([cat, apps]) => ({
    category: cat,
    label: categoryLabels[cat] ?? cat,
    apps,
  }));
}

export default function Routing() {
  const [profiles, { refetch: refetchProfiles }] = createResource(fetchProfiles);
  const [vpnList, setVpnList] = createSignal<VPNOption[]>([]);
  const [selectedVpnId, setSelectedVpnId] = createSignal<number | null>(null);
  const [routes, setRoutes] = createSignal<AppRouteStatus[]>([]);
  const [routesLoading, setRoutesLoading] = createSignal(false);
  const [routingMode, setRoutingMode] = createSignal<RoutingMode>("all");
  const [toggling, setToggling] = createSignal<string | null>(null);
  const [actionMsg, setActionMsg] = createSignal("");
  const [actionErr, setActionErr] = createSignal("");

  // Custom profile modal state.
  const [showAddModal, setShowAddModal] = createSignal(false);

  const deleteCustomProfile = async (profileId: string, name: string) => {
    if (!confirm(`Delete custom profile "${name}"?`)) return;
    try {
      const { ok } = await apiDelete(`/api/app-profiles/${profileId}`);
      if (ok) {
        refetchProfiles();
      }
    } catch { /* ignore */ }
  };

  // IP ranges upload modal state.
  const [uploadHint, setUploadHint] = createSignal<{ appId: string; appName: string; hint: URLTableHint } | null>(null);

  // Search and filter state.
  const [search, setSearch] = createSignal("");
  const [filterCategory, setFilterCategory] = createSignal<string | null>(null);
  const [filterStatus, setFilterStatus] = createSignal<"all" | "enabled" | "disabled">("all");

  const routeStatusMap = createMemo(() => {
    const map = new Map<string, AppRouteStatus>();
    for (const route of routes()) {
      map.set(route.app_id, route);
    }
    return map;
  });

  const enabledRouteIds = createMemo(() => {
    const enabled = new Set<string>();
    for (const route of routes()) {
      if (route.enabled) enabled.add(route.app_id);
    }
    return enabled;
  });

  const filteredApps = createMemo(() => {
    let apps = profiles()?.apps ?? [];
    const q = search().toLowerCase().trim();
    if (q) {
      apps = apps.filter(
        (a) =>
          a.name.toLowerCase().includes(q) ||
          a.category.toLowerCase().includes(q) ||
          a.rules.some((r) => r.ports.includes(q) || r.protocol.toLowerCase().includes(q)),
      );
    }
    const cat = filterCategory();
    if (cat) {
      apps = apps.filter((a) => a.category === cat);
    }
    const status = filterStatus();
    if (status === "enabled") {
      apps = apps.filter((a) => isEnabled(a.id));
    } else if (status === "disabled") {
      apps = apps.filter((a) => !isEnabled(a.id));
    }
    return apps;
  });

  const availableCategories = createMemo(() => {
    const apps = profiles()?.apps ?? [];
    const cats = new Set(apps.map((a) => a.category));
    return categoryOrder.filter((c) => cats.has(c));
  });

  const enabledCount = createMemo(() => {
    return enabledRouteIds().size;
  });

  const totalCount = createMemo(() => {
    return (profiles()?.apps ?? []).length;
  });

  const categoryInfos = createMemo(() => {
    const apps = profiles()?.apps ?? [];
    const enabled = enabledRouteIds();
    return availableCategories().map((cat) => ({
      key: cat,
      label: categoryLabels[cat] ?? cat,
      enabledCount: apps.filter((a) => a.category === cat && enabled.has(a.id)).length,
    }));
  });

  // Pending firewall confirmation state.
  const [pendingRevision, setPendingRevision] = createSignal<string | null>(null);
  const [countdown, setCountdown] = createSignal(60);
  let countdownTimer: ReturnType<typeof setInterval> | undefined;

  const startCountdown = (revision: string, initialSeconds?: number) => {
    clearCountdownTimer();
    setPendingRevision(revision);
    setCountdown(initialSeconds ?? 60);
    countdownTimer = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          clearCountdownTimer();
          setPendingRevision(null);
          setActionMsg("Changes auto-reverted (confirmation timed out).");
          const vpnId = selectedVpnId();
          if (vpnId) void loadRoutes(vpnId);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
  };

  const clearCountdownTimer = () => {
    if (countdownTimer) {
      clearInterval(countdownTimer);
      countdownTimer = undefined;
    }
  };

  onCleanup(clearCountdownTimer);

  const confirmChanges = async () => {
    const rev = pendingRevision();
    if (!rev) return;
    try {
      const { ok } = await apiPost("/api/opnsense/firewall/confirm", { revision: rev });
      if (ok) {
        setActionMsg("Changes confirmed.");
      } else {
        setActionErr("Failed to confirm changes.");
      }
    } catch {
      setActionErr("Failed to confirm changes.");
    } finally {
      clearCountdownTimer();
      setPendingRevision(null);
    }
  };

  const revertChanges = async () => {
    const rev = pendingRevision();
    if (!rev) return;
    try {
      const { ok } = await apiPost("/api/opnsense/firewall/revert", { revision: rev });
      if (ok) {
        setActionMsg("Changes reverted.");
        const vpnId = selectedVpnId();
        if (vpnId) void loadRoutes(vpnId);
      } else {
        setActionErr("Failed to revert changes.");
      }
    } catch {
      setActionErr("Failed to revert changes.");
    } finally {
      clearCountdownTimer();
      setPendingRevision(null);
    }
  };

  onMount(async () => {
    try {
      const vpns = await fetchVPNList();
      setVpnList(vpns);
      // Auto-select the active VPN, or the first deployed one.
      const active = vpns.find((v) => v.routing_applied);
      const selected = active ?? vpns[0];
      if (selected) {
        setSelectedVpnId(selected.id);
        await loadRoutes(selected.id);
      }
    } catch {
      // VPN list will be empty.
    }

    // Resume confirmation countdown if a pending revision exists on the server
    // (e.g. user navigated away and came back).
    try {
      const { ok, data } = await apiGet<{ pending: boolean; revision: string; remaining_seconds: number }>("/api/opnsense/firewall/pending");
      if (ok && data.pending && data.revision && data.remaining_seconds > 0) {
        startCountdown(data.revision, data.remaining_seconds);
      }
    } catch {
      // Non-fatal — pending check is best-effort.
    }
  });

  const loadRoutes = async (vpnId: number) => {
    setRoutesLoading(true);
    try {
      const data = await fetchAppRoutes(vpnId);
      setRoutes(data.routes);
      setRoutingMode(data.mode);
    } catch {
      setRoutes([]);
    } finally {
      setRoutesLoading(false);
    }
  };

  // Load routes when VPN selection changes.
  const selectVPN = (id: number) => {
    setSelectedVpnId(id);
    setActionMsg("");
    setActionErr("");
    void loadRoutes(id);
  };

  const isEnabled = (appId: string) => {
    return routeStatusMap().get(appId)?.enabled ?? false;
  };

  const isApplied = (appId: string) => {
    return routeStatusMap().get(appId)?.applied ?? false;
  };

  const toggleApp = async (appId: string) => {
    const vpnId = selectedVpnId();
    if (!vpnId || toggling()) return;

    setToggling(appId);
    setActionMsg("");
    setActionErr("");

    const enabled = isEnabled(appId);
    const endpoint = enabled ? "disable" : "enable";

    try {
      const { ok, data } = await apiPost<Record<string, any>>(`/api/opnsense/vpn/${vpnId}/app-routes/${appId}/${endpoint}`, {});
      if (!ok) {
        // Check if the backend is requesting a file upload for large IP range providers.
        if (data?.needs_upload) {
          const app = (profiles()?.apps ?? []).find((a) => a.id === appId);
          setUploadHint({
            appId,
            appName: app?.name ?? appId,
            hint: {
              download_url: data.download_url,
              jq_filter: data.jq_filter ?? "",
              description: data.description ?? "",
              filename: data.upload_filename ?? "",
            },
          });
          return;
        }
        setActionErr(data?.error ?? `Failed to ${endpoint} ${appId}`);
        return;
      }

      // Update mode if the backend auto-switched (e.g. "all" -> "selective").
      if (data?.routing_mode) {
        setRoutingMode(data.routing_mode);
      }

      // Start confirmation countdown if we got a revision.
      if (data?.revision) {
        startCountdown(data.revision);
      }

      if (data?.warnings?.length) {
        setActionMsg(data.warnings.join("; "));
      }
      // Refresh routes.
      await loadRoutes(vpnId);
    } catch {
      setActionErr(`Failed to ${endpoint} ${appId}. Check backend connectivity.`);
    } finally {
      setToggling(null);
    }
  };

  const applyPreset = async (preset: AppPreset) => {
    const vpnId = selectedVpnId();
    if (!vpnId || toggling()) return;

    setActionMsg("");
    setActionErr("");

    const allApps = profiles()?.apps ?? [];
    const allIds = allApps.map((a) => a.id);
    const enabled = enabledRouteIds();

    // Determine which apps should be VPN-routed.
    let enableIds: string[] = [];
    let disableIds: string[] = [];

    if (preset.vpn_on && preset.vpn_on.length > 0) {
      enableIds = preset.vpn_on;
      // Everything else not in vpn_on but currently enabled should be disabled.
      disableIds = allIds.filter((id) => !enableIds.includes(id) && enabled.has(id));
    }
    if (preset.vpn_off && preset.vpn_off.length > 0) {
      disableIds = [...disableIds, ...preset.vpn_off.filter((id) => enabled.has(id))];
      // Enable everything NOT in vpn_off.
      enableIds = [
        ...enableIds,
        ...allIds.filter((id) => !preset.vpn_off.includes(id) && !enabled.has(id)),
      ];
    }
    // Privacy mode: enable everything.
    if (preset.id === "privacy_mode") {
      enableIds = allIds.filter((id) => !enabled.has(id));
      disableIds = [];
    }

    // Deduplicate.
    enableIds = [...new Set(enableIds)];
    disableIds = [...new Set(disableIds.filter((id) => !enableIds.includes(id)))];

    // Apply sequentially, capturing the last revision for confirmation.
    let lastRevision = "";
    for (const id of disableIds) {
      if (enabled.has(id)) {
        setToggling(id);
        try {
          const { data } = await apiPost<Record<string, any>>(`/api/opnsense/vpn/${vpnId}/app-routes/${id}/disable`, {});
          if (data?.revision) lastRevision = data.revision;
        } catch { /* continue */ }
      }
    }
    for (const id of enableIds) {
      if (!enabled.has(id)) {
        setToggling(id);
        try {
          const { data } = await apiPost<Record<string, any>>(`/api/opnsense/vpn/${vpnId}/app-routes/${id}/enable`, {});
          if (data?.revision) lastRevision = data.revision;
        } catch { /* continue */ }
      }
    }

    setToggling(null);
    if (lastRevision) {
      startCountdown(lastRevision);
    } else {
      setActionMsg(`Preset "${preset.name}" applied.`);
    }
    await loadRoutes(vpnId);
  };

  const changeMode = async (mode: RoutingMode) => {
    const vpnId = selectedVpnId();
    if (!vpnId) return;
    setActionMsg("");
    setActionErr("");
    try {
      const { ok, data } = await apiPost<Record<string, any>>(`/api/opnsense/vpn/${vpnId}/routing-mode`, { mode });
      if (!ok) {
        setActionErr(data?.error ?? "Failed to change routing mode");
        return;
      }
      setRoutingMode(mode);

      if (data?.revision) {
        startCountdown(data.revision);
      } else {
        setActionMsg(`Routing mode set to "${mode}".`);
      }
    } catch {
      setActionErr("Failed to change routing mode.");
    }
  };

  const selectedVpnName = () => vpnList().find((v) => v.id === selectedVpnId())?.name ?? "";

  return (
    <div class="space-y-5">
      {/* Header */}
      <div>
        <h1 class="text-[24px] font-semibold tracking-tight text-[var(--text-primary)]">Routing</h1>
        <p class="mt-1 text-[14px] text-[var(--text-tertiary)]">
          Route specific apps and protocols through your VPN
        </p>
      </div>

      {/* Controls Card */}
      <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-tertiary)] p-4">
        <div class="flex flex-wrap items-center gap-4">

        {/* VPN selector */}
        <Show when={vpnList().length > 0}>
          <div class="flex flex-wrap items-center gap-4">
            <div class="flex items-center gap-2">
              <label class="text-[12px] font-medium text-[var(--text-tertiary)]">Target VPN</label>
              <select
                value={selectedVpnId() ?? ""}
                onChange={(e) => {
                  const id = parseInt(e.currentTarget.value);
                  if (!isNaN(id)) selectVPN(id);
                }}
                class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-[13px] text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none"
              >
                <For each={vpnList()}>
                  {(vpn) => (
                    <option value={vpn.id}>
                      {vpn.name}{vpn.routing_applied ? " (active)" : ""}
                    </option>
                  )}
                </For>
              </select>
            </div>
            <div class="flex items-center gap-2">
              <label class="text-[12px] font-medium text-[var(--text-tertiary)]">Mode</label>
              <select
                value={routingMode()}
                onChange={(e) => void changeMode(e.currentTarget.value as RoutingMode)}
                class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-[13px] text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none"
              >
                <option value="all">Route all traffic</option>
                <option value="selective">Only selected apps</option>
                <option value="bypass">All except selected</option>
              </select>
            </div>
          </div>
          <Show when={routingMode() !== "all"}>
            <p class="mt-3 text-[12px] text-[var(--text-tertiary)]">
              {routingMode() === "selective"
                ? "Only toggled apps route through VPN. All other traffic uses the default gateway."
                : "All traffic routes through VPN. Toggled apps bypass it and use the default gateway."}
            </p>
          </Show>
        </Show>

        <Show when={vpnList().length === 0}>
          <Card variant="elevated" class="overflow-hidden">
            <EmptyState
              variant="routing"
              title="No VPN ready for routing"
              description="Deploy a VPN profile with a gateway enabled before you can configure app-based routing."
              action={
                <Button variant="secondary" size="md" onClick={() => window.location.reload()}>
                  Go to VPN setup
                </Button>
              }
            />
          </Card>
        </Show>
      </div>
      </div>

      {/* Pending confirmation banner */}
      <Show when={pendingRevision()}>
        <div class="rounded-xl border border-[var(--status-warning)]/40 bg-[var(--warning-subtle)] px-5 py-4">
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-[14px] font-semibold text-[var(--status-warning)]">
                Firewall changes pending — auto-reverts in {countdown()}s
              </p>
              <p class="mt-1 text-[12px] text-[var(--status-warning)]/70">
                Verify your connectivity is still working, then confirm or revert.
              </p>
            </div>
            <div class="flex shrink-0 gap-2">
              <button
                type="button"
                onClick={() => void confirmChanges()}
                class="rounded-lg bg-[var(--accent-primary)] px-4 py-2 text-[13px] font-semibold text-[var(--bg-primary)] transition-all hover:brightness-110"
              >
                Confirm
              </button>
              <button
                type="button"
                onClick={() => void revertChanges()}
                class="rounded-lg border border-[var(--status-error)]/30 bg-[var(--error-subtle)] px-4 py-2 text-[13px] font-semibold text-[var(--status-error)] transition-all hover:bg-[var(--status-error)]/20"
              >
                Revert
              </button>
            </div>
          </div>
          <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-[var(--status-warning)]/20">
            <div
              class="h-full rounded-full bg-[var(--status-warning)] transition-all duration-1000 ease-linear"
              style={{ width: `${(countdown() / 60) * 100}%` }}
            />
          </div>
        </div>
      </Show>

      {/* Messages */}
      <Show when={!pendingRevision() && actionMsg()}>
        <div class="rounded-lg border border-[var(--status-success)]/30 bg-[var(--success-subtle)] px-4 py-3 text-[14px] text-[var(--status-success)]">
          {actionMsg()}
        </div>
      </Show>
      <Show when={actionErr()}>
        <div class="rounded-lg border border-[var(--status-error)]/30 bg-[var(--error-subtle)] px-4 py-3 text-[14px] text-[var(--status-error)]">
          {actionErr()}
        </div>
      </Show>

      {/* Presets + Filters + App Grid */}
      <Show when={selectedVpnId() && profiles()}>
        <RoutingToolbar
          presets={profiles()?.presets ?? []}
          categories={categoryInfos()}
          search={search()}
          filterCategory={filterCategory()}
          filterStatus={filterStatus()}
          enabledCount={enabledCount()}
          totalCount={totalCount()}
          isToggling={!!toggling()}
          onApplyPreset={(preset) => void applyPreset(preset)}
          onSearchChange={setSearch}
          onFilterCategoryChange={setFilterCategory}
          onFilterStatusChange={setFilterStatus}
          onAddCustom={() => setShowAddModal(true)}
        />

        {/* Loading state */}
        <Show when={routesLoading()}>
          <div class="flex items-center justify-center gap-3 py-8 text-[var(--text-tertiary)]">
            <svg class="h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            <span class="text-[14px]">Loading app routes...</span>
          </div>
        </Show>

        {/* App grid by category */}
        <Show when={!routesLoading()}>
          <Show when={filteredApps().length === 0}>
            <div class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-tertiary)] p-8 text-center">
              <p class="text-[14px] text-[var(--text-tertiary)]">No apps match your search</p>
            </div>
          </Show>

          <For each={groupByCategory(filteredApps())}>
            {(group) => (
              <div class="space-y-4">
                <p class="text-[12px] font-semibold uppercase tracking-[0.08em] text-[var(--text-muted)]">
                  {group.label}
                </p>
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <For each={group.apps}>
                    {(app) => (
                      <AppCard
                        app={app}
                        enabled={isEnabled(app.id)}
                        applied={isApplied(app.id)}
                        busy={toggling() === app.id}
                        toggleDisabled={!!toggling()}
                        onToggle={() => void toggleApp(app.id)}
                        onDelete={() => void deleteCustomProfile(app.id, app.name)}
                      />
                    )}
                  </For>
                </div>
              </div>
            )}
          </For>
        </Show>
      </Show>

      {/* IP Ranges Upload Modal */}
      <Show when={uploadHint()}>
        {(hint) => (
          <IpRangeUploadModal
            appName={hint().appName}
            hint={hint().hint}
            onClose={() => setUploadHint(null)}
            onUploaded={() => { const h = uploadHint(); setUploadHint(null); if (h) void toggleApp(h.appId); }}
          />
        )}
      </Show>

      {/* Add Custom Profile Modal */}
      <Show when={showAddModal()}>
        <CustomProfileModal
          onClose={() => setShowAddModal(false)}
          onSaved={() => { setShowAddModal(false); refetchProfiles(); }}
        />
      </Show>
    </div>
  );
}
