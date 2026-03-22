import { Suspense, createSignal, For, Match, Show, Switch, lazy, onMount } from "solid-js";
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
const Tunnels = lazy(() => import("./Tunnels"));
const VpnSetup = lazy(() => import("./VpnSetup"));

type Props = {
  onReconfigure: () => void;
  onInstanceSwitched: () => void;
};

type Section =
  | "dashboard"
  | "vpn"
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

  const activeInstance = () => instances().find((i) => i.active);

  const loadInstances = async () => {
    try {
      const { ok, data } = await apiGet<{ instances?: Instance[] }>("/api/instances");
      if (ok) setInstances(data.instances ?? []);
    } catch {}
  };

  onMount(() => {
    void loadInstances();
  });

  const switchInstance = async (id: number) => {
    setSwitching(true);
    try {
      const { ok } = await apiPost(`/api/instances/${id}/activate`);
      if (ok) {
        setInstances((prev) => prev.map((inst) => ({ ...inst, active: inst.id === id })));
        props.onInstanceSwitched();
      }
    } catch {}
    finally {
      setSwitching(false);
      setSwitcherOpen(false);
    }
  };

  const getInstanceIcon = (type: string) => {
    const baseClasses = "h-2.5 w-2.5 rounded-full";
    if (type === "opnsense") {
      return <span class={`${baseClasses} bg-[var(--status-warning)]`} />;
    }
    return <span class={`${baseClasses} bg-[var(--status-info)]`} />;
  };

  return (
    <div class="min-h-screen bg-[var(--bg-primary)] text-[var(--text-primary)]">
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
      <header class="sticky top-0 z-50 border-b border-[var(--border-default)] bg-[var(--bg-secondary)]/95 backdrop-blur">
        <div class="mx-auto flex h-14 max-w-7xl items-center justify-between px-4 lg:px-6">
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
            <div class="flex items-center gap-2">
              <div class="flex h-7 w-7 items-center justify-center rounded-[var(--radius-md)] bg-gradient-to-br from-[var(--accent-primary)] to-[var(--accent-primary-muted)]">
                <svg class="h-4 w-4 text-[var(--bg-primary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
                </svg>
              </div>
              <div class="hidden sm:block">
                <h1 class="text-[var(--text-lg)] font-semibold tracking-tight">gator</h1>
                <p class="text-[var(--text-xs)] text-[var(--text-tertiary)]">VPN control center</p>
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
                    "flex items-center gap-2 rounded-[var(--radius-lg)] border px-3 py-1.5",
                    "text-[var(--text-sm)] font-medium transition-all duration-[var(--transition-base)]",
                    switcherOpen()
                      ? "border-[var(--accent-primary)] bg-[var(--accent-primary-subtle)] text-[var(--accent-primary)]"
                      : "border-[var(--border-strong)] bg-[var(--bg-tertiary)] text-[var(--text-secondary)] hover:border-[var(--border-focus)] hover:text-[var(--text-primary)]",
                  ].join(" ")}
                >
                  {getInstanceIcon(activeInstance()?.type ?? "")}
                  <span class="hidden max-w-[140px] truncate sm:inline">
                    {activeInstance()?.label ?? "No instance"}
                  </span>
                  <Show when={instances().length > 1}>
                    <svg
                      class={[
                        "h-3.5 w-3.5 text-[var(--text-tertiary)] transition-transform duration-[var(--transition-base)]",
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
                    class="absolute right-0 top-full z-50 mt-2 w-72 rounded-[var(--radius-xl)] border border-[var(--border-strong)] bg-[var(--bg-elevated)] p-2 shadow-[var(--shadow-lg)]"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <p class="px-2 pb-2 text-[var(--text-xs)] font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
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
                              "flex w-full items-center gap-2.5 rounded-[var(--radius-lg)] px-3 py-2 text-left",
                              "text-[var(--text-sm)] transition-all duration-[var(--transition-fast)]",
                              inst.active
                                ? "bg-[var(--accent-primary-subtle)] text-[var(--accent-primary)]"
                                : "text-[var(--text-secondary)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]",
                            ].join(" ")}
                          >
                            {getInstanceIcon(inst.type)}
                            <div class="min-w-0 flex-1">
                              <p class="truncate font-medium">{inst.label}</p>
                              <p class="truncate text-[var(--text-xs)] text-[var(--text-tertiary)]">
                                {inst.type}
                              </p>
                            </div>
                            <Show when={inst.active}>
                              <span class="shrink-0 text-[var(--text-xs)] font-semibold uppercase text-[var(--accent-primary)]">
                                active
                              </span>
                            </Show>
                          </button>
                        )}
                      </For>
                    </div>
                    <div class="mt-2 border-t border-[var(--border-subtle)] pt-2">
                      <button
                        type="button"
                        onClick={() => {
                          setSwitcherOpen(false);
                          props.onReconfigure();
                        }}
                        class="flex w-full items-center gap-2 rounded-[var(--radius-lg)] px-3 py-2 text-[var(--text-sm)] text-[var(--text-tertiary)] transition-colors duration-[var(--transition-fast)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-secondary)]"
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
          </div>
        </div>
      </header>

      {/* Click outside to close switcher */}
      <Show when={switcherOpen()}>
        <div class="fixed inset-0 z-40" onClick={() => setSwitcherOpen(false)} />
      </Show>

      {/* Mobile navigation overlay */}
      <Show when={mobileNavOpen()}>
        <div
          class="fixed inset-0 z-30 bg-[var(--bg-primary)]/80 backdrop-blur-sm lg:hidden"
          onClick={() => setMobileNavOpen(false)}
        />
      </Show>

      {/* Main layout */}
      <div class="flex">
        {/* Sidebar navigation - fixed width */}
        <aside
          class={[
            "fixed left-0 top-14 z-40 h-[calc(100vh-3.5rem)] w-56 shrink-0 border-r border-[var(--border-default)] bg-[var(--bg-secondary)]",
            "transform transition-transform duration-[var(--transition-slow)] ease-out lg:translate-x-0",
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
                        "text-[var(--text-sm)] font-medium transition-all duration-200",
                        isActive()
                          ? "bg-[var(--accent-primary)]/8 text-[var(--accent-primary)]"
                          : "text-[var(--text-secondary)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]",
                      ].join(" ")}
                    >
                      <span class={[
                        "transition-colors duration-200",
                        isActive() ? "text-[var(--accent-primary)]" : "text-[var(--text-tertiary)]",
                      ].join(" ")}>
                        {item.icon}
                      </span>
                      {item.label}
                      <Show when={isActive()}>
                        <span class="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-[var(--accent-primary)] rounded-r-full" />
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
                <div class="flex min-h-[240px] items-center justify-center text-[var(--text-sm)] text-[var(--text-tertiary)]">
                  Loading section...
                </div>
              }
            >
              <Switch>
                <Match when={section() === "dashboard"}>
                  <Dashboard />
                </Match>
                <Match when={section() === "vpn"}>
                  <VpnSetup onNavigate={(s) => setSection(s as Section)} />
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
