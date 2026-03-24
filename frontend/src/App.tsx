import { Show, createSignal, Match, Switch, onMount } from "solid-js";
import ControlCenter from "./pages/ControlCenter";
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
};

async function fetchSetupStatus(): Promise<SetupStatus> {
  const { ok, data } = await apiGet<SetupStatus & { error?: string }>("/api/setup/status");
  if (!ok) throw new Error(data.error ?? "Failed to load setup status");
  return data;
}

export default function App() {
  const [forceSetup, setForceSetup] = createSignal(false);
  const [setupStatus, setSetupStatus] = createSignal<SetupStatus | null>(null);
  const [statusLoading, setStatusLoading] = createSignal(true);
  const [statusError, setStatusError] = createSignal(false);

  const loadSetupStatus = async () => {
    setStatusLoading(true);
    setStatusError(false);

    try {
      const status = await fetchSetupStatus();
      setSetupStatus(status);
    } catch {
      setStatusError(true);
    } finally {
      setStatusLoading(false);
    }
  };

  onMount(() => {
    void loadSetupStatus();
  });

  const handleSetupComplete = () => {
    setForceSetup(false);
    void loadSetupStatus();
  };

  return (
    <>
      <Switch>
        <Match when={statusLoading()}>
          <div class="flex min-h-screen items-center justify-center bg-[var(--bg-primary)]">
            <div class="flex flex-col items-center gap-4">
              <img src="/gator-logo.svg" alt="Gator logo" class="h-18 w-18 drop-shadow-[0_10px_30px_rgba(0,0,0,0.35)]" />
              <p class="text-sm text-[var(--text-tertiary)]">Loading Gator...</p>
            </div>
          </div>
        </Match>

        <Match when={statusError()}>
          <div class="flex min-h-screen items-center justify-center bg-[var(--bg-primary)] px-4">
            <div class="w-full max-w-md rounded-xl border border-[var(--border-default)] bg-[var(--bg-secondary)] p-6 text-center">
              <p class="text-sm text-red-300">Could not load setup status.</p>
              <button
                type="button"
                onClick={() => void loadSetupStatus()}
                class="mt-4 rounded-lg bg-[var(--accent-primary)] px-4 py-2 text-[13px] font-semibold text-[var(--bg-primary)] hover:brightness-110"
              >
                Retry
              </button>
            </div>
          </div>
        </Match>

        <Match when={forceSetup() || !setupStatus()?.configured}>
          <Setup onComplete={handleSetupComplete} />
        </Match>

        <Match when={setupStatus()?.configured}>
          <Show when={setupStatus()} keyed>
            {() => (
              <ControlCenter
                onReconfigure={() => setForceSetup(true)}
                onInstanceSwitched={() => void loadSetupStatus()}
              />
            )}
          </Show>
        </Match>
      </Switch>
      <ToastContainer />
    </>
  );
}
