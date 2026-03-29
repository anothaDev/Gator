import type { JSX } from "solid-js";

type EmptyStateVariant = "vpn" | "tunnel" | "routing" | "generic";

interface Props {
  variant?: EmptyStateVariant;
  title: string;
  description: string;
  icon?: JSX.Element;
  action?: JSX.Element;
}

const illustrations = {
  vpn: (
    <svg class="h-20 w-20" viewBox="0 0 80 80" fill="none">
      <circle cx="40" cy="40" r="36" stroke="var(--color-border-strong)" stroke-width="1.5" stroke-dasharray="4 4" />
      <path d="M40 16L40 64" stroke="var(--color-brand)" stroke-width="1.5" opacity="0.3" />
      <path d="M16 40L64 40" stroke="var(--color-brand)" stroke-width="1.5" opacity="0.3" />
      <circle cx="40" cy="40" r="16" stroke="var(--color-brand)" stroke-width="2" opacity="0.5" />
      <path d="M32 40L38 46L48 34" stroke="var(--color-brand)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
      <circle cx="40" cy="40" r="4" fill="var(--color-brand)" opacity="0.8" />
    </svg>
  ),
  tunnel: (
    <svg class="h-20 w-20" viewBox="0 0 80 80" fill="none">
      <ellipse cx="24" cy="40" rx="12" ry="20" stroke="var(--color-border-strong)" stroke-width="1.5" />
      <ellipse cx="24" cy="40" rx="6" ry="10" stroke="var(--color-brand)" stroke-width="1.5" opacity="0.4" />
      <path d="M36 28L56 28M36 40L56 40M36 52L56 52" stroke="var(--color-border-strong)" stroke-width="1.5" stroke-dasharray="4 3" />
      <ellipse cx="56" cy="40" rx="12" ry="20" stroke="var(--color-border-strong)" stroke-width="1.5" />
      <ellipse cx="56" cy="40" rx="6" ry="10" stroke="var(--color-brand)" stroke-width="1.5" opacity="0.4" />
      <circle cx="40" cy="40" r="3" fill="var(--color-brand)" />
    </svg>
  ),
  routing: (
    <svg class="h-20 w-20" viewBox="0 0 80 80" fill="none">
      <rect x="12" y="28" width="20" height="24" rx="3" stroke="var(--color-border-strong)" stroke-width="1.5" />
      <rect x="48" y="28" width="20" height="24" rx="3" stroke="var(--color-border-strong)" stroke-width="1.5" />
      <path d="M32 40H48" stroke="var(--color-brand)" stroke-width="2" stroke-dasharray="4 3" />
      <circle cx="40" cy="40" r="4" fill="var(--color-brand)" opacity="0.6" />
      <path d="M22 36H22.01M22 44H22.01" stroke="var(--color-fg-muted)" stroke-width="2" stroke-linecap="round" />
      <path d="M58 36H58.01M58 44H58.01" stroke="var(--color-fg-muted)" stroke-width="2" stroke-linecap="round" />
    </svg>
  ),
  generic: (
    <svg class="h-20 w-20" viewBox="0 0 80 80" fill="none">
      <rect x="16" y="16" width="48" height="48" rx="8" stroke="var(--color-border-strong)" stroke-width="1.5" />
      <path d="M28 40H52" stroke="var(--color-brand)" stroke-width="2" stroke-linecap="round" opacity="0.5" />
      <path d="M28 32H44" stroke="var(--color-border-strong)" stroke-width="2" stroke-linecap="round" />
      <path d="M28 48H48" stroke="var(--color-border-strong)" stroke-width="2" stroke-linecap="round" />
      <circle cx="56" cy="24" r="8" fill="var(--color-brand)" opacity="0.2" />
      <path d="M53 24L55 26L59 22" stroke="var(--color-brand)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  ),
};

export default function EmptyState(props: Props) {
  return (
    <div class="flex flex-col items-center justify-center py-16 px-8 text-center">
      <div class="relative mb-6">
        <div class="absolute inset-0 bg-brand/5 blur-2xl rounded-full scale-150" />
        <div class="relative opacity-60">
          {props.icon || illustrations[props.variant || "generic"]}
        </div>
      </div>
      <h3 class="text-title-h3 font-semibold text-fg mb-2">
        {props.title}
      </h3>
      <p class="text-body-sm text-fg-muted max-w-sm leading-relaxed mb-6">
        {props.description}
      </p>
      {props.action && (
        <div class="animate-slide-up" style={{ "animation-delay": "100ms" }}>
          {props.action}
        </div>
      )}
    </div>
  );
}
