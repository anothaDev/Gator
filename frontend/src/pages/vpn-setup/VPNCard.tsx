import { createEffect, createSignal, Show, For, onCleanup } from "solid-js";
import { apiDelete, apiGet, apiPost, apiPut } from "../../lib/api";
import { parseWireGuardConfig as parseWireGuardFields, wireGuardStemFromFile } from "../../lib/wireguard";
import DeployModal from "./DeployModal";
import ConfirmModal from "../../components/ConfirmModal";
import Select from "../../components/Select";

type IPVersion = "ipv4" | "ipv6";

type OwnershipStatus = "local_only" | "managed_pending" | "managed_verified" | "managed_drifted" | "needs_reimport";

type VPNStatus = {
  id: number;
  name: string;
  protocol: "wireguard" | "openvpn";
  ip_version: IPVersion;
  local_cidr?: string;
  remote_cidr?: string;
  endpoint?: string;
  enabled: boolean;
  has_private_key: boolean;
  has_peer_public_key: boolean;
  has_pre_shared_key: boolean;
  applied: boolean;
  routing_applied: boolean;
  gateway_online: boolean;
  gateway_applied: boolean;
  nat_applied: boolean;
  policy_applied: boolean;
  source_interfaces?: string[];
  wg_interface?: string;
  wg_device?: string;
  interface_assigned: boolean;
  gateway_name?: string;
  last_applied_at?: string;
  ownership_status?: OwnershipStatus;
  drift_reason?: string;
  last_verified_at?: string;
};

type SelectableInterface = {
  identifier: string;
  device: string;
  description: string;
};

type VPNDetail = VPNStatus & {
  dns?: string;
};

type VPNForm = {
  name: string;
  protocol: "wireguard" | "openvpn";
  ipVersion: IPVersion;
  localCIDR: string;
  remoteCIDR: string;
  endpoint: string;
  dns: string;
  privateKey: string;
  peerPublicKey: string;
  preSharedKey: string;
  // Stash both parsed addresses so switching ip_version can re-pick.
  _parsedAddresses?: string;
  _parsedAllowedIPs?: string;
  _parsedDNS?: string;
};

type VPNListResponse = { vpns: VPNStatus[] };

type VPNStateKey = "draft" | "tunnel" | "standby" | "active";

const emptyForm: VPNForm = {
  name: "",
  protocol: "wireguard",
  ipVersion: "ipv4",
  localCIDR: "",
  remoteCIDR: "",
  endpoint: "",
  dns: "",
  privateKey: "",
  peerPublicKey: "",
  preSharedKey: "",
};

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => entry !== "");
}

function pickByIPVersion(values: string[], ipVersion: IPVersion): string {
  if (ipVersion === "ipv6") {
    const v6 = values.find((v) => v.includes(":"));
    if (v6) return v6;
    return values[0] ?? "";
  }
  const v4 = values.find((v) => v.includes("."));
  if (v4) return v4;
  return values[0] ?? "";
}

function hasIPv6(csv: string): boolean {
  return splitCSV(csv).some((v) => v.includes(":"));
}

function parseWireGuardConfig(content: string, fileName: string): Partial<VPNForm> {
  const parsed = parseWireGuardFields(content);

  // Default to ipv4. Pick addresses based on that.
  const ipVersion: IPVersion = "ipv4";
  const localCIDR = pickByIPVersion(splitCSV(parsed.interfaceAddress), ipVersion);
  const remoteCIDR = pickByIPVersion(splitCSV(parsed.peerAllowedIPs), ipVersion);

  if (!localCIDR || !remoteCIDR || !parsed.endpoint || !parsed.privateKey || !parsed.peerPublicKey) {
    throw new Error(
      "Unsupported config format. Expected [Interface] PrivateKey/Address and [Peer] PublicKey/AllowedIPs/Endpoint.",
    );
  }

  return {
    name: parsed.deviceName || wireGuardStemFromFile(fileName).replace(/[-_]+/g, " ").trim() || "Imported WireGuard",
    protocol: "wireguard",
    ipVersion,
    localCIDR,
    remoteCIDR,
    endpoint: parsed.endpoint,
    dns: pickByIPVersion(splitCSV(parsed.interfaceDNS), ipVersion),
    privateKey: parsed.privateKey,
    peerPublicKey: parsed.peerPublicKey,
    preSharedKey: parsed.preSharedKey,
    // Stash raw values so switching ip_version can re-pick.
    _parsedAddresses: parsed.interfaceAddress,
    _parsedAllowedIPs: parsed.peerAllowedIPs,
    _parsedDNS: parsed.interfaceDNS,
  };
}

function formFromDraft(draft?: Partial<VPNForm> | null): VPNForm {
  return { ...emptyForm, ...(draft ?? {}) };
}

function formFromStatus(vpn: VPNStatus): VPNForm {
  return {
    name: vpn.name,
    protocol: vpn.protocol,
    ipVersion: vpn.ip_version ?? "ipv4",
    localCIDR: vpn.local_cidr ?? "",
    remoteCIDR: vpn.remote_cidr ?? "",
    endpoint: vpn.endpoint ?? "",
    dns: "",
    privateKey: "",
    peerPublicKey: "",
    preSharedKey: "",
  };
}

function formFromDetail(vpn: VPNDetail): VPNForm {
  return {
    ...formFromStatus(vpn),
    dns: vpn.dns ?? "",
  };
}

function vpnStateMeta(vpn: Pick<VPNStatus, "applied" | "policy_applied" | "routing_applied" | "gateway_applied" | "ownership_status" | "drift_reason">): {
  key: VPNStateKey;
  label: string;
  summary: string;
  badgeClass: string;
  dotClass: string;
  panelClass: string;
  deployLabel: string;
} {
  const os = vpn.ownership_status ?? "local_only";

  // Ownership-based states take priority over deployment flags.
  if (os === "needs_reimport") {
    return {
      key: "draft",
      label: "Import needed",
      summary: "OPNsense resources not found. Re-scan or delete this profile.",
      badgeClass: "bg-red-500/15 text-red-300",
      dotClass: "bg-red-400",
      panelClass: "border-red-500/30 bg-red-500/10 text-red-200",
      deployLabel: "Deploy to OPNsense",
    };
  }
  if (os === "managed_drifted") {
    const reason = vpn.drift_reason ? ` (${vpn.drift_reason})` : "";
    return {
      key: "draft",
      label: "Drifted",
      summary: `OPNsense state has drifted from Gator config${reason}. Re-deploy to fix.`,
      badgeClass: "bg-amber-500/15 text-amber-300",
      dotClass: "bg-amber-400",
      panelClass: "border-amber-500/30 bg-amber-500/10 text-amber-200",
      deployLabel: "Re-deploy to OPNsense",
    };
  }

  // Externally managed: has gateway but no Gator filter rules.
  const externallyManaged = vpn.gateway_applied && !vpn.policy_applied;

  if (vpn.routing_applied) {
    return {
      key: "active",
      label: externallyManaged ? "Active (external)" : "Active",
      summary: externallyManaged
        ? "Routing via legacy rules. Deploy through Gator to take control."
        : "Routing LAN traffic through this VPN right now.",
      badgeClass: "bg-[var(--status-success)]/15 text-[var(--status-success)]",
      dotClass: "bg-[var(--status-success)]",
      panelClass: externallyManaged
        ? "border-[var(--status-success)]/30 bg-[var(--status-success)]/10 text-[var(--status-success)]"
        : "border-[var(--status-success)]/30 bg-[var(--status-success)]/10 text-[var(--status-success)]",
      deployLabel: externallyManaged ? "Deploy with Gator" : "Deploy update",
    };
  }
  if (vpn.policy_applied) {
    return {
      key: "standby",
      label: "Standby",
      summary: "Fully deployed on OPNsense and ready to become the active route.",
      badgeClass: "bg-blue-500/15 text-blue-300",
      dotClass: "bg-blue-400",
      panelClass: "border-blue-500/30 bg-blue-500/10 text-blue-200",
      deployLabel: "Deploy update",
    };
  }
  if (vpn.applied) {
    return {
      key: "tunnel",
      label: "Tunnel Ready",
      summary: "The tunnel exists on OPNsense, but routing has not been deployed yet.",
      badgeClass: "bg-amber-500/15 text-amber-300",
      dotClass: "bg-amber-400",
      panelClass: "border-amber-500/30 bg-amber-500/10 text-amber-200",
      deployLabel: "Deploy to OPNsense",
    };
  }
  return {
    key: "draft",
    label: "Draft",
    summary: "Saved locally only. Nothing has been deployed to OPNsense yet.",
    badgeClass: "bg-[var(--bg-tertiary)] text-[var(--text-tertiary)]",
    dotClass: "bg-[var(--text-muted)]",
    panelClass: "border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 text-[var(--text-tertiary)]",
    deployLabel: "Deploy to OPNsense",
  };
}

function interfaceMeta(vpn: Pick<VPNStatus, "wg_device" | "wg_interface" | "interface_assigned">): {
  headline: string;
  detail: string;
  badgeClass: string;
} {
  if (!vpn.wg_device) {
    return {
      headline: "Pending discovery",
      detail: "The WireGuard interface will appear after the first tunnel deploy.",
      badgeClass: "bg-[var(--bg-tertiary)] text-[var(--text-tertiary)]",
    };
  }
  if (!vpn.interface_assigned) {
    return {
      headline: `${vpn.wg_device} needs assignment`,
      detail: "Assign and enable this interface in OPNsense before routing can complete.",
      badgeClass: "bg-amber-500/15 text-amber-300",
    };
  }
  return {
    headline: vpn.wg_interface ? `${vpn.wg_device} -> ${vpn.wg_interface}` : vpn.wg_device,
    detail: "Assigned and ready for gateway/routing rules.",
    badgeClass: "bg-blue-500/15 text-blue-300",
  };
}

// ─── Collapsible VPN card ─────────────────────────────────────────

function VPNCard(props: {
  vpn: VPNStatus | null;
  expanded?: boolean;
  onToggle?: () => void;
  onSaved: (newId?: number) => void;
  onCancel?: () => void;
  onDeleted?: () => void;
  onReadopt?: () => void;
  refetchList: () => void;
  activeVpnName?: string;
  initialForm?: Partial<VPNForm> | null;
  initialNotice?: string;
  /** When > 0, legacy rules exist — deploy/readopt actions are blocked until migration. */
  legacyRuleCount?: number;
}) {
  const isNew = () => props.vpn === null;
  const vpn = () => props.vpn;

  const [form, setForm] = createSignal<VPNForm>(
    isNew() ? formFromDraft(props.initialForm) : formFromStatus(vpn()!),
  );
  const [saving, setSaving] = createSignal(false);
  const [showDeploy, setShowDeploy] = createSignal(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = createSignal(false);
  const [deleting, setDeleting] = createSignal(false);
  const [toggling, setToggling] = createSignal(false);
  const [detailLoading, setDetailLoading] = createSignal(false);
  const [detailsLoadedId, setDetailsLoadedId] = createSignal<number | null>(null);
  const [error, setError] = createSignal("");
  const [success, setSuccess] = createSignal("");
  const busy = () => saving() || showDeploy() || deleting() || toggling() || detailLoading();
  const detailReady = () => isNew() || detailsLoadedId() === vpn()!.id;

  const update = <K extends keyof VPNForm>(field: K, value: VPNForm[K]) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  createEffect(() => {
    if (!isNew()) return;
    setForm(formFromDraft(props.initialForm));
    setError("");
    setSuccess(props.initialNotice ?? "");
  });

  const loadDetails = async () => {
    if (isNew() || !vpn()) return;
    setDetailLoading(true);
    try {
      const { ok, data } = await apiGet<VPNDetail>(`/api/vpn/configs/${vpn()!.id}`);
      if (!ok) {
        setError(data?.error ?? "Failed to load saved VPN details.");
        return;
      }
      setForm(formFromDetail(data as VPNDetail));
      setDetailsLoadedId(vpn()!.id);
    } catch {
      setError("Failed to load saved VPN details. Check backend connectivity.");
    } finally {
      setDetailLoading(false);
    }
  };

  createEffect(() => {
    if (isNew() || !props.expanded || !vpn()) return;
    if (detailsLoadedId() === vpn()!.id) return;
    void loadDetails();
  });

  const formPayload = () => ({
    name: form().name,
    protocol: form().protocol,
    ip_version: form().ipVersion,
    local_cidr: form().localCIDR,
    remote_cidr: form().remoteCIDR,
    endpoint: form().endpoint,
    dns: form().dns,
    private_key: form().privateKey,
    peer_public_key: form().peerPublicKey,
    pre_shared_key: form().preSharedKey,
    enabled: true,
  });

  // ── File import ──
  const handleConfigFileSelect = async (e: Event) => {
    setError("");
    setSuccess("");
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    try {
      const content = await file.text();
      const parsed = parseWireGuardConfig(content, file.name);
      setForm((prev) => ({ ...prev, ...parsed }));
      setSuccess(`Loaded config from ${file.name}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to parse config file.");
    } finally {
      input.value = "";
    }
  };

  // ── Save / Create ──
  const save = async (e: Event) => {
    e.preventDefault();
    if (busy() || !detailReady()) return;
    setError("");
    setSuccess("");
    setSaving(true);
    try {
      if (isNew()) {
        const { ok, data } = await apiPost("/api/vpn/configs", formPayload());
        if (!ok) {
          setError(data?.error ?? "Failed to create VPN.");
          return;
        }
        setSuccess("VPN profile created.");
        props.onSaved(typeof data?.id === "number" ? data.id : undefined);
      } else {
        const { ok, data } = await apiPut(`/api/vpn/configs/${vpn()!.id}`, formPayload());
        if (!ok) {
          setError(data?.error ?? "Failed to save VPN.");
          return;
        }
        setSuccess("VPN configuration saved.");
        update("privateKey", "");
        update("peerPublicKey", "");
        update("preSharedKey", "");
        props.onSaved();
      }
    } catch {
      setError("Failed to save. Check backend connectivity.");
    } finally {
      setSaving(false);
    }
  };

  // ── Deploy ──
  const startDeploy = async () => {
    if (busy() || isNew() || !detailReady()) return;
    setError("");
    setSuccess("");
    setShowDeploy(true);
  };

  const saveBeforeDeploy = async (): Promise<string | null> => {
    setSaving(true);
    try {
      const saveRes = await apiPut(`/api/vpn/configs/${vpn()!.id}`, formPayload());
      if (!saveRes.ok) {
        const message = saveRes.data?.error ?? "Failed to save before deploying.";
        setError(message);
        return message;
      }
    } catch {
      const message = "Failed to save. Check backend connectivity.";
      setError(message);
      return message;
    } finally {
      setSaving(false);
    }
    return null;
  };

  const deployComplete = () => {
    update("privateKey", "");
    update("peerPublicKey", "");
    update("preSharedKey", "");
    props.onSaved();
  };

  const closeDeploy = () => {
    setShowDeploy(false);
    props.onSaved();
  };

  // ── Delete ──
  const deleteVPN = async () => {
    const v = vpn()!;
    setDeleting(true);
    setError("");
    setSuccess("");
    try {
      const { ok, data } = await apiDelete<{ error?: string; warnings?: string[] }>(`/api/vpn/configs/${v.id}`);
      if (!ok) {
        let message = data?.error ?? "Failed to delete VPN config.";
        if (data?.warnings?.length) message += " Warnings: " + data.warnings.join("; ");
        setError(message);
        return;
      }
      if (data?.warnings?.length) {
        setSuccess(`Deleted "${v.name}". Warnings: ${data.warnings.join("; ")}`);
      }
      setShowDeleteConfirm(false);
      props.onDeleted?.();
    } catch {
      setError("Failed to delete. Check backend connectivity.");
    } finally {
      setDeleting(false);
    }
  };

  // ── Toggle active state ──
  const toggleActive = async () => {
    const v = vpn()!;
    setToggling(true);
    setError("");
    setSuccess("");
    try {
      const action = v.routing_applied ? "deactivate" : "activate";
      const { ok, data } = await apiPost<{ error?: string; warnings?: string[] }>(`/api/opnsense/vpn/${v.id}/${action}`);
      if (!ok) {
        setError(data?.error ?? `Failed to ${action} VPN.`);
        return;
      }
      if (data?.warnings?.length) {
        setSuccess(data.warnings.join("; "));
      } else {
        setSuccess(v.routing_applied ? "Routing disabled. Traffic will use the default gateway." : `${v.name} is now the active routed VPN.`);
      }
      props.refetchList();
    } catch {
      setError("Failed to toggle VPN. Check backend connectivity.");
    } finally {
      setToggling(false);
    }
  };

  // Can toggle = has Gator-managed filter rules AND ownership is managed (verified or pending).
  const canToggle = () => {
    const v = vpn();
    if (!v?.policy_applied) return false;
    const os = v.ownership_status ?? "local_only";
    return os === "managed_verified" || os === "managed_pending";
  };
  const toggleLabel = () => (vpn()?.routing_applied ? "Deactivate route" : "Make active route");

  // ── Collapsed header ──
  const CollapsedHeader = () => {
    const v = vpn()!;
    const state = vpnStateMeta(v);
    return (
      <div
        class="flex w-full items-center gap-4 rounded-2xl border border-[var(--border-strong)] bg-[var(--bg-secondary)]/60 px-5 py-4 text-left transition-colors hover:border-[var(--border-default)] hover:bg-[var(--bg-secondary)]/80 cursor-pointer"
        onClick={props.onToggle}
      >
        {/* Status dot */}
        <div class={`h-2.5 w-2.5 shrink-0 rounded-full ${state.dotClass}`} />

        {/* Name + meta */}
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="truncate font-semibold text-[var(--text-primary)]">{v.name}</span>
            <span class="shrink-0 rounded bg-[var(--bg-tertiary)] px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-[var(--text-tertiary)]">
              {v.protocol}
            </span>
            <Show when={v.wg_device}>
              <span class="shrink-0 rounded bg-blue-500/10 border border-blue-500/20 px-1.5 py-0.5 text-[10px] font-medium text-blue-300">
                {v.wg_device}
              </span>
              <Show when={v.interface_assigned && v.wg_interface}>
                <span class="shrink-0 rounded border border-[var(--border-default)] bg-[var(--bg-tertiary)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--text-secondary)]">
                  {v.wg_interface}
                </span>
              </Show>
              <Show when={!v.interface_assigned}>
                <span class="shrink-0 rounded bg-amber-500/10 border border-amber-500/20 px-1.5 py-0.5 text-[10px] font-medium text-amber-300">
                  Needs assignment
                </span>
              </Show>
            </Show>
            <Show when={v.source_interfaces && v.source_interfaces.length > 0}>
              <span class="shrink-0 rounded border border-violet-500/20 bg-violet-500/10 px-1.5 py-0.5 text-[10px] font-medium text-violet-300">
                {v.source_interfaces!.join(", ")}
              </span>
            </Show>
          </div>
          <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-[var(--text-tertiary)]">
            <span class="truncate">{v.endpoint}</span>
            <span class="text-[var(--text-muted)]">•</span>
            <span>{state.summary}</span>
          </div>
        </div>

        {/* Status + route switch */}
        <div class="flex shrink-0 items-center gap-3">
          <span class={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${state.badgeClass}`}>
            {state.label}
          </span>

          {/* Toggle switch (only for fully deployed VPNs) */}
          <Show when={canToggle()}>
            <div class="flex items-center gap-2 rounded-full border border-[var(--border-strong)] bg-[var(--bg-primary)]/70 px-2 py-1">
              <span class="text-[10px] font-medium uppercase tracking-[0.18em] text-[var(--text-tertiary)]">
                Route
              </span>
              <button
                type="button"
                aria-label={toggleLabel()}
                onClick={(e) => {
                  e.stopPropagation();
                  void toggleActive();
                }}
                disabled={toggling()}
                class={`relative inline-flex h-5 w-10 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${
                  v.routing_applied ? "bg-[var(--status-success)]" : "bg-[var(--bg-active)]"
                }`}
              >
                <span
                  class={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                    v.routing_applied ? "translate-x-[20px]" : "translate-x-[2px]"
                  }`}
                />
              </button>
            </div>
          </Show>

          {/* Chevron */}
          <svg
            class="h-4 w-4 shrink-0 text-[var(--text-tertiary)] transition-transform"
            classList={{ "rotate-180": props.expanded }}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </div>
      </div>
    );
  };

  // ── Expanded form (shared between new + existing) ──
  const ExpandedForm = () => (
    <form
      class="space-y-5 rounded-2xl border border-[var(--border-strong)] bg-[var(--bg-secondary)]/60 p-5"
      onSubmit={save}
    >
      <Show when={!isNew()}>
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-[var(--text-primary)]">{vpn()!.name}</h2>
          <button
            type="button"
            onClick={props.onToggle}
            class="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]"
          >
            Collapse
          </button>
        </div>
      </Show>

      <Show when={isNew()}>
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-[var(--text-primary)]">New VPN profile</h2>
          <button
            type="button"
            onClick={props.onCancel}
            class="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]"
          >
            Cancel
          </button>
        </div>
      </Show>

      <Show when={!isNew()}>
        {(() => {
          const state = vpnStateMeta(vpn()!);
          const iface = interfaceMeta(vpn()!);
          return (
            <div class={`flex flex-wrap items-center gap-3 rounded-xl border px-4 py-3 ${state.panelClass}`}>
              <span class="text-sm font-semibold">{state.label}</span>
              <span class="text-xs opacity-80">{state.summary}</span>
              <Show when={vpn()!.wg_device}>
                <span class="rounded bg-blue-500/10 border border-blue-500/20 px-1.5 py-0.5 text-[10px] font-medium text-blue-300">
                  {iface.headline}
                </span>
              </Show>
              <Show when={vpn()!.last_applied_at}>
                <span class="ml-auto text-xs opacity-60">
                  Deployed {new Date(vpn()!.last_applied_at!).toLocaleString()}
                </span>
              </Show>
            </div>
          );
        })()}

        {/* Migration notice for externally managed VPNs */}
        <Show when={vpn()!.gateway_applied && !vpn()!.policy_applied}>
          <div class="rounded-xl border border-amber-500/20 bg-amber-500/5 px-4 py-3">
            <p class="text-xs font-semibold text-amber-300">Legacy firewall rules detected</p>
            <p class="mt-1 text-xs text-[var(--text-tertiary)]">
              This VPN's routing rules were created with the old OPNsense firewall interface and are
              not visible to the API. Use the <span class="font-semibold text-[var(--text-secondary)]">Migration</span> page
              in the sidebar to migrate your rules to the new system, then deploy this VPN through Gator.
            </p>
          </div>
        </Show>
      </Show>

      <Show when={detailLoading()}>
        <div class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/60 px-3 py-2 text-xs text-[var(--text-secondary)]">
          Loading the saved profile details...
        </div>
      </Show>

      <Show when={!isNew() && !detailLoading() && !detailReady()}>
        <div class="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
          Saved values have not been refreshed yet. Reload them before saving to avoid overwriting missing fields.
          <button
            type="button"
            onClick={() => void loadDetails()}
            class="ml-3 rounded border border-amber-400/30 px-2 py-1 text-[11px] font-semibold text-amber-100 transition-all hover:bg-amber-400/10"
          >
            Reload saved values
          </button>
        </div>
      </Show>

      {/* Messages */}
      <Show when={error()}>
        <div class="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-300">
          {error()}
        </div>
      </Show>
      <Show when={success()}>
        <div class="rounded-lg border border-[var(--status-success)]/30 bg-[var(--status-success)]/10 px-3 py-2 text-xs text-[var(--status-success)]">
          {success()}
        </div>
      </Show>

      <Show when={!isNew()}>
        {(() => {
          const [actionsOpen, setActionsOpen] = createSignal(false);
          let menuRef: HTMLDivElement | undefined;
          const handleClickOutside = (e: MouseEvent) => {
            if (menuRef && !menuRef.contains(e.target as Node)) setActionsOpen(false);
          };
          if (typeof document !== "undefined") {
            document.addEventListener("mousedown", handleClickOutside);
            onCleanup(() => document.removeEventListener("mousedown", handleClickOutside));
          }

          type ActionItem = { label: string; variant: "default" | "primary" | "danger"; onClick: () => void; disabled?: boolean; title?: string };
          const migrationBlocked = () => (props.legacyRuleCount ?? 0) > 0;
          const items = (): ActionItem[] => {
            const v = vpn()!;
            const os = v.ownership_status ?? "local_only";
            const list: ActionItem[] = [];
            const migrateHint = "Migrate legacy firewall rules first";

            // needs_reimport: only delete — no save/deploy/activate
            if (os !== "needs_reimport") {
              list.push(
                { label: saving() ? "Saving..." : "Save changes", variant: "default", onClick: () => { const form = menuRef?.closest("form"); form?.requestSubmit(); }, disabled: busy() || !detailReady() },
                { label: vpnStateMeta(v).deployLabel, variant: "primary", onClick: startDeploy, disabled: busy() || !detailReady() || migrationBlocked(), title: migrationBlocked() ? migrateHint : undefined },
              );
            }
            if (canToggle()) {
              list.push({
                label: v.routing_applied ? "Deactivate route" : "Make active route",
                variant: v.routing_applied ? "danger" : "default",
                onClick: () => void toggleActive(),
                disabled: busy(),
              });
            }
            // Re-adopt action for drifted or needs-reimport profiles
            if ((os === "managed_drifted" || os === "needs_reimport") && props.onReadopt) {
              list.push({
                label: "Re-adopt from OPNsense",
                variant: "primary",
                onClick: props.onReadopt,
                disabled: busy() || migrationBlocked(),
                title: migrationBlocked() ? migrateHint : undefined,
              });
            }
            return list;
          };

          return (
            <div class="relative" ref={menuRef}>
              <button
                type="button"
                onClick={() => setActionsOpen((v) => !v)}
                class="inline-flex items-center gap-1.5 rounded-[var(--radius-md)] border border-[var(--border-strong)] bg-[var(--bg-elevated)] px-3 py-1.5 text-[13px] font-medium text-[var(--text-primary)] transition-all hover:bg-[var(--bg-hover)]"
              >
                Actions
                <svg class="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
                </svg>
              </button>
              <Show when={actionsOpen()}>
                <div class="absolute left-0 top-full z-50 mt-1 min-w-[180px] overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)] py-1 shadow-xl shadow-black/40">
                  <For each={items()}>
                    {(item) => (
                      <button
                        type={item.variant === "default" && item.label.includes("Sav") ? "submit" : "button"}
                        disabled={item.disabled}
                        title={item.title}
                        onClick={() => { setActionsOpen(false); item.onClick(); }}
                        class={[
                          "flex w-full items-center gap-2 px-3 py-2 text-left text-[var(--text-sm)] font-medium transition-colors",
                          item.variant === "danger"
                            ? "text-[var(--status-error)] hover:bg-[var(--status-error)]/10"
                            : item.variant === "primary"
                              ? "text-[var(--accent-primary)] hover:bg-[var(--accent-primary)]/10"
                              : "text-[var(--text-primary)] hover:bg-[var(--bg-hover)]",
                          "disabled:opacity-40 disabled:cursor-not-allowed",
                        ].join(" ")}
                      >
                        {item.label}
                      </button>
                    )}
                  </For>
                </div>
              </Show>
            </div>
          );
        })()}
      </Show>

      {/* Import */}
      <div class="rounded-xl border border-[var(--border-strong)] bg-[var(--bg-secondary)]/40 p-4">
        <div class="mb-3">
          <p class="text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--text-tertiary)]">Import</p>
          <p class="mt-1 text-sm font-semibold text-[var(--text-primary)]">Load from a WireGuard config</p>
          <p class="mt-1 text-xs text-[var(--text-tertiary)]">
            Import first for the fastest path, then adjust any fields that need to be customized.
          </p>
        </div>
        <input
          type="file"
          accept=".conf,.wg,.txt"
          onChange={handleConfigFileSelect}
          class="block w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-1.5 text-xs text-[var(--text-secondary)] file:mr-2 file:rounded file:border-0 file:bg-[var(--bg-active)] file:px-2 file:py-1 file:text-xs file:font-medium file:text-[var(--text-primary)] hover:file:bg-[var(--bg-active)]"
        />
      </div>

      {/* Fields */}
      <div class="rounded-xl border border-[var(--border-strong)] bg-[var(--bg-secondary)]/35 p-4">
        <div class="mb-4">
          <p class="text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--text-tertiary)]">Connection</p>
          <p class="mt-1 text-sm font-semibold text-[var(--text-primary)]">Core profile settings</p>
          <p class="mt-1 text-xs text-[var(--text-tertiary)]">
            These values describe the tunnel, endpoint, and allowed networks.
          </p>
        </div>

        <div>
          <label class="mb-1.5 block text-xs font-medium text-[var(--text-tertiary)]">Profile name</label>
          <input
            type="text"
            value={form().name}
            onInput={(e) => update("name", e.currentTarget.value)}
            placeholder="Branch Office VPN"
            class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
          />
        </div>

        <div class="mt-4 grid gap-3 sm:grid-cols-3">
          <div>
            <Select
              label="Protocol"
              value={form().protocol}
              options={[
                { value: "wireguard", label: "WireGuard" },
                { value: "openvpn", label: "OpenVPN" },
              ]}
              onChange={(v) => update("protocol", v as "wireguard" | "openvpn")}
            />
          </div>
          <div>
            <Select
              label="IP version"
              value={form().ipVersion}
              options={[
                { value: "ipv4", label: "IPv4" },
                { value: "ipv6", label: "IPv6" },
              ]}
              onChange={(v) => {
                const newVersion = v as IPVersion;
                setForm((prev) => {
                  const updated = { ...prev, ipVersion: newVersion };
                  if (prev._parsedAddresses) {
                    updated.localCIDR = pickByIPVersion(splitCSV(prev._parsedAddresses), newVersion);
                  }
                  if (prev._parsedAllowedIPs) {
                    updated.remoteCIDR = pickByIPVersion(splitCSV(prev._parsedAllowedIPs), newVersion);
                  }
                  if (prev._parsedDNS) {
                    updated.dns = pickByIPVersion(splitCSV(prev._parsedDNS), newVersion);
                  }
                  return updated;
                });
              }}
            />
          </div>
          <div>
            <label class="mb-1.5 block text-xs font-medium text-[var(--text-tertiary)]">Remote endpoint</label>
            <input
              type="text"
              value={form().endpoint}
              onInput={(e) => update("endpoint", e.currentTarget.value)}
              placeholder="vpn.example.com:51820"
              class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
            />
          </div>
        </div>

        <div class="mt-4 grid gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1.5 block text-xs font-medium text-[var(--text-tertiary)]">Local CIDR</label>
            <input
              type="text"
              value={form().localCIDR}
              onInput={(e) => update("localCIDR", e.currentTarget.value)}
              placeholder="10.73.211.155/32"
              class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
            />
          </div>
          <div>
            <label class="mb-1.5 block text-xs font-medium text-[var(--text-tertiary)]">Remote CIDR</label>
            <input
              type="text"
              value={form().remoteCIDR}
              onInput={(e) => update("remoteCIDR", e.currentTarget.value)}
              placeholder="0.0.0.0/0"
              class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
            />
          </div>
        </div>

        <div class="mt-4">
          <label class="mb-1.5 block text-xs font-medium text-[var(--text-tertiary)]">DNS (optional)</label>
          <input
            type="text"
            value={form().dns}
            onInput={(e) => update("dns", e.currentTarget.value)}
            placeholder="10.64.0.1"
            class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
          />
        </div>
      </div>

      <div class="rounded-xl border border-[var(--border-strong)] bg-[var(--bg-secondary)]/35 p-4">
        <div class="mb-4">
          <p class="text-[10px] font-semibold uppercase tracking-[0.18em] text-[var(--text-tertiary)]">Keys</p>
          <p class="mt-1 text-sm font-semibold text-[var(--text-primary)]">Credential material</p>
          <p class="mt-1 text-xs text-[var(--text-tertiary)]">
            Leave secret fields blank to keep the values already stored for this profile.
          </p>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1.5 block text-xs font-medium text-[var(--text-tertiary)]">Interface private key</label>
            <input
              type="password"
              value={form().privateKey}
              onInput={(e) => update("privateKey", e.currentTarget.value)}
              placeholder={
                vpn()?.has_private_key ? "Leave blank to keep existing" : "Base64 private key"
              }
              class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 font-mono text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
            />
          </div>
          <div>
            <label class="mb-1.5 block text-xs font-medium text-[var(--text-tertiary)]">Peer public key</label>
            <input
              type="text"
              value={form().peerPublicKey}
              onInput={(e) => update("peerPublicKey", e.currentTarget.value)}
              placeholder={
                vpn()?.has_peer_public_key ? "Leave blank to keep existing" : "Base64 public key"
              }
              class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 font-mono text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
            />
          </div>
        </div>

        <div class="mt-4">
          <label class="mb-1.5 block text-xs font-medium text-[var(--text-tertiary)]">Pre-shared key (optional)</label>
          <input
            type="password"
            value={form().preSharedKey}
            onInput={(e) => update("preSharedKey", e.currentTarget.value)}
            placeholder={
              vpn()?.has_pre_shared_key ? "Leave blank to keep existing" : "Enter pre-shared key"
            }
            class="w-full rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 px-3 py-2 font-mono text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] focus:border-[var(--accent-primary)] focus:outline-none"
          />
        </div>
      </div>

      {/* Action buttons */}
      <Show when={isNew()}>
        <button
          type="submit"
          disabled={busy()}
          class="w-full rounded-lg bg-[var(--accent-primary)] px-4 py-2.5 text-sm font-semibold text-[var(--bg-primary)] shadow-lg shadow-[var(--accent-primary)]/20 transition-all hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {saving() ? "Creating..." : "Create VPN profile"}
        </button>
      </Show>

      <Show when={!isNew()}>
        <div class="rounded-xl border border-red-500/20 bg-red-500/[0.04] p-4">
          <p class="text-[10px] font-semibold uppercase tracking-[0.18em] text-red-300/70">Danger zone</p>
          <div class="mt-3 flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <p class="text-sm text-red-100/90">
              Delete this profile and remove any associated tunnel, gateway, and routing objects from OPNsense.
            </p>
            <button
              type="button"
              onClick={() => setShowDeleteConfirm(true)}
              disabled={busy()}
              class="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2.5 text-sm font-semibold text-red-200 transition-all hover:bg-red-500/20 disabled:cursor-not-allowed disabled:opacity-60"
            >
              Delete profile
            </button>
          </div>
        </div>
      </Show>

      {/* Deploy modal */}
      <Show when={showDeploy()}>
        <DeployModal
          vpnId={vpn()!.id}
          vpnName={vpn()!.name}
          activeVpnName={props.activeVpnName && props.activeVpnName !== vpn()!.name ? props.activeVpnName : undefined}
          wgDevice={vpn()!.wg_device}
          interfaceAssigned={vpn()!.interface_assigned}
          currentSourceInterfaces={vpn()!.source_interfaces}
          gatewayName={vpn()!.gateway_name}
          onBeforeStart={saveBeforeDeploy}
          onComplete={deployComplete}
          onClose={closeDeploy}
        />
      </Show>

      <Show when={showDeleteConfirm()}>
        <ConfirmModal
          title={`Delete ${vpn()!.name}?`}
          description="This removes the profile from Gator and attempts to clean up the associated OPNsense resources."
          confirmLabel={deleting() ? "Deleting..." : "Delete profile"}
          cancelLabel="Keep profile"
          tone="danger"
          busy={deleting()}
          onConfirm={() => void deleteVPN()}
          onCancel={() => setShowDeleteConfirm(false)}
        >
          <div class="rounded-lg border border-red-500/20 bg-red-500/[0.04] px-3 py-2 text-xs text-red-100/80">
            Tunnel, gateway, NAT, and routing policy objects created by this profile will be removed when possible.
          </div>
        </ConfirmModal>
      </Show>
    </form>
  );

  // ── Render ──
  return (
    <Show when={!isNew()} fallback={<ExpandedForm />}>
      <Show when={!props.expanded} fallback={<ExpandedForm />}>
        <CollapsedHeader />
      </Show>
    </Show>
  );
}

export default VPNCard;
