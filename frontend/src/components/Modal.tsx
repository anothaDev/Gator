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
      style={{ background: "rgba(0, 0, 0, 0.5)", "backdrop-filter": "blur(4px)" }}
      onClick={(e) => {
        if (e.target === e.currentTarget && props.onBackdropClick) {
          props.onBackdropClick();
        }
      }}
    >
      <div
        class={[
          "relative w-full rounded-lg border border-border bg-surface-raised shadow-lg animate-scale-in p-6",
          sizeClasses[props.size ?? "lg"],
        ].join(" ")}
      >
        {props.children}
      </div>
    </div>
  );
}
