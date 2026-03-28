import { Suspense, createEffect, createSignal, For, Match, Show, Switch, lazy, onMount } from "solid-js";
import type { JSX } from "solid-js";
import Button from "../components/Button";
import IconButton from "../components/IconButton";
import { apiGet, apiPost } from "../lib/api";

const Aliases = lazy(() => import("./Aliases"));
const Backups = lazy(() => import("./Backups"));
const Dashboard = lazy(() => import("./Dashboard"));
const Gateways = lazy(() => import("./Gateways"));
const Interfaces = lazy(() => import("./Interfaces"));
const Migration = lazy(() => import("./Migration"));
const Nat = lazy(() => import("./Nat"));
const Routing = lazy(() => import("./Routing"));
const Rules = lazy(() => import("./Rules"));
const Tailscale = lazy(() => import("./Tailscale"));
const Tunnels = lazy(() => import("./Tunnels"));
const VpnSetup = lazy(() => import("./VpnSetup"));

type InstanceRuntimeState = {
  connected: boolean;
  message?: string;
};

type Props = {
  onReconfigure: () => void;
  onInstanceSwitched: () => void;
  onLogout: () => void;
};

type Section =
  | "dashboard"
  | "vpn"
  | "tailscale"
  | "tunnels"
  | "routing"
  | "interfaces"
  | "gateways"
  | "aliases"
  | "nat"
  | "rules"
  | "migration"
  | "backups";

type Instance = {
  id: number;
  label: string;
  type: string;
  host: string;
  active: boolean;
};

interface NavItem {
  id: Section;
  label: string;
  icon: JSX.Element;
}

const navItems: NavItem[] = [
  {
    id: "dashboard",
    label: "Dashboard",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <rect x="3" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="14" width="7" height="7" rx="1" />
        <rect x="3" y="14" width="7" height="7" rx="1" />
      </svg>
    ),
  },
  {
    id: "vpn",
    label: "VPN",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
      </svg>
    ),
  },
  {
    id: "tailscale",
    label: "Tailscale",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor">
        <circle cx="8" cy="8" r="3" />
        <circle cx="16" cy="8" r="3" />
        <circle cx="8" cy="16" r="3" />
        <circle cx="16" cy="16" r="3" />
      </svg>
    ),
  },
  {
    id: "tunnels",
    label: "Tunnels",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="3" />
        <path d="M12 1v6m0 6v6m11-7h-6m-6 0H1" />
      </svg>
    ),
  },
  {
    id: "routing",
    label: "Routing",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polygon points="12 2 2 7 12 12 22 7 12 2" />
        <polyline points="2 17 12 22 22 17" />
        <polyline points="2 12 12 17 22 12" />
      </svg>
    ),
  },
  {
    id: "interfaces",
    label: "Interfaces",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <rect x="2" y="4" width="20" height="16" rx="2" />
        <path d="M6 8h.01M6 12h.01M6 16h.01" />
      </svg>
    ),
  },
  {
    id: "gateways",
    label: "Gateways",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
        <circle cx="12" cy="12" r="3" />
      </svg>
    ),
  },
  {
    id: "aliases",
    label: "Aliases",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
        <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
      </svg>
    ),
  },
  {
    id: "nat",
    label: "NAT",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
        <polyline points="22,6 12,13 2,6" />
      </svg>
    ),
  },
  {
    id: "rules",
    label: "Rules",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
        <polyline points="14,2 14,8 20,8" />
        <line x1="16" y1="13" x2="8" y2="13" />
        <line x1="16" y1="17" x2="8" y2="17" />
        <polyline points="10,9 9,9 8,9" />
      </svg>
    ),
  },
  {
    id: "migration",
    label: "Migration",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
      </svg>
    ),
  },
  {
    id: "backups",
    label: "Backups",
    icon: (
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
        <polyline points="7,10 12,15 17,10" />
        <line x1="12" y1="15" x2="12" y2="3" />
      </svg>
    ),
  },
];

export default function ControlCenter(props: Props) {
  const [section, setSection] = createSignal<Section>("dashboard");
  const [instances, setInstances] = createSignal<Instance[]>([]);
  const [switcherOpen, setSwitcherOpen] = createSignal(false);
  const [switching, setSwitching] = createSignal(false);
  const [mobileNavOpen, setMobileNavOpen] = createSignal(false);
  const [runtimeState, setRuntimeState] = createSignal<InstanceRuntimeState | null>(null);
  const [firmwareUpdate, setFirmwareUpdate] = createSignal<{
    needs_update: boolean;
    latest_version: string;
    current_version: string;
    status_msg: string;
    needs_reboot: boolean;
  } | null>(null);
  const [updateDismissed, setUpdateDismissed] = createSignal(false);

  const activeInstance = () => instances().find((i) => i.active);
  const instanceUnavailable = () => runtimeState()?.connected === false;
  const isSectionDisabled = (item: Section) => item !== "dashboard" && instanceUnavailable();

  createEffect(() => {
    if (instanceUnavailable() && section() !== "dashboard") {
      setSection("dashboard");
    }
  });

  const loadInstances = async () => {
    try {
      const { ok, data } = await apiGet<{ instances?: Instance[] }>("/api/instances");
      if (ok) setInstances(data.instances ?? []);
    } catch {}
  };

  const checkFirmwareUpdate = async () => {
    try {
      const { ok, data } = await apiGet<{
        needs_update: boolean;
        latest_version: string;
        current_version: string;
        status_msg: string;
        needs_reboot: boolean;
      }>("/api/opnsense/firmware-status");
      if (ok) setFirmwareUpdate(data);
    } catch {}
  };

  onMount(() => {
    void loadInstances();
    void checkFirmwareUpdate();
  });

  const switchInstance = async (id: number) => {
    setSwitching(true);
    try {
      const { ok } = await apiPost(`/api/instances/${id}/activate`);
      if (ok) {
        setInstances((prev) => prev.map((inst) => ({ ...inst, active: inst.id === id })));
        setRuntimeState(null);
        props.onInstanceSwitched();
      }
    } catch {}
    finally {
      setSwitching(false);
      setSwitcherOpen(false);
    }
  };

  const handleLogout = async () => {
    await apiPost("/api/auth/logout");
    props.onLogout();
  };

  const getInstanceIcon = (type: string) => {
    const baseClasses = "h-2.5 w-2.5 rounded-full";
    if (type === "opnsense") {
      return <span class={`${baseClasses} bg-warning`} />;
    }
    return <span class={`${baseClasses} bg-info`} />;
  };

  return (
    <div class="min-h-screen bg-surface text-fg">
      {/* Ambient background gradient */}
      <div
        class="pointer-events-none fixed inset-0 opacity-[0.03]"
        style={{
          background: `
            radial-gradient(circle at 20% 20%, rgba(34, 197, 94, 0.25), transparent 40%),
            radial-gradient(circle at 80% 10%, rgba(96, 165, 250, 0.2), transparent 35%),
            radial-gradient(circle at 40% 80%, rgba(74, 222, 128, 0.12), transparent 50%)
          `,
        }}
      />

      {/* Header */}
      <header class="sticky top-0 z-50 border-b border-line bg-surface-secondary/95 backdrop-blur">
        <div class="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 lg:px-6">
          {/* Logo & Title */}
          <div class="flex items-center gap-3">
            {/* Mobile menu toggle */}
            <IconButton
              variant="ghost"
              size="md"
              class="lg:hidden"
              onClick={() => setMobileNavOpen((v) => !v)}
              title="Toggle navigation"
            >
              <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M3 12h18M3 6h18M3 18h18" />
              </svg>
            </IconButton>

            {/* Logo */}
            <div class="flex items-center gap-3">
              <img src="/gator64px.svg" alt="Gator logo" class="h-[3.3rem] w-auto max-w-none object-contain sm:h-[3.7rem]" />
              <div class="hidden sm:block">
                <h1 class="text-lg font-semibold tracking-tight">Gator</h1>
                <p class="text-xs text-fg-tertiary">Firewall control plane</p>
              </div>
            </div>
          </div>

          {/* Instance switcher & Actions */}
          <div class="flex items-center gap-2">
            {/* Instance switcher */}
            <Show when={instances().length > 0}>
              <div class="relative">
                <button
                  type="button"
                  onClick={() => setSwitcherOpen((v) => !v)}
                  disabled={switching()}
                  class={[
                    "flex items-center gap-2 rounded-lg border px-3 py-1.5",
                    "text-sm font-medium transition-all duration-base",
                    instanceUnavailable()
                      ? "border-error/40 bg-error-subtle text-error"
                      : "",
                    switcherOpen()
                      ? "border-accent bg-accent-subtle text-accent"
                      : "border-line-strong bg-surface-tertiary text-fg-secondary hover:border-line-focus hover:text-fg",
                  ].join(" ")}
                >
                  {getInstanceIcon(activeInstance()?.type ?? "")}
                  <span class="hidden max-w-[140px] truncate sm:inline">
                    {activeInstance()?.label ?? "No instance"}
                  </span>
                  <Show when={instanceUnavailable()}>
                    <span class="hidden rounded-full bg-error/10 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-error sm:inline">
                      Down
                    </span>
                  </Show>
                  <Show when={instances().length > 1}>
                    <svg
                      class={[
                        "h-3.5 w-3.5 text-fg-tertiary transition-transform duration-base",
                        switcherOpen() ? "rotate-180" : "",
                      ].join(" ")}
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2"
                    >
                      <path d="M6 9l6 6 6-6" />
                    </svg>
                  </Show>
                </button>

                {/* Instance dropdown */}
                <Show when={switcherOpen()}>
                  <div
                    class="absolute right-0 top-full z-50 mt-2 w-72 rounded-xl border border-line-strong bg-elevated p-2 shadow-lg"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <p class="px-2 pb-2 text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
                      Firewall instances
                    </p>
                    <div class="space-y-1">
                      <For each={instances()}>
                        {(inst) => (
                          <button
                            type="button"
                            disabled={inst.active || switching()}
                            onClick={() => void switchInstance(inst.id)}
                            class={[
                              "flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left",
                              "text-sm transition-all duration-fast",
                              inst.active
                                ? "bg-accent-subtle text-accent"
                                : "text-fg-secondary hover:bg-hover hover:text-fg",
                            ].join(" ")}
                          >
                            {getInstanceIcon(inst.type)}
                            <div class="min-w-0 flex-1">
                              <p class="truncate font-medium">{inst.label}</p>
                              <p class="truncate text-xs text-fg-tertiary">
                                {inst.type}
                              </p>
                            </div>
                            <Show when={inst.active}>
                              <span class="shrink-0 text-xs font-semibold uppercase text-accent">
                                active
                              </span>
                            </Show>
                          </button>
                        )}
                      </For>
                    </div>
                    <div class="mt-2 border-t border-line-faint pt-2">
                      <button
                        type="button"
                        onClick={() => {
                          setSwitcherOpen(false);
                          props.onReconfigure();
                        }}
                        class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-fg-tertiary transition-colors duration-fast hover:bg-hover hover:text-fg-secondary"
                      >
                        <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                          <path d="M12 5v14M5 12h14" />
                        </svg>
                        Add new instance
                      </button>
                    </div>
                  </div>
                </Show>
              </div>
            </Show>

            {/* Reconfigure button */}
            <Button variant="secondary" size="sm" onClick={props.onReconfigure}>
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 20h9M12 20a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h7a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2M12 20v2M3 8l2.5-2.5L8 8M3 8h5M8 8v5" />
              </svg>
              <span class="hidden sm:inline">Reconfigure</span>
            </Button>

            <Button variant="ghost" size="sm" onClick={() => void handleLogout()}>
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
                <polyline points="16 17 21 12 16 7" />
                <line x1="21" y1="12" x2="9" y2="12" />
              </svg>
              <span class="hidden sm:inline">Logout</span>
            </Button>
          </div>
        </div>
      </header>

      <Show when={instanceUnavailable()}>
        <div class="border-b border-error/20 bg-error-subtle/70">
          <div class="mx-auto flex max-w-7xl items-start gap-3 px-4 py-3 text-[13px] lg:px-6">
            <svg class="mt-0.5 h-4 w-4 shrink-0 text-error" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <div>
              <p class="font-medium text-error">
                {activeInstance()?.label ?? "This instance"} is currently unreachable.
              </p>
              <p class="mt-0.5 text-fg-secondary">
                {runtimeState()?.message ?? "Gator cannot manage this firewall until it comes back online."}
              </p>
            </div>
          </div>
        </div>
      </Show>

      <Show when={firmwareUpdate()?.needs_update && !updateDismissed()}>
        <div class="border-b border-warning/20 bg-warning-subtle/70">
          <div class="mx-auto flex max-w-7xl items-center gap-3 px-4 py-3 text-[13px] lg:px-6">
            <svg class="h-4 w-4 shrink-0 text-warning" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 9v4M12 17h.01" />
              <path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
            </svg>
            <div class="flex-1">
              <p class="font-medium text-warning">
                OPNsense update required
              </p>
              <p class="mt-0.5 text-fg-secondary">
                {firmwareUpdate()!.current_version ? `Running ${firmwareUpdate()!.current_version}. ` : ""}
                {firmwareUpdate()!.latest_version && firmwareUpdate()!.latest_version !== "available"
                  ? `Version ${firmwareUpdate()!.latest_version} is available. `
                  : firmwareUpdate()!.status_msg
                    ? `${firmwareUpdate()!.status_msg} `
                    : "Updates are available. "}
                Apply in <span class="font-medium">System &gt; Firmware &gt; Updates</span> on OPNsense.
                {firmwareUpdate()!.needs_reboot ? " A reboot will be required." : ""}
              </p>
            </div>
            <Show when={activeInstance()?.host}>
              <a
                href={`${activeInstance()!.host}/ui/core/firmware#updates`}
                target="_blank"
                rel="noopener noreferrer"
                class="shrink-0 rounded-md border border-warning/30 bg-warning/10 px-3 py-1.5 text-[12px] font-medium text-warning hover:bg-warning/20"
              >
                Open Updates
              </a>
            </Show>
            <button
              class="shrink-0 rounded p-1 text-fg-tertiary hover:bg-surface-tertiary hover:text-fg"
              onClick={() => setUpdateDismissed(true)}
              title="Dismiss"
            >
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      </Show>

      {/* Click outside to close switcher */}
      <Show when={switcherOpen()}>
        <div class="fixed inset-0 z-40" onClick={() => setSwitcherOpen(false)} />
      </Show>

      {/* Mobile navigation overlay */}
      <Show when={mobileNavOpen()}>
        <div
          class="fixed inset-0 z-30 bg-surface/80 backdrop-blur-sm lg:hidden"
          onClick={() => setMobileNavOpen(false)}
        />
      </Show>

      {/* Main layout */}
      <div class="flex">
        {/* Sidebar navigation - fixed width */}
        <aside
          class={[
            "fixed left-0 top-16 z-40 h-[calc(100vh-4rem)] w-56 shrink-0 border-r border-line bg-surface-secondary",
            "transform transition-transform duration-slow ease-out lg:translate-x-0",
            mobileNavOpen() ? "translate-x-0" : "-translate-x-full",
          ].join(" ")}
        >
          <nav class="h-full overflow-y-auto py-3">
            <div class="space-y-0.5 px-2">
              <For each={navItems}>
                {(item) => {
                  const isActive = () => section() === item.id;
                  return (
                    <button
                      type="button"
                      onClick={() => {
                        setSection(item.id);
                        setMobileNavOpen(false);
                      }}
                      class={[
                        "relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5",
                        "text-sm font-medium transition-all duration-200",
                        isSectionDisabled(item.id) ? "cursor-not-allowed opacity-40" : "",
                        isActive()
                          ? "bg-accent/8 text-accent"
                          : "text-fg-secondary hover:bg-hover hover:text-fg",
                      ].join(" ")}
                      disabled={isSectionDisabled(item.id)}
                    >
                      <span class={[
                        "transition-colors duration-200",
                        isActive() ? "text-accent" : "text-fg-tertiary",
                      ].join(" ")}>
                        {item.icon}
                      </span>
                      {item.label}
                      <Show when={isActive()}>
                        <span class="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-accent rounded-r-full" />
                      </Show>
                    </button>
                  );
                }}
              </For>
            </div>
          </nav>
        </aside>

        {/* Main content area - constrained width */}
        <main class="flex-1 min-w-0 p-4 lg:ml-56 lg:p-6">
          <div class="mx-auto max-w-5xl min-h-[calc(100vh-7rem)]">
            <Suspense
              fallback={
                <div class="flex min-h-[240px] items-center justify-center text-sm text-fg-tertiary">
                  Loading section...
                </div>
              }
            >
              <Switch>
                <Match when={section() === "dashboard"}>
                  <Dashboard onConnectionStateChange={setRuntimeState} />
                </Match>
                <Match when={section() === "vpn"}>
                  <VpnSetup onNavigate={(s) => setSection(s as Section)} />
                </Match>
                <Match when={section() === "tailscale"}>
                  <Tailscale />
                </Match>
                <Match when={section() === "tunnels"}>
                  <Tunnels />
                </Match>
                <Match when={section() === "routing"}>
                  <Routing />
                </Match>
                <Match when={section() === "interfaces"}>
                  <Interfaces />
                </Match>
                <Match when={section() === "gateways"}>
                  <Gateways />
                </Match>
                <Match when={section() === "aliases"}>
                  <Aliases />
                </Match>
                <Match when={section() === "nat"}>
                  <Nat />
                </Match>
                <Match when={section() === "rules"}>
                  <Rules onNavigate={(s) => setSection(s as Section)} />
                </Match>
                <Match when={section() === "migration"}>
                  <Migration />
                </Match>
                <Match when={section() === "backups"}>
                  <Backups />
                </Match>
              </Switch>
            </Suspense>
          </div>
        </main>
      </div>
    </div>
  );
}
