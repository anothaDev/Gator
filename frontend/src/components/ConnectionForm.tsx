import { Show } from "solid-js";

export type ConnectionDetails = {
  host: string;
  apiKey: string;
  apiSecret: string;
  apiToken: string;
  skipTls: boolean;
};

type Props = {
  firewallType: string;
  value: ConnectionDetails;
  onChange: (value: ConnectionDetails) => void;
};

export default function ConnectionForm(props: Props) {
  const update = (field: keyof ConnectionDetails, val: string | boolean) => {
    props.onChange({ ...props.value, [field]: val });
  };

  const isOPNsense = () => props.firewallType === "opnsense";
  const label = () => (isOPNsense() ? "OPNsense" : "pfSense");

  return (
    <div class="space-y-6">
      <div>
        <p class="mb-6 text-body-sm text-fg-secondary">
          Enter the connection details for your{" "}
          <span class="font-medium text-fg">{label()}</span> instance.
        </p>
      </div>

      <div>
        <label class="mb-2 block text-label-md text-fg-secondary">
          Host
        </label>
        <input
          type="text"
          value={props.value.host}
          onInput={(e) => update("host", e.currentTarget.value)}
          placeholder="10.0.0.2 or https://10.0.0.2"
          class="w-full rounded-lg border border-border bg-surface px-4 py-2.5 text-body-sm text-fg placeholder-fg-muted transition-colors focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand/50"
        />
        <p class="mt-1.5 text-body-xs text-fg-muted">
          Hostname/IP accepted. If protocol is omitted, https:// is assumed.
        </p>
        <p class="mt-1 text-body-xs text-fg-muted">
          With TLS verification enabled, use a hostname that matches the
          certificate SAN/CN.
        </p>
      </div>

      <Show when={isOPNsense()}>
        <div>
          <label class="mb-2 block text-label-md text-fg-secondary">
            API Key
          </label>
          <input
            type="text"
            value={props.value.apiKey}
            onInput={(e) => update("apiKey", e.currentTarget.value)}
            placeholder="e.g. w86XNZob/8Oq..."
            class="w-full rounded-lg border border-border bg-surface px-4 py-2.5 text-mono-md text-fg placeholder-fg-muted transition-colors focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand/50"
          />
          <p class="mt-1.5 text-body-xs text-fg-muted">
            Found under System &rarr; Access &rarr; Users &rarr; API keys
          </p>
        </div>

        <div>
          <label class="mb-2 block text-label-md text-fg-secondary">
            API Secret
          </label>
          <input
            type="password"
            value={props.value.apiSecret}
            onInput={(e) => update("apiSecret", e.currentTarget.value)}
            placeholder="••••••••••••"
            class="w-full rounded-lg border border-border bg-surface px-4 py-2.5 text-mono-md text-fg placeholder-fg-muted transition-colors focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand/50"
          />
        </div>
      </Show>

      <Show when={!isOPNsense()}>
        <div>
          <label class="mb-2 block text-label-md text-fg-secondary">
            API Token
          </label>
          <input
            type="password"
            value={props.value.apiToken}
            onInput={(e) => update("apiToken", e.currentTarget.value)}
            placeholder="••••••••••••"
            class="w-full rounded-lg border border-border bg-surface px-4 py-2.5 text-mono-md text-fg placeholder-fg-muted transition-colors focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand/50"
          />
          <p class="mt-1.5 text-body-xs text-fg-muted">
            Requires the pfSense API package. Generate a token under System
            &rarr; API.
          </p>
        </div>
      </Show>

      <div class="flex items-center gap-3 rounded-lg border border-border-faint bg-surface-raised px-4 py-3">
        <button
          type="button"
          role="switch"
          aria-checked={props.value.skipTls}
          onClick={() => update("skipTls", !props.value.skipTls)}
          class={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer items-center rounded-full transition-colors ${
            props.value.skipTls ? "bg-amber-500" : "bg-active"
          }`}
        >
          <span
            class={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${
              props.value.skipTls ? "translate-x-[18px]" : "translate-x-[3px]"
            }`}
          />
        </button>
        <div>
          <p class="text-label-md text-fg-secondary">
            Skip TLS verification
          </p>
          <p class="text-body-xs text-fg-muted">
            Enable if using a self-signed certificate
          </p>
        </div>
      </div>

      <Show when={!props.value.skipTls}>
        <div class="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3">
          <p class="text-label-xs font-semibold uppercase tracking-wider text-amber-300">
            TLS verification enabled
          </p>
          <p class="mt-1 text-body-xs text-amber-200/90">
            Implications: the certificate must be trusted and match the host.
            Using an IP can fail if the certificate does not include an IP SAN.
          </p>
        </div>
      </Show>
    </div>
  );
}
