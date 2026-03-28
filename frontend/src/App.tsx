import { Show, createSignal, Match, Switch, onMount } from "solid-js";
import ControlCenter from "./pages/ControlCenter";
import AuthBootstrap from "./pages/AuthBootstrap";
import Login from "./pages/Login";
import Setup from "./pages/Setup";
import ToastContainer from "./components/Toast";
import { apiGet } from "./lib/api";

type SetupStatus = {
  configured: boolean;
  type?: string;
  host?: string;
  skip_tls?: boolean;
  instance_id?: number;
  instance_label?: string;
  auth_configured?: boolean;
};

type AuthStatus = {
  configured: boolean;
  authenticated: boolean;
};

async function fetchSetupStatus(): Promise<SetupStatus> {
  const { ok, data } = await apiGet<SetupStatus & { error?: string }>("/api/setup/status");
  if (!ok) throw new Error(data.error ?? "Failed to load setup status");
  return data;
}

async function fetchAuthStatus(): Promise<AuthStatus> {
  const { ok, data } = await apiGet<AuthStatus & { error?: string }>("/api/auth/status");
  if (!ok) throw new Error(data.error ?? "Failed to load auth status");
  return data;
}

export default function App() {
  const [forceSetup, setForceSetup] = createSignal(false);
  const [setupStatus, setSetupStatus] = createSignal<SetupStatus | null>(null);
  const [authStatus, setAuthStatus] = createSignal<AuthStatus | null>(null);
  const [statusLoading, setStatusLoading] = createSignal(true);
  const [statusError, setStatusError] = createSignal(false);

  const loadAppState = async () => {
    setStatusLoading(true);
    setStatusError(false);

    try {
      const [setup, auth] = await Promise.all([fetchSetupStatus(), fetchAuthStatus()]);
      setSetupStatus(setup);
      setAuthStatus(auth);
      if (auth.authenticated && (window.location.pathname === "/login" || window.location.pathname === "/setup")) {
        window.history.replaceState({}, "", "/");
      }
    } catch {
      setStatusError(true);
    } finally {
      setStatusLoading(false);
    }
  };

  onMount(() => {
    void loadAppState();
  });

  const handleSetupComplete = () => {
    window.history.replaceState({}, "", "/");
    setForceSetup(false);
    void loadAppState();
  };

  const handleAuthComplete = () => {
    window.history.replaceState({}, "", "/");
    void loadAppState();
  };

  const handleLoggedOut = () => {
    window.history.replaceState({}, "", "/login");
    void loadAppState();
  };

  return (
    <>
      <Switch>
        <Match when={statusLoading()}>
          <div class="flex min-h-screen items-center justify-center bg-surface">
            <div class="flex flex-col items-center gap-4">
              <img src="/gator64px.svg" alt="Gator logo" class="h-18 w-18 drop-shadow-[0_10px_30px_rgba(0,0,0,0.35)]" />
              <p class="text-sm text-fg-tertiary">Loading Gator...</p>
            </div>
          </div>
        </Match>

        <Match when={statusError()}>
          <div class="flex min-h-screen items-center justify-center bg-surface px-4">
            <div class="w-full max-w-md rounded-xl border border-line bg-surface-secondary p-6 text-center">
              <p class="text-sm text-red-300">Could not load setup status.</p>
              <button
                type="button"
                onClick={() => void loadAppState()}
                class="mt-4 rounded-lg bg-accent px-4 py-2 text-[13px] font-semibold text-surface hover:brightness-110"
              >
                Retry
              </button>
            </div>
          </div>
        </Match>

        <Match when={forceSetup() || !setupStatus()?.configured}>
          <Setup onComplete={handleSetupComplete} />
        </Match>

        <Match when={setupStatus()?.configured && !authStatus()?.configured}>
          <AuthBootstrap onComplete={handleAuthComplete} />
        </Match>

        <Match when={authStatus()?.configured && !authStatus()?.authenticated}>
          <Login onComplete={handleAuthComplete} />
        </Match>

        <Match when={setupStatus()?.configured && authStatus()?.configured && authStatus()?.authenticated}>
          <Show when={setupStatus()}>
              <ControlCenter
                onLogout={handleLoggedOut}
                onReconfigure={() => setForceSetup(true)}
                onInstanceSwitched={() => void loadAppState()}
              />
          </Show>
        </Match>
      </Switch>
      <ToastContainer />
    </>
  );
}
