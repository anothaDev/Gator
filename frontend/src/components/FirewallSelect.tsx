type Props = {
  value: string | null;
  onChange: (value: "opnsense" | "pfsense") => void;
};

function FirewallCard(props: {
  id: "opnsense" | "pfsense";
  name: string;
  description: string;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={props.onSelect}
      class={`group relative flex flex-col items-center gap-4 rounded-xl border-2 px-8 py-10 text-center transition-all duration-200 ${
        props.selected
          ? "border-[var(--status-success)] bg-[var(--success-subtle)] shadow-[0_0_24px_rgba(0,255,157,0.08)]"
          : "border-[var(--border-default)] bg-[var(--bg-tertiary)] hover:border-[var(--border-strong)] hover:bg-[var(--bg-hover)]"
      }`}
    >
      <div
        class={`flex h-16 w-16 items-center justify-center rounded-xl transition-colors ${
          props.selected
            ? "bg-[var(--success-subtle)] text-[var(--status-success)]"
            : "bg-[var(--bg-elevated)] text-[var(--text-tertiary)] group-hover:text-[var(--text-secondary)]"
        }`}
      >
        {props.id === "opnsense" ? (
          <svg class="h-8 w-8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 017.843 4.582M12 3a8.997 8.997 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5a17.92 17.92 0 01-8.716-2.247m0 0A8.966 8.966 0 013 12c0-1.264.26-2.467.732-3.559" />
          </svg>
        ) : (
          <svg class="h-8 w-8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
          </svg>
        )}
      </div>

      <div>
        <h3
          class={`text-lg font-semibold ${
            props.selected ? "text-[var(--status-success)]" : "text-[var(--text-primary)]"
          }`}
        >
          {props.name}
        </h3>
        <p class="mt-1.5 text-sm text-[var(--text-tertiary)]">{props.description}</p>
      </div>

      <div
        class={`absolute right-3 top-3 flex h-5 w-5 items-center justify-center rounded-full border-2 transition-all ${
          props.selected
            ? "border-[var(--status-success)] bg-[var(--status-success)]"
            : "border-[var(--border-default)]"
        }`}
      >
        {props.selected && (
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
        )}
      </div>
    </button>
  );
}

export default function FirewallSelect(props: Props) {
  return (
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <FirewallCard
        id="opnsense"
        name="OPNsense"
        description="Open source, FreeBSD-based firewall and routing platform"
        selected={props.value === "opnsense"}
        onSelect={() => props.onChange("opnsense")}
      />
      <FirewallCard
        id="pfsense"
        name="pfSense"
        description="Trusted open source firewall and router distribution"
        selected={props.value === "pfsense"}
        onSelect={() => props.onChange("pfsense")}
      />
    </div>
  );
}
