import type { JSX } from "solid-js";
import Card from "./Card";

export function LoadingStateCard(props: { message: string }) {
  return (
    <Card class="py-12">
      <div class="flex items-center justify-center gap-3 text-[var(--text-tertiary)]">
        <svg class="h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
        <span class="text-[var(--text-sm)]">{props.message}</span>
      </div>
    </Card>
  );
}

export function ErrorStateCard(props: { message: string }) {
  return (
    <Card variant="elevated" class="border-l-4 border-l-[var(--status-error)]">
      <div class="flex items-center gap-3 text-[var(--status-error)]">
        <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <span class="text-[var(--text-sm)]">{props.message}</span>
      </div>
    </Card>
  );
}

export function EmptyStateCard(props: { message: string; children: JSX.Element }) {
  return (
    <Card class="py-12 text-center">
      {props.children}
      <p class="mt-3 text-[var(--text-sm)] text-[var(--text-secondary)]">{props.message}</p>
    </Card>
  );
}
