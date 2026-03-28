import { onMount, onCleanup, type JSX } from "solid-js";

type ModalSize = "sm" | "md" | "lg" | "xl";

const sizeClasses: Record<ModalSize, string> = {
  sm: "max-w-sm",
  md: "max-w-md",
  lg: "max-w-lg",
  xl: "max-w-2xl",
};

export default function Modal(props: {
  size?: ModalSize;
  onBackdropClick?: () => void;
  children: JSX.Element;
}) {
  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape" && props.onBackdropClick) {
      props.onBackdropClick();
    }
  };

  onMount(() => {
    document.addEventListener("keydown", handleKeyDown);
    document.body.style.overflow = "hidden";
  });

  onCleanup(() => {
    document.removeEventListener("keydown", handleKeyDown);
    document.body.style.overflow = "";
  });

  return (
    <div
      class="fixed inset-0 z-50 flex items-center justify-center px-4 animate-fade-in"
      style={{ background: "rgba(5, 5, 7, 0.85)" }}
      onClick={(e) => {
        if (e.target === e.currentTarget && props.onBackdropClick) {
          props.onBackdropClick();
        }
      }}
    >
      <div class="fixed inset-0 pointer-events-none" style={{ "backdrop-filter": "blur(8px)" }} />
      <div
        class={`relative w-full rounded-2xl border border-line-strong bg-surface-secondary shadow-2xl animate-scale-up ${sizeClasses[props.size ?? "lg"]}`}
        style={{
          background: "linear-gradient(180deg, rgba(16, 16, 20, 1) 0%, rgba(10, 10, 12, 1) 100%)",
        }}
      >
        <div class="absolute inset-0 rounded-2xl pointer-events-none" 
          style={{
            background: "linear-gradient(180deg, rgba(255,255,255,0.02) 0%, transparent 50%)",
          }}
        />
        <div class="relative p-6">
          {props.children}
        </div>
      </div>
    </div>
  );
}
