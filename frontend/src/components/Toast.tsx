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

const icons = {
  success: (
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
    </svg>
  ),
  error: (
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
    </svg>
  ),
  warning: (
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
    </svg>
  ),
  info: (
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
      <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  ),
};

const styles = {
  success: "border-[var(--status-success)]/30 bg-[var(--status-success)]/[0.07] text-[var(--status-success)]",
  error: "border-[var(--status-error)]/30 bg-[var(--status-error)]/[0.07] text-[var(--status-error)]",
  warning: "border-[var(--status-warning)]/30 bg-[var(--status-warning)]/[0.07] text-[var(--status-warning)]",
  info: "border-[var(--status-info)]/30 bg-[var(--status-info)]/[0.07] text-[var(--status-info)]",
};

const iconStyles = {
  success: "bg-[var(--status-success)]/15 text-[var(--status-success)]",
  error: "bg-[var(--status-error)]/15 text-[var(--status-error)]",
  warning: "bg-[var(--status-warning)]/15 text-[var(--status-warning)]",
  info: "bg-[var(--status-info)]/15 text-[var(--status-info)]",
};

export default function ToastContainer() {
  return (
    <div class="fixed bottom-5 right-5 z-[100] flex flex-col gap-2 pointer-events-none max-w-sm w-full">
      <For each={toasts()}>
        {(t, index) => (
          <div
            class="pointer-events-auto group flex items-center gap-3 rounded-xl border px-4 py-3.5 shadow-2xl backdrop-blur-md animate-slide-up"
            style={{
              "background": "linear-gradient(135deg, rgba(10, 10, 12, 0.95), rgba(16, 16, 20, 0.9))",
              "animation-delay": `${index() * 50}ms`,
            }}
            classList={{ [styles[t.type]]: true }}
            role="alert"
          >
            <span class={`flex h-7 w-7 shrink-0 items-center justify-center rounded-lg ${iconStyles[t.type]}`}>
              {icons[t.type]}
            </span>
            <p class="flex-1 text-[13px] font-medium leading-snug">{t.message}</p>
            <button
              type="button"
              onClick={() => dismissToast(t.id)}
              class="shrink-0 p-1 rounded-md text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-hover)] transition-all opacity-0 group-hover:opacity-100"
            >
              <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        )}
      </For>
    </div>
  );
}
