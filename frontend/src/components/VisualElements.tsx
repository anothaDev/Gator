// Visual network flow indicator for Routing page
interface FlowIndicatorProps {
  mode: "all" | "selective" | "bypass";
  activeCount: number;
  totalCount: number;
}

export function FlowIndicator(props: FlowIndicatorProps) {
  const flowPercentage = () => {
    if (props.mode === "all") return 100;
    if (props.mode === "selective") return Math.round((props.activeCount / props.totalCount) * 100);
    return Math.round(((props.totalCount - props.activeCount) / props.totalCount) * 100);
  };

  return (
    <div class="relative overflow-hidden rounded-xl bg-surface-raised p-6">
      {/* Animated background gradient */}
      <div 
        class="absolute inset-0 opacity-30"
        style={{
          background: `linear-gradient(135deg, 
            rgba(22, 163, 74, 0.1) 0%, 
            transparent 50%,
            rgba(59, 130, 246, 0.1) 100%)`
        }}
      />
      
      <div class="relative flex items-center justify-between gap-8">
        {/* Traffic source */}
        <div class="flex flex-col items-center gap-2">
          <div class="flex h-12 w-12 items-center justify-center rounded-lg bg-surface shadow-lg">
            <svg class="h-6 w-6 text-fg-secondary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="2" y="3" width="20" height="14" rx="2" />
              <line x1="8" y1="21" x2="16" y2="21" />
              <line x1="12" y1="17" x2="12" y2="21" />
            </svg>
          </div>
          <span class="text-label-sm text-fg-muted">Devices</span>
        </div>

        {/* Flow visualization */}
        <div class="flex-1">
          <div class="relative h-2 overflow-hidden rounded-full bg-surface">
            {/* Animated flow lines */}
            <div 
              class="absolute inset-y-0 left-0 rounded-full bg-brand transition-all duration-1000"
              style={{ width: `${flowPercentage()}%` }}
            >
              <div class="absolute inset-0 animate-pulse bg-gradient-to-r from-transparent via-white/20 to-transparent" />
            </div>
            
            {/* Traffic dots */}
            <div class="absolute inset-0">
              {[...Array(5)].map((_, i) => (
                <div
                  class="absolute top-1/2 h-1 w-1 -translate-y-1/2 rounded-full bg-white/60"
                  style={{
                    left: `${i * 25}%`,
                    animation: `flow-dot 2s ease-in-out ${i * 0.4}s infinite`
                  }}
                />
              ))}
            </div>
          </div>
          
          <div class="mt-2 flex justify-between text-body-xs text-fg-muted">
            <span>{flowPercentage()}% through VPN</span>
            <span>{100 - flowPercentage()}% direct</span>
          </div>
        </div>

        {/* VPN destination */}
        <div class="flex flex-col items-center gap-2">
          <div class="flex h-12 w-12 items-center justify-center rounded-lg bg-brand/10 shadow-lg ring-1 ring-brand/20">
            <svg class="h-6 w-6 text-brand" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            </svg>
          </div>
          <span class="text-label-sm text-brand">VPN</span>
        </div>

        {/* Internet destination */}
        <div class="flex flex-col items-center gap-2">
          <div class="flex h-12 w-12 items-center justify-center rounded-lg bg-surface shadow-lg">
            <svg class="h-6 w-6 text-fg-secondary" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="2" y1="12" x2="22" y2="12" />
              <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
            </svg>
          </div>
          <span class="text-label-sm text-fg-muted">Internet</span>
        </div>
      </div>

      <style>{`
        @keyframes flow-dot {
          0%, 100% { transform: translateY(-50%) translateX(0); opacity: 0; }
          50% { opacity: 1; }
          100% { transform: translateY(-50%) translateX(20px); opacity: 0; }
        }
      `}</style>
    </div>
  );
}

// Stat card with animated value
interface StatCardProps {
  label: string;
  value: string | number;
  unit?: string;
  icon: JSX.Element;
  color?: "default" | "success" | "warning" | "error";
  trend?: "up" | "down" | "neutral";
}

export function StatCard(props: StatCardProps) {
  const colorClasses = {
    default: "bg-surface-raised",
    success: "bg-success-subtle",
    warning: "bg-warning-subtle",
    error: "bg-error-subtle",
  };

  return (
    <div class={`rounded-lg ${colorClasses[props.color ?? "default"]} p-4 transition-all hover:scale-[1.02]`}>
      <div class="flex items-start justify-between">
        <div>
          <p class="text-label-xs font-semibold uppercase tracking-wider text-fg-muted">{props.label}</p>
          <div class="mt-2 flex items-baseline gap-1">
            <span class="text-2xl font-bold text-fg">{props.value}</span>
            {props.unit && <span class="text-body-sm text-fg-secondary">{props.unit}</span>}
          </div>
        </div>
        <div class="rounded-lg bg-surface p-2 text-fg-secondary">
          {props.icon}
        </div>
      </div>
      
      {props.trend && (
        <div class="mt-3 flex items-center gap-1 text-body-xs">
          {props.trend === "up" && (
            <>
              <svg class="h-3 w-3 text-success" viewBox="0 0 20 20" fill="currentColor">
                <path fill-rule="evenodd" d="M12 7a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0V8.414l-4.293 4.293a1 1 0 01-1.414 0L8 10.414l-4.293 4.293a1 1 0 01-1.414-1.414l5-5a1 1 0 011.414 0L11 10.586 14.586 7H12z" clip-rule="evenodd" />
              </svg>
              <span class="text-success">Increasing</span>
            </>
          )}
          {props.trend === "down" && (
            <>
              <svg class="h-3 w-3 text-success" viewBox="0 0 20 20" fill="currentColor">
                <path fill-rule="evenodd" d="M12 13a1 1 0 110 2h5a1 1 0 011-1v-5a1 1 0 11-2 0v2.586l-4.293-4.293a1 1 0 01-1.414 0L8 9.586 3.707 5.293a1 1 0 01-1.414 1.414l5 5a1 1 0 011.414 0L11 9.414 14.586 13H12z" clip-rule="evenodd" />
              </svg>
              <span class="text-success">Decreasing</span>
            </>
          )}
        </div>
      )}
    </div>
  );
}

import type { JSX } from "solid-js";
