import type { JSX } from "solid-js";
import Card from "./Card";
import Spinner from "./Spinner";

export function LoadingStateCard(props: { message: string }) {
  return (
    <Card class="py-12">
      <div class="flex items-center justify-center gap-3 text-fg-tertiary">
        <Spinner size="md" />
        <span class="text-sm">{props.message}</span>
      </div>
    </Card>
  );
}

export function ErrorStateCard(props: { message: string }) {
  return (
    <Card variant="elevated" class="border-l-4 border-l-error">
      <div class="flex items-center gap-3 text-error">
        <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <span class="text-sm">{props.message}</span>
      </div>
    </Card>
  );
}

export function EmptyStateCard(props: { message: string; children: JSX.Element }) {
  return (
    <Card class="py-12 text-center">
      {props.children}
      <p class="mt-3 text-sm text-fg-secondary">{props.message}</p>
    </Card>
  );
}
