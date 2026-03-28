import { createSignal, For, onMount, Show } from "solid-js";
import { apiGet } from "../lib/api";
import { createLegacyCheck } from "../lib/legacy";
import TunnelDeployModal from "./tunnels/TunnelDeployModal";
import TunnelDiscoveryModal from "./tunnels/TunnelDiscoveryModal";
import TunnelCard from "./tunnels/TunnelCard";
import CreateTunnelForm from "./tunnels/CreateTunnelForm";
import type { TunnelStatus } from "./tunnels/types";
import Card from "../components/Card";
import OpnsenseLink from "../components/OpnsenseLink";
import Button from "../components/Button";
import AlertBanner from "../components/AlertBanner";
import EmptyState from "../components/EmptyState";
import LegacyRulesWarning from "../components/LegacyRulesWarning";
import { SkeletonList } from "../components/Skeleton";

// ─── Legacy rule check (same pattern as VpnSetup.tsx) ────────────

// ─── Main Component ──────────────────────────────────────────────

export default function Tunnels() {
  const [tunnels, setTunnels] = createSignal<TunnelStatus[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal("");
  const [showCreate, setShowCreate] = createSignal(false);

  // Deploy modal state.
  const [deployId, setDeployId] = createSignal<number | null>(null);
  const [deployName, setDeployName] = createSignal("");
  const [deployMode, setDeployMode] = createSignal<"full" | "setup-remote">("full");

  // Page-level messages (set by TunnelCard callbacks).
  const [actionMsg, setActionMsg] = createSignal("");
  const [actionErr, setActionErr] = createSignal("");

  // Legacy rules state.
  const { legacyCount, legacyChecked, checkLegacyRules } = createLegacyCheck();

  // Import/discover state.
  const [showImport, setShowImport] = createSignal(false);

  // Re-adopt state: when set, the discovery modal opens in readopt mode.
  const [readoptTarget, setReadoptTarget] = createSignal<{
    id: number; name: string; remote_host?: string; listen_port?: number; firewall_ip?: string;
  } | null>(null);

  const loadTunnels = async () => {
    setLoading(true);
    setLoadError("");
    try {
      const { ok, data } = await apiGet<{ tunnels: TunnelStatus[] }>("/api/tunnels");
      if (ok) setTunnels(data.tunnels ?? []);
      else setLoadError("Failed to load tunnels");
    } catch {
      setLoadError("Failed to load tunnels");
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    void loadTunnels();
    void checkLegacyRules();
  });

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-fg">
            Site-to-Site Tunnels
          </h1>
          <p class="mt-1 text-sm text-fg-tertiary">
            WireGuard tunnels between your firewall and remote VPS endpoints.
          </p>
        </div>
        <div class="flex items-center gap-2">
          <OpnsenseLink path="/ui/wireguard/general" label="WireGuard" />
          <Button
            variant="secondary"
            size="md"
            onClick={() => void loadTunnels()}
            loading={loading()}
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
            </svg>
            Refresh
          </Button>
          <Button
            variant="secondary"
            size="md"
            onClick={() => setShowImport(true)}
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="17,8 12,3 7,8" />
              <line x1="12" y1="3" x2="12" y2="15" />
            </svg>
            Import from OPNsense
          </Button>
          <Button
            variant="primary"
            size="md"
            onClick={() => setShowCreate(true)}
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            New Tunnel
          </Button>
        </div>
      </div>

      {/* Action messages */}
      <Show when={actionMsg()}>
        <AlertBanner tone="success">{actionMsg()}</AlertBanner>
      </Show>
      <Show when={actionErr()}>
        <AlertBanner tone="error">{actionErr()}</AlertBanner>
      </Show>

      {/* Legacy rules warning */}
      <Show when={legacyChecked() && legacyCount() > 0}>
        <LegacyRulesWarning
          count={legacyCount()}
          detail="Deploy and re-adopt actions are blocked until migration is complete."
        />
      </Show>

      {/* Loading */}
      <Show when={loading()}>
        <SkeletonList items={2} />
      </Show>

      {/* Error */}
      <Show when={loadError()}>
        <AlertBanner tone="error">{loadError()}</AlertBanner>
      </Show>

      {/* Tunnel cards */}
      <Show when={!loading() && !loadError()}>
        <Show when={tunnels().length === 0 && !showCreate()}>
          <Card variant="elevated" class="overflow-hidden">
            <EmptyState
              variant="tunnel"
              title="No tunnels configured"
              description="Create a site-to-site WireGuard tunnel to connect your firewall to remote VPS endpoints securely."
              action={
                <div class="flex flex-wrap justify-center gap-3">
                  <Button variant="primary" size="md" onClick={() => setShowCreate(true)}>
                    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <line x1="12" y1="5" x2="12" y2="19" />
                      <line x1="5" y1="12" x2="19" y2="12" />
                    </svg>
                    Create tunnel
                  </Button>
                  <Button variant="secondary" size="md" onClick={() => setShowImport(true)}>
                    Import from OPNsense
                  </Button>
                </div>
              }
            />
          </Card>
        </Show>

        <For each={tunnels()}>
          {(t) => (
            <TunnelCard
              tunnel={t}
              onDeploy={(mode) => { setDeployMode(mode); setDeployId(t.id); setDeployName(t.name); }}
              onUpdated={() => void loadTunnels()}
              onMessage={setActionMsg}
              onError={setActionErr}
              legacyRuleCount={legacyCount()}
              onReadopt={() => {
                setReadoptTarget({
                  id: t.id, name: t.name,
                  remote_host: t.remote_host, listen_port: t.listen_port, firewall_ip: t.firewall_ip,
                });
                setShowImport(true);
              }}
            />
          )}
        </For>
      </Show>

      {/* Create form */}
      <Show when={showCreate()}>
        <CreateTunnelForm
          onCreated={() => { setShowCreate(false); void loadTunnels(); }}
          onCancel={() => setShowCreate(false)}
        />
      </Show>

      {/* Import / Re-adopt from OPNsense Modal */}
      <Show when={showImport()}>
        <TunnelDiscoveryModal
          onClose={() => { setShowImport(false); setReadoptTarget(null); }}
          onImported={() => { setShowImport(false); setReadoptTarget(null); void loadTunnels(); }}
          readoptId={readoptTarget()?.id}
          readoptName={readoptTarget()?.name}
          readoptEndpoint={readoptTarget()?.remote_host}
          readoptListenPort={readoptTarget()?.listen_port}
          readoptFirewallIP={readoptTarget()?.firewall_ip}
        />
      </Show>

      {/* Deploy Modal */}
      <Show when={deployId()}>
        <TunnelDeployModal
          tunnelId={deployId()!}
          tunnelName={deployName()}
          mode={deployMode()}
          onClose={() => setDeployId(null)}
          onComplete={() => { setDeployId(null); void loadTunnels(); }}
        />
      </Show>
    </div>
  );
}
