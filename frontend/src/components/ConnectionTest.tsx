import { createSignal, Show } from "solid-js";
import type { ConnectionDetails } from "./ConnectionForm";
import Spinner from "./Spinner";

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
      <p class="text-sm text-fg-secondary">
        Test the connection to your{" "}
        <span class="font-medium text-fg">
          {props.firewallType === "opnsense" ? "OPNsense" : "pfSense"}
        </span>{" "}
        instance to make sure everything is configured correctly.
      </p>

      <div class="rounded-lg border border-line-faint bg-surface-tertiary p-4">
        <h4 class="mb-3 text-xs font-semibold uppercase tracking-wider text-fg-tertiary">
          Connection Summary
        </h4>
        <dl class="space-y-2 text-sm">
          <div class="flex justify-between">
            <dt class="text-fg-tertiary">Type</dt>
            <dd class="font-medium text-fg">
              {props.firewallType === "opnsense" ? "OPNsense" : "pfSense"}
            </dd>
          </div>
          <div class="flex justify-between">
            <dt class="text-fg-tertiary">Host</dt>
            <dd class="font-mono text-fg">{props.connection.host}</dd>
          </div>
          {props.firewallType === "opnsense" ? (
            <div class="flex justify-between">
              <dt class="text-fg-tertiary">API Key</dt>
              <dd class="font-mono text-fg">
                {props.connection.apiKey.slice(0, 8)}...
              </dd>
            </div>
          ) : (
            <div class="flex justify-between">
              <dt class="text-fg-tertiary">API Token</dt>
              <dd class="font-mono text-fg">
                {props.connection.apiToken.slice(0, 8)}...
              </dd>
            </div>
          )}
          <div class="flex justify-between">
            <dt class="text-fg-tertiary">TLS Verification</dt>
            <dd class="text-fg">
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
        class="flex w-full items-center justify-center gap-2 rounded-lg border border-line bg-elevated px-5 py-3 text-sm font-medium text-fg transition-colors hover:border-line-strong hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
      >
        <Show when={status() === "testing"}>
          <Spinner />
        </Show>
        {status() === "testing" ? "Testing connection..." : "Test Connection"}
      </button>

      <Show when={result()}>
          <div
            class={`rounded-lg border p-4 ${
              result()!.success
                ? "border-success/30 bg-success-subtle"
                : "border-error/30 bg-error-subtle"
            }`}
          >
            <div class="flex items-start gap-3">
              <div
                class={`mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full ${
                  result()!.success ? "bg-success" : "bg-error"
                }`}
              >
                <Show when={result()!.success} fallback={
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
                }>
                  <svg
                    class="h-3 w-3 text-surface"
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
                </Show>
              </div>
              <div>
                <p
                  class={`text-sm font-medium ${
                    result()!.success ? "text-success" : "text-error"
                  }`}
                >
                  {result()!.success ? "Connection successful" : "Connection failed"}
                </p>
                <p class="mt-1 text-sm text-fg-secondary">{result()!.message}</p>
                <Show when={result()!.version}>
                  <p class="mt-1 text-xs text-fg-tertiary">
                    Version: {result()!.version}
                  </p>
                </Show>
                <Show when={result()!.hostname}>
                  <p class="text-xs text-fg-tertiary">
                    Hostname: {result()!.hostname}
                  </p>
                </Show>
              </div>
            </div>
          </div>
      </Show>

      <Show when={status() === "success"}>
        <button
          type="button"
          onClick={props.onComplete}
          class="w-full rounded-lg bg-accent px-5 py-3 text-sm font-semibold text-surface shadow-lg shadow-accent/20 transition-all hover:brightness-110 hover:shadow-accent/30"
        >
          Save & Continue
        </button>
      </Show>
    </div>
  );
}
