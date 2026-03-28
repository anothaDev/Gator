import { createSignal, onMount, Show } from "solid-js";
import { apiGet, apiPost } from "../../lib/api";
import type { TunnelForm, SubnetSuggestion } from "./types";
import { emptyForm } from "./types";
import Card from "../../components/Card";
import Button from "../../components/Button";
import AlertBanner from "../../components/AlertBanner";

// ─── CSS ─────────────────────────────────────────────────────────

const inputClass =
  "w-full rounded-lg border border-line bg-surface-secondary px-3 py-2.5 text-sm text-fg placeholder-fg-muted focus:border-accent focus:outline-none";
const labelClass = "block text-xs font-medium text-fg-secondary mb-1.5";

// ─── Create Tunnel Form ─────────────────────────────────────────

function CreateTunnelForm(props: {
  onCreated: () => void;
  onCancel: () => void;
}) {
  const [form, setForm] = createSignal<TunnelForm>({ ...emptyForm });
  const [creating, setCreating] = createSignal(false);
  const [createError, setCreateError] = createSignal("");
  const [sshTestResult, setSSHTestResult] = createSignal<{ success: boolean; info?: Record<string, string>; error?: string } | null>(null);
  const [sshTesting, setSSHTesting] = createSignal(false);

  // Load subnet suggestion on mount.
  onMount(() => {
    void loadSubnetSuggestion();
  });

  const loadSubnetSuggestion = async () => {
    try {
      const { ok, data } = await apiGet<SubnetSuggestion>("/api/tunnels/next-subnet");
      if (ok) {
        // Only apply suggestion if the user hasn't manually entered values.
        setForm((f) => ({
          ...f,
          tunnel_subnet: f.tunnel_subnet || data.tunnel_subnet,
          firewall_ip: f.firewall_ip || data.firewall_ip,
          remote_ip: f.remote_ip || data.remote_ip,
        }));
      }
    } catch {}
  };

  const testSSH = async () => {
    const f = form();
    if (!f.remote_host) return;
    setSSHTesting(true);
    setSSHTestResult(null);
    try {
      const { data } = await apiPost<{ success: boolean; info?: Record<string, string>; error?: string }>(
        "/api/tunnels/test-ssh",
        {
          host: f.remote_host,
          port: f.ssh_port,
          user: f.ssh_user,
          private_key: f.ssh_private_key,
          password: f.ssh_password,
        },
      );
      setSSHTestResult(data);
    } catch {
      setSSHTestResult({ success: false, error: "Request failed" });
    } finally {
      setSSHTesting(false);
    }
  };

  const createTunnel = async () => {
    const f = form();
    if (!f.name || !f.remote_host) return;
    setCreating(true);
    setCreateError("");
    try {
      const { ok, data } = await apiPost("/api/tunnels", f);
      if (!ok) {
        setCreateError((data as { error?: string }).error ?? "Failed to create tunnel");
        return;
      }
      props.onCreated();
    } catch {
      setCreateError("Failed to create tunnel");
    } finally {
      setCreating(false);
    }
  };

  return (
    <Card variant="elevated">
      <h2 class="text-lg font-semibold text-fg">New Tunnel</h2>
      <p class="mb-5 mt-1 text-sm text-fg-tertiary">
        Configure a WireGuard tunnel to a remote Linux VPS.
      </p>

      <div class="space-y-5">
        {/* Name + Description */}
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class={labelClass}>Tunnel Name</label>
            <input
              type="text"
              class={inputClass}
              placeholder="hetzner-fsn1"
              value={form().name}
              onInput={(e) => setForm((f) => ({ ...f, name: e.currentTarget.value }))}
            />
          </div>
          <div>
            <label class={labelClass}>Description (optional)</label>
            <input
              type="text"
              class={inputClass}
              placeholder="Primary VPS"
              value={form().description}
              onInput={(e) => setForm((f) => ({ ...f, description: e.currentTarget.value }))}
            />
          </div>
        </div>

        {/* SSH connection */}
        <div>
          <p class="mb-3 text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
            SSH Connection
          </p>
          <div class="grid gap-4 md:grid-cols-3">
            <div>
              <label class={labelClass}>Host / IP</label>
              <input
                type="text"
                class={inputClass}
                placeholder="203.0.113.10"
                value={form().remote_host}
                onInput={(e) => setForm((f) => ({ ...f, remote_host: e.currentTarget.value }))}
              />
            </div>
            <div>
              <label class={labelClass}>SSH Port</label>
              <input
                type="number"
                class={inputClass}
                value={form().ssh_port}
                onInput={(e) => { const v = parseInt(e.currentTarget.value); setForm((f) => ({ ...f, ssh_port: isNaN(v) ? 0 : v })); }}
              />
            </div>
            <div>
              <label class={labelClass}>SSH User</label>
              <input
                type="text"
                class={inputClass}
                value={form().ssh_user}
                onInput={(e) => setForm((f) => ({ ...f, ssh_user: e.currentTarget.value }))}
              />
            </div>
          </div>
          <div class="mt-4">
            <label class={labelClass}>SSH Private Key</label>
            <textarea
              class={inputClass + " h-28 font-mono text-xs"}
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;...&#10;-----END OPENSSH PRIVATE KEY-----"
              value={form().ssh_private_key}
              onInput={(e) => setForm((f) => ({ ...f, ssh_private_key: e.currentTarget.value }))}
            />
            <p class="mt-1 text-xs text-fg-tertiary">
              Paste the full PEM key, or leave empty and use password below.
            </p>
          </div>
          <div class="mt-3">
            <label class={labelClass}>SSH Password (fallback)</label>
            <input
              type="password"
              class={inputClass}
              placeholder="Only if no SSH key"
              value={form().ssh_password}
              onInput={(e) => setForm((f) => ({ ...f, ssh_password: e.currentTarget.value }))}
            />
          </div>

          {/* SSH Test */}
          <div class="mt-3 flex items-center gap-3">
            <Button
              variant="secondary"
              size="md"
              onClick={() => void testSSH()}
              disabled={sshTesting() || !form().remote_host}
              loading={sshTesting()}
            >
              Test SSH Connection
            </Button>
            <Show when={sshTestResult()}>
              {(result) => (
                <span class={`text-sm ${result().success ? "text-success" : "text-error"}`}>
                  {result().success
                    ? `Connected — ${result().info?.hostname ?? ""} (${result().info?.os ?? "Linux"})`
                    : `Failed: ${result().error}`}
                </span>
              )}
            </Show>
          </div>
        </div>

        {/* Tunnel addressing */}
        <div>
          <p class="mb-3 text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
            Tunnel Addressing
          </p>
          <div class="grid gap-4 md:grid-cols-3">
            <div>
              <label class={labelClass}>Tunnel Subnet</label>
              <input
                type="text"
                class={inputClass}
                placeholder="10.200.200.0/24"
                value={form().tunnel_subnet}
                onInput={(e) => setForm((f) => ({ ...f, tunnel_subnet: e.currentTarget.value }))}
              />
            </div>
            <div>
              <label class={labelClass}>Firewall Tunnel IP</label>
              <input
                type="text"
                class={inputClass}
                placeholder="10.200.200.2"
                value={form().firewall_ip}
                onInput={(e) => setForm((f) => ({ ...f, firewall_ip: e.currentTarget.value }))}
              />
            </div>
            <div>
              <label class={labelClass}>Remote Tunnel IP</label>
              <input
                type="text"
                class={inputClass}
                placeholder="10.200.200.1"
                value={form().remote_ip}
                onInput={(e) => setForm((f) => ({ ...f, remote_ip: e.currentTarget.value }))}
              />
            </div>
          </div>
          <div class="mt-4 grid gap-4 md:grid-cols-2">
            <div>
              <label class={labelClass}>WireGuard Listen Port (remote)</label>
              <input
                type="number"
                class={inputClass}
                value={form().listen_port}
                onInput={(e) => setForm((f) => ({ ...f, listen_port: parseInt(e.currentTarget.value) || 51820 }))}
              />
            </div>
            <div>
              <label class={labelClass}>Keepalive (seconds)</label>
              <input
                type="number"
                class={inputClass}
                value={form().keepalive}
                onInput={(e) => setForm((f) => ({ ...f, keepalive: parseInt(e.currentTarget.value) || 25 }))}
              />
            </div>
          </div>
        </div>

        {/* Error + Actions */}
        <Show when={createError()}>
          <AlertBanner tone="error">{createError()}</AlertBanner>
        </Show>

        <div class="flex items-center gap-3 border-t border-line pt-4">
          <Button
            variant="primary"
            size="md"
            onClick={() => void createTunnel()}
            disabled={creating() || !form().name || !form().remote_host}
            loading={creating()}
          >
            Create Tunnel
          </Button>
          <Button
            variant="secondary"
            size="md"
            onClick={props.onCancel}
          >
            Cancel
          </Button>
        </div>
      </div>
    </Card>
  );
}

export default CreateTunnelForm;
