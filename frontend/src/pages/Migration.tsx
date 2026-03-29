import { createSignal, For, Show, onMount } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import Button from "../components/Button";
import OpnsenseLink from "../components/OpnsenseLink";
import Spinner from "../components/Spinner";

type MigrationState = "loading" | "ready" | "downloading" | "uploading" | "applying" | "confirming" | "flushing" | "done" | "error";

export default function Migration() {
  const [state, setState] = createSignal<MigrationState>("loading");
  const [legacyCount, setLegacyCount] = createSignal(0);
  const [mvcCount, setMvcCount] = createSignal(0);
  const [legacyAvailable, setLegacyAvailable] = createSignal(false);
  const [csv, setCsv] = createSignal("");
  const [revision, setRevision] = createSignal("");
  const [error, setError] = createSignal("");
  const [countdown, setCountdown] = createSignal(60);
  const [countdownInterval, setCountdownInterval] = createSignal<ReturnType<typeof setInterval> | null>(null);
  const [stepsDone, setStepsDone] = createSignal<string[]>([]);

  const addStep = (s: string) => setStepsDone((prev) => [...prev, s]);

  const loadStatus = async () => {
    setState("loading");
    setError("");
    try {
      const res = await fetch("/api/opnsense/migration/status");
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? "Failed to check status");
      const data = await res.json();
      setLegacyCount(data.legacy_count ?? 0);
      setMvcCount(data.mvc_count ?? 0);
      setLegacyAvailable(data.legacy_available ?? false);
      setState("ready");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load status");
      setState("error");
    }
  };

  onMount(() => void loadStatus());

  // Step 1: Download legacy rules
  const downloadLegacy = async () => {
    setState("downloading");
    setError("");
    try {
      const res = await fetch("/api/opnsense/migration/download");
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? "Download failed");
      const data = await res.json();
      if (!data.csv || data.count === 0) {
        setError("No legacy rules found to download.");
        setState("ready");
        return;
      }
      setCsv(data.csv);
      setLegacyCount(data.count);
      addStep(`Downloaded ${data.count} legacy rules`);
      setState("ready");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Download failed");
      setState("error");
    }
  };

  // Step 2: Upload to MVC
  const uploadToMVC = async () => {
    if (!csv()) {
      setError("Download legacy rules first.");
      return;
    }
    setState("uploading");
    setError("");
    try {
      const res = await fetch("/api/opnsense/migration/upload", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ csv: csv() }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error ?? "Upload failed");
      }
      addStep("Rules uploaded to new MVC system");
      setState("ready");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Upload failed");
      setState("error");
    }
  };

  // Step 3: Apply with savepoint
  const applyWithSavepoint = async () => {
    setState("applying");
    setError("");
    try {
      const res = await fetch("/api/opnsense/migration/apply", { method: "POST" });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? "Apply failed");
      const data = await res.json();
      setRevision(data.revision ?? "");
      addStep(`Applied with savepoint ${data.revision}`);
      setState("confirming");

      // Start 60s countdown
      setCountdown(60);
      const iv = setInterval(() => {
        setCountdown((c) => {
          if (c <= 1) {
            clearInterval(iv);
            setCountdownInterval(null);
            setError("Rollback timer expired. OPNsense has reverted the changes.");
            setState("error");
            return 0;
          }
          return c - 1;
        });
      }, 1000);
      setCountdownInterval(iv);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Apply failed");
      setState("error");
    }
  };

  // Step 4: Confirm (cancel rollback)
  const confirmMigration = async () => {
    const iv = countdownInterval();
    if (iv) clearInterval(iv);
    setCountdownInterval(null);

    setState("confirming");
    setError("");
    try {
      const res = await fetch("/api/opnsense/migration/confirm", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ revision: revision() }),
      });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? "Confirm failed");
      addStep("Migration confirmed — rollback cancelled");
      setState("ready");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Confirm failed");
      setState("error");
    }
  };

  // Step 5: Flush legacy rules
  const flushLegacy = async () => {
    setState("flushing");
    setError("");
    try {
      const res = await fetch("/api/opnsense/migration/flush", { method: "POST" });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error ?? "Flush failed");
      addStep("Legacy rules removed");
      setState("done");
      void loadStatus();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Flush failed");
      setState("error");
    }
  };

  const busy = () => ["loading", "downloading", "uploading", "applying", "flushing"].includes(state());

  return (
    <div class="space-y-6">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-title-h2 font-semibold tracking-tight text-fg">
            Migration Assistant
          </h1>
          <p class="mt-1 text-body-sm text-fg-muted">
            Migrate legacy OPNsense firewall rules to the new MVC/API system.
          </p>
        </div>
        <OpnsenseLink path="/ui/firewall/migration" label="Migration" />
      </div>

      {/* Status card */}
      <Card variant="elevated">
        <h2 class="text-sm font-semibold text-fg">Current Status</h2>

        <Show when={state() === "loading"}>
          <div class="mt-3 flex items-center gap-3 text-sm text-fg-muted">
            <Spinner />
            Checking rule status...
          </div>
        </Show>

        <Show when={state() !== "loading"}>
          <div class="mt-3 grid grid-cols-2 gap-4">
            <div class="rounded-lg border border-border bg-surface-raised p-3">
              <p class="text-2xl font-bold text-warning">{legacyCount()}</p>
              <p class="text-xs text-fg-muted">Legacy rules</p>
            </div>
            <div class="rounded-lg border border-border bg-surface-raised p-3">
              <p class="text-2xl font-bold text-success">{mvcCount()}</p>
              <p class="text-xs text-fg-muted">MVC rules (API-visible)</p>
            </div>
          </div>

          <Show when={!legacyAvailable() && state() === "ready"}>
            <div class="mt-3 rounded-lg border border-success/30 bg-success-subtle px-3 py-2 text-sm text-success">
              No legacy rules found. Your firewall is already using the new MVC system.
            </div>
          </Show>

          <Show when={legacyAvailable() && legacyCount() === 0 && state() === "ready"}>
            <div class="mt-3 rounded-lg border border-border-faint bg-hover px-3 py-2 text-sm text-fg-secondary">
              Legacy rule system exists but contains no rules. You can flush it to clean up.
            </div>
          </Show>
        </Show>
      </Card>

      {/* Error */}
      <Show when={error()}>
        <Card variant="elevated" class="border-l-4 border-l-error">
          <div class="flex items-center justify-between gap-4">
            <div class="flex items-center gap-3 text-error">
              <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="8" x2="12" y2="12" />
                <line x1="12" y1="16" x2="12.01" y2="16" />
              </svg>
              <span class="text-sm">{error()}</span>
            </div>
            <Show when={state() === "error"}>
              <Button variant="secondary" size="sm" onClick={() => { setError(""); void loadStatus(); }}>
                Retry
              </Button>
            </Show>
          </div>
        </Card>
      </Show>

      {/* Confirming state — countdown */}
      <Show when={state() === "confirming"}>
        <Card variant="elevated" class="border-l-4 border-l-warning">
          <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p class="text-sm font-semibold text-warning">Verify your connection</p>
              <p class="mt-1 text-xs text-fg-secondary">
                Rules applied with savepoint. OPNsense will auto-rollback in{" "}
                <span class="font-mono font-bold text-fg">{countdown()}s</span> if you don't confirm.
              </p>
              <p class="mt-1 text-xs text-fg-muted">
                Check that you can still access OPNsense and your network works correctly before confirming.
              </p>
            </div>
            <Button variant="primary" size="md" onClick={() => void confirmMigration()}>
              Confirm — keep changes
            </Button>
          </div>
          {/* Countdown bar */}
          <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-active">
            <div
              class="h-full rounded-full bg-warning transition-all duration-1000"
              style={{ width: `${(countdown() / 60) * 100}%` }}
            />
          </div>
        </Card>
      </Show>

      {/* Steps */}
      <Show when={legacyAvailable() && state() !== "loading"}>
        <Card variant="elevated">
          <h2 class="text-sm font-semibold text-fg">Migration Steps</h2>
          <p class="mt-1 text-xs text-fg-muted">
            Run each step in order. A config backup is recommended before starting.
          </p>

          <div class="mt-4 space-y-3">
            {/* Step 1: Download */}
            <StepCard
              number={1}
              title="Download legacy rules"
              description="Export your existing firewall rules from the legacy system as CSV."
              done={csv() !== ""}
              doneLabel={`${legacyCount()} rules downloaded`}
              buttonLabel="Download"
              onClick={() => void downloadLegacy()}
              disabled={busy() || state() === "confirming"}
              loading={state() === "downloading"}
            />

            {/* Step 2: Upload */}
            <StepCard
              number={2}
              title="Upload to new MVC system"
              description="Import the downloaded rules into OPNsense's new API-managed rule system."
              done={stepsDone().some((s) => s.includes("uploaded"))}
              doneLabel="Rules uploaded"
              buttonLabel="Upload"
              onClick={() => void uploadToMVC()}
              disabled={busy() || !csv() || state() === "confirming"}
              loading={state() === "uploading"}
            />

            {/* Step 3: Apply */}
            <StepCard
              number={3}
              title="Apply with savepoint"
              description="Activate the new rules with a 60-second safety rollback. If you lose access, OPNsense reverts automatically."
              done={stepsDone().some((s) => s.includes("confirmed"))}
              doneLabel="Applied and confirmed"
              buttonLabel="Apply"
              onClick={() => void applyWithSavepoint()}
              disabled={busy() || !stepsDone().some((s) => s.includes("uploaded")) || state() === "confirming"}
              loading={state() === "applying"}
            />

            {/* Step 4: Remove legacy */}
            <StepCard
              number={4}
              title="Remove legacy rules"
              description="Delete the old legacy rules. Only do this after confirming the new rules work correctly."
              done={stepsDone().some((s) => s.includes("Legacy rules removed"))}
              doneLabel="Legacy rules removed"
              buttonLabel="Remove legacy"
              onClick={() => void flushLegacy()}
              disabled={busy() || !stepsDone().some((s) => s.includes("confirmed")) || state() === "confirming"}
              loading={state() === "flushing"}
              danger
            />
          </div>
        </Card>
      </Show>

      {/* Done */}
      <Show when={state() === "done"}>
        <Card variant="elevated" class="border-l-4 border-l-success">
          <div class="flex items-center gap-3 text-success">
            <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
              <polyline points="22 4 12 14.01 9 11.01" />
            </svg>
            <span class="text-sm">Migration complete. All rules are now managed through the MVC/API system and visible to Gator.</span>
          </div>
        </Card>
      </Show>

      {/* Log */}
      <Show when={stepsDone().length > 0}>
        <Card>
          <h2 class="text-sm font-semibold text-fg">Log</h2>
          <div class="mt-2 space-y-1">
            <For each={stepsDone()}>
              {(step, i) => (
                <p class="text-xs text-fg-muted">
                  <span class="font-mono text-fg-muted">{String(i() + 1).padStart(2, " ")}.</span>{" "}
                  {step}
                </p>
              )}
            </For>
          </div>
        </Card>
      </Show>
    </div>
  );
}

function StepCard(props: {
  number: number;
  title: string;
  description: string;
  done: boolean;
  doneLabel: string;
  buttonLabel: string;
  onClick: () => void;
  disabled: boolean;
  loading: boolean;
  danger?: boolean;
}) {
  return (
    <div
      class={[
        "flex items-center gap-4 rounded-lg border px-4 py-3",
        props.done
          ? "border-success/20 bg-success-subtle"
          : "border-transparent bg-surface-raised",
      ].join(" ")}
    >
      <div
        class={[
          "flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold",
          props.done
            ? "bg-success text-surface"
            : "bg-active text-fg-secondary",
        ].join(" ")}
      >
        {props.done ? (
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
            <path d="M5 13l4 4L19 7" />
          </svg>
        ) : (
          props.number
        )}
      </div>

      <div class="min-w-0 flex-1">
        <p class={["text-sm font-medium", props.done ? "text-success" : "text-fg"].join(" ")}>
          {props.title}
        </p>
        <p class="text-xs text-fg-muted">
          {props.done ? props.doneLabel : props.description}
        </p>
      </div>

      <Show when={!props.done}>
        <Button
          variant={props.danger ? "danger" : "primary"}
          size="sm"
          onClick={props.onClick}
          disabled={props.disabled}
          loading={props.loading}
        >
          {props.buttonLabel}
        </Button>
      </Show>
    </div>
  );
}
