import { Show } from "solid-js";
import Card from "./Card";
import Button from "./Button";

/**
 * Warning banner shown when legacy (non-API-visible) firewall rules are detected.
 * Used by VpnSetup, Tunnels, and Rules pages.
 */
export default function LegacyRulesWarning(props: {
  count: number;
  /** Extra detail appended after the standard preamble. */
  detail: string;
  /** When provided, an "Open Migration" button is rendered. */
  onNavigate?: () => void;
}) {
  return (
    <Card variant="elevated" class="border-l-4 border-l-warning">
      <div class="flex items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <svg class="h-5 w-5 shrink-0 text-warning" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
            <line x1="12" y1="9" x2="12" y2="13" />
            <line x1="12" y1="17" x2="12.01" y2="17" />
          </svg>
          <div>
            <p class="text-sm font-medium text-fg">
              Legacy firewall rules detected
            </p>
            <p class="mt-0.5 text-xs text-fg-muted">
              Your firewall has {props.count} rule{props.count !== 1 ? "s" : ""} in the old format. {props.detail}
            </p>
          </div>
        </div>
        <Show when={props.onNavigate}>
          <Button variant="secondary" size="sm" onClick={() => props.onNavigate!()}>
            Open Migration
          </Button>
        </Show>
      </div>
    </Card>
  );
}
