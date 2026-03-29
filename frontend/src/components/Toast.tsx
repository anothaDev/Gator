import { createSignal, For } from "solid-js";

type ToastType = "success" | "error" | "warning" | "info";

interface Toast {
  id: number;
  message: string;
  type: ToastType;
  duration?: number;
}

const [toasts, setToasts] = createSignal<Toast[]>([]);
let toastId = 0;

export function toast(message: string, type: ToastType = "info", duration = 4000) {
  const id = ++toastId;
  setToasts((prev) => [...prev, { id, message, type, duration }]);
  if (duration > 0) {
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, duration);
  }
  return id;
}

export function dismissToast(id: number) {
  setToasts((prev) => prev.filter((t) => t.id !== id));
}

const icons: Record<ToastType, () => import("solid-js").JSX.Element> = {
  success: () => (
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
    </svg>
  ),
  error: () => (
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
    </svg>
  ),
  warning: () => (
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01" />
    </svg>
  ),
  info: () => (
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <circle cx="12" cy="12" r="10" /><line x1="12" y1="16" x2="12" y2="12" /><line x1="12" y1="8" x2="12.01" y2="8" />
    </svg>
  ),
};

const toneStyles: Record<ToastType, string> = {
  success: "border-success/20 text-success",
  error: "border-error/20 text-error",
  warning: "border-warning/20 text-warning",
  info: "border-border text-fg",
};

export default function ToastContainer() {
  return (
    <div class="fixed bottom-4 right-4 z-[100] flex flex-col gap-2 pointer-events-none max-w-sm w-full">
      <For each={toasts()}>
        {(t) => (
          <div
            class={[
              "pointer-events-auto group flex items-center gap-3 rounded-lg border px-4 py-3",
              "bg-surface-raised shadow-md animate-slide-up",
              toneStyles[t.type],
            ].join(" ")}
            role="alert"
          >
            <span class="shrink-0">{icons[t.type]()}</span>
            <p class="flex-1 text-body-sm text-fg">{t.message}</p>
            <button
              type="button"
              onClick={() => dismissToast(t.id)}
              class="shrink-0 p-1 rounded text-fg-muted hover:text-fg hover:bg-hover transition-colors opacity-0 group-hover:opacity-100"
            >
              <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        )}
      </For>
    </div>
  );
}
