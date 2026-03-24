import { For, Show, createSignal, createEffect, onMount, onCleanup } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import Button from "../components/Button";
import { apiGet } from "../lib/api";

type Props = {
  onConnectionStateChange?: (state: { connected: boolean; message?: string }) => void;
};

type Overview = {
  connected: boolean;
  error?: string;
  error_detail?: string;
  host?: string;
  firewall_type?: string;
  name?: string;
  version?: string;
  updates?: string;
  uptime?: string;
  datetime?: string;
  load_avg?: string;
  cpu_count?: number;
  memory?: {
    used_mb: number;
    total_mb: number;
  };
  disk?: {
    mountpoint?: string;
    used_pct: number;
  };
  gateways?: {
    total: number;
    online: number;
    offline: number;
  };
  wireguard?: {
    interfaces: number;
    peers: number;
    online: number;
  };
  vpn?: {
    configured: boolean;
    applied: boolean;
    routing_applied: boolean;
    name?: string;
    last_applied_at?: string;
  };
  tunnels?: {
    total: number;
    deployed: number;
    errors: number;
  };
  vpn_count?: number;
};

async function fetchOverview(): Promise<Overview> {
  const { ok, data } = await apiGet<Overview & { error?: string }>("/api/opnsense/overview");
  if (!ok) throw new Error(data.error ?? "Failed to load OPNsense overview");
  return data;
}

const POLL_INTERVAL = 15000;

function formatBytes(mb: number): string {
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
  return `${mb} MB`;
}

// Interpret load average relative to CPU cores.
// load_avg is "1m, 5m, 15m" comma-separated. We use the 1-minute value.
function formatLoad(raw: string | undefined, cpuCount: number | undefined): { label: string; color: string } {
  if (!raw) return { label: "N/A", color: "text-[var(--text-muted)]" };
  const first = parseFloat(raw.split(",")[0]?.trim() || "0");
  const cores = cpuCount || 1;
  const ratio = first / cores;
  if (ratio < 0.3) return { label: "Idle", color: "text-[var(--status-success)]" };
  if (ratio < 0.7) return { label: "Low", color: "text-[var(--status-success)]" };
  if (ratio < 1.0) return { label: "Moderate", color: "text-[var(--status-warning)]" };
  if (ratio < 2.0) return { label: "High", color: "text-[var(--status-warning)]" };
  return { label: "Critical", color: "text-[var(--status-error)]" };
}

function formatUptime(raw: string | undefined): { value: string; unit: string } {
  if (!raw) return { value: "--", unit: "" };
  // OPNsense returns "HH:MM:SS" or "D days, HH:MM:SS" or similar
  const parts = raw.split(" ");
  if (parts.length >= 2 && parts[1]?.startsWith("day")) {
    return { value: parts[0], unit: parseInt(parts[0]) === 1 ? "day" : "days" };
  }
  // Just a time string like "01:02:30"
  const timeParts = (parts[0] || "").split(":");
  if (timeParts.length === 3) {
    const h = parseInt(timeParts[0]);
    const m = parseInt(timeParts[1]);
    if (h > 0) return { value: `${h}h ${m}m`, unit: "" };
    return { value: `${m}m`, unit: "" };
  }
  return { value: parts[0] || "--", unit: parts[1] || "" };
}

// Pulsing status dot
function StatusDot(props: { status: "success" | "warning" | "error" | "neutral"; pulse?: boolean }) {
  const colors = {
    success: "bg-[var(--status-success)]",
    warning: "bg-[var(--status-warning)]",
    error: "bg-[var(--status-error)]",
    neutral: "bg-[var(--text-muted)]",
  };
  return (
    <span class={`inline-block h-2 w-2 rounded-full ${colors[props.status]} ${props.pulse ? "animate-pulse-subtle" : ""}`} />
  );
}

// Resource bar
function ResourceBar(props: {
  label: string;
  pct: number;
  detail?: string;
  color?: string;
}) {
  const pct = Math.min(100, Math.round(props.pct));
  const barColor = pct > 90 ? "bg-[var(--status-error)]" : pct > 70 ? "bg-[var(--status-warning)]" : (props.color || "bg-[var(--accent-primary)]");

  return (
    <div class="space-y-2">
      <div class="flex items-center justify-between">
        <span class="text-[12px] text-[var(--text-secondary)]">{props.label}</span>
        <div class="flex items-center gap-2">
          <Show when={props.detail}>
            <span class="text-[11px] text-[var(--text-muted)]">{props.detail}</span>
          </Show>
          <span class="text-[12px] font-mono text-[var(--text-primary)]">{pct}%</span>
        </div>
      </div>
      <div class="h-1.5 overflow-hidden rounded-full bg-[var(--bg-hover)]">
        <div
          class={`h-full rounded-full ${barColor} transition-all duration-700 ease-out`}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}

// Stat tile for the top row
function StatTile(props: {
  icon: string;
  iconColor: string;
  label: string;
  value: string;
  sub?: string;
  accent?: boolean;
  children?: any;
}) {
  const icons: Record<string, () => any> = {
    clock: () => (
      <svg class={`h-5 w-5 ${props.iconColor}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="10" />
        <polyline points="12,6 12,12 16,14" />
      </svg>
    ),
    gateway: () => (
      <svg class={`h-5 w-5 ${props.iconColor}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M5 12.55a11 11 0 0 1 14.08 0" />
        <path d="M1.42 9a16 16 0 0 1 21.16 0" />
        <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
        <circle cx="12" cy="20" r="1" />
      </svg>
    ),
    wireguard: () => (
      <svg class={`h-5 w-5 ${props.iconColor}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
      </svg>
    ),
    tunnel: () => (
      <svg class={`h-5 w-5 ${props.iconColor}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z" />
      </svg>
    ),
  };

  const IconComponent = icons[props.icon];

  return (
    <div class="flex items-start gap-3">
      <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-hover)]">
        {IconComponent && <IconComponent />}
      </div>
      <div class="flex-1 min-w-0">
        <div class="text-[11px] uppercase tracking-[0.12em] text-[var(--text-muted)]">{props.label}</div>
        <div class="mt-0.5 text-[20px] font-semibold tracking-tight text-[var(--text-primary)] leading-tight">
          {props.value}
        </div>
        <Show when={props.sub}>
          <div class="mt-1 text-[12px] text-[var(--text-tertiary)]">{props.sub}</div>
        </Show>
        {props.children}
      </div>
    </div>
  );
}

export default function Dashboard(props: Props) {
  const [overview, setOverview] = createSignal<Overview | null>(null);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal(false);
  const [live, setLive] = createSignal(false);
  let eventSource: EventSource | null = null;

  createEffect(() => {
    const data = overview();
    if (!data) return;
    props.onConnectionStateChange?.({
      connected: !!data.connected,
      message: data.error || data.error_detail,
    });
  });

  const connectSSE = () => {
    if (eventSource) return;
    eventSource = new EventSource("/api/opnsense/overview/stream");

    const handleEvent = (event: MessageEvent) => {
      try {
        // Gin's c.SSEvent wraps the JSON in quotes — parse the outer string then the JSON.
        let raw = event.data;
        if (typeof raw === "string" && raw.startsWith('"')) {
          raw = JSON.parse(raw);
        }
        const data: Overview = typeof raw === "string" ? JSON.parse(raw) : raw;
        setOverview(data);
        setLoading(false);
        setLoadError(false);
        setLive(true);
      } catch { /* ignore malformed events */ }
    };

    // Gin's c.SSEvent("message", ...) sends named events, so we need addEventListener.
    // Also listen on onmessage for unnamed events as fallback.
    eventSource.addEventListener("message", handleEvent);

    eventSource.onerror = () => {
      // EventSource auto-reconnects, but mark as disconnected until next message.
      setLive(false);
    };
  };

  const disconnectSSE = () => {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
      setLive(false);
    }
  };

  // Manual refresh: do a one-shot fetch (useful if SSE is lagging).
  const loadOverview = async () => {
    setLoading(true);
    setLoadError(false);
    try {
      const data = await fetchOverview();
      setOverview(data);
    } catch {
      setLoadError(true);
      props.onConnectionStateChange?.({
        connected: false,
        message: "Gator could not reach the active firewall instance.",
      });
    } finally {
      setLoading(false);
    }
  };

  const handleVisibilityChange = () => {
    if (document.hidden) {
      disconnectSSE();
    } else {
      connectSSE();
    }
  };

  onMount(() => {
    connectSSE();
    document.addEventListener("visibilitychange", handleVisibilityChange);
  });

  onCleanup(() => {
    disconnectSSE();
    document.removeEventListener("visibilitychange", handleVisibilityChange);
  });

  const memPct = () => {
    const d = overview();
    if (!d?.memory?.total_mb) return 0;
    return Math.round((d.memory.used_mb / d.memory.total_mb) * 100);
  };

  const gwStatus = () => {
    const d = overview();
    if (!d?.gateways?.total) return "neutral" as const;
    if (d.gateways.offline > 0) return "warning" as const;
    return "success" as const;
  };

  const wgStatus = () => {
    const d = overview();
    if (!d?.wireguard?.peers) return "neutral" as const;
    if (d.wireguard.online === d.wireguard.peers) return "success" as const;
    if (d.wireguard.online === 0) return "error" as const;
    return "warning" as const;
  };

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-3">
          <h1 class="text-[24px] font-semibold tracking-tight text-[var(--text-primary)]">
            Overview
          </h1>
          <Show when={live()}>
            <span class="flex items-center gap-1.5 rounded-full bg-[var(--success-subtle)] px-2 py-0.5">
              <span class="h-1.5 w-1.5 rounded-full bg-[var(--status-success)] animate-pulse-subtle" />
              <span class="text-[10px] font-medium text-[var(--status-success)]">LIVE</span>
            </span>
          </Show>
        </div>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => void loadOverview()}
          loading={loading()}
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
        </Button>
      </div>

      {/* Error state */}
      <Show when={loadError()}>
        <Card class="border-l-2 border-l-[var(--status-error)] bg-[var(--error-subtle)]">
          <div class="flex items-center gap-3">
            <svg class="h-5 w-5 text-[var(--status-error)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span class="text-[14px] text-[var(--status-error)]">Failed to reach OPNsense API</span>
          </div>
        </Card>
      </Show>

      <Show when={!loading() && !loadError() && overview()}>
        {(data) => (
          <>
            {/* Firewall identity card */}
            <Card class={["relative overflow-hidden", !data().connected ? "border border-[var(--status-error)]/25 bg-[var(--error-subtle)]/40" : ""].join(" ")}>
              <div class={["absolute inset-x-0 top-0 h-0.5 bg-gradient-to-r", data().connected ? "from-[var(--accent-primary)] to-transparent" : "from-[var(--status-error)] to-transparent"].join(" ")} />
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div class="flex items-center gap-3">
                  <div class={["flex h-10 w-10 shrink-0 items-center justify-center rounded-lg", data().connected ? "bg-[var(--bg-hover)]" : "bg-[var(--status-error)]/10"].join(" ")}>
                    <svg class={["h-5 w-5", data().connected ? "text-[var(--accent-primary)]" : "text-[var(--status-error)]"].join(" ")} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
                      <line x1="8" y1="21" x2="16" y2="21" />
                      <line x1="12" y1="17" x2="12" y2="21" />
                    </svg>
                  </div>
                  <div>
                    <div class="flex items-center gap-2">
                      <h2 class="text-[18px] font-semibold text-[var(--text-primary)]">
                        {(data().host || "Firewall").replace(/^https?:\/\//, "")}
                      </h2>
                      <StatusDot status={data().connected ? "success" : "error"} pulse={data().connected} />
                    </div>
                    <p class="text-[12px] text-[var(--text-tertiary)]">
                      {data().name || "OPNsense"}{data().version ? ` ${data().version}` : ""}
                    </p>
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <Show when={!data().connected}>
                    <Badge variant="danger" size="sm">Instance down</Badge>
                  </Show>
                  <Show when={data().connected && data().updates}>
                    <Badge variant="warning" size="sm">{data().updates}</Badge>
                  </Show>
                  <Show when={data().connected && !data().updates}>
                    <Badge variant="muted" size="sm">Up to date</Badge>
                  </Show>
                </div>
              </div>
            </Card>

            <Show when={!data().connected}>
              <Card class="border-l-2 border-l-[var(--status-error)] bg-[var(--bg-secondary)]">
                <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                  <div class="flex items-start gap-3">
                    <div class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--status-error)]/10">
                      <svg class="h-5 w-5 text-[var(--status-error)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M12 9v4" />
                        <path d="M12 17h.01" />
                        <path d="M5 3h14a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H9l-4 4v-4H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Z" />
                      </svg>
                    </div>
                    <div>
                      <h3 class="text-[16px] font-semibold text-[var(--text-primary)]">This firewall is offline</h3>
                      <p class="mt-1 text-[13px] text-[var(--text-secondary)]">
                        Gator can see the saved instance, but it cannot reach the live firewall right now. Start the VM or switch to another instance before making changes.
                      </p>
                      <Show when={data().error}>
                        <p class="mt-2 text-[12px] text-[var(--status-error)]">{data().error}</p>
                      </Show>
                    </div>
                  </div>
                  <div class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)] px-3 py-2 text-[12px] text-[var(--text-tertiary)]">
                    Management actions are temporarily disabled
                  </div>
                </div>
              </Card>
            </Show>

            <Show when={data().connected}>
              <>

            {/* Stats grid -- 2 cols on mobile, 4 on desktop */}
            <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
              {/* Uptime */}
              <Card padding="sm">
                {(() => {
                  const up = formatUptime(data().uptime);
                  const load = formatLoad(data().load_avg, data().cpu_count);
                  return (
                    <StatTile
                      icon="clock"
                      iconColor="text-[var(--status-success)]"
                      label="Uptime"
                      value={up.value}
                      sub={up.unit || undefined}
                    >
                      <Show when={data().load_avg}>
                        <div class="mt-1 flex items-center gap-1.5">
                          <span class={`text-[11px] font-medium ${load.color}`}>{load.label}</span>
                          <span class="text-[11px] text-[var(--text-muted)]" title={`Load: ${data().load_avg}`}>
                            {data().cpu_count ? `${data().cpu_count}-core` : ""}
                          </span>
                        </div>
                      </Show>
                    </StatTile>
                  );
                })()}
              </Card>

              {/* Gateways */}
              <Card padding="sm">
                <StatTile
                  icon="gateway"
                  iconColor="text-[var(--status-info)]"
                  label="Gateways"
                  value={`${data().gateways?.online || 0}/${data().gateways?.total || 0}`}
                >
                  <Show when={(data().gateways?.total || 0) > 0}>
                    <div class="mt-1 flex items-center gap-1.5">
                      <StatusDot status={gwStatus()} />
                      <span class="text-[11px] text-[var(--text-tertiary)]">
                        {data().gateways?.offline ? `${data().gateways.offline} offline` : "All online"}
                      </span>
                    </div>
                  </Show>
                </StatTile>
              </Card>

              {/* WireGuard peers */}
              <Card padding="sm">
                <StatTile
                  icon="wireguard"
                  iconColor="text-[var(--status-warning)]"
                  label="WireGuard"
                  value={`${data().wireguard?.online || 0}/${data().wireguard?.peers || 0}`}
                  sub={`${data().wireguard?.interfaces || 0} interface${(data().wireguard?.interfaces || 0) !== 1 ? "s" : ""}`}
                >
                  <Show when={(data().wireguard?.peers || 0) > 0}>
                    <div class="mt-1 flex items-center gap-1.5">
                      <StatusDot status={wgStatus()} />
                      <span class="text-[11px] text-[var(--text-tertiary)]">
                        {data().wireguard?.online === data().wireguard?.peers ? "All peers up" : `${(data().wireguard?.peers || 0) - (data().wireguard?.online || 0)} peer${((data().wireguard?.peers || 0) - (data().wireguard?.online || 0)) !== 1 ? "s" : ""} down`}
                      </span>
                    </div>
                  </Show>
                </StatTile>
              </Card>

              {/* Tunnels */}
              <Card padding="sm">
                <StatTile
                  icon="tunnel"
                  iconColor="text-[var(--accent-primary)]"
                  label="Tunnels"
                  value={String(data().tunnels?.total || 0)}
                >
                  <Show when={(data().tunnels?.total || 0) > 0}>
                    <div class="mt-1 flex items-center gap-1.5">
                      <StatusDot status={data().tunnels?.errors ? "error" : data().tunnels?.deployed === data().tunnels?.total ? "success" : "warning"} />
                      <span class="text-[11px] text-[var(--text-tertiary)]">
                        {data().tunnels?.errors
                          ? `${data().tunnels!.errors} error${data().tunnels!.errors !== 1 ? "s" : ""}`
                          : `${data().tunnels?.deployed || 0} deployed`}
                      </span>
                    </div>
                  </Show>
                  <Show when={(data().tunnels?.total || 0) === 0}>
                    <div class="mt-1 text-[11px] text-[var(--text-muted)]">No tunnels</div>
                  </Show>
                </StatTile>
              </Card>
            </div>

            {/* Resources row */}
            <div class="grid gap-3 sm:grid-cols-2">
              <Card padding="sm">
                <ResourceBar
                  label="Memory"
                  pct={memPct()}
                  detail={data().memory?.total_mb ? `${formatBytes(data().memory!.used_mb)} / ${formatBytes(data().memory!.total_mb)}` : undefined}
                />
              </Card>
              <Card padding="sm">
                <ResourceBar
                  label="Disk"
                  pct={data().disk?.used_pct || 0}
                  detail={data().disk?.mountpoint || undefined}
                  color="bg-[var(--status-info)]"
                />
              </Card>
            </div>

            {/* VPN Status */}
            <Card class="relative overflow-hidden">
              <div class="absolute right-0 top-0 h-full w-1/2 bg-gradient-to-l from-[var(--accent-primary-subtle)] to-transparent opacity-20 pointer-events-none" />
              <div class="relative flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div class="flex items-center gap-4">
                  <div class={[
                    "flex h-12 w-12 shrink-0 items-center justify-center rounded-xl",
                    data().vpn?.routing_applied
                      ? "bg-[var(--success-subtle)]"
                      : data().vpn?.applied
                        ? "bg-[var(--warning-subtle)]"
                        : "bg-[var(--bg-hover)]"
                  ].join(" ")}>
                    <svg
                      class={[
                        "h-6 w-6",
                        data().vpn?.routing_applied
                          ? "text-[var(--status-success)]"
                          : data().vpn?.applied
                            ? "text-[var(--status-warning)]"
                            : "text-[var(--text-muted)]"
                      ].join(" ")}
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="1.5"
                    >
                      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
                    </svg>
                  </div>
                  <div>
                    <div class="flex items-center gap-2.5">
                      <h2 class="text-[18px] font-semibold text-[var(--text-primary)]">
                        {data().vpn?.name || "VPN"}
                      </h2>
                      {data().vpn?.routing_applied ? (
                        <Badge variant="success" size="sm">Active</Badge>
                      ) : data().vpn?.applied ? (
                        <Badge variant="warning" size="sm">Partial</Badge>
                      ) : data().vpn?.configured ? (
                        <Badge variant="info" size="sm">Ready</Badge>
                      ) : (
                        <Badge variant="muted" size="sm">Not configured</Badge>
                      )}
                    </div>
                    <p class="mt-1 text-[13px] text-[var(--text-tertiary)]">
                      {data().vpn?.configured
                        ? data().vpn?.applied
                          ? data().vpn?.routing_applied
                            ? "Routing traffic through VPN"
                            : "Tunnel connected, routing pending"
                          : "Deployed to OPNsense, activation needed"
                        : "Import a WireGuard config to get started"}
                    </p>
                  </div>
                </div>
                <div class="flex flex-col items-end gap-1.5">
                  <Show when={(data().vpn_count || 0) > 1}>
                    <span class="text-[11px] text-[var(--text-muted)]">
                      {data().vpn_count} VPN profile{data().vpn_count !== 1 ? "s" : ""}
                    </span>
                  </Show>
                  <Show when={data().vpn?.last_applied_at}>
                    <span class="text-[11px] font-mono text-[var(--text-muted)]">
                      {new Date(data().vpn!.last_applied_at!).toLocaleDateString()}
                    </span>
                  </Show>
                </div>
              </div>
            </Card>

            {/* Permission warning */}
            <Show when={data().error && data().connected}>
              <Card class="border-l-2 border-l-[var(--status-warning)]">
                <div class="flex items-start gap-3">
                  <svg class="mt-0.5 h-4 w-4 shrink-0 text-[var(--status-warning)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                    <line x1="12" y1="9" x2="12" y2="13" />
                    <line x1="12" y1="17" x2="12.01" y2="17" />
                  </svg>
                  <span class="text-[12px] text-[var(--text-secondary)]">{data().error}</span>
                </div>
              </Card>
            </Show>
              </>
            </Show>
          </>
        )}
      </Show>
    </div>
  );
}
