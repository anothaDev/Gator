import { createSignal, For, Show, onMount } from "solid-js";
import { apiGet, apiPost } from "../../lib/api";
import Modal from "../../components/Modal";
import Button from "../../components/Button";
import AlertBanner from "../../components/AlertBanner";
import Badge from "../../components/Badge";

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

export default function DiscoveryModal(props: { onClose: () => void; onImported: () => void }) {
  const [scanning, setScanning] = createSignal(true);
  const [scanError, setScanError] = createSignal("");
  const [discovered, setDiscovered] = createSignal<DiscoveredVPN[]>([]);
  const [importing, setImporting] = createSignal<string | null>(null);
  const [importError, setImportError] = createSignal("");
  const [importName, setImportName] = createSignal("");
  const [importTarget, setImportTarget] = createSignal<DiscoveredVPN | null>(null);

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
      const { ok, data } = await apiPost("/api/opnsense/vpn/import", {
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
      <h2 class="text-[var(--text-lg)] font-semibold text-[var(--text-primary)]">Scan OPNsense</h2>
      <p class="mt-1 text-[var(--text-xs)] text-[var(--text-tertiary)]">
        Discover existing WireGuard VPN setups and import them into Gator.
      </p>

      <Show when={scanning()}>
        <div class="mt-4 flex items-center gap-3 text-[var(--text-sm)] text-[var(--text-tertiary)]">
          <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          Scanning WireGuard, gateways, rules...
        </div>
      </Show>

      <Show when={scanError()}>
        <div class="mt-4">
          <AlertBanner tone="error">{scanError()}</AlertBanner>
        </div>
      </Show>

      <Show when={!scanning() && !scanError() && discovered().length === 0}>
        <div class="mt-4 rounded-lg border border-[var(--border-strong)] bg-[var(--bg-tertiary)] px-3 py-3 text-[var(--text-sm)] text-[var(--text-secondary)]">
          No WireGuard VPN setups found on OPNsense. Make sure WireGuard is configured with at least one server and peer.
        </div>
      </Show>

      <Show when={!scanning() && discovered().length > 0}>
        <div class="mt-4 space-y-3 max-h-80 overflow-y-auto">
          <For each={discovered()}>
            {(vpn) => (
              <div class="rounded-lg border border-[var(--border-strong)] bg-[var(--bg-tertiary)] p-3">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0 flex-1">
                    <p class="font-medium text-[var(--text-primary)]">{vpn.server_name || vpn.peer_name || "Unknown"}</p>
                    <p class="mt-0.5 text-[var(--text-xs)] text-[var(--text-tertiary)]">
                      {vpn.endpoint}
                      <Show when={vpn.local_cidr}>
                        <span class="text-[var(--text-muted)]">{" "}&mdash; {vpn.local_cidr}</span>
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
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => startImport(vpn)}
                    disabled={importing() !== null}
                  >
                    Import
                  </Button>
                </div>

                {/* Import name input (shown when this VPN is the import target) */}
                <Show when={importTarget()?.server_uuid === vpn.server_uuid}>
                  <div class="mt-3 border-t border-[var(--border-default)] pt-3">
                    <label class="text-[var(--text-xs)] font-medium text-[var(--text-secondary)]">
                      Name for this VPN profile
                    </label>
                    <input
                      type="text"
                      value={importName()}
                      onInput={(e) => setImportName(e.currentTarget.value)}
                      class="mt-1 w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2 text-[var(--text-sm)] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
                      placeholder="e.g. Mullvad NL"
                    />
                    <Show when={importError()}>
                      <p class="mt-1 text-[var(--text-xs)] text-[var(--status-error)]">{importError()}</p>
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
            )}
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
