import { createSignal, Show, For, onCleanup } from "solid-js";
import { apiGet, apiPost, apiPut, apiDelete } from "../../lib/api";
import CrossCheckRow from "./CrossCheckRow";
import type { TunnelStatus, TunnelDetail } from "./types";
import Card from "../../components/Card";
import Button from "../../components/Button";
import Badge from "../../components/Badge";
import AlertBanner from "../../components/AlertBanner";

// ─── Render helpers ──────────────────────────────────────────────

const inputClass =
  "w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-secondary)] px-3 py-2.5 text-[var(--text-sm)] text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none";
const labelClass = "block text-[var(--text-xs)] font-medium text-[var(--text-secondary)] mb-1.5";

function getStatusBadge(t: TunnelStatus) {
  if (t.status === "deployed" && t.remote_reachable)
    return <Badge variant="success" size="sm">Connected</Badge>;
  if (t.status === "deployed")
    return <Badge variant="warning" size="sm">Deployed</Badge>;
  if (t.status === "error")
    return <Badge variant="error" size="sm">Error</Badge>;
  return <Badge variant="muted" size="sm">Pending</Badge>;
}

// ─── Tunnel Actions Dropdown ─────────────────────────────────────

interface ActionItem {
  label: string;
  variant: "default" | "danger";
  onClick: () => void;
  disabled?: boolean;
  loading?: boolean;
}

function TunnelActions(props: {
  deployed: boolean;
  busy: boolean;
  crossChecking: boolean;
  onDeploy: (mode: "full" | "setup-remote") => void;
  onRestart: () => void;
  onLockSSH: () => void;
  onTeardown: () => void;
  onCrossCheck: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const [open, setOpen] = createSignal(false);
  let menuRef: HTMLDivElement | undefined;

  // Close on outside click.
  const handleClickOutside = (e: MouseEvent) => {
    if (menuRef && !menuRef.contains(e.target as Node)) setOpen(false);
  };
  if (typeof document !== "undefined") {
    document.addEventListener("mousedown", handleClickOutside);
    onCleanup(() => document.removeEventListener("mousedown", handleClickOutside));
  }

  const actions = (): ActionItem[] => {
    const items: ActionItem[] = [];
    if (!props.deployed) {
      items.push({ label: "Deploy", variant: "default", onClick: () => props.onDeploy("full") });
    } else {
      items.push({ label: "Setup Remote", variant: "default", onClick: () => props.onDeploy("setup-remote") });
      items.push({ label: "Restart", variant: "default", onClick: props.onRestart, disabled: props.busy });
      items.push({ label: "Lock SSH", variant: "default", onClick: props.onLockSSH, disabled: props.busy });
      items.push({ label: "Teardown", variant: "danger", onClick: props.onTeardown, disabled: props.busy });
    }
    items.push({ label: "Cross-check", variant: "default", onClick: props.onCrossCheck, disabled: props.crossChecking, loading: props.crossChecking });
    items.push({ label: "Edit", variant: "default", onClick: props.onEdit, disabled: props.busy });
    items.push({ label: "Delete", variant: "danger", onClick: props.onDelete, disabled: props.busy });
    return items;
  };

  return (
    <div class="relative" ref={menuRef}>
      <Button variant="secondary" size="sm" onClick={() => setOpen((v) => !v)}>
        <span>Actions</span>
        <svg class="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor">
          <path
            fill-rule="evenodd"
            d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z"
            clip-rule="evenodd"
          />
        </svg>
      </Button>
      <Show when={open()}>
        <div class="absolute right-0 top-full z-50 mt-1 min-w-[160px] overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)] py-1 shadow-xl shadow-black/40">
          <For each={actions()}>
            {(item) => (
              <button
                type="button"
                disabled={item.disabled}
                onClick={() => {
                  setOpen(false);
                  item.onClick();
                }}
                class={[
                  "flex w-full items-center gap-2 px-3 py-2 text-left text-[var(--text-sm)] font-medium transition-colors",
                  item.variant === "danger"
                    ? "text-[var(--status-error)] hover:bg-[var(--status-error)]/10"
                    : "text-[var(--text-primary)] hover:bg-[var(--bg-hover)]",
                  "disabled:opacity-40 disabled:cursor-not-allowed",
                ].join(" ")}
              >
                <Show when={item.loading}>
                  <svg class="h-3.5 w-3.5 animate-spin" viewBox="0 0 24 24" fill="none">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" />
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                  </svg>
                </Show>
                {item.label}
              </button>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}

// ─── Tunnel Card ─────────────────────────────────────────────────

function TunnelCard(props: {
  tunnel: TunnelStatus;
  onDeploy: (mode: "full" | "setup-remote") => void;
  onUpdated: () => void;
  onMessage: (msg: string) => void;
  onError: (err: string) => void;
}) {
  const t = props.tunnel;

  // Action busy state (delete, restart, teardown, lockdown).
  const [busy, setBusy] = createSignal(false);

  // Cross-check state (owned by this card).
  const [crossChecking, setCrossChecking] = createSignal(false);
  const [crossCheckResult, setCrossCheckResult] = createSignal<Record<string, unknown> | null>(null);

  // Edit state (owned by this card).
  const [editing, setEditing] = createSignal(false);
  const [editData, setEditData] = createSignal<Record<string, unknown>>({});
  const [editSaving, setEditSaving] = createSignal(false);
  const [editErr, setEditErr] = createSignal("");

  // ─── Action helpers ──────────────────────────────────────────

  const deleteTunnel = async () => {
    props.onMessage("");
    props.onError("");
    setBusy(true);
    try {
      const { ok, data } = await apiDelete(`/api/tunnels/${t.id}`);
      if (ok) {
        props.onMessage("Tunnel deleted.");
        props.onUpdated();
      } else {
        props.onError((data as { error?: string }).error ?? "Failed to delete");
      }
    } catch {
      props.onError("Failed to delete tunnel");
    } finally {
      setBusy(false);
    }
  };

  const teardownTunnel = async () => {
    props.onMessage("");
    props.onError("");
    setBusy(true);
    try {
      const { ok, data } = await apiPost(`/api/tunnels/${t.id}/teardown`);
      if (ok) {
        props.onMessage("Tunnel torn down.");
        props.onUpdated();
      } else {
        props.onError((data as { error?: string }).error ?? "Failed to teardown");
      }
    } catch {
      props.onError("Failed to teardown");
    } finally {
      setBusy(false);
    }
  };

  const restartTunnel = async () => {
    props.onMessage("");
    props.onError("");
    setBusy(true);
    try {
      const { ok, data } = await apiPost(`/api/tunnels/${t.id}/restart`);
      if (ok) {
        props.onMessage("Tunnel restarted.");
        props.onUpdated();
      } else {
        props.onError((data as { error?: string }).error ?? "Failed to restart");
      }
    } catch {
      props.onError("Failed to restart");
    } finally {
      setBusy(false);
    }
  };

  const lockdownSSH = async () => {
    props.onMessage("");
    props.onError("");
    setBusy(true);
    try {
      const { ok, data } = await apiPost(`/api/tunnels/${t.id}/lockdown-ssh`);
      if (ok) {
        props.onMessage("SSH locked down to tunnel subnet.");
      } else {
        props.onError((data as { error?: string }).error ?? "Failed to lock down SSH");
      }
    } catch {
      props.onError("Failed to lock down SSH");
    } finally {
      setBusy(false);
    }
  };

  // ─── Cross-check ────────────────────────────────────────────

  const runCrossCheck = async () => {
    setCrossChecking(true);
    setCrossCheckResult(null);
    try {
      const { ok, data } = await apiPost<{ cross_check: Record<string, unknown> }>(`/api/tunnels/${t.id}/cross-check`);
      if (ok) {
        setCrossCheckResult(data.cross_check);
      } else {
        setCrossCheckResult({ ssh_ok: false, ssh_error: (data as unknown as { error?: string }).error ?? "Cross-check failed" });
      }
    } catch {
      setCrossCheckResult({ ssh_ok: false, ssh_error: "Request failed" });
    } finally {
      setCrossChecking(false);
    }
  };

  // ─── Edit ────────────────────────────────────────────────────

  const startEdit = async () => {
    const { ok, data } = await apiGet<TunnelDetail>(`/api/tunnels/${t.id}`);
    if (!ok) return;
    setEditData({
      name: data.name,
      description: data.description,
      remote_host: data.remote_host,
      ssh_port: data.ssh_port,
      ssh_user: data.ssh_user,
      ssh_private_key: "",
      ssh_password: "",
    });
    setEditErr("");
    setEditing(true);
  };

  const saveEdit = async () => {
    setEditSaving(true);
    setEditErr("");
    try {
      const { ok, data } = await apiPut(`/api/tunnels/${t.id}`, editData());
      if (ok) {
        setEditing(false);
        props.onMessage("Tunnel updated.");
        props.onUpdated();
      } else {
        setEditErr((data as { error?: string }).error ?? "Failed to save");
      }
    } catch {
      setEditErr("Failed to save tunnel");
    } finally {
      setEditSaving(false);
    }
  };

  return (
    <Card variant="elevated">
      <div class="flex items-start justify-between">
        <div>
          <div class="flex items-center gap-3">
            <h3 class="text-[var(--text-lg)] font-semibold text-[var(--text-primary)]">{t.name}</h3>
            {getStatusBadge(t)}
          </div>
          <Show when={t.description}>
            <p class="mt-1 text-[var(--text-sm)] text-[var(--text-tertiary)]">{t.description}</p>
          </Show>
        </div>

        <TunnelActions
          deployed={t.deployed}
          busy={busy()}
          crossChecking={crossChecking()}
          onDeploy={(mode) => props.onDeploy(mode)}
          onRestart={() => void restartTunnel()}
          onLockSSH={() => void lockdownSSH()}
          onTeardown={() => void teardownTunnel()}
          onCrossCheck={() => void runCrossCheck()}
          onEdit={() => void startEdit()}
          onDelete={() => void deleteTunnel()}
        />
      </div>

      {/* Connection details */}
      <div class="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 text-[var(--text-sm)] md:grid-cols-4">
        <div>
          <span class="text-[var(--text-tertiary)]">Remote</span>
          <p class="font-mono text-[var(--text-primary)]">{t.remote_host}</p>
        </div>
        <div>
          <span class="text-[var(--text-tertiary)]">Tunnel</span>
          <p class="font-mono text-[var(--text-primary)]">{t.tunnel_subnet || "-"}</p>
        </div>
        <div>
          <span class="text-[var(--text-tertiary)]">Firewall IP</span>
          <p class="font-mono text-[var(--text-primary)]">{t.firewall_ip || "-"}</p>
        </div>
        <div>
          <span class="text-[var(--text-tertiary)]">Remote IP</span>
          <p class="font-mono text-[var(--text-primary)]">{t.remote_ip || "-"}</p>
        </div>
      </div>

      {/* Live status (when deployed) */}
      <Show when={t.deployed}>
        <div class="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 border-t border-[var(--border-default)] pt-3 text-[var(--text-sm)] md:grid-cols-4">
          <div>
            <span class="text-[var(--text-tertiary)]">Interface</span>
            <p class="font-mono text-[var(--text-primary)]">{t.remote_wg_interface}</p>
          </div>
          <div>
            <span class="text-[var(--text-tertiary)]">Handshake</span>
            <p class="text-[var(--text-primary)]">{t.handshake || "none"}</p>
          </div>
          <div>
            <span class="text-[var(--text-tertiary)]">Received</span>
            <p class="text-[var(--text-primary)]">{t.transfer_rx || "-"}</p>
          </div>
          <div>
            <span class="text-[var(--text-tertiary)]">Sent</span>
            <p class="text-[var(--text-primary)]">{t.transfer_tx || "-"}</p>
          </div>
        </div>
      </Show>

      {/* Cross-check results */}
      <Show when={crossCheckResult()}>
        <div class="mt-3 border-t border-[var(--border-default)] pt-3">
          <div class="flex items-center justify-between">
            <p class="text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">Cross-check results</p>
            <Button variant="ghost" size="sm" onClick={() => setCrossCheckResult(null)}>
              Dismiss
            </Button>
          </div>
          <div class="mt-2 space-y-1 text-[var(--text-sm)]">
            <CrossCheckRow label="SSH connection" ok={crossCheckResult()!.ssh_ok as boolean} detail={crossCheckResult()!.ssh_error as string} />
            <Show when={crossCheckResult()!.hostname}>
              <p class="text-[var(--text-secondary)]">Host: {crossCheckResult()!.hostname as string} ({crossCheckResult()!.os as string})</p>
            </Show>
            <Show when={crossCheckResult()!.ssh_ok}>
              <CrossCheckRow label="WireGuard installed" ok={crossCheckResult()!.wg_installed as boolean} />
              <CrossCheckRow label="WireGuard configured" ok={crossCheckResult()!.wg_configured as boolean} />
              <Show when={crossCheckResult()!.matched_interface}>
                <CrossCheckRow
                  label={"Config match (" + (crossCheckResult()!.matched_interface as string) + ")"}
                  ok={crossCheckResult()!.config_matches_firewall as boolean}
                />
              </Show>
              <Show when={typeof crossCheckResult()!.address_matches === "boolean"}>
                <CrossCheckRow label="Tunnel address matches" ok={crossCheckResult()!.address_matches as boolean} />
              </Show>
              <Show when={typeof crossCheckResult()!.pubkey_matches === "boolean"}>
                <CrossCheckRow label="Public key matches" ok={crossCheckResult()!.pubkey_matches as boolean} />
              </Show>
              <Show when={typeof crossCheckResult()!.handshake_active === "boolean"}>
                <CrossCheckRow label="Active handshake" ok={crossCheckResult()!.handshake_active as boolean} />
              </Show>
            </Show>
          </div>
        </div>
      </Show>

      {/* Inline edit form */}
      <Show when={editing()}>
        <div class="mt-4 border-t border-[var(--border-default)] pt-4">
          <p class="mb-3 text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">Edit Tunnel</p>
          <Show when={editErr()}>
            <div class="mb-3">
              <AlertBanner tone="error">{editErr()}</AlertBanner>
            </div>
          </Show>
          <div class="grid gap-3 md:grid-cols-2">
            <div>
              <label class={labelClass}>Name</label>
              <input
                type="text"
                class={inputClass}
                value={(editData().name as string) ?? ""}
                onInput={(e) => setEditData((d) => ({ ...d, name: e.currentTarget.value }))}
              />
            </div>
            <div>
              <label class={labelClass}>Description</label>
              <input
                type="text"
                class={inputClass}
                value={(editData().description as string) ?? ""}
                onInput={(e) => setEditData((d) => ({ ...d, description: e.currentTarget.value }))}
              />
            </div>
          </div>
          <div class="mt-3 grid gap-3 md:grid-cols-3">
            <div>
              <label class={labelClass}>SSH Host</label>
              <input
                type="text"
                class={inputClass}
                value={(editData().remote_host as string) ?? ""}
                onInput={(e) => setEditData((d) => ({ ...d, remote_host: e.currentTarget.value }))}
              />
            </div>
            <div>
              <label class={labelClass}>SSH Port</label>
              <input
                type="number"
                class={inputClass}
                value={(editData().ssh_port as number) ?? 22}
                onInput={(e) => { const v = parseInt(e.currentTarget.value); setEditData((d) => ({ ...d, ssh_port: isNaN(v) ? 0 : v })); }}
              />
            </div>
            <div>
              <label class={labelClass}>SSH User</label>
              <input
                type="text"
                class={inputClass}
                value={(editData().ssh_user as string) ?? "root"}
                onInput={(e) => setEditData((d) => ({ ...d, ssh_user: e.currentTarget.value }))}
              />
            </div>
          </div>
          <div class="mt-3">
            <label class={labelClass}>SSH Private Key <span class="text-[var(--text-muted)]">(leave empty to keep current)</span></label>
            <textarea
              class={inputClass + " h-20 font-mono text-[var(--text-xs)]"}
              placeholder="Paste new key to replace, or leave empty"
              value={(editData().ssh_private_key as string) ?? ""}
              onInput={(e) => setEditData((d) => ({ ...d, ssh_private_key: e.currentTarget.value }))}
            />
          </div>
          <div class="mt-3">
            <label class={labelClass}>SSH Password <span class="text-[var(--text-muted)]">(leave empty to keep current)</span></label>
            <input
              type="password"
              class={inputClass}
              placeholder="Leave empty to keep current"
              value={(editData().ssh_password as string) ?? ""}
              onInput={(e) => setEditData((d) => ({ ...d, ssh_password: e.currentTarget.value }))}
            />
          </div>
          <div class="mt-4 flex gap-3">
            <Button
              variant="primary"
              size="md"
              onClick={() => void saveEdit()}
              disabled={editSaving()}
              loading={editSaving()}
            >
              Save
            </Button>
            <Button
              variant="secondary"
              size="md"
              onClick={() => setEditing(false)}
            >
              Cancel
            </Button>
          </div>
        </div>
      </Show>
    </Card>
  );
}

export default TunnelCard;
