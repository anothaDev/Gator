import { createSignal, For, Show, onMount } from "solid-js";
import { apiGet, apiPost, apiPut } from "../../lib/api";
import Modal from "../../components/Modal";
import Button from "../../components/Button";
import AlertBanner from "../../components/AlertBanner";
import Badge from "../../components/Badge";
import Spinner from "../../components/Spinner";

type DiscoveredVPN = {
  type: "vpn_client" | "tunnel";
  server_uuid: string;
  server_name: string;
  peer_uuid: string;
  peer_name: string;
  local_cidr: string;
  remote_cidr: string;
  endpoint: string;
  peer_pubkey: string;
  has_psk: boolean;
  dns: string;
  listen_port: string;
  wg_device: string;
  wg_iface: string;
  iface_desc: string;
  gateway_uuid: string;
  gateway_name: string;
  gateway_ip: string;
  filter_uuids: string[] | null;
  snat_uuids: string[] | null;
  source_interfaces: string[] | null;
};

/** Compute which fingerprint fields match between a discovered VPN and the readopt target.
 *  Returns labels of matched fields and total score. */
function matchDetails(vpn: DiscoveredVPN, endpoint?: string, wgDevice?: string, gatewayName?: string): { score: number; matched: string[] } {
  const matched: string[] = [];
  let score = 0;
  if (endpoint && vpn.endpoint === endpoint) { score += 3; matched.push("endpoint"); }
  if (wgDevice && vpn.wg_device === wgDevice) { score += 1; matched.push("wg_device"); }
  if (gatewayName && vpn.gateway_name === gatewayName) { score += 1; matched.push("gateway"); }
  return { score, matched };
}

/** Require at least 2 matched fields for "Suggested match" status. */
function isSuggestedMatch(details: { score: number; matched: string[] }): boolean {
  return details.matched.length >= 2;
}

export default function DiscoveryModal(props: {
  onClose: () => void;
  onImported: () => void;
  /** When set, the modal is in "re-adopt" mode — clicking a discovered entry
   *  re-links it to this existing VPN profile instead of creating a new one. */
  readoptId?: number;
  readoptName?: string;
  /** Fingerprint fields from the drifted profile, used to suggest matches. */
  readoptEndpoint?: string;
  readoptWGDevice?: string;
  readoptGatewayName?: string;
}) {
  const [scanning, setScanning] = createSignal(true);
  const [scanError, setScanError] = createSignal("");
  const [discovered, setDiscovered] = createSignal<DiscoveredVPN[]>([]);
  const [importing, setImporting] = createSignal<string | null>(null);
  const [importError, setImportError] = createSignal("");
  const [importName, setImportName] = createSignal("");
  const [importTarget, setImportTarget] = createSignal<DiscoveredVPN | null>(null);
  const [failedUUID, setFailedUUID] = createSignal<string | null>(null);
  const isReadopt = () => props.readoptId !== undefined;
  const hasFingerprint = () => !!(props.readoptEndpoint || props.readoptWGDevice || props.readoptGatewayName);

  const [confirmTarget, setConfirmTarget] = createSignal<DiscoveredVPN | null>(null);

  /** In readopt mode, sort discovered VPNs by fingerprint match (best first). */
  const sortedDiscovered = () => {
    const list = discovered();
    if (!isReadopt() || !hasFingerprint()) return list;
    return [...list].sort((a, b) =>
      matchDetails(b, props.readoptEndpoint, props.readoptWGDevice, props.readoptGatewayName).score -
      matchDetails(a, props.readoptEndpoint, props.readoptWGDevice, props.readoptGatewayName).score
    );
  };

  onMount(async () => {
    try {
      const { ok, data } = await apiGet<{ vpns?: DiscoveredVPN[]; error?: string }>("/api/opnsense/vpn/discover");
      if (!ok) throw new Error(data.error ?? "Scan failed");
      setDiscovered(data.vpns ?? []);
    } catch (err) {
      setScanError(err instanceof Error ? err.message : "Scan failed");
    } finally {
      setScanning(false);
    }
  });

  const startImport = (vpn: DiscoveredVPN) => {
    setImportTarget(vpn);
    setImportName(vpn.server_name || vpn.peer_name || "Imported VPN");
    setImportError("");
  };

  const doReadopt = async (vpn: DiscoveredVPN) => {
    setImporting(vpn.server_uuid);
    setImportError("");
    setFailedUUID(null);

    try {
      const { ok, data } = await apiPut(`/api/opnsense/vpn/${props.readoptId}/readopt`, {
        server_uuid: vpn.server_uuid,
        peer_uuid: vpn.peer_uuid,
        local_cidr: vpn.local_cidr,
        remote_cidr: vpn.remote_cidr,
        endpoint: vpn.endpoint,
        peer_pubkey: vpn.peer_pubkey,
        dns: vpn.dns,
        wg_iface: vpn.wg_iface,
        wg_device: vpn.wg_device,
        gateway_uuid: vpn.gateway_uuid,
        gateway_name: vpn.gateway_name,
        filter_uuids: vpn.filter_uuids ?? [],
        snat_uuids: vpn.snat_uuids ?? [],
        source_interfaces: vpn.source_interfaces ?? [],
      });

      if (!ok) {
        setImportError((data as { error?: string })?.error ?? "Re-adopt failed");
        setFailedUUID(vpn.server_uuid);
        setImporting(null);
        return;
      }

      props.onImported();
    } catch {
      setImportError("Network error");
      setFailedUUID(vpn.server_uuid);
      setImporting(null);
    }
  };

  const doImport = async () => {
    const vpn = importTarget();
    if (!vpn) return;
    const name = importName().trim();
    if (!name) {
      setImportError("Name is required");
      return;
    }

    setImporting(vpn.server_uuid);
    setImportError("");

    try {
      const { ok, data } = await apiPost<{ error?: string }>("/api/opnsense/vpn/import", {
        name,
        server_uuid: vpn.server_uuid,
        peer_uuid: vpn.peer_uuid,
        local_cidr: vpn.local_cidr,
        remote_cidr: vpn.remote_cidr,
        endpoint: vpn.endpoint,
        peer_pubkey: vpn.peer_pubkey,
        dns: vpn.dns,
        wg_iface: vpn.wg_iface,
        wg_device: vpn.wg_device,
        gateway_uuid: vpn.gateway_uuid,
        gateway_name: vpn.gateway_name,
        filter_uuids: vpn.filter_uuids ?? [],
        snat_uuids: vpn.snat_uuids ?? [],
        source_interfaces: vpn.source_interfaces ?? [],
      });

      if (!ok) {
        setImportError(data?.error ?? "Import failed");
        setImporting(null);
        return;
      }

      props.onImported();
    } catch {
      setImportError("Network error");
      setImporting(null);
    }
  };

  return (
    <Modal size="lg" onBackdropClick={props.onClose}>
      <h2 class="text-lg font-semibold text-fg">
        {isReadopt() ? `Re-adopt: ${props.readoptName}` : "Scan OPNsense"}
      </h2>
      <p class="mt-1 text-xs text-fg-tertiary">
        {isReadopt()
          ? "Select the OPNsense resource to link to this profile."
          : "Discover existing WireGuard VPN setups and import them into Gator."}
      </p>

      <Show when={scanning()}>
        <div class="mt-4 flex items-center gap-3 text-sm text-fg-tertiary">
          <Spinner />
          Scanning WireGuard, gateways, rules...
        </div>
      </Show>

      <Show when={scanError()}>
        <div class="mt-4">
          <AlertBanner tone="error">{scanError()}</AlertBanner>
        </div>
      </Show>

      <Show when={!scanning() && !scanError() && discovered().length === 0}>
        <div class="mt-4 rounded-lg border border-line-strong bg-surface-tertiary px-3 py-3 text-sm text-fg-secondary">
          No WireGuard VPN setups found on OPNsense. Make sure WireGuard is configured with at least one server and peer.
        </div>
      </Show>

      <Show when={!scanning() && discovered().length > 0}>
        <div class="mt-4 space-y-3 max-h-80 overflow-y-auto">
          <For each={sortedDiscovered()}>
            {(vpn) => {
              const details = () => isReadopt() && hasFingerprint()
                ? matchDetails(vpn, props.readoptEndpoint, props.readoptWGDevice, props.readoptGatewayName)
                : { score: 0, matched: [] as string[] };
              const suggested = () => isSuggestedMatch(details());
              return (
              <div class={`rounded-lg border p-3 ${suggested() ? "border-success/50 bg-success/5" : "border-line-strong bg-surface-tertiary"}`}>
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2">
                      <p class="font-medium text-fg">{vpn.server_name || vpn.peer_name || "Unknown"}</p>
                      <Show when={suggested()}>
                        <Badge variant="success" size="sm">Suggested match</Badge>
                      </Show>
                      <Show when={isReadopt() && details().matched.length > 0}>
                        <span class="text-xs text-fg-muted">
                          matched: {details().matched.join(", ")}
                        </span>
                      </Show>
                    </div>
                    <p class="mt-0.5 text-xs text-fg-tertiary">
                      {vpn.endpoint}
                      <Show when={vpn.local_cidr}>
                        <span class="text-fg-muted">{" "}&mdash; {vpn.local_cidr}</span>
                      </Show>
                    </p>
                    <div class="mt-1.5 flex flex-wrap gap-1.5">
                      <Badge variant={vpn.type === "vpn_client" ? "success" : "muted"} size="sm">
                        {vpn.type === "vpn_client" ? "VPN Client" : "Tunnel"}
                      </Badge>
                      <Show when={vpn.wg_device}>
                        <Badge variant="info" size="sm">{vpn.wg_device}</Badge>
                      </Show>
                      <Show when={vpn.wg_iface}>
                        <Badge variant="default" size="sm">
                          {vpn.wg_iface}{vpn.iface_desc ? ` (${vpn.iface_desc})` : ""}
                        </Badge>
                      </Show>
                      <Show when={vpn.gateway_name}>
                        <Badge variant="warning" size="sm">{vpn.gateway_name}</Badge>
                      </Show>
                      <Show when={(vpn.filter_uuids?.length ?? 0) > 0}>
                        <Badge variant="success" size="sm">{vpn.filter_uuids!.length} rule(s)</Badge>
                      </Show>
                      <Show when={(vpn.snat_uuids?.length ?? 0) > 0}>
                        <Badge variant="error" size="sm">{vpn.snat_uuids!.length} NAT</Badge>
                      </Show>
                    </div>
                  </div>
                  <Show when={isReadopt()} fallback={
                    <Button
                      variant="primary"
                      size="sm"
                      onClick={() => startImport(vpn)}
                      disabled={importing() !== null}
                    >
                      Import
                    </Button>
                  }>
                    <Show when={confirmTarget()?.server_uuid !== vpn.server_uuid}>
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => setConfirmTarget(vpn)}
                        disabled={importing() !== null}
                      >
                        Link to {props.readoptName}
                      </Button>
                    </Show>
                  </Show>
                </div>

                {/* Re-adopt confirmation (shown when this VPN is selected for re-adopt) */}
                <Show when={isReadopt() && confirmTarget()?.server_uuid === vpn.server_uuid}>
                  <div class="mt-3 border-t border-line pt-3">
                    <p class="text-xs text-fg-secondary">
                      Re-link <strong>{props.readoptName}</strong> to this OPNsense entry? This will update all stored UUIDs.
                    </p>
                    <Show when={importError() && failedUUID() === vpn.server_uuid}>
                      <p class="mt-1 text-xs text-error">{importError()}</p>
                    </Show>
                    <div class="mt-2 flex justify-end gap-2">
                      <Button variant="secondary" size="sm" onClick={() => setConfirmTarget(null)}>
                        Cancel
                      </Button>
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => void doReadopt(vpn)}
                        disabled={importing() !== null}
                        loading={importing() === vpn.server_uuid}
                      >
                        Confirm re-adopt
                      </Button>
                    </div>
                  </div>
                </Show>

                {/* Re-adopt error (scoped, shown only when NOT in confirmation panel) */}
                <Show when={isReadopt() && importError() && failedUUID() === vpn.server_uuid && confirmTarget()?.server_uuid !== vpn.server_uuid}>
                  <p class="mt-2 text-xs text-error">{importError()}</p>
                </Show>

                {/* Import name input (shown when this VPN is the import target, not in readopt mode) */}
                <Show when={!isReadopt() && importTarget()?.server_uuid === vpn.server_uuid}>
                  <div class="mt-3 border-t border-line pt-3">
                    <label class="text-xs font-medium text-fg-secondary">
                      Name for this VPN profile
                    </label>
                    <input
                      type="text"
                      value={importName()}
                      onInput={(e) => setImportName(e.currentTarget.value)}
                      class="mt-1 w-full rounded-lg border border-line bg-surface-secondary px-3 py-2 text-sm text-fg placeholder-fg-muted focus:border-accent focus:outline-none"
                      placeholder="e.g. Mullvad NL"
                    />
                    <Show when={importError()}>
                      <p class="mt-1 text-xs text-error">{importError()}</p>
                    </Show>
                    <div class="mt-2 flex justify-end gap-2">
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => setImportTarget(null)}
                      >
                        Cancel
                      </Button>
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => void doImport()}
                        disabled={importing() !== null}
                        loading={importing() !== null}
                      >
                        Confirm import
                      </Button>
                    </div>
                  </div>
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
          disabled={importing() !== null}
        >
          Close
        </Button>
      </div>
    </Modal>
  );
}
