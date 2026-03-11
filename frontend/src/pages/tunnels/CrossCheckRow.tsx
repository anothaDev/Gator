import { Show } from "solid-js";

function CrossCheckRow(props: { label: string; ok: boolean; detail?: string }) {
  return (
    <div class="flex items-center gap-2">
      <span class={`inline-block h-2 w-2 rounded-full ${props.ok ? "bg-[var(--status-success)]" : "bg-[var(--status-error)]"}`} />
      <span class={props.ok ? "text-[var(--text-secondary)]" : "text-red-300"}>{props.label}</span>
      <Show when={props.detail}>
        <span class="text-xs text-[var(--text-tertiary)]">— {props.detail}</span>
      </Show>
    </div>
  );
}

export default CrossCheckRow;
