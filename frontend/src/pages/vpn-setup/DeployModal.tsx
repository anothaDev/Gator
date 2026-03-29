import { createEffect, createSignal, For, Match, onMount, Show, Switch } from "solid-js";
import { apiPost } from "../../lib/api";
import Modal from "../../components/Modal";
import Button from "../../components/Button";
import AlertBanner from "../../components/AlertBanner";
import Badge from "../../components/Badge";
import Card from "../../components/Card";
import Spinner from "../../components/Spinner";

type SelectableInterface = {
  identifier: string;
  device: string;
  description: string;
};

type StepStatus = "pending" | "running" | "done" | "error";
type DeployStep = {
  label: string;
  doneLabel: string;
  endpoint: string;
  status: StepStatus;
  error?: string;
};

export default function DeployModal(props: {
  vpnId: number;
  vpnName: string;
  activeVpnName?: string;
  wgDevice?: string;
  interfaceAssigned?: boolean;
  currentSourceInterfaces?: string[];
  gatewayName?: string;
  onBeforeStart: () => Promise<string | null>;
  onClose: () => void;
  onComplete: () => void;
}) {
  const [steps, setSteps] = createSignal<DeployStep[]>([
    { label: "Deploying WireGuard tunnel", doneLabel: "WireGuard tunnel ready", endpoint: "apply", status: "pending" },
    { label: "Configuring gateway", doneLabel: "Gateway configured", endpoint: "apply-gateway", status: "pending" },
    { label: "Configuring outbound NAT", doneLabel: "Outbound NAT configured", endpoint: "apply-nat", status: "pending" },
    { label: "Enabling routing policy", doneLabel: "Routing policy enabled", endpoint: "apply-policy-rule", status: "pending" },
  ]);
  const [stage, setStage] = createSignal<"confirm" | "running" | "done" | "error">("confirm");
  const [finished, setFinished] = createSignal(false);
  const [failed, setFailed] = createSignal(false);
  const [starting, setStarting] = createSignal(false);
  const [preflightError, setPreflightError] = createSignal("");
  const [selectableIfaces, setSelectableIfaces] = createSignal<SelectableInterface[]>([]);
  const [selectedIfaces, setSelectedIfaces] = createSignal<string[]>(props.currentSourceInterfaces ?? ["lan"]);
  const [ifacesLoading, setIfacesLoading] = createSignal(true);
  const [natMode, setNatMode] = createSignal<{ mode: string; compatible: boolean; message: string } | null>(null);
  const [natModeLoading, setNatModeLoading] = createSignal(true);

  // Auto-adopted rules info (filled by apply-policy-rule step).
  type AdoptedRule = { uuid: string; description: string; source: string; destination: string; protocol: string };
  type DeployStepResponse = {
    error?: string;
    gateway_name?: string;
    filter_uuid?: string;
    adopted_rules?: AdoptedRule[];
  };
  const [adoptedRules, setAdoptedRules] = createSignal<AdoptedRule[]>([]);

  // Post-deploy stale rule cleanup state.
  type StaleRule = {
    uuid: string; interface: string; action: string; quick: string;
    direction: string; ipprotocol: string; protocol: string;
    source: string; destination: string; destination_port: string;
    gateway: string; description: string; enabled: boolean;
  };
  const [resolvedGatewayName, setResolvedGatewayName] = createSignal(props.gatewayName ?? "");
  const [resolvedFilterUUID, setResolvedFilterUUID] = createSignal("");
  const [staleRules, setStaleRules] = createSignal<StaleRule[]>([]);
  const [staleLoading, setStaleLoading] = createSignal(false);
  const [staleError, setStaleError] = createSignal("");
  const [busyUuids, setBusyUuids] = createSignal<Set<string>>(new Set());
  const [handledUuids, setHandledUuids] = createSignal<Set<string>>(new Set());
  const [actionErrors, setActionErrors] = createSignal<Record<string, string>>({});

  // Fetch selectable interfaces and NAT mode in parallel on mount.
  onMount(async () => {
    const ifacePromise = fetch("/api/opnsense/interfaces/selectable")
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json();
          const ifaces: SelectableInterface[] = data.interfaces ?? [];
          setSelectableIfaces(ifaces);
          if (!props.currentSourceInterfaces?.length) {
            const hasLan = ifaces.some((i) => i.identifier === "lan");
            setSelectedIfaces(hasLan ? ["lan"] : ifaces.length > 0 ? [ifaces[0].identifier] : []);
          }
        }
      })
      .catch(() => {})
      .finally(() => setIfacesLoading(false));

    const natPromise = fetch("/api/opnsense/nat/mode")
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json();
          setNatMode(data);
        }
      })
      .catch(() => {})
      .finally(() => setNatModeLoading(false));

    await Promise.all([ifacePromise, natPromise]);
  });

  type ConflictInfo = {
    conflicts: { uuid: string; interface: string; source: string; destination: string; gateway: string; description: string }[];
    has_gateway_rules: boolean;
    recommendation: "selective" | "all";
  };
  const [conflictInfo, setConflictInfo] = createSignal<ConflictInfo | null>(null);
  const [conflictLoading, setConflictLoading] = createSignal(false);

  const toggleIface = (id: string) => {
    setSelectedIfaces((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  };

  // Detect conflicts whenever selected interfaces change.
  createEffect(() => {
    const ifaces = selectedIfaces();
    if (ifaces.length === 0) {
      setConflictInfo(null);
      return;
    }
    setConflictLoading(true);
    apiPost("/api/opnsense/firewall/detect-conflicts", { interfaces: ifaces })
      .then(({ ok, data }) => {
        if (ok) setConflictInfo(data as ConflictInfo);
        else setConflictInfo(null);
      })
      .catch(() => setConflictInfo(null))
      .finally(() => setConflictLoading(false));
  });

  const updateStep = (index: number, patch: Partial<DeployStep>) => {
    setSteps((prev) => prev.map((s, i) => (i === index ? { ...s, ...patch } : s)));
  };

  const runFrom = async (startIndex: number) => {
    setFailed(false);
    setStage("running");
    for (let i = startIndex; i < steps().length; i++) {
      updateStep(i, { status: "running", error: undefined });
      const step = steps()[i];
      try {
        const { ok, data } = await apiPost<DeployStepResponse>(`/api/opnsense/vpn/${props.vpnId}/${step.endpoint}`);
        if (!ok) {
          updateStep(i, { status: "error", error: data?.error ?? "Unknown error" });
          setFailed(true);
          setStage("error");
          return;
        }
        // Capture gateway name from the apply-gateway step response.
        if (step.endpoint === "apply-gateway" && data?.gateway_name) {
          setResolvedGatewayName(data.gateway_name);
        }
        // Capture filter UUID and adopted rules from the apply-policy-rule step response.
        if (step.endpoint === "apply-policy-rule") {
          if (data?.filter_uuid) setResolvedFilterUUID(data.filter_uuid);
          if (data?.adopted_rules?.length) setAdoptedRules(data.adopted_rules);
        }
        updateStep(i, { status: "done" });
      } catch {
        updateStep(i, { status: "error", error: "Network error — check backend connectivity." });
        setFailed(true);
        setStage("error");
        return;
      }
    }
    setFinished(true);
    setStage("done");
    props.onComplete();

    // Fetch stale rules after successful deployment.
    const gwName = resolvedGatewayName();
    if (gwName && selectedIfaces().length > 0) {
      setStaleLoading(true);
      setStaleError("");
      try {
        const { ok, data } = await apiPost<{ stale_rules?: StaleRule[] }>("/api/opnsense/firewall/stale-rules", {
          gateway_name: gwName,
          interfaces: selectedIfaces(),
        });
        if (ok && (data?.stale_rules?.length ?? 0) > 0) {
          setStaleRules(data.stale_rules!);
        }
      } catch {
        setStaleError("Failed to check for stale rules.");
      }
      setStaleLoading(false);
    }
  };

  const start = async () => {
    setPreflightError("");

    if (selectedIfaces().length === 0) {
      setPreflightError("Select at least one source interface.");
      return;
    }

    setStarting(true);

    // Save the VPN config first (onBeforeStart).
    const err = await props.onBeforeStart();
    if (err) {
      setStarting(false);
      setPreflightError(err);
      return;
    }

    // Save selected source interfaces.
    try {
      const { ok, data } = await apiPost<{ error?: string }>(`/api/opnsense/vpn/${props.vpnId}/source-interfaces`, {
        interfaces: selectedIfaces(),
      });
      if (!ok) {
        setStarting(false);
        setPreflightError(data?.error ?? "Failed to save source interfaces.");
        return;
      }
    } catch {
      setStarting(false);
      setPreflightError("Failed to save source interfaces. Check backend connectivity.");
      return;
    }

    setStarting(false);
    void runFrom(0);
  };

  const retry = () => {
    const failedIndex = steps().findIndex((s) => s.status === "error");
    if (failedIndex >= 0) void runFrom(failedIndex);
  };

  const markBusy = (uuid: string) => setBusyUuids((prev) => new Set([...prev, uuid]));
  const unmarkBusy = (uuid: string) => setBusyUuids((prev) => { const c = new Set(prev); c.delete(uuid); return c; });
  const markHandled = (uuid: string) => setHandledUuids((prev) => new Set([...prev, uuid]));
  const clearError = (uuid: string) => setActionErrors((prev) => { const c = { ...prev }; delete c[uuid]; return c; });
  const setError = (uuid: string, msg: string) => setActionErrors((prev) => ({ ...prev, [uuid]: msg }));

  const adoptStaleRule = async (uuid: string) => {
    const filterUUID = resolvedFilterUUID();
    if (!filterUUID) {
      setError(uuid, "No Gator filter rule UUID available");
      return;
    }
    markBusy(uuid);
    clearError(uuid);
    try {
      const { ok, data } = await apiPost<{ error?: string }>("/api/opnsense/firewall/adopt-rule", {
        stale_uuid: uuid,
        gator_uuid: filterUUID,
      });
      if (ok) {
        markHandled(uuid);
      } else {
        setError(uuid, data?.error ?? "Failed to adopt");
      }
    } catch {
      setError(uuid, "Network error");
    }
    unmarkBusy(uuid);
  };

  const deleteStaleRule = async (uuid: string) => {
    markBusy(uuid);
    clearError(uuid);
    try {
      const res = await fetch(`/api/opnsense/firewall/cleanup/${uuid}`, { method: "DELETE" });
      const data = await res.json().catch(() => ({}));
      if (res.ok) {
        markHandled(uuid);
      } else {
        setError(uuid, data?.error ?? "Failed to delete");
      }
    } catch {
      setError(uuid, "Network error");
    }
    unmarkBusy(uuid);
  };

  const deleteAllStaleRules = async () => {
    const remaining = staleRules().filter((r) => !handledUuids().has(r.uuid));
    for (const rule of remaining) {
      await deleteStaleRule(rule.uuid);
    }
  };

  return (
    <Modal size="md" onBackdropClick={stage() === "confirm" ? props.onClose : undefined}>
      <h2 class="text-lg font-semibold text-fg">{props.vpnName} deployment</h2>
      <p class="mt-1 text-xs text-fg-muted">
        {stage() === "confirm"
          ? "This will save the current profile and run the OPNsense deployment steps automatically."
          : "Running the OPNsense deployment flow..."}
      </p>

      <Show when={props.activeVpnName}>
        <div class="mt-3">
          <AlertBanner tone="warning">
            <span class="font-medium">{props.activeVpnName}</span> is currently active.
            It will be deactivated and <span class="font-medium">{props.vpnName}</span> will become the new active VPN.
          </AlertBanner>
        </div>
      </Show>

      <Show when={props.wgDevice && props.interfaceAssigned === false}>
        <div class="mt-3">
          <AlertBanner tone="info">
            <span class="font-medium">{props.wgDevice}</span> exists, but it is not assigned in OPNsense yet.
            Deployment will pause at the gateway step until that interface is assigned and enabled.
          </AlertBanner>
        </div>
      </Show>

      <Show when={natModeLoading()}>
        <div class="mt-3 rounded-lg border border-border-faint bg-surface-raised px-3 py-2 text-xs text-fg-muted">
          Checking outbound NAT mode...
        </div>
      </Show>

      <Show when={!natModeLoading() && natMode() && !natMode()!.compatible}>
        <div class="mt-3">
          <AlertBanner tone="error">
            <p class="font-semibold">Outbound NAT mode: {natMode()!.mode}</p>
            <p class="mt-1">{natMode()!.message}</p>
            <p class="mt-2">After changing it, click <span class="font-semibold">Re-check</span> below.</p>
            <Button
              variant="secondary"
              size="sm"
              class="mt-2"
              onClick={async () => {
                setNatModeLoading(true);
                try {
                  const res = await fetch("/api/opnsense/nat/mode");
                  if (res.ok) setNatMode(await res.json());
                } catch {}
                setNatModeLoading(false);
              }}
              loading={natModeLoading()}
            >
              Re-check NAT mode
            </Button>
          </AlertBanner>
        </div>
      </Show>

      <Show when={!natModeLoading() && natMode()?.compatible}>
        <div class="mt-3 rounded-lg border border-success/20 bg-success-subtle px-3 py-2 text-xs text-success">
          Outbound NAT: <span class="font-medium">{natMode()!.mode}</span> mode
        </div>
      </Show>

      <Show when={preflightError()}>
        <div class="mt-3">
          <AlertBanner tone="error">{preflightError()}</AlertBanner>
        </div>
      </Show>

      {/* Source interface selection — shown during confirm stage */}
      <Show when={stage() === "confirm"}>
        <div class="mt-4 rounded-lg border border-border-faint bg-surface-raised p-3">
          <p class="text-xs font-semibold text-fg-secondary">Source interfaces</p>
          <p class="mt-1 text-xs text-fg-muted">
            Select which interfaces should route traffic through this VPN.
          </p>
          <Show when={ifacesLoading()}>
            <p class="mt-2 text-xs text-fg-muted">Loading interfaces...</p>
          </Show>
          <Show when={!ifacesLoading() && selectableIfaces().length === 0}>
            <p class="mt-2 text-xs text-warning">
              No selectable interfaces found. Will default to LAN.
            </p>
          </Show>
          <Show when={!ifacesLoading() && selectableIfaces().length > 0}>
            <div class="mt-2 space-y-1.5">
              <For each={selectableIfaces()}>
                {(iface) => (
                  <label class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-hover">
                    <input
                      type="checkbox"
                      checked={selectedIfaces().includes(iface.identifier)}
                      onChange={() => toggleIface(iface.identifier)}
                      class="h-3.5 w-3.5 rounded border-transparent bg-surface text-brand focus:ring-brand/30"
                    />
                    <span class="text-xs font-medium text-fg">{iface.description}</span>
                    <span class="text-xs text-fg-muted">({iface.identifier} / {iface.device})</span>
                  </label>
                )}
              </For>
            </div>
          </Show>
        </div>
      </Show>

      {/* Conflict warning */}
      <Show when={stage() === "confirm" && conflictInfo()?.has_gateway_rules}>
        <div class="mt-3">
          <AlertBanner tone="warning">
            <p class="font-semibold">Existing gateway rules detected on selected interfaces</p>
            <p class="mt-1">
              The selected interfaces already have {conflictInfo()!.conflicts.length} non-Gator rule(s) with custom gateways.
              Deploying a catch-all VPN policy could conflict with your existing routing.
            </p>
            <div class="mt-2 max-h-24 space-y-1 overflow-y-auto">
              <For each={conflictInfo()!.conflicts}>
                {(c) => (
                  <div class="rounded bg-warning-subtle px-2 py-1 text-xs text-warning">
                    <span class="font-medium">{c.gateway}</span>
                    {" ← "}
                    {c.source} {"\u2192"} {c.destination}
                    <Show when={c.description}>
                      {" "}
                      <span class="text-fg-muted">({c.description})</span>
                    </Show>
                  </div>
                )}
              </For>
            </div>
            <p class="mt-2 font-medium">
              Gator will never modify or reorder your existing rules. Use <span class="font-semibold">selective mode</span> on
              the Routing page to only route specific apps through VPN without affecting your current setup.
            </p>
          </AlertBanner>
        </div>
      </Show>

      <div class="mt-5 space-y-3">
        <For each={steps()}>
          {(step) => (
            <div class="flex items-start gap-3">
              <div class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center">
                <Switch>
                  <Match when={step.status === "pending"}>
                    <div class="h-4 w-4 rounded-full border-2 border-transparent" />
                  </Match>
                  <Match when={step.status === "running"}>
                    <Spinner size="md" class="text-brand" />
                  </Match>
                  <Match when={step.status === "done"}>
                    <svg class="h-5 w-5 text-success" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                      <path d="M5 13l4 4L19 7" />
                    </svg>
                  </Match>
                  <Match when={step.status === "error"}>
                    <svg class="h-5 w-5 text-error" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                      <path d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </Match>
                </Switch>
              </div>
              <div class="min-w-0 flex-1">
                <p class={`text-sm ${
                  step.status === "done" ? "text-success" :
                  step.status === "error" ? "text-error" :
                  step.status === "running" ? "text-fg" :
                  "text-fg-muted"
                }`}>
                  {step.status === "done" ? step.doneLabel : step.label}
                  {step.status === "running" ? "..." : ""}
                </p>
                <Show when={step.status === "error" && step.error}>
                  <p class="mt-1 text-xs text-error">{step.error}</p>
                </Show>
              </div>
            </div>
          )}
        </For>
      </div>

      <Show when={finished()}>
        <div class="mt-4">
          <AlertBanner tone="success">VPN routing is now active.</AlertBanner>
        </div>
      </Show>

      {/* Auto-adopted rules notification */}
      <Show when={finished() && adoptedRules().length > 0}>
        <div class="mt-3">
          <AlertBanner tone="info">
            <p class="font-semibold">
              Adopted {adoptedRules().length} legacy rule{adoptedRules().length !== 1 ? "s" : ""}
            </p>
            <p class="mt-1">
              Existing routing logic was merged into the Gator rule and the old rule{adoptedRules().length !== 1 ? "s were" : " was"} removed.
            </p>
            <div class="mt-2 space-y-1">
              <For each={adoptedRules()}>
                {(rule) => (
                  <div class="rounded bg-info-subtle px-2 py-1 text-xs text-info">
                    {rule.source || "any"} {"\u2192"} <span class="font-medium">{rule.destination || "any"}</span>
                    <Show when={rule.protocol && rule.protocol !== "any"}>
                      {" "}<span class="rounded bg-hover px-1 py-0.5 text-fg-secondary">{rule.protocol}</span>
                    </Show>
                    <Show when={rule.description}>
                      {" "}<span class="text-fg-muted">({rule.description})</span>
                    </Show>
                  </div>
                )}
              </For>
            </div>
          </AlertBanner>
        </div>
      </Show>

      {/* Post-deploy stale rule cleanup (fallback for rules not auto-adopted) */}
      <Show when={finished() && staleLoading()}>
        <div class="mt-3 rounded-lg border border-border-faint bg-surface-raised px-3 py-2 text-xs text-fg-muted">
          Checking for redundant rules...
        </div>
      </Show>

      <Show when={finished() && staleError()}>
        <div class="mt-3">
          <AlertBanner tone="warning">{staleError()}</AlertBanner>
        </div>
      </Show>

      <Show when={finished() && !staleLoading() && staleRules().length > 0}>
        {(() => {
          const remaining = () => staleRules().filter((r) => !handledUuids().has(r.uuid));
          const allHandled = () => remaining().length === 0;
          // A rule is "interesting" if it has non-default source/destination/protocol/port.
          const hasCustomFields = (r: StaleRule) =>
            (r.destination && r.destination !== "any") ||
            (r.source && r.source !== "any" && r.source !== r.interface) ||
            (r.protocol && r.protocol !== "any") ||
            !!r.destination_port;
          return (
            <div class="mt-3">
              <AlertBanner tone="warning">
                <Show when={!allHandled()}>
                  <p class="font-semibold">
                    {remaining().length} legacy rule{remaining().length !== 1 ? "s" : ""} on same gateway
                  </p>
                  <p class="mt-1">
                    These non-Gator rules use the same gateway and interfaces. You can <strong>adopt</strong> a
                    rule's fields (source, destination, protocol, ports) into the Gator rule, or <strong>delete</strong> it
                    if it's no longer needed.
                  </p>
                  <div class="mt-2 max-h-48 space-y-2 overflow-y-auto">
                    <For each={staleRules()}>
                      {(rule) => (
                        <Show when={!handledUuids().has(rule.uuid)}>
                          <div class="rounded border-transparent bg-surface px-2.5 py-2">
                            <div class="flex items-start gap-2">
                              <div class="min-w-0 flex-1">
                                <p class="text-xs text-fg-secondary">
                                  {rule.source || "any"} {"\u2192"} <span class="font-medium">{rule.destination || "any"}</span>
                                  <Show when={rule.protocol && rule.protocol !== "any"}>
                                    {" "}<span class="rounded bg-hover px-1 py-0.5 text-label-xs text-fg-muted">{rule.protocol}</span>
                                  </Show>
                                  <Show when={rule.destination_port}>
                                    {" "}:{rule.destination_port}
                                  </Show>
                                </p>
                                <Show when={rule.description}>
                                  <p class="mt-0.5 truncate text-xs text-fg-muted">{rule.description}</p>
                                </Show>
                                <p class="mt-0.5 text-xs text-fg-muted">
                                  {rule.interface} / {rule.direction} / {rule.gateway}
                                  <Show when={!rule.enabled}> / <span class="text-fg-disabled">disabled</span></Show>
                                </p>
                                <Show when={actionErrors()[rule.uuid]}>
                                  <p class="mt-1 text-xs text-error">{actionErrors()[rule.uuid]}</p>
                                </Show>
                              </div>
                              <div class="flex shrink-0 gap-1.5">
                                <Show when={hasCustomFields(rule) && resolvedFilterUUID()}>
                                  <Button
                                    variant="secondary"
                                    size="sm"
                                    disabled={busyUuids().has(rule.uuid)}
                                    onClick={() => void adoptStaleRule(rule.uuid)}
                                  >
                                    {busyUuids().has(rule.uuid) ? "..." : "Adopt"}
                                  </Button>
                                </Show>
                                <Button
                                  variant="danger"
                                  size="sm"
                                  disabled={busyUuids().has(rule.uuid)}
                                  onClick={() => void deleteStaleRule(rule.uuid)}
                                >
                                  {busyUuids().has(rule.uuid) ? "..." : "Delete"}
                                </Button>
                              </div>
                            </div>
                          </div>
                        </Show>
                      )}
                    </For>
                  </div>
                  <Show when={remaining().length > 1}>
                    <Button
                      variant="danger"
                      size="sm"
                      class="mt-2"
                      disabled={busyUuids().size > 0}
                      onClick={() => void deleteAllStaleRules()}
                    >
                      Delete all {remaining().length} rules
                    </Button>
                  </Show>
                </Show>
                <Show when={allHandled()}>
                  <p class="text-success">All legacy rules have been handled.</p>
                </Show>
              </AlertBanner>
            </div>
          );
        })()}
      </Show>

      <div class="mt-5 flex justify-end gap-2">
        <Show when={stage() === "confirm"}>
          <Button
            variant="secondary"
            size="md"
            onClick={props.onClose}
            disabled={starting()}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            size="md"
            onClick={start}
            disabled={starting() || natModeLoading() || (natMode() != null && !natMode()!.compatible)}
            loading={starting()}
          >
            {natModeLoading() ? "Checking NAT..." : "Start deployment"}
          </Button>
        </Show>

        <Show when={stage() === "running"}>
          <div class="rounded-lg border border-border-faint bg-surface-raised px-4 py-2 text-sm font-medium text-fg-secondary">
            Deploying...
          </div>
        </Show>

        <Show when={stage() === "error"}>
          <Button variant="secondary" size="md" onClick={props.onClose}>
            Close
          </Button>
          <Button variant="primary" size="md" onClick={retry}>
            Retry
          </Button>
        </Show>

        <Show when={stage() === "done"}>
          <Button variant="primary" size="md" onClick={props.onClose}>
            Done
          </Button>
        </Show>
      </div>
    </Modal>
  );
}
