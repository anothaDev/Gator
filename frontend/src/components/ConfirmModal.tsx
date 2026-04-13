import { Show } from "solid-js";
import type { JSX } from "solid-js";
import Modal from "./Modal";
import Button from "./Button";

// Generic confirmation dialog — extracted from VpnSetup.tsx.
// Supports neutral and danger tones, optional children slot,
// configurable button labels, and busy state.

export default function ConfirmModal(props: {
  title: string;
  description: string;
  confirmLabel: string;
  cancelLabel?: string;
  tone?: "neutral" | "danger";
  busy?: boolean;
  children?: JSX.Element;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <Modal size="md">
      <h2 class="text-title-h3 text-fg">{props.title}</h2>
      <p class="mt-2 text-body-sm text-fg-secondary">{props.description}</p>
      <Show when={props.children}>
        <div class="mt-4">{props.children}</div>
      </Show>
      <div class="mt-6 flex justify-end gap-3">
        <Button
          variant="secondary"
          size="md"
          onClick={props.onCancel}
          disabled={props.busy}
        >
          {props.cancelLabel ?? "Cancel"}
        </Button>
        <Button
          variant={props.tone === "danger" ? "danger" : "primary"}
          size="md"
          onClick={props.onConfirm}
          disabled={props.busy}
          loading={props.busy}
        >
          {props.confirmLabel}
        </Button>
      </div>
    </Modal>
  );
}
