import type { JSX } from "solid-js";
import Card from "./Card";

// Shared alert banner — replaces the repeated error/success/warning/info
// banner patterns across all pages.

type AlertTone = "error" | "success" | "warning" | "info";

const toneClasses: Record<AlertTone, string> = {
  error: "border-l-error bg-error-subtle",
  success: "border-l-success bg-success-subtle",
  warning: "border-l-warning bg-warning-subtle",
  info: "border-l-info bg-info-subtle",
};

const textClasses: Record<AlertTone, string> = {
  error: "text-error",
  success: "text-success",
  warning: "text-warning",
  info: "text-info",
};

export default function AlertBanner(props: {
  tone: AlertTone;
  class?: string;
  children: JSX.Element;
}) {
  return (
    <Card
      variant="elevated"
      class={`border-l-4 ${toneClasses[props.tone]} ${props.class ?? ""}`}
    >
      <div class={`flex items-center gap-3 ${textClasses[props.tone]}`}>
        {props.tone === "error" && (
          <svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="8" x2="12" y2="12" />
            <line x1="12" y1="16" x2="12.01" y2="16" />
          </svg>
        )}
        {props.tone === "success" && (
          <svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
            <polyline points="22 4 12 14.01 9 11.01" />
          </svg>
        )}
        {props.tone === "warning" && (
          <svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
            <line x1="12" y1="9" x2="12" y2="13" />
            <line x1="12" y1="17" x2="12.01" y2="17" />
          </svg>
        )}
        {props.tone === "info" && (
          <svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="16" x2="12" y2="12" />
            <line x1="12" y1="8" x2="12.01" y2="8" />
          </svg>
        )}
        <span class="text-sm">{props.children}</span>
      </div>
    </Card>
  );
}
