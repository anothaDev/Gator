import { createSignal, Show } from "solid-js";
import { apiPost } from "../../lib/api";
import CrossCheckRow from "./CrossCheckRow";
import type { TunnelDetail, DiscoveredTunnel } from "./types";

// ─── Import Form ─────────────────────────────────────────────────

function TunnelImportForm(props: {
  tunnel: DiscoveredTunnel;
  onImported: () => void;
  onBack: () => void;
}) {
  const endpointHost = props.tunnel.endpoint.includes(":") ? props.tunnel.endpoint.split(":")[0] : props.tunnel.endpoint;
  // Extract remote tunnel IP from remote_cidr (e.g. "10.200.201.1/32" → "10.200.201.1").
  const remoteTunnelIP = props.tunnel.remote_cidr ? props.tunnel.remote_cidr.split("/")[0] : "";

  const [sshHost, setSSHHost] = createSignal(endpointHost);
  const [sshPort, setSSHPort] = createSignal<number | "">(22);
  const [sshUser, setSSHUser] = createSignal("root");
  const [sshKey, setSSHKey] = createSignal("");
  const [sshPassword, setSSHPassword] = createSignal("");
  const [importing, setImporting] = createSignal(false);
  const [importError, setImportError] = createSignal("");
  const [crossCheck, setCrossCheck] = createSignal<Record<string, unknown> | null>(null);
  const [importDone, setImportDone] = createSignal(false);

  const inputClass =
    "w-full rounded-lg border border-line bg-surface-secondary px-3 py-2.5 text-sm text-fg placeholder-fg-muted focus:border-success focus:outline-none focus:ring-1 focus:ring-success";
  const labelClass = "block text-xs font-medium text-fg-tertiary mb-1.5";

  const doImport = async () => {
    setImporting(true);
    setImportError("");
    setCrossCheck(null);
    try {
      const { ok, data } = await apiPost<{
        status: string;
        tunnel: TunnelDetail;
        cross_check?: Record<string, unknown>;
      }>("/api/tunnels/import", {
        name: props.tunnel.peer_name || props.tunnel.server_name,
        server_uuid: props.tunnel.server_uuid,
        peer_uuid: props.tunnel.peer_uuid,
        local_cidr: props.tunnel.local_cidr,
        endpoint: props.tunnel.endpoint,
        listen_port: parseInt(props.tunnel.listen_port) || 51820,
        peer_pubkey: props.tunnel.peer_pubkey,
        ssh_host: sshHost(),
        ssh_port: sshPort() || 22,
        ssh_user: sshUser(),
        ssh_private_key: sshKey(),
        ssh_password: sshPassword(),
      });

      if (!ok) {
        setImportError((data as unknown as { error?: string }).error ?? "Import failed");
        return;
      }

      if (data.cross_check) {
        setCrossCheck(data.cross_check);
      }
      setImportDone(true);
    } catch {
      setImportError("Import failed");
    } finally {
      setImporting(false);
    }
  };

  return (
    <div class="mt-4">
      {/* Tunnel summary */}
      <div class="rounded-lg border border-line-strong bg-surface-secondary/40 p-4">
        <p class="text-sm font-medium text-fg-secondary">Importing: {props.tunnel.peer_name || props.tunnel.server_name}</p>
        <div class="mt-2 grid grid-cols-2 gap-2 text-xs text-fg-tertiary">
          <span>Endpoint: {props.tunnel.endpoint}</span>
          <span>Tunnel IP: {props.tunnel.local_cidr}</span>
          <span>Device: {props.tunnel.wg_device}</span>
          <span>Peer: {props.tunnel.peer_name}</span>
        </div>
      </div>

      <Show when={!importDone()}>
        {/* SSH credentials */}
        <div class="mt-4 space-y-3">
          <p class="text-xs font-semibold uppercase tracking-wide text-fg-tertiary">
            SSH Credentials (for cross-check and management)
          </p>
          <div class="grid gap-3 md:grid-cols-3">
            <div>
              <label class={labelClass}>SSH Host</label>
              <input type="text" class={inputClass} value={sshHost()} onInput={(e) => setSSHHost(e.currentTarget.value)} />
              <Show when={remoteTunnelIP && sshHost() !== remoteTunnelIP}>
                <p class="mt-1 text-xs text-fg-tertiary">
                  Tunnel IP available:{" "}
                  <button
                    type="button"
                    class="font-mono text-amber-400 hover:text-amber-300"
                    onClick={() => setSSHHost(remoteTunnelIP)}
                  >
                    {remoteTunnelIP}
                  </button>
                  {" "}<span class="text-fg-muted">— use if SSH is only reachable via tunnel</span>
                </p>
              </Show>
            </div>
            <div>
              <label class={labelClass}>SSH Port</label>
              <input type="number" class={inputClass} value={sshPort()} onInput={(e) => { const v = parseInt(e.currentTarget.value); setSSHPort(isNaN(v) ? "" : v); }} />
            </div>
            <div>
              <label class={labelClass}>SSH User</label>
              <input type="text" class={inputClass} value={sshUser()} onInput={(e) => setSSHUser(e.currentTarget.value)} />
            </div>
          </div>
          <div>
            <label class={labelClass}>SSH Private Key</label>
            <textarea
              class={inputClass + " h-24 font-mono text-xs"}
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
              value={sshKey()}
              onInput={(e) => setSSHKey(e.currentTarget.value)}
            />
          </div>
          <div>
            <label class={labelClass}>SSH Password (fallback)</label>
            <input type="password" class={inputClass} value={sshPassword()} onInput={(e) => setSSHPassword(e.currentTarget.value)} />
          </div>
          <p class="text-xs text-fg-tertiary">
            SSH access is optional for import but required for ongoing management (status, restart, teardown).
            If provided, Gator will cross-check the remote WireGuard config matches OPNsense.
          </p>
        </div>

        <Show when={importError()}>
          <div class="mt-3 rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">
            {importError()}
          </div>
        </Show>

        <div class="mt-4 flex gap-3">
          <button
            type="button"
            onClick={() => void doImport()}
            disabled={importing()}
            class="rounded-lg bg-amber-500 px-5 py-2.5 text-sm font-semibold text-surface hover:bg-amber-400 disabled:opacity-50"
          >
            {importing() ? "Importing..." : "Import Tunnel"}
          </button>
          <button
            type="button"
            onClick={props.onBack}
            class="rounded-lg border border-line bg-surface-tertiary px-4 py-2.5 text-sm font-medium text-fg-secondary hover:bg-hover"
          >
            Back
          </button>
        </div>
      </Show>

      {/* Cross-check results */}
      <Show when={importDone()}>
        <div class="mt-4 rounded-lg border border-success/30 bg-success/5 p-4">
          <p class="text-sm font-medium text-success">Tunnel imported successfully.</p>
        </div>

        <Show when={crossCheck()}>
          <div class="mt-3 rounded-lg border border-line-strong bg-surface-secondary/40 p-4">
            <p class="mb-2 text-xs font-semibold uppercase tracking-wide text-fg-tertiary">
              Cross-check results
            </p>
            <div class="space-y-1 text-sm">
              <CrossCheckRow label="SSH connection" ok={crossCheck()!.ssh_ok as boolean} detail={crossCheck()!.ssh_error as string} />
              <Show when={crossCheck()!.hostname}>
                <p class="text-fg-tertiary">Host: {crossCheck()!.hostname as string} ({crossCheck()!.os as string})</p>
              </Show>
              <CrossCheckRow label="WireGuard installed" ok={crossCheck()!.wg_installed as boolean} />
              <CrossCheckRow label="WireGuard configured" ok={crossCheck()!.wg_configured as boolean} />
              <Show when={crossCheck()!.matched_interface}>
                <CrossCheckRow
                  label={"Config match (" + (crossCheck()!.matched_interface as string) + ")"}
                  ok={crossCheck()!.config_matches_firewall as boolean}
                />
              </Show>
              <Show when={typeof crossCheck()!.address_matches === "boolean"}>
                <CrossCheckRow label="Tunnel address matches" ok={crossCheck()!.address_matches as boolean} />
              </Show>
              <Show when={typeof crossCheck()!.pubkey_matches === "boolean"}>
                <CrossCheckRow label="Public key matches" ok={crossCheck()!.pubkey_matches as boolean} />
              </Show>
              <Show when={typeof crossCheck()!.handshake_active === "boolean"}>
                <CrossCheckRow label="Active handshake" ok={crossCheck()!.handshake_active as boolean} />
              </Show>
            </div>
          </div>
        </Show>

        <div class="mt-4 flex justify-end">
          <button
            type="button"
            onClick={props.onImported}
            class="rounded-lg bg-accent px-4 py-2 text-[13px] font-semibold text-surface hover:brightness-110"
          >
            Done
          </button>
        </div>
      </Show>
    </div>
  );
}

export default TunnelImportForm;
