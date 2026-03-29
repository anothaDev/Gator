import { createSignal, Show, Match, Switch } from "solid-js";
import StepIndicator from "../components/StepIndicator";
import FirewallSelect from "../components/FirewallSelect";
import ConnectionForm, {
  type ConnectionDetails,
} from "../components/ConnectionForm";
import ConnectionTest from "../components/ConnectionTest";
import Input from "../components/Input";
import { apiPost } from "../lib/api";

const STEPS = [
  { label: "Platform", description: "Choose your firewall" },
  { label: "Connection", description: "Enter credentials" },
  { label: "Verify", description: "Test & confirm" },
];

type Props = {
  onComplete: () => void;
};

export default function Setup(props: Props) {
  const [step, setStep] = createSignal(0);
  const [firewallType, setFirewallType] = createSignal<string | null>(null);
  const [saveError, setSaveError] = createSignal("");
  const [adminPassword, setAdminPassword] = createSignal("");
  const [confirmAdminPassword, setConfirmAdminPassword] = createSignal("");
  const [connection, setConnection] = createSignal<ConnectionDetails>({
    host: "",
    apiKey: "",
    apiSecret: "",
    apiToken: "",
    skipTls: true,
  });

  const canProceedStep0 = () => firewallType() !== null;
  const canProceedStep1 = () => {
    const c = connection();
    if (c.host.trim() === "") return false;
    if (firewallType() === "opnsense") {
      return c.apiKey.trim() !== "" && c.apiSecret.trim() !== "";
    }
    return c.apiToken.trim() !== "";
  };

  const next = () => setStep((s) => Math.min(s + 1, STEPS.length - 1));
  const back = () => setStep((s) => Math.max(s - 1, 0));

  const handleComplete = async () => {
    setSaveError("");

    if (adminPassword().trim().length < 8) {
      setSaveError("Admin password must be at least 8 characters.");
      return;
    }

    if (adminPassword() !== confirmAdminPassword()) {
      setSaveError("Admin passwords do not match.");
      return;
    }

    try {
      const res = await fetch("/api/setup/save", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          type: firewallType(),
          host: connection().host,
          api_key: connection().apiKey,
          api_secret: connection().apiSecret,
          api_token: connection().apiToken,
          skip_tls: connection().skipTls,
        }),
      });

      if (!res.ok) {
        let message = "Failed to save setup configuration.";
        try {
          const payload = await res.json();
          if (typeof payload?.error === "string" && payload.error.trim() !== "") {
            message = payload.error;
          }
        } catch {
          // Keep default message when response is not JSON.
        }
        setSaveError(message);
        return;
      }

      const bootstrap = await apiPost<{ error?: string }>("/api/auth/bootstrap", {
        password: adminPassword(),
      });

      if (!bootstrap.ok) {
        setSaveError(bootstrap.data.error ?? "Failed to enable authentication.");
        return;
      }

      props.onComplete();
    } catch {
      setSaveError("Failed to save setup configuration. Check backend connectivity and try again.");
    }
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-surface px-4">
      {/* Subtle background grain */}
      <div class="pointer-events-none fixed inset-0 opacity-[0.015]" style={{ "background-image": "url(\"data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E\")" }} />

      <div class="w-full max-w-xl">
        {/* Logo / Header */}
        <div class="mb-10 text-center">
          <h1 class="text-title-h2 font-semibold tracking-tight text-fg">
            gator
          </h1>
          <p class="mt-2 text-body-sm text-fg-muted">
            Firewall management, simplified
          </p>
        </div>

        {/* Step indicator */}
        <div class="mb-8">
          <StepIndicator steps={STEPS} current={step()} />
        </div>

        {/* Card */}
        <div class="rounded-xl border border-border bg-surface p-8 shadow-2xl backdrop-blur">
          {/* Step title */}
          <h2 class="mb-1 text-lg font-semibold text-fg">
            <Switch>
              <Match when={step() === 0}>Choose your platform</Match>
              <Match when={step() === 1}>Connection details</Match>
              <Match when={step() === 2}>Verify connection</Match>
            </Switch>
          </h2>
          <p class="mb-6 text-sm text-fg-muted">
            <Switch>
              <Match when={step() === 0}>
                Select the firewall platform you want to manage.
              </Match>
              <Match when={step() === 1}>
                Provide the host and API credentials for your firewall.
              </Match>
              <Match when={step() === 2}>
                Test the connection before saving your configuration.
              </Match>
            </Switch>
          </p>

          {/* Step content */}
          <Switch>
            <Match when={step() === 0}>
              <FirewallSelect
                value={firewallType()}
                onChange={(v) => setFirewallType(v)}
              />
            </Match>
            <Match when={step() === 1}>
              <ConnectionForm
                firewallType={firewallType()!}
                value={connection()}
                onChange={setConnection}
              />
            </Match>
            <Match when={step() === 2}>
              <div class="space-y-6">
                <ConnectionTest
                  firewallType={firewallType()!}
                  connection={connection()}
                  onComplete={handleComplete}
                />

                <div class="rounded-lg border border-border bg-surface-raised p-4">
                  <h4 class="mb-1 text-sm font-semibold text-fg">Create admin password</h4>
                  <p class="mb-4 text-sm text-fg-muted">Gator will require authentication for the UI and API after setup.</p>

                  <div class="grid gap-4 sm:grid-cols-2">
                    <Input
                      label="Password"
                      type="password"
                      value={adminPassword()}
                      onInput={setAdminPassword}
                      placeholder="Minimum 8 characters"
                    />
                    <Input
                      label="Confirm password"
                      type="password"
                      value={confirmAdminPassword()}
                      onInput={setConfirmAdminPassword}
                      placeholder="Re-enter password"
                    />
                  </div>
                </div>
              </div>
            </Match>
          </Switch>

          <Show when={step() === 2 && saveError() !== ""}>
            <div class="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">
              {saveError()}
            </div>
          </Show>

          {/* Navigation */}
          <Show when={step() < 2}>
            <div class="mt-8 flex items-center justify-between">
              <Show
                when={step() > 0}
                fallback={<div />}
              >
                <button
                  type="button"
                  onClick={back}
                  class="rounded-lg px-4 py-2 text-sm font-medium text-fg-secondary transition-colors hover:text-fg"
                >
                  Back
                </button>
              </Show>
              <button
                type="button"
                onClick={next}
                disabled={step() === 0 ? !canProceedStep0() : !canProceedStep1()}
                class="rounded-lg bg-brand px-5 py-2.5 text-sm font-semibold text-surface shadow-lg shadow-accent/20 transition-all hover:brightness-110 hover:shadow-accent/30 disabled:cursor-not-allowed disabled:opacity-40 disabled:shadow-none"
              >
                Continue
              </button>
            </div>
          </Show>
        </div>

        {/* Footer */}
        <p class="mt-6 text-center text-xs text-fg-muted">
          You can change these settings later in the configuration panel.
        </p>
      </div>
    </div>
  );
}
