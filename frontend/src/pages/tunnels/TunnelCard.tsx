import { createSignal, Show, For, onCleanup } from "solid-js";
import { apiGet, apiPost, apiPut, apiDelete } from "../../lib/api";
import CrossCheckRow from "./CrossCheckRow";
import type { TunnelStatus, TunnelDetail } from "./types";
import Card from "../../components/Card";
import Button from "../../components/Button";
import Badge from "../../components/Badge";
import AlertBanner from "../../components/AlertBanner";
import Spinner from "../../components/Spinner";

// ─── Render helpers ──────────────────────────────────────────────

const inputClass =
  "w-full rounded-lg border border-border bg-surface px-3 py-2.5 text-body-sm text-fg placeholder-fg-muted hover:border-border-strong focus:border-brand focus:ring-2 focus:ring-brand/20 focus:outline-none";
const labelClass = "block text-label-sm text-fg-secondary mb-1.5";

function getStatusBadge(t: TunnelStatus) {
  const os = t.ownership_status ?? "local_only";
  if (os === "needs_reimport")
    return <Badge variant="error" size="sm">Import needed</Badge>;
  if (os === "managed_drifted")
    return <Badge variant="warning" size="sm">Drifted</Badge>;
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
  title?: string;
}

function TunnelActions(props: {
  deployed: boolean;
  busy: boolean;
  crossChecking: boolean;
  ownershipStatus: string;
  onDeploy: (mode: "full" | "setup-remote") => void;
  onRestart: () => void;
  onLockSSH: () => void;
  onTeardown: () => void;
  onCrossCheck: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onReadopt?: () => void;
  /** When > 0, legacy rules exist — deploy/readopt actions are blocked until migration. */
  legacyRuleCount?: number;
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
    const os = props.ownershipStatus;
    const migrationBlocked = (props.legacyRuleCount ?? 0) > 0;
    const migrateHint = "Migrate legacy firewall rules first";

    // needs_reimport: re-adopt + edit + delete
    if (os === "needs_reimport") {
      if (props.onReadopt) {
        items.push({ label: "Re-adopt from OPNsense", variant: "default", onClick: props.onReadopt, disabled: props.busy || migrationBlocked, title: migrationBlocked ? migrateHint : undefined });
      }
      items.push({ label: "Edit", variant: "default", onClick: props.onEdit, disabled: props.busy });
      items.push({ label: "Delete", variant: "danger", onClick: props.onDelete, disabled: props.busy });
      return items;
    }

    // managed_drifted: re-deploy + re-adopt + edit + delete
    if (os === "managed_drifted") {
      items.push({ label: "Re-deploy", variant: "default", onClick: () => props.onDeploy("full"), disabled: migrationBlocked, title: migrationBlocked ? migrateHint : undefined });
      if (props.onReadopt) {
        items.push({ label: "Re-adopt from OPNsense", variant: "default", onClick: props.onReadopt, disabled: props.busy || migrationBlocked, title: migrationBlocked ? migrateHint : undefined });
      }
      items.push({ label: "Edit", variant: "default", onClick: props.onEdit, disabled: props.busy });
      items.push({ label: "Delete", variant: "danger", onClick: props.onDelete, disabled: props.busy });
      return items;
    }

    if (!props.deployed) {
      items.push({ label: "Deploy", variant: "default", onClick: () => props.onDeploy("full"), disabled: migrationBlocked, title: migrationBlocked ? migrateHint : undefined });
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
        <div class="absolute right-0 top-full z-50 mt-1 min-w-[160px] overflow-hidden rounded-lg border border-border bg-surface-raised py-1 shadow-lg">
          <For each={actions()}>
            {(item) => (
              <button
                type="button"
                disabled={item.disabled}
                title={item.title}
                onClick={() => {
                  setOpen(false);
                  item.onClick();
                }}
                class={[
                  "flex w-full items-center gap-2 px-3 py-2 text-left text-label-md transition-colors",
                  item.variant === "danger"
                    ? "text-error hover:bg-error/10"
                    : "text-fg hover:bg-hover",
                  "disabled:opacity-40 disabled:cursor-not-allowed",
                ].join(" ")}
              >
                <Show when={item.loading}>
                  <Spinner size="xs" />
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
  onReadopt?: () => void;
  /** When > 0, legacy rules exist — deploy/readopt actions are blocked until migration. */
  legacyRuleCount?: number;
}) {
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
      const { ok, data } = await apiDelete(`/api/tunnels/${props.tunnel.id}`);
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
      const { ok, data } = await apiPost(`/api/tunnels/${props.tunnel.id}/teardown`);
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
      const { ok, data } = await apiPost(`/api/tunnels/${props.tunnel.id}/restart`);
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
      const { ok, data } = await apiPost(`/api/tunnels/${props.tunnel.id}/lockdown-ssh`);
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
      const { ok, data } = await apiPost<{ cross_check: Record<string, unknown> }>(`/api/tunnels/${props.tunnel.id}/cross-check`);
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
    const { ok, data } = await apiGet<TunnelDetail>(`/api/tunnels/${props.tunnel.id}`);
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
      const { ok, data } = await apiPut(`/api/tunnels/${props.tunnel.id}`, editData());
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
            <h3 class="text-title-h3 text-fg">{props.tunnel.name}</h3>
            {getStatusBadge(props.tunnel)}
          </div>
          <Show when={props.tunnel.description}>
            <p class="mt-1 text-body-sm text-fg-muted">{props.tunnel.description}</p>
          </Show>
        </div>

        <TunnelActions
          deployed={props.tunnel.deployed}
          busy={busy()}
          crossChecking={crossChecking()}
          ownershipStatus={props.tunnel.ownership_status ?? "local_only"}
          onDeploy={(mode) => props.onDeploy(mode)}
          onRestart={() => void restartTunnel()}
          onLockSSH={() => void lockdownSSH()}
          onTeardown={() => void teardownTunnel()}
          onCrossCheck={() => void runCrossCheck()}
          onEdit={() => void startEdit()}
          onDelete={() => void deleteTunnel()}
          onReadopt={props.onReadopt}
          legacyRuleCount={props.legacyRuleCount}
        />
      </div>

      {/* Drift / reimport warning */}
      <Show when={props.tunnel.ownership_status === "managed_drifted"}>
        <div class="mt-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-body-xs text-amber-200">
          OPNsense state has drifted from Gator config{props.tunnel.drift_reason ? ` (${props.tunnel.drift_reason})` : ""}. Re-deploy to fix.
        </div>
      </Show>
      <Show when={props.tunnel.ownership_status === "needs_reimport"}>
        <div class="mt-3 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-body-xs text-red-300">
          OPNsense resources not found. Re-scan OPNsense or delete this tunnel.
        </div>
      </Show>

      {/* Connection details */}
      <div class="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 md:grid-cols-4">
        <div>
          <span class="text-label-xs text-fg-muted">Remote</span>
          <p class="text-mono-sm text-fg">{props.tunnel.remote_host}</p>
        </div>
        <div>
          <span class="text-label-xs text-fg-muted">Subnet</span>
          <p class="text-mono-sm text-fg">{props.tunnel.tunnel_subnet || "-"}</p>
        </div>
        <div>
          <span class="text-label-xs text-fg-muted">FW IP</span>
          <p class="text-mono-sm text-fg">{props.tunnel.firewall_ip || "-"}</p>
        </div>
        <div>
          <span class="text-label-xs text-fg-muted">Remote IP</span>
          <p class="text-mono-sm text-fg">{props.tunnel.remote_ip || "-"}</p>
        </div>
      </div>

      {/* Live status (when deployed) */}
      <Show when={props.tunnel.deployed}>
        <div class="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 border-t border-border-faint pt-2 md:grid-cols-4">
          <div>
            <span class="text-label-xs text-fg-muted">Iface</span>
            <p class="text-mono-sm text-fg">{props.tunnel.remote_wg_interface}</p>
          </div>
          <div>
            <span class="text-label-xs text-fg-muted">Handshake</span>
            <p class="text-body-sm text-fg">{props.tunnel.handshake || "none"}</p>
          </div>
          <div>
            <span class="text-label-xs text-fg-muted">RX</span>
            <p class="text-body-sm text-fg">{props.tunnel.transfer_rx || "-"}</p>
          </div>
          <div>
            <span class="text-label-xs text-fg-muted">TX</span>
            <p class="text-body-sm text-fg">{props.tunnel.transfer_tx || "-"}</p>
          </div>
        </div>
      </Show>

      {/* Cross-check results */}
      <Show when={crossCheckResult()}>
        <div class="mt-3 border-t border-border-faint pt-3">
          <div class="flex items-center justify-between">
            <p class="text-label-xs font-semibold uppercase tracking-wider text-fg-muted">Cross-check results</p>
            <Button variant="ghost" size="sm" onClick={() => setCrossCheckResult(null)}>
              Dismiss
            </Button>
          </div>
          <div class="mt-2 space-y-1 text-body-sm">
            <CrossCheckRow label="SSH connection" ok={crossCheckResult()!.ssh_ok as boolean} detail={crossCheckResult()!.ssh_error as string} />
            <Show when={crossCheckResult()!.hostname}>
              <p class="text-fg-secondary">Host: {crossCheckResult()!.hostname as string} ({crossCheckResult()!.os as string})</p>
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
        <div class="mt-4 border-t border-border-faint pt-4">
          <p class="mb-3 text-label-xs font-semibold uppercase tracking-wider text-fg-muted">Edit Tunnel</p>
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
            <label class={labelClass}>SSH Private Key <span class="text-fg-muted">(leave empty to keep current)</span></label>
            <textarea
              class={inputClass + " h-20 text-mono-sm"}
              placeholder="Paste new key to replace, or leave empty"
              value={(editData().ssh_private_key as string) ?? ""}
              onInput={(e) => setEditData((d) => ({ ...d, ssh_private_key: e.currentTarget.value }))}
            />
          </div>
          <div class="mt-3">
            <label class={labelClass}>SSH Password <span class="text-fg-muted">(leave empty to keep current)</span></label>
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
