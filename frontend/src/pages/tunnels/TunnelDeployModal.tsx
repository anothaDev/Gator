import { createSignal, For, Show, Switch, Match } from "solid-js";
import { apiPost } from "../../lib/api";
import type { DeployStep } from "./types";

// ─── Deploy Modal ────────────────────────────────────────────────

function TunnelDeployModal(props: {
  tunnelId: number;
  tunnelName: string;
  mode: "full" | "setup-remote";
  onClose: () => void;
  onComplete: () => void;
}) {
  const [migrateSSH, setMigrateSSH] = createSignal(true);

  const buildSteps = (withMigrateSSH: boolean): DeployStep[] => {
    const migrateStep: DeployStep = {
      label: "Moving SSH to tunnel interface",
      doneLabel: "SSH migrated to tunnel",
      step: "migrate-ssh",
      status: "pending",
    };

    if (props.mode === "setup-remote") {
      const base: DeployStep[] = [
        { label: "Generating remote keys", doneLabel: "Keys generated + OPNsense peer updated", step: "generate-keys", status: "pending" },
        { label: "Configuring remote endpoint", doneLabel: "Remote configured", step: "configure-remote", status: "pending" },
      ];
      if (withMigrateSSH) base.push(migrateStep);
      base.push({ label: "Verifying tunnel", doneLabel: "Tunnel verified", step: "verify", status: "pending" });
      return base;
    }

    const base: DeployStep[] = [
      { label: "Generating WireGuard keys", doneLabel: "Keys generated", step: "generate-keys", status: "pending" },
      { label: "Configuring remote endpoint", doneLabel: "Remote configured", step: "configure-remote", status: "pending" },
      { label: "Configuring OPNsense", doneLabel: "Firewall configured", step: "configure-firewall", status: "pending" },
    ];
    if (withMigrateSSH) base.push(migrateStep);
    base.push({ label: "Verifying tunnel", doneLabel: "Tunnel verified", step: "verify", status: "pending" });
    return base;
  };

  const [steps, setSteps] = createSignal<DeployStep[]>(buildSteps(true));
  const [stage, setStage] = createSignal<"confirm" | "running" | "done" | "error">("confirm");
  const [verifyResult, setVerifyResult] = createSignal<Record<string, unknown> | null>(null);

  const updateStep = (index: number, patch: Partial<DeployStep>) => {
    setSteps((prev) => prev.map((s, i) => (i === index ? { ...s, ...patch } : s)));
  };

  const run = async () => {
    setStage("running");
    for (let i = 0; i < steps().length; i++) {
      updateStep(i, { status: "running" });
      try {
        const { ok, data } = await apiPost(`/api/tunnels/${props.tunnelId}/deploy`, { step: steps()[i].step });
        if (!ok) {
          const errMsg = (data as { error?: string }).error ?? "Step failed";
          updateStep(i, { status: "error", error: errMsg });
          setStage("error");
          return;
        }
        updateStep(i, { status: "done", result: data as Record<string, unknown> });

        // Capture verify result for display.
        if (steps()[i].step === "verify") {
          setVerifyResult(data as Record<string, unknown>);
        }
      } catch (err) {
        updateStep(i, { status: "error", error: err instanceof Error ? err.message : "Network error" });
        setStage("error");
        return;
      }
    }
    setStage("done");
  };

  const retryFrom = (index: number) => {
    for (let i = index; i < steps().length; i++) {
      updateStep(i, { status: "pending", error: undefined });
    }
    void runFrom(index);
  };

  const runFrom = async (startIndex: number) => {
    setStage("running");
    for (let i = startIndex; i < steps().length; i++) {
      updateStep(i, { status: "running" });
      try {
        const { ok, data } = await apiPost(`/api/tunnels/${props.tunnelId}/deploy`, { step: steps()[i].step });
        if (!ok) {
          const errMsg = (data as { error?: string }).error ?? "Step failed";
          updateStep(i, { status: "error", error: errMsg });
          setStage("error");
          return;
        }
        updateStep(i, { status: "done", result: data as Record<string, unknown> });
        if (steps()[i].step === "verify") {
          setVerifyResult(data as Record<string, unknown>);
        }
      } catch (err) {
        updateStep(i, { status: "error", error: err instanceof Error ? err.message : "Network error" });
        setStage("error");
        return;
      }
    }
    setStage("done");
  };

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-bg/80 backdrop-blur-sm">
      <div class="w-full max-w-lg rounded-xl border border-border-faint bg-surface p-6 shadow-2xl">
        <h2 class="text-xl font-bold">
          {props.mode === "setup-remote" ? "Setup Remote: " : "Deploy Tunnel: "}{props.tunnelName}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {props.mode === "setup-remote"
            ? "Install WireGuard and configure the remote VPS. OPNsense peer will be updated automatically."
            : "This will set up WireGuard on the remote VPS and your OPNsense firewall."}
        </p>

        {/* Steps */}
        <div class="mt-5 space-y-3">
          <For each={steps()}>
            {(step, idx) => (
              <div class="flex items-start gap-3">
                <div class={`mt-0.5 h-5 w-5 shrink-0 rounded-full border-2 ${
                  step.status === "done" ? "border-success bg-success/20" :
                  step.status === "running" ? "border-amber-400 bg-amber-400/20 animate-pulse" :
                  step.status === "error" ? "border-red-500 bg-red-500/20" :
                  "border-transparent"
                }`} />
                <div class="flex-1">
                  <p class={`text-sm font-medium ${
                    step.status === "done" ? "text-success" :
                    step.status === "running" ? "text-amber-300" :
                    step.status === "error" ? "text-red-300" :
                    "text-fg-muted"
                  }`}>
                    {step.status === "done" ? step.doneLabel : step.label}
                  </p>
                  <Show when={step.error}>
                    <p class="mt-1 text-xs text-red-400">{step.error}</p>
                    <button
                      type="button"
                      onClick={() => retryFrom(idx())}
                      class="mt-1 text-xs font-medium text-amber-400 hover:text-amber-300"
                    >
                      Retry from here
                    </button>
                  </Show>
                  <Show when={step.status === "done" && step.result}>
                    {/* Show key details for generate-keys step */}
                    <Show when={step.step === "generate-keys" && step.result?.firewall_public_key}>
                      <p class="mt-1 font-mono text-xs text-fg-muted">
                        FW pubkey: {(step.result!.firewall_public_key as string).slice(0, 20)}...
                      </p>
                    </Show>
                    <Show when={step.step === "configure-remote" && step.result?.wg_interface}>
                      <p class="mt-1 text-xs text-fg-muted">
                        Interface: {step.result!.wg_interface as string}
                        {step.result?.wireguard_installed ? " (freshly installed)" : ""}
                      </p>
                    </Show>
                    <Show when={step.step === "migrate-ssh" && step.result?.new_ssh}>
                      <p class="mt-1 text-xs text-fg-muted">
                        SSH: {step.result!.old_ssh as string} → {step.result!.new_ssh as string}
                        {step.result?.listener_confirmed ? " (confirmed)" : " (check manually)"}
                      </p>
                    </Show>
                  </Show>
                </div>
              </div>
            )}
          </For>
        </div>

        {/* Verify results */}
        <Show when={stage() === "done" && verifyResult()}>
          <div class="mt-4 rounded-lg border border-success/30 bg-success/5 p-4">
            <p class="text-sm font-medium text-success">
              {(verifyResult() as Record<string, unknown>).remote_ping_ok ? "Tunnel is up and passing traffic." : "Tunnel deployed. Handshake may take a moment."}
            </p>
            <Show when={(verifyResult() as Record<string, unknown>).remote_handshake}>
              <p class="mt-1 text-xs text-success">Remote handshake confirmed.</p>
            </Show>
            <Show when={(verifyResult() as Record<string, unknown>).issues}>
              <div class="mt-2 text-xs text-amber-400">
                <For each={(verifyResult() as Record<string, unknown>).issues as string[]}>
                  {(issue) => <p>{issue}</p>}
                </For>
              </div>
            </Show>
          </div>
        </Show>

        {/* Options (shown before starting) */}
        <Show when={stage() === "confirm"}>
          <div class="mt-5 rounded-lg border border-border-faint bg-surface/40 p-4">
            <label class="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={migrateSSH()}
                onChange={(e) => {
                  setMigrateSSH(e.currentTarget.checked);
                  setSteps(buildSteps(e.currentTarget.checked));
                }}
                class="h-4 w-4 rounded border border-border bg-surface-raised text-success focus:ring-success"
              />
              <div>
                <p class="text-sm font-medium text-fg-secondary">Move SSH to tunnel interface</p>
                <p class="text-xs text-fg-muted">
                  SSH will listen on the tunnel IP with the WireGuard port. Port 22 kept as fallback.
                </p>
              </div>
            </label>
          </div>
        </Show>

        {/* Actions */}
        <div class="mt-4 flex items-center justify-end gap-3">
          <Switch>
            <Match when={stage() === "confirm"}>
              <button
                type="button"
                onClick={props.onClose}
                class="rounded-lg border border-border bg-surface-raised px-4 py-2 text-body-sm font-medium text-fg-secondary hover:bg-hover"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={() => void run()}
                class="rounded-lg bg-brand px-4 py-2 text-body-sm font-semibold text-surface hover:brightness-110"
              >
                {props.mode === "setup-remote" ? "Start Setup" : "Start Deploy"}
              </button>
            </Match>
            <Match when={stage() === "running"}>
              <p class="text-sm text-fg-muted">Deploying...</p>
            </Match>
            <Match when={stage() === "done"}>
              <button
                type="button"
                onClick={props.onComplete}
                class="rounded-lg bg-brand px-4 py-2 text-body-sm font-semibold text-surface hover:brightness-110"
              >
                Done
              </button>
            </Match>
            <Match when={stage() === "error"}>
              <button
                type="button"
                onClick={props.onClose}
                class="rounded-lg border border-border bg-surface-raised px-4 py-2 text-body-sm font-medium text-fg-secondary hover:bg-hover"
              >
                Close
              </button>
            </Match>
          </Switch>
        </div>
      </div>
    </div>
  );
}

export default TunnelDeployModal;
