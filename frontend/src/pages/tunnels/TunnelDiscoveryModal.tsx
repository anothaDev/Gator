import { createSignal, For, Show, onMount } from "solid-js";
import { apiGet, apiPut } from "../../lib/api";
import Modal from "../../components/Modal";
import Button from "../../components/Button";
import AlertBanner from "../../components/AlertBanner";
import Badge from "../../components/Badge";
import TunnelImportForm from "./TunnelImportForm";
import type { DiscoveredTunnel } from "./types";

// ─── Fingerprint matching ────────────────────────────────────────

/** Compute which fingerprint fields match between a discovered tunnel and the readopt target.
 *  Returns labels of matched fields and total score. */
function tunnelMatchDetails(
  tunnel: DiscoveredTunnel,
  endpoint?: string,
  listenPort?: number,
  firewallIP?: string,
): { score: number; matched: string[] } {
  const matched: string[] = [];
  let score = 0;
  // Compare host portions only (endpoints may include ":port").
  if (endpoint && tunnel.endpoint) {
    const stripPort = (s: string) => s.includes(":") ? s.split(":")[0] : s;
    if (stripPort(tunnel.endpoint) === stripPort(endpoint)) { score += 3; matched.push("endpoint"); }
  }
  // Listen port.
  if (listenPort && tunnel.listen_port) {
    if (String(listenPort) === tunnel.listen_port) { score += 1; matched.push("listen_port"); }
  }
  // Firewall IP from local_cidr (e.g. "10.200.200.2/24" → "10.200.200.2").
  if (firewallIP && tunnel.local_cidr) {
    const cidrIP = tunnel.local_cidr.includes("/")
      ? tunnel.local_cidr.split("/")[0]
      : tunnel.local_cidr;
    if (cidrIP === firewallIP) { score += 2; matched.push("firewall_ip"); }
  }
  return { score, matched };
}

/** Require at least 2 matched fields for "Suggested match" status. */
function isTunnelSuggestedMatch(details: { score: number; matched: string[] }): boolean {
  return details.matched.length >= 2;
}

// ─── Tunnel Discovery Modal ──────────────────────────────────────
// Shows discovered OPNsense WireGuard tunnels. In normal mode, lets
// the user pick one and import it (via TunnelImportForm). In readopt
// mode, links the discovered tunnel to an existing Gator profile.

export default function TunnelDiscoveryModal(props: {
  onClose: () => void;
  onImported: () => void;
  /** When set, the modal is in "re-adopt" mode. */
  readoptId?: number;
  readoptName?: string;
  /** Fingerprint fields from the drifted profile, used to suggest matches. */
  readoptEndpoint?: string;
  readoptListenPort?: number;
  readoptFirewallIP?: string;
}) {
  const [scanning, setScanning] = createSignal(true);
  const [scanError, setScanError] = createSignal("");
  const [discovered, setDiscovered] = createSignal<DiscoveredTunnel[]>([]);
  const [readopting, setReadopting] = createSignal<string | null>(null);
  const [readoptError, setReadoptError] = createSignal("");
  const [failedUUID, setFailedUUID] = createSignal<string | null>(null);
  const [importTarget, setImportTarget] = createSignal<DiscoveredTunnel | null>(null);
  const [confirmTarget, setConfirmTarget] = createSignal<DiscoveredTunnel | null>(null);
  const isReadopt = () => props.readoptId !== undefined;
  const hasFingerprint = () => !!(props.readoptEndpoint || props.readoptListenPort || props.readoptFirewallIP);

  /** In readopt mode, sort discovered tunnels by fingerprint match (best first). */
  const sortedDiscovered = () => {
    const list = discovered();
    if (!isReadopt() || !hasFingerprint()) return list;
    return [...list].sort((a, b) =>
      tunnelMatchDetails(b, props.readoptEndpoint, props.readoptListenPort, props.readoptFirewallIP).score -
      tunnelMatchDetails(a, props.readoptEndpoint, props.readoptListenPort, props.readoptFirewallIP).score
    );
  };

  onMount(async () => {
    try {
      const { ok, data } = await apiGet<{ tunnels?: DiscoveredTunnel[]; error?: string }>("/api/tunnels/discover");
      if (!ok) throw new Error(data.error ?? "Discovery failed");
      setDiscovered(data.tunnels ?? []);
    } catch (err) {
      setScanError(err instanceof Error ? err.message : "Discovery failed");
    } finally {
      setScanning(false);
    }
  });

  const doReadopt = async (tunnel: DiscoveredTunnel) => {
    setReadopting(tunnel.server_uuid);
    setReadoptError("");
    setFailedUUID(null);

    try {
      const { ok, data } = await apiPut(`/api/tunnels/${props.readoptId}/readopt`, {
        server_uuid: tunnel.server_uuid,
        peer_uuid: tunnel.peer_uuid,
        local_cidr: tunnel.local_cidr,
        endpoint: tunnel.endpoint,
        listen_port: parseInt(tunnel.listen_port) || 51820,
        peer_pubkey: tunnel.peer_pubkey,
      });

      if (!ok) {
        setReadoptError((data as { error?: string })?.error ?? "Re-adopt failed");
        setFailedUUID(tunnel.server_uuid);
        setReadopting(null);
        return;
      }

      props.onImported();
    } catch {
      setReadoptError("Network error");
      setFailedUUID(tunnel.server_uuid);
      setReadopting(null);
    }
  };

  return (
    <Modal size="lg" onBackdropClick={props.onClose}>
      {/* Import step: show TunnelImportForm for the selected tunnel */}
      <Show when={!isReadopt() && importTarget()}>
        <h2 class="text-[var(--text-lg)] font-semibold text-[var(--text-primary)]">
          Import Tunnel
        </h2>
        <TunnelImportForm
          tunnel={importTarget()!}
          onImported={props.onImported}
          onBack={() => setImportTarget(null)}
        />
      </Show>

      {/* Discovery list */}
      <Show when={isReadopt() || !importTarget()}>
        <h2 class="text-[var(--text-lg)] font-semibold text-[var(--text-primary)]">
          {isReadopt() ? `Re-adopt: ${props.readoptName}` : "Discover Tunnels on OPNsense"}
        </h2>
        <p class="mt-1 text-[var(--text-xs)] text-[var(--text-tertiary)]">
          {isReadopt()
            ? "Select the OPNsense tunnel to link to this profile."
            : "Discover existing WireGuard site-to-site tunnels and import them into Gator."}
        </p>

        <Show when={scanning()}>
          <div class="mt-4 flex items-center gap-3 text-[var(--text-sm)] text-[var(--text-tertiary)]">
            <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
            </svg>
            Scanning WireGuard tunnels...
          </div>
        </Show>

        <Show when={scanError()}>
          <div class="mt-4">
            <AlertBanner tone="error">{scanError()}</AlertBanner>
          </div>
        </Show>

        <Show when={!scanning() && !scanError() && discovered().length === 0}>
          <div class="mt-4 rounded-lg border border-[var(--border-strong)] bg-[var(--bg-tertiary)] px-3 py-3 text-[var(--text-sm)] text-[var(--text-secondary)]">
            No importable WireGuard tunnels found on OPNsense.
          </div>
        </Show>

        <Show when={!scanning() && discovered().length > 0}>
          <div class="mt-4 space-y-3 max-h-80 overflow-y-auto">
            <For each={sortedDiscovered()}>
              {(tunnel) => {
                const details = () => isReadopt() && hasFingerprint()
                  ? tunnelMatchDetails(tunnel, props.readoptEndpoint, props.readoptListenPort, props.readoptFirewallIP)
                  : { score: 0, matched: [] as string[] };
                const suggested = () => isTunnelSuggestedMatch(details());
                return (
                <div class={`rounded-lg border p-3 ${suggested() ? "border-[var(--status-success)]/50 bg-[var(--status-success)]/5" : "border-[var(--border-strong)] bg-[var(--bg-tertiary)]"}`}>
                  <div class="flex items-start justify-between gap-3">
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2">
                        <p class="font-medium text-[var(--text-primary)]">
                          {tunnel.peer_name || tunnel.server_name || "Unknown"}
                        </p>
                        <Show when={suggested()}>
                          <Badge variant="success" size="sm">Suggested match</Badge>
                        </Show>
                        <Show when={isReadopt() && details().matched.length > 0}>
                          <span class="text-[var(--text-xs)] text-[var(--text-muted)]">
                            matched: {details().matched.join(", ")}
                          </span>
                        </Show>
                      </div>
                      <p class="mt-0.5 text-[var(--text-xs)] text-[var(--text-tertiary)]">
                        {tunnel.endpoint}
                        <Show when={tunnel.local_cidr}>
                          <span class="text-[var(--text-muted)]">{" "}&mdash; {tunnel.local_cidr}</span>
                        </Show>
                      </p>
                      <div class="mt-1.5 flex flex-wrap gap-1.5">
                        <Show when={tunnel.wg_device}>
                          <Badge variant="info" size="sm">{tunnel.wg_device}</Badge>
                        </Show>
                        <Show when={tunnel.wg_iface}>
                          <Badge variant="default" size="sm">
                            {tunnel.wg_iface}{tunnel.iface_desc ? ` (${tunnel.iface_desc})` : ""}
                          </Badge>
                        </Show>
                        <Show when={tunnel.gateway_name}>
                          <Badge variant="warning" size="sm">{tunnel.gateway_name}</Badge>
                        </Show>
                      </div>
                    </div>
                    <Show when={isReadopt()} fallback={
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => setImportTarget(tunnel)}
                      >
                        Import
                      </Button>
                    }>
                      <Show when={confirmTarget()?.server_uuid !== tunnel.server_uuid}>
                        <Button
                          variant="primary"
                          size="sm"
                          onClick={() => setConfirmTarget(tunnel)}
                          disabled={readopting() !== null}
                        >
                          Link to {props.readoptName}
                        </Button>
                      </Show>
                    </Show>
                  </div>

                  {/* Re-adopt confirmation (shown when this tunnel is selected for re-adopt) */}
                  <Show when={isReadopt() && confirmTarget()?.server_uuid === tunnel.server_uuid}>
                    <div class="mt-3 border-t border-[var(--border-default)] pt-3">
                      <p class="text-[var(--text-xs)] text-[var(--text-secondary)]">
                        Re-link <strong>{props.readoptName}</strong> to this OPNsense tunnel? This will update all stored UUIDs.
                      </p>
                      <Show when={readoptError() && failedUUID() === tunnel.server_uuid}>
                        <p class="mt-1 text-[var(--text-xs)] text-[var(--status-error)]">{readoptError()}</p>
                      </Show>
                      <div class="mt-2 flex justify-end gap-2">
                        <Button variant="secondary" size="sm" onClick={() => setConfirmTarget(null)}>
                          Cancel
                        </Button>
                        <Button
                          variant="primary"
                          size="sm"
                          onClick={() => void doReadopt(tunnel)}
                          disabled={readopting() !== null}
                          loading={readopting() === tunnel.server_uuid}
                        >
                          Confirm re-adopt
                        </Button>
                      </div>
                    </div>
                  </Show>

                  {/* Re-adopt error (scoped, shown only when NOT in confirmation panel) */}
                  <Show when={isReadopt() && readoptError() && failedUUID() === tunnel.server_uuid && confirmTarget()?.server_uuid !== tunnel.server_uuid}>
                    <p class="mt-2 text-[var(--text-xs)] text-[var(--status-error)]">{readoptError()}</p>
                  </Show>
                </div>
                );
              }}
            </For>
          </div>
        </Show>

        <div class="mt-5 flex justify-end">
          <Button
            variant="secondary"
            size="md"
            onClick={props.onClose}
            disabled={readopting() !== null}
          >
            Close
          </Button>
        </div>
      </Show>
    </Modal>
  );
}
