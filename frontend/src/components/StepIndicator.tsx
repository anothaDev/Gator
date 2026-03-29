import { For } from "solid-js";

type Step = {
  label: string;
  description: string;
};

type Props = {
  steps: Step[];
  current: number;
};

export default function StepIndicator(props: Props) {
  return (
    <div class="flex items-center gap-2">
      <For each={props.steps}>
        {(step, i) => {
          const isActive = () => i() === props.current;
          const isCompleted = () => i() < props.current;

          return (
            <>
              <div class="flex items-center gap-3">
                <div
                  class={`flex h-9 w-9 items-center justify-center rounded-full border-2 text-sm font-semibold transition-all duration-300 ${
                    isActive()
                      ? "border-success bg-success-subtle text-success shadow-[0_0_12px_rgba(0,255,157,0.3)]"
                      : isCompleted()
                        ? "border-success bg-success text-surface"
                        : "border-transparent text-fg-muted"
                  }`}
                >
                  {isCompleted() ? (
                    <svg
                      class="h-4 w-4"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="2.5"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M5 13l4 4L19 7"
                      />
                    </svg>
                  ) : (
                    i() + 1
                  )}
                </div>
                <div class="hidden sm:block">
                  <p
                    class={`text-sm font-medium leading-none ${
                      isActive()
                        ? "text-fg"
                        : isCompleted()
                          ? "text-fg-secondary"
                          : "text-fg-muted"
                    }`}
                  >
                    {step.label}
                  </p>
                  <p
                    class={`mt-1 text-xs ${
                      isActive()
                        ? "text-fg-secondary"
                        : isCompleted()
                          ? "text-fg-muted"
                          : "text-fg-muted"
                    }`}
                  >
                    {step.description}
                  </p>
                </div>
              </div>

              {i() < props.steps.length - 1 && (
                <div
                  class={`mx-1 h-px flex-1 transition-colors duration-300 ${
                    isCompleted() ? "bg-success/50" : "bg-surface-raised"
                  }`}
                />
              )}
            </>
          );
        }}
      </For>
    </div>
  );
}
