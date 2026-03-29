import { Show, type JSX } from "solid-js";

type AlertTone = "error" | "success" | "warning" | "info";

const toneStyles: Record<AlertTone, string> = {
  error: "border-error/20 bg-error-subtle text-error",
  success: "border-success/20 bg-success-subtle text-success",
  warning: "border-warning/20 bg-warning-subtle text-warning",
  info: "border-info/20 bg-info-subtle text-info",
};

const toneIcons: Record<AlertTone, () => JSX.Element> = {
  error: () => (
    <svg class="h-4 w-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" />
    </svg>
  ),
  success: () => (
    <svg class="h-4 w-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" /><polyline points="22 4 12 14.01 9 11.01" />
    </svg>
  ),
  warning: () => (
    <svg class="h-4 w-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
      <line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" />
    </svg>
  ),
  info: () => (
    <svg class="h-4 w-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="12" cy="12" r="10" /><line x1="12" y1="16" x2="12" y2="12" /><line x1="12" y1="8" x2="12.01" y2="8" />
    </svg>
  ),
};

export default function AlertBanner(props: {
  tone: AlertTone;
  class?: string;
  children: JSX.Element;
}) {
  return (
    <div
      class={[
        "flex items-start gap-3 rounded-lg border px-4 py-3",
        toneStyles[props.tone],
        props.class ?? "",
      ].join(" ")}
    >
      {toneIcons[props.tone]()}
      <span class="text-body-sm">{props.children}</span>
    </div>
  );
}
