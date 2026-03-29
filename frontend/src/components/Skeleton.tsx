interface SkeletonProps {
  class?: string;
}

export function Skeleton(props: SkeletonProps) {
  return (
    <div class={`animate-pulse rounded-md bg-hover ${props.class || ""}`} />
  );
}

export function SkeletonText(props: { lines?: number; class?: string }) {
  const lines = props.lines || 3;
  return (
    <div class={`space-y-2 ${props.class || ""}`}>
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton class={`h-3 ${i === lines - 1 ? "w-3/4" : "w-full"}`} />
      ))}
    </div>
  );
}

export function SkeletonCard(props: { class?: string }) {
  return (
    <div class={`rounded-lg border border-border-faint bg-surface p-5 ${props.class || ""}`}>
      <div class="flex items-start justify-between mb-4">
        <Skeleton class="h-5 w-24" />
        <Skeleton class="h-5 w-16" />
      </div>
      <SkeletonText lines={2} />
    </div>
  );
}

export function SkeletonTable(props: { rows?: number; cols?: number; class?: string }) {
  const rows = props.rows || 5;
  const cols = props.cols || 4;

  return (
    <div class={`rounded-lg border border-border-faint overflow-hidden ${props.class || ""}`}>
      <div class="bg-hover px-4 py-3 border-b border-border-faint">
        <div class="flex gap-4">
          {Array.from({ length: cols }).map(() => (
            <Skeleton class="h-3 flex-1" />
          ))}
        </div>
      </div>
      <div class="bg-surface">
        {Array.from({ length: rows }).map(() => (
          <div class="flex gap-4 px-4 py-3 border-b border-border-faint last:border-b-0">
            {Array.from({ length: cols }).map(() => (
              <Skeleton class="h-3 flex-1" />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

export function SkeletonList(props: { items?: number; class?: string }) {
  const items = props.items || 3;

  return (
    <div class={`space-y-3 ${props.class || ""}`}>
      {Array.from({ length: items }).map(() => (
        <div class="flex items-center gap-3 p-4 rounded-lg border border-border-faint bg-surface">
          <Skeleton class="h-10 w-10 rounded-lg shrink-0" />
          <div class="flex-1 space-y-2">
            <Skeleton class="h-4 w-1/3" />
            <Skeleton class="h-3 w-1/2" />
          </div>
          <Skeleton class="h-6 w-16 rounded-full shrink-0" />
        </div>
      ))}
    </div>
  );
}
