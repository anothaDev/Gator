import { For, Show, createSignal, onMount, onCleanup } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import Button from "../components/Button";
import { apiGet } from "../lib/api";

type Overview = {
  connected: boolean;
  error?: string;
  error_detail?: string;
  name?: string;
  version?: string;
  updates?: string;
  uptime?: string;
  datetime?: string;
  load_avg?: string;
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

// Compact metric display
function Metric(props: { label: string; value: string; unit?: string }) {
  return (
    <div class="flex items-baseline gap-1.5">
      <span class="text-[24px] font-semibold tracking-tight text-[var(--text-primary)]">
        {props.value}
      </span>
      {props.unit && (
        <span class="text-[13px] text-[var(--text-tertiary)]">{props.unit}</span>
      )}
      <span class="ml-1.5 text-[11px] uppercase tracking-[0.15em] text-[var(--text-muted)]">
        {props.label}
      </span>
    </div>
  );
}

// Resource bar with clean design
function ResourceBar(props: {
  label: string;
  used: number;
  total: number;
  color?: string;
}) {
  const pct = Math.min(100, Math.round((props.used / props.total) * 100));
  const color = props.color || "bg-[var(--accent-primary)]";
  
  return (
    <div class="space-y-1.5">
      <div class="flex items-center justify-between">
        <span class="text-[12px] text-[var(--text-secondary)]">{props.label}</span>
        <span class="text-[12px] font-mono text-[var(--text-primary)]">{pct}%</span>
      </div>
      <div class="h-1.5 overflow-hidden rounded-full bg-[var(--bg-hover)]">
        <div 
          class={`h-full rounded-full ${color} transition-all duration-700 ease-out`}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}

// Status indicator
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

export default function Dashboard() {
  const [overview, setOverview] = createSignal<Overview | null>(null);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal(false);
  let pollTimer: ReturnType<typeof setInterval> | undefined;

  const stopPolling = () => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = undefined;
    }
  };

  const startPolling = () => {
    if (pollTimer || document.hidden) return;
    pollTimer = setInterval(() => void loadOverview(true), POLL_INTERVAL);
  };

  const handleVisibilityChange = () => {
    if (document.hidden) {
      stopPolling();
      return;
    }
    void loadOverview(true);
    startPolling();
  };

  const loadOverview = async (silent = false) => {
    if (!silent) setLoading(true);
    setLoadError(false);
    try {
      const data = await fetchOverview();
      setOverview(data);
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    void loadOverview();
    startPolling();
    document.addEventListener("visibilitychange", handleVisibilityChange);
  });

  onCleanup(() => {
    stopPolling();
    document.removeEventListener("visibilitychange", handleVisibilityChange);
  });

  return (
    <div class="space-y-6">
      {/* Header - clean and minimal */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-[24px] font-semibold tracking-tight text-[var(--text-primary)]">
            Overview
          </h1>
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
            <span class="text-[14px] text-[var(--status-error)]">Connection failed</span>
          </div>
        </Card>
      </Show>

      {/* Main content */}
      <Show when={!loading() && !loadError() && overview()}>
        {(data) => (
          <>
            {/* Top row - System status */}
            <div class="grid gap-4 lg:grid-cols-3">
              {/* System Info */}
              <Card class="relative overflow-hidden">
                <div class="absolute inset-x-0 top-0 h-0.5 bg-gradient-to-r from-[var(--accent-primary)] to-transparent" />
                <div class="flex items-start justify-between">
                  <div class="flex items-start gap-3">
                    <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-hover)]">
                      <svg class="h-5 w-5 text-[var(--accent-primary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
                        <line x1="8" y1="21" x2="16" y2="21" />
                        <line x1="12" y1="17" x2="12" y2="21" />
                      </svg>
                    </div>
                    <div>
                      <div class="flex items-center gap-2">
                        <h2 class="text-[18px] font-semibold text-[var(--text-primary)]">
                          {data().name || "Unknown"}
                        </h2>
                        <Show when={data().connected}>
                          <StatusDot status="success" pulse />
                        </Show>
                      </div>
                      <p class="mt-0.5 text-[13px] text-[var(--text-tertiary)]">
                        {data().version || "Version unknown"}
                      </p>
                    </div>
                  </div>
                </div>
                <div class="mt-4 flex items-center gap-2">
                  <Badge variant={data().updates ? "warning" : "muted"} size="sm">
                    {data().updates || "No updates"}
                  </Badge>
                </div>
              </Card>

              {/* Runtime */}
              <Card>
                <div class="flex items-start gap-3">
                  <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-hover)]">
                    <svg class="h-5 w-5 text-[var(--status-success)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="12" cy="12" r="10" />
                      <polyline points="12,6 12,12 16,14" />
                    </svg>
                  </div>
                  <div class="flex-1">
                    <Metric label="Uptime" value={data().uptime?.split(" ")[0] || "--"} unit={data().uptime?.split(" ")[1] || ""} />
                    <div class="mt-2 flex items-center gap-4 text-[12px] text-[var(--text-tertiary)]">
                      <span>Load: {data().load_avg || "--"}</span>
                    </div>
                  </div>
                </div>
              </Card>

              {/* Network */}
              <Card>
                <div class="flex items-start gap-3">
                  <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-hover)]">
                    <svg class="h-5 w-5 text-[var(--status-info)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M5 12.55a11 11 0 0 1 14.08 0" />
                      <path d="M1.42 9a16 16 0 0 1 21.16 0" />
                      <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
                      <circle cx="12" cy="20" r="1" />
                    </svg>
                  </div>
                  <div class="flex-1">
                    <Metric 
                      label="Gateways" 
                      value={`${data().gateways?.online || 0}/${data().gateways?.total || 0}`} 
                    />
                    <div class="mt-2 flex items-center gap-4 text-[12px] text-[var(--text-tertiary)]">
                      <span class="flex items-center gap-1.5">
                        <StatusDot status={data().wireguard?.online === data().wireguard?.peers ? "success" : "warning"} />
                        {data().wireguard?.online || 0}/{data().wireguard?.peers || 0} peers
                      </span>
                    </div>
                  </div>
                </div>
              </Card>
            </div>

            {/* Resources */}
            <Card>
              <div class="flex items-center gap-2 mb-4">
                <svg class="h-4 w-4 text-[var(--accent-primary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M22 12h-4l-3 9L9 3l-3 9H2" />
                </svg>
                <span class="text-[11px] uppercase tracking-[0.15em] text-[var(--text-muted)]">Resources</span>
              </div>
              <div class="grid gap-6 md:grid-cols-2">
                <ResourceBar
                  label="Memory"
                  used={data().memory?.used_mb || 0}
                  total={data().memory?.total_mb || 1}
                />
                <ResourceBar
                  label="Disk"
                  used={data().disk?.used_pct || 0}
                  total={100}
                  color="bg-[var(--status-success)]"
                />
              </div>
            </Card>

            {/* VPN Status - Feature card */}
            <Card class="relative overflow-hidden">
              {/* Subtle gradient background */}
              <div class="absolute right-0 top-0 h-full w-2/3 bg-gradient-to-l from-[var(--accent-primary-subtle)] to-transparent opacity-30" />
              
              <div class="relative">
                <div class="flex items-start gap-5">
                  {/* Large status icon */}
                  <div class={[
                    "flex h-14 w-14 shrink-0 items-center justify-center rounded-xl",
                    data().vpn?.routing_applied
                      ? "bg-[var(--success-subtle)]"
                      : data().vpn?.applied
                        ? "bg-[var(--warning-subtle)]"
                        : "bg-[var(--bg-hover)]"
                  ].join(" ")}>
                    <svg 
                      class={[
                        "h-7 w-7",
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

                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-3">
                      <h2 class="text-[20px] font-semibold text-[var(--text-primary)]">
                        {data().vpn?.name || "VPN"}
                      </h2>
                      {data().vpn?.routing_applied ? (
                        <Badge variant="success" size="sm">
                          <StatusDot status="success" pulse />
                          Active
                        </Badge>
                      ) : data().vpn?.applied ? (
                        <Badge variant="warning" size="sm">Partial</Badge>
                      ) : data().vpn?.configured ? (
                        <Badge variant="info" size="sm">Configured</Badge>
                      ) : (
                        <Badge variant="muted" size="sm">Not set</Badge>
                      )}
                    </div>

                    <p class="mt-1.5 text-[14px] text-[var(--text-secondary)]">
                      {data().vpn?.configured
                        ? data().vpn?.applied
                          ? data().vpn?.routing_applied
                            ? "Routing traffic securely through OPNsense"
                            : "Tunnel connected, routing pending"
                          : "Configuration saved, deployment needed"
                        : "No VPN configured"}
                    </p>

                    <Show when={data().vpn?.last_applied_at}>
                      <p class="mt-3 text-[12px] font-mono text-[var(--text-muted)]">
                        {new Date(data().vpn!.last_applied_at!).toLocaleString()}
                      </p>
                    </Show>
                  </div>
                </div>
              </div>
            </Card>
          </>
        )}
      </Show>
    </div>
  );
}
