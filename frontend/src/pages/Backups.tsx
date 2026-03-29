import { createSignal, For, Show, onMount } from "solid-js";
import Card from "../components/Card";
import Badge from "../components/Badge";
import Button from "../components/Button";
import IconButton from "../components/IconButton";
import DropdownMenu from "../components/DropdownMenu";
import type { MenuEntry } from "../components/DropdownMenu";
import Spinner from "../components/Spinner";
import { formatBytes } from "../lib/format";
import { getOpnsenseHost } from "../lib/api";

type Backup = {
  filename: string;
  size: number;
  created: string;
};

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

export default function Backups() {
  const [backups, setBackups] = createSignal<Backup[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [loadError, setLoadError] = createSignal("");
  const [creating, setCreating] = createSignal(false);
  const [createError, setCreateError] = createSignal("");
  const [deleting, setDeleting] = createSignal<string | null>(null);
  const [opnHost, setOpnHost] = createSignal("");

  const loadBackups = async () => {
    setLoading(true);
    setLoadError("");
    try {
      const res = await fetch("/api/opnsense/backups");
      if (!res.ok) throw new Error("Failed to load backups");
      const data = await res.json();
      setBackups(data.backups ?? []);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load backups");
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    void loadBackups();
    void getOpnsenseHost().then(setOpnHost);
  });

  const createBackup = async () => {
    setCreating(true);
    setCreateError("");
    try {
      const res = await fetch("/api/opnsense/backups", { method: "POST" });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error ?? "Failed to create backup");
      }
      await loadBackups();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "Failed to create backup");
    } finally {
      setCreating(false);
    }
  };

  const downloadBackup = (filename: string) => {
    const a = document.createElement("a");
    a.href = `/api/opnsense/backups/${encodeURIComponent(filename)}`;
    a.download = filename;
    a.click();
  };

  const deleteBackup = async (filename: string) => {
    if (!confirm(`Delete backup "${filename}"?`)) return;
    setDeleting(filename);
    try {
      const res = await fetch(`/api/opnsense/backups/${encodeURIComponent(filename)}`, {
        method: "DELETE",
      });
      if (!res.ok) throw new Error("Failed to delete backup");
      setBackups((prev) => prev.filter((b) => b.filename !== filename));
    } catch {
      // Silently fail — user can retry.
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div class="space-y-5">
      {/* Header */}
      <div class="flex items-start justify-between gap-4">
        <div>
          <h1 class="text-title-h2 font-semibold tracking-tight text-fg">
            Backups
          </h1>
          <p class="mt-1 text-body-sm text-fg-muted">
            OPNsense configuration snapshots.
            <Show when={backups().length > 0}>
              <span class="ml-2 text-fg-secondary">{backups().length} stored</span>
            </Show>
          </p>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="primary" size="md" onClick={() => void createBackup()} loading={creating()}>
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="7,10 12,15 17,10" />
              <line x1="12" y1="15" x2="12" y2="3" />
            </svg>
            Create backup
          </Button>
          <DropdownMenu items={(() => {
            const items: MenuEntry[] = [
              { label: "Refresh", onClick: () => void loadBackups(), loading: loading() },
            ];
            if (opnHost()) {
              items.push({ divider: true as const });
              items.push({ label: "Open in OPNsense", href: `${opnHost()}/ui/core/backup`, external: true });
            }
            return items;
          })()} />
        </div>
      </div>

      {/* Error */}
      <Show when={createError()}>
        <Card variant="elevated" class="border-l-4 border-l-error">
          <div class="flex items-center gap-3 text-error">
            <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span class="text-sm">{createError()}</span>
          </div>
        </Card>
      </Show>

      {/* Loading */}
      <Show when={loading()}>
        <Card class="py-12">
          <div class="flex items-center justify-center gap-3 text-fg-muted">
            <Spinner size="md" />
            <span class="text-sm">Loading backups...</span>
          </div>
        </Card>
      </Show>

      {/* Error state */}
      <Show when={loadError()}>
        <Card variant="elevated" class="border-l-4 border-l-error">
          <div class="flex items-center gap-3 text-error">
            <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span class="text-sm">{loadError()}</span>
          </div>
        </Card>
      </Show>

      {/* Table */}
      <Show when={!loading() && !loadError()}>
        <Show when={backups().length === 0}>
          <Card class="py-12 text-center">
            <svg class="mx-auto h-12 w-12 text-fg-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="7,10 12,15 17,10" />
              <line x1="12" y1="15" x2="12" y2="3" />
            </svg>
            <p class="mt-3 text-sm text-fg-secondary">No backups stored yet</p>
            <p class="mt-1 text-xs text-fg-muted">
              Create a backup before making changes to OPNsense.
            </p>
          </Card>
        </Show>

        <Show when={backups().length > 0}>
          <Card padding="none" class="overflow-hidden">
            <div class="overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-border-faint">
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Filename
                    </th>
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Size
                    </th>
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Created
                    </th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-fg-muted">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <For each={backups()}>
                    {(backup) => (
                      <tr class="border-b border-border-faint transition-colors duration-fast hover:bg-hover">
                        <td class="px-4 py-3">
                          <span class="font-mono text-xs text-fg">{backup.filename}</span>
                        </td>
                        <td class="px-4 py-3 text-xs text-fg-muted">{formatBytes(backup.size)}</td>
                        <td class="px-4 py-3 text-xs text-fg-muted">{formatDate(backup.created)}</td>
                        <td class="px-4 py-3">
                          <div class="flex items-center justify-end gap-2">
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => downloadBackup(backup.filename)}
                            >
                              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                                <polyline points="7,10 12,15 17,10" />
                                <line x1="12" y1="15" x2="12" y2="3" />
                              </svg>
                              Download
                            </Button>
                            <IconButton
                              variant="ghost"
                              size="sm"
                              title={`Delete ${backup.filename}`}
                              onClick={() => void deleteBackup(backup.filename)}
                              disabled={deleting() === backup.filename}
                            >
                              <Show
                                when={deleting() === backup.filename}
                                fallback={
                                  <svg class="h-4 w-4 text-error" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                    <polyline points="3 6 5 6 21 6" />
                                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                  </svg>
                                }
                              >
                                <Spinner />
                              </Show>
                            </IconButton>
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Card>
        </Show>
      </Show>
    </div>
  );
}
