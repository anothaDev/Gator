import { createSignal, Show } from "solid-js";
import type { ConnectionDetails } from "./ConnectionForm";

type TestResult = {
  success: boolean;
  message: string;
  version?: string;
  hostname?: string;
};

type Props = {
  firewallType: string;
  connection: ConnectionDetails;
  onComplete: () => void;
};

export default function ConnectionTest(props: Props) {
  const [status, setStatus] = createSignal<
    "idle" | "testing" | "success" | "error"
  >("idle");
  const [result, setResult] = createSignal<TestResult | null>(null);

  const runTest = async () => {
    setStatus("testing");
    setResult(null);

    try {
      const endpoint =
        props.firewallType === "opnsense"
          ? "/api/opnsense/test-connection"
          : "/api/pfsense/test-connection";

      const body =
        props.firewallType === "opnsense"
          ? {
              host: props.connection.host,
              api_key: props.connection.apiKey,
              api_secret: props.connection.apiSecret,
              skip_tls: props.connection.skipTls,
            }
          : {
              host: props.connection.host,
              api_token: props.connection.apiToken,
              skip_tls: props.connection.skipTls,
            };

      const res = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      const data: TestResult = await res.json();
      setResult(data);
      setStatus(data.success ? "success" : "error");
    } catch {
      setResult({
        success: false,
        message: "Failed to reach the Gator API. Is the backend running?",
      });
      setStatus("error");
    }
  };

  return (
    <div class="space-y-6">
      <p class="text-sm text-[var(--text-secondary)]">
        Test the connection to your{" "}
        <span class="font-medium text-[var(--text-primary)]">
          {props.firewallType === "opnsense" ? "OPNsense" : "pfSense"}
        </span>{" "}
        instance to make sure everything is configured correctly.
      </p>

      <div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--bg-tertiary)] p-4">
        <h4 class="mb-3 text-xs font-semibold uppercase tracking-wider text-[var(--text-tertiary)]">
          Connection Summary
        </h4>
        <dl class="space-y-2 text-sm">
          <div class="flex justify-between">
            <dt class="text-[var(--text-tertiary)]">Type</dt>
            <dd class="font-medium text-[var(--text-primary)]">
              {props.firewallType === "opnsense" ? "OPNsense" : "pfSense"}
            </dd>
          </div>
          <div class="flex justify-between">
            <dt class="text-[var(--text-tertiary)]">Host</dt>
            <dd class="font-mono text-[var(--text-primary)]">{props.connection.host}</dd>
          </div>
          {props.firewallType === "opnsense" ? (
            <div class="flex justify-between">
              <dt class="text-[var(--text-tertiary)]">API Key</dt>
              <dd class="font-mono text-[var(--text-primary)]">
                {props.connection.apiKey.slice(0, 8)}...
              </dd>
            </div>
          ) : (
            <div class="flex justify-between">
              <dt class="text-[var(--text-tertiary)]">API Token</dt>
              <dd class="font-mono text-[var(--text-primary)]">
                {props.connection.apiToken.slice(0, 8)}...
              </dd>
            </div>
          )}
          <div class="flex justify-between">
            <dt class="text-[var(--text-tertiary)]">TLS Verification</dt>
            <dd class="text-[var(--text-primary)]">
              {props.connection.skipTls ? (
                <span class="text-amber-400">Skipped</span>
              ) : (
                "Enabled"
              )}
            </dd>
          </div>
        </dl>
      </div>

      <button
        type="button"
        onClick={runTest}
        disabled={status() === "testing"}
        class="flex w-full items-center justify-center gap-2 rounded-lg border border-[var(--border-default)] bg-[var(--bg-elevated)] px-5 py-3 text-sm font-medium text-[var(--text-primary)] transition-colors hover:border-[var(--border-strong)] hover:bg-[var(--bg-hover)] disabled:cursor-not-allowed disabled:opacity-50"
      >
        <Show when={status() === "testing"}>
          <svg
            class="h-4 w-4 animate-spin"
            viewBox="0 0 24 24"
            fill="none"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
            />
          </svg>
        </Show>
        {status() === "testing" ? "Testing connection..." : "Test Connection"}
      </button>

      <Show when={result()}>
        {(r) => (
          <div
            class={`rounded-lg border p-4 ${
              r().success
                ? "border-[var(--status-success)]/30 bg-[var(--success-subtle)]"
                : "border-[var(--status-error)]/30 bg-[var(--error-subtle)]"
            }`}
          >
            <div class="flex items-start gap-3">
              <div
                class={`mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full ${
                  r().success ? "bg-[var(--status-success)]" : "bg-[var(--status-error)]"
                }`}
              >
                {r().success ? (
                  <svg
                    class="h-3 w-3 text-[var(--bg-primary)]"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="3"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M5 13l4 4L19 7"
                    />
                  </svg>
                ) : (
                  <svg
                    class="h-3 w-3 text-white"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="3"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M6 18L18 6M6 6l12 12"
                    />
                  </svg>
                )}
              </div>
              <div>
                <p
                  class={`text-sm font-medium ${
                    r().success ? "text-[var(--status-success)]" : "text-[var(--status-error)]"
                  }`}
                >
                  {r().success ? "Connection successful" : "Connection failed"}
                </p>
                <p class="mt-1 text-sm text-[var(--text-secondary)]">{r().message}</p>
                <Show when={r().version}>
                  <p class="mt-1 text-xs text-[var(--text-tertiary)]">
                    Version: {r().version}
                  </p>
                </Show>
                <Show when={r().hostname}>
                  <p class="text-xs text-[var(--text-tertiary)]">
                    Hostname: {r().hostname}
                  </p>
                </Show>
              </div>
            </div>
          </div>
        )}
      </Show>

      <Show when={status() === "success"}>
        <button
          type="button"
          onClick={props.onComplete}
          class="w-full rounded-lg bg-[var(--accent-primary)] px-5 py-3 text-sm font-semibold text-[var(--bg-primary)] shadow-lg shadow-[var(--accent-primary)]/20 transition-all hover:brightness-110 hover:shadow-[var(--accent-primary)]/30"
        >
          Save & Continue
        </button>
      </Show>
    </div>
  );
}
