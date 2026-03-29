import { Show, createSignal, onMount } from "solid-js";
import { getOpnsenseHost } from "../lib/api";

/**
 * Small button that opens the corresponding page on the OPNsense web UI.
 * Usage: <OpnsenseLink path="/ui/firewall/alias" />
 */
export default function OpnsenseLink(props: { path: string; label?: string }) {
  const [host, setHost] = createSignal("");
  onMount(() => void getOpnsenseHost().then(setHost));

  return (
    <Show when={host()}>
      <a
        href={`${host()}${props.path}`}
        target="_blank"
        rel="noopener noreferrer"
        class="inline-flex items-center gap-1.5 rounded-md border border-border-faint bg-surface px-2.5 py-1.5 text-label-sm text-fg-secondary hover:bg-surface-raised hover:text-fg"
        title={`Open ${props.label ?? "this page"} in OPNsense`}
      >
        <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
          <polyline points="15 3 21 3 21 9" />
          <line x1="10" y1="14" x2="21" y2="3" />
        </svg>
        {props.label ?? "OPNsense"}
      </a>
    </Show>
  );
}
