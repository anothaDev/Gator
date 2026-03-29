import { For, Show, createMemo, createSignal, onMount } from "solid-js";

import Badge from "../components/Badge";
import Button from "../components/Button";
import Card from "../components/Card";
import Input from "../components/Input";
import DropdownMenu from "../components/DropdownMenu";
import type { MenuEntry } from "../components/DropdownMenu";
import { ErrorStateCard, LoadingStateCard } from "../components/PageState";
import { apiDelete, apiGet, apiPost, getOpnsenseHost } from "../lib/api";

type TailscaleStatus = {
  installed: boolean;
  configured?: boolean;
  login_server?: string;
  service_enabled?: boolean;
  service_running?: boolean;
  service_status?: string;
  tailscale_ip?: string;
  message?: string;
  opnsense_host?: string;
  interface?: {
    found: boolean;
    assigned: boolean;
    enabled: boolean;
    device?: string;
    identifier?: string;
    description?: string;
    status?: string;
  };
};

async function fetchStatus(): Promise<TailscaleStatus> {
  const { ok, data } = await apiGet<TailscaleStatus & { error?: string }>("/api/opnsense/tailscale/status");
  if (!ok) throw new Error(data.error ?? "Failed to load Tailscale status");
  return data;
}

export default function Tailscale() {
  const [status, setStatus] = createSignal<TailscaleStatus | null>(null);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal("");
  const [actionError, setActionError] = createSignal("");
  const [installing, setInstalling] = createSignal(false);
  const [configuring, setConfiguring] = createSignal(false);
  const [preAuthKey, setPreAuthKey] = createSignal("");
  const [loginServer, setLoginServer] = createSignal("https://controlplane.tailscale.com");

  const canConfigure = createMemo(() => status()?.installed && preAuthKey().trim() !== "");
  const iface = () => status()?.interface;
  const [opnHost, setOpnHost] = createSignal("");

  const loadStatus = async () => {
    setLoading(true);
    setError("");
    try {
      const data = await fetchStatus();
      setStatus(data);
      if (data.login_server) {
        setLoginServer(data.login_server);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load Tailscale status");
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    void loadStatus();
    void getOpnsenseHost().then(setOpnHost);
  });

  const [installProgress, setInstallProgress] = createSignal("");

  type InstallStatusResponse = {
    firmware_running: boolean;
    firmware_status: string;
    firmware_log: string;
    plugin_ready: boolean;
  };

  const handleInstall = async () => {
    setInstalling(true);
    setActionError("");
    setInstallProgress("Requesting plugin install...");
    try {
      const { ok, data } = await apiPost<{ error?: string }>("/api/opnsense/tailscale/install");
      if (!ok) {
        setActionError(data.error ?? "Failed to install Tailscale plugin.");
        return;
      }

      // Poll firmware install progress until done
      setInstallProgress("Install started. Waiting for OPNsense firmware task...");
      const maxAttempts = 40; // ~3.3 minutes at 5s intervals
      for (let i = 1; i <= maxAttempts; i++) {
        await new Promise((r) => setTimeout(r, 5000));
        const { ok: pollOk, data: progress } = await apiGet<InstallStatusResponse>(
          "/api/opnsense/tailscale/install-status",
        );
        if (!pollOk) continue;

        if (progress.plugin_ready) {
          setInstallProgress("");
          await loadStatus();
          return;
        }

        if (progress.firmware_status === "done" && !progress.plugin_ready) {
          // Firmware task finished but plugin API isn't responding -- install likely failed
          setInstallProgress("");
          const logTail = (progress.firmware_log || "").slice(-500);
          setActionError(
            `Plugin install completed but os-tailscale is not available. Check OPNsense firmware logs.\n${logTail}`,
          );
          return;
        }

        if (progress.firmware_status === "error") {
          setInstallProgress("");
          setActionError("Firmware install failed. Check System > Firmware in OPNsense for details.");
          return;
        }

        if (progress.firmware_running) {
          setInstallProgress(`Firmware task running... (${i}/${maxAttempts})`);
        } else {
          setInstallProgress(`Waiting for plugin to become available... (${i}/${maxAttempts})`);
        }
      }

      // Timed out
      setInstallProgress("");
      await loadStatus();
      setActionError("Install is taking longer than expected. Try clicking Refresh in a minute.");
    } finally {
      setInstalling(false);
      setInstallProgress("");
    }
  };

  const handleConfigure = async () => {
    setConfiguring(true);
    setActionError("");
    try {
      const { ok, data } = await apiPost<{ error?: string }>("/api/opnsense/tailscale/configure", {
        pre_auth_key: preAuthKey(),
        login_server: loginServer(),
      });
      if (!ok) {
        setActionError(data.error ?? "Failed to configure Tailscale.");
        return;
      }
      setPreAuthKey("");
      await loadStatus();
    } finally {
      setConfiguring(false);
    }
  };

  return (
    <div class="space-y-5">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h1 class="text-title-h2 font-semibold tracking-tight text-fg">Tailscale</h1>
          <p class="mt-1 text-body-sm text-fg-muted">
            Guided setup for the OPNsense Tailscale community plugin.
          </p>
        </div>
        <DropdownMenu items={(() => {
          const items: MenuEntry[] = [
            { label: "Refresh", onClick: () => void loadStatus(), loading: loading() },
          ];
          if (opnHost()) {
            items.push({ divider: true as const });
            items.push({ label: "Open in OPNsense", href: `${opnHost()}/ui/tailscale/settings`, external: true });
          }
          return items;
        })()} />
      </div>

      <Show when={loading()}>
        <LoadingStateCard message="Checking Tailscale status..." />
      </Show>

      <Show when={error() !== ""}>
        <ErrorStateCard message={error()} />
      </Show>

      <Show when={!loading() && error() === "" && status()}>
          <>
            <Card variant="elevated" class="border-l-4 border-l-accent">
              <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <div class="flex flex-wrap items-center gap-2">
                    <h2 class="text-lg font-semibold text-fg">Setup status</h2>
                    <Badge variant={status()!.installed ? "success" : "warning"} size="sm">
                      {status()!.installed ? "plugin installed" : "plugin missing"}
                    </Badge>
                    <Show when={status()!.configured}>
                      <Badge variant="info" size="sm">configured</Badge>
                    </Show>
                    <Show when={status()!.service_running}>
                      <Badge variant="success" size="sm">service running</Badge>
                    </Show>
                  </div>
                  <p class="mt-2 text-sm text-fg-secondary">{status()!.message}</p>
                </div>

                <div class="grid grid-cols-2 gap-3 text-sm lg:min-w-[300px]">
                  <div class="rounded-md border border-border-faint bg-surface px-3 py-2">
                    <div class="text-xs uppercase tracking-wider text-fg-muted">Service</div>
                    <div class="mt-1 font-medium text-fg">{status()!.service_status || "not installed"}</div>
                  </div>
                  <div class="rounded-md border border-border-faint bg-surface px-3 py-2">
                    <div class="text-xs uppercase tracking-wider text-fg-muted">Tailscale IP</div>
                    <div class="mt-1 font-mono text-fg">{status()!.tailscale_ip || "--"}</div>
                  </div>
                </div>
              </div>
            </Card>

            <Show when={actionError() !== ""}>
              <ErrorStateCard message={actionError()} />
            </Show>

            {/* Setup steps: only show the current step that needs action */}
            <Show when={!status()!.installed}>
              <Card>
                <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                  <div>
                    <h3 class="font-semibold text-fg">Install plugin</h3>
                    <p class="mt-2 text-sm text-fg-secondary">
                      The OPNsense Tailscale community plugin needs to be installed before setup can continue.
                    </p>
                  </div>
                  <Button variant="primary" size="lg" loading={installing()} onClick={() => void handleInstall()}>
                    Install Tailscale
                  </Button>
                </div>
                <Show when={installProgress()}>
                  <p class="mt-3 text-sm text-fg-secondary">{installProgress()}</p>
                </Show>
              </Card>
            </Show>

            <Show when={status()!.installed && !status()!.configured}>
              <Card>
                <h3 class="font-semibold text-fg">Configure authentication</h3>
                <p class="mt-2 text-sm text-fg-secondary">
                  Gator enables Tailscale with the default control plane and your pre-authentication key.
                </p>

                <div class="mt-4 grid gap-4 lg:grid-cols-2">
                  <Input
                    label="Pre-authentication key"
                    type="password"
                    value={preAuthKey()}
                    onInput={setPreAuthKey}
                    placeholder="tskey-auth-..."
                    hint="Required. This key is sent directly to OPNsense and not stored by Gator."
                  />
                  <Input
                    label="Login server"
                    type="url"
                    value={loginServer()}
                    onInput={setLoginServer}
                    placeholder="https://controlplane.tailscale.com"
                    hint="Leave the default unless you are using a custom control plane." 
                  />
                </div>

                <div class="mt-4 flex justify-end">
                  <Button variant="primary" size="lg" disabled={!canConfigure()} loading={configuring()} onClick={() => void handleConfigure()}>
                    Apply Tailscale config
                  </Button>
                </div>
              </Card>
            </Show>

            <Show when={status()!.installed && status()!.configured && !(iface()?.found && iface()?.assigned && iface()?.enabled)}>
              <Card variant="elevated" class="border-l-4 border-l-warning">
                <div class="flex items-start justify-between gap-4">
                  <div>
                    <h3 class="font-semibold text-fg">Manual configuration needed</h3>
                    <p class="mt-1 text-sm text-fg-secondary">
                      Tailscale is installed and running, but OPNsense needs the interface assigned manually. Gator can't do this programmatically yet.
                    </p>
                  </div>
                  <Show when={status()!.opnsense_host}>
                    <a
                      href={`${status()!.opnsense_host}/interfaces_assign.php`}
                      target="_blank"
                      rel="noopener noreferrer"
                      class="shrink-0 rounded-md border border-warning/30 bg-warning/10 px-3 py-1.5 text-label-sm font-medium text-warning hover:bg-warning/20"
                    >
                      Open Assignments
                    </a>
                  </Show>
                </div>
                <div class="mt-4 grid gap-2 text-sm">
                  <StepItem done={!!iface()?.found} text="tailscale0 device detected" />
                  <StepItem done={!!iface()?.assigned} text="Assign tailscale0 in Interfaces > Assignments" />
                  <StepItem done={!!iface()?.enabled} text="Enable the assigned interface" />
                </div>
                <p class="mt-4 text-xs text-fg-muted">
                  After completing these steps, click Refresh above.
                </p>
              </Card>
            </Show>

            <Show when={status()!.installed && status()!.configured && iface()?.found && iface()?.assigned && iface()?.enabled}>
              <Card variant="elevated" class="border-l-4 border-l-success">
                <h3 class="font-semibold text-fg">Setup complete</h3>
                <p class="mt-1 text-sm text-fg-secondary">
                  Tailscale is installed, configured, and the interface is assigned and enabled.
                </p>
              </Card>
            </Show>

            <Show when={status()!.installed && status()!.configured}>
              <AdvertisedRoutes />
            </Show>
          </>
      </Show>
    </div>
  );
}

type Subnet = { uuid: string; subnet: string; description: string };

function AdvertisedRoutes() {
  const [subnets, setSubnets] = createSignal<Subnet[]>([]);
  const [loadingRoutes, setLoadingRoutes] = createSignal(true);
  const [subnet, setSubnet] = createSignal("");
  const [description, setDescription] = createSignal("");
  const [adding, setAdding] = createSignal(false);
  const [deleting, setDeleting] = createSignal<string | null>(null);
  const [routeError, setRouteError] = createSignal("");

  const loadSubnets = async () => {
    setLoadingRoutes(true);
    try {
      const { ok, data } = await apiGet<{ rows: Subnet[] }>("/api/opnsense/tailscale/subnets");
      if (ok) setSubnets(data.rows ?? []);
    } finally {
      setLoadingRoutes(false);
    }
  };

  onMount(() => void loadSubnets());

  const handleAdd = async () => {
    const value = subnet().trim();
    if (!value) return;
    setAdding(true);
    setRouteError("");
    try {
      const { ok, data } = await apiPost<{ error?: string }>("/api/opnsense/tailscale/subnets", {
        subnet: value,
        description: description().trim(),
      });
      if (!ok) {
        setRouteError(data.error ?? "Failed to add subnet.");
        return;
      }
      setSubnet("");
      setDescription("");
      await loadSubnets();
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = async (uuid: string) => {
    setDeleting(uuid);
    setRouteError("");
    try {
      const { ok, data } = await apiDelete<{ error?: string }>(`/api/opnsense/tailscale/subnets/${uuid}`);
      if (!ok) {
        setRouteError(data.error ?? "Failed to delete subnet.");
        return;
      }
      await loadSubnets();
    } finally {
      setDeleting(null);
    }
  };

  return (
    <Card>
      <h3 class="font-semibold text-fg">Advertised routes</h3>
      <p class="mt-1 text-sm text-fg-secondary">
        Subnets this OPNsense node advertises to your Tailscale network.
      </p>

      <Show when={routeError()}>
        <p class="mt-3 text-sm text-error">{routeError()}</p>
      </Show>

      <div class="mt-4">
        <Show when={!loadingRoutes()} fallback={<p class="text-sm text-fg-muted">Loading...</p>}>
          <Show
            when={subnets().length > 0}
            fallback={<p class="text-sm text-fg-muted">No advertised routes configured.</p>}
          >
            <div class="space-y-2">
              <For each={subnets()}>
                {(entry) => (
                  <div class="flex items-center justify-between gap-3 rounded-md border border-border-faint bg-surface px-3 py-2">
                    <div class="min-w-0">
                      <span class="font-mono text-sm text-fg">{entry.subnet}</span>
                      <Show when={entry.description}>
                        <span class="ml-2 text-xs text-fg-muted">{entry.description}</span>
                      </Show>
                    </div>
                    <button
                      class="shrink-0 rounded p-1 text-fg-muted hover:bg-surface-raised hover:text-error"
                      onClick={() => void handleDelete(entry.uuid)}
                      disabled={deleting() === entry.uuid}
                      title="Remove route"
                    >
                      <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M18 6L6 18M6 6l12 12" />
                      </svg>
                    </button>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Show>
      </div>

      <div class="mt-4 grid grid-cols-[1fr_1fr_auto] items-end gap-3">
        <Input
          label="Subnet"
          value={subnet()}
          onInput={setSubnet}
          placeholder="192.168.1.0/24"
        />
        <Input
          label="Description"
          value={description()}
          onInput={setDescription}
          placeholder="LAN (optional)"
        />
        <Button variant="primary" size="md" disabled={!subnet().trim()} loading={adding()} onClick={() => void handleAdd()}>
          Add
        </Button>
      </div>
    </Card>
  );
}

function StepItem(props: { done: boolean; text: string }) {
  return (
    <div class="flex items-center gap-2.5">
      <div class={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full ${props.done ? "bg-success/20 text-success" : "border-transparent text-fg-muted"}`}>
        <Show
          when={props.done}
          fallback={<div class="h-1.5 w-1.5 rounded-full bg-current" />}
        >
          <svg class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
            <polyline points="20 6 9 17 4 12" />
          </svg>
        </Show>
      </div>
      <span class={props.done ? "text-fg-muted line-through" : "text-fg"}>
        {props.text}
      </span>
    </div>
  );
}
