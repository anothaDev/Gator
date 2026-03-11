import { createSignal, Show } from "solid-js";

// ─── Types ───────────────────────────────────────────────────────

type URLTableHint = {
  download_url: string;
  jq_filter: string;
  description: string;
  filename: string;
};

// ─── IP Range Upload Modal ───────────────────────────────────────

function IpRangeUploadModal(props: {
  appName: string;
  hint: URLTableHint;
  onClose: () => void;
  onUploaded: () => void;
}) {
  const [uploading, setUploading] = createSignal(false);
  const [uploadErr, setUploadErr] = createSignal("");
  const [uploadSuccess, setUploadSuccess] = createSignal(false);

  const handleUploadFile = async () => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".json";
    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) return;

      setUploading(true);
      setUploadErr("");
      setUploadSuccess(false);

      try {
        const form = new FormData();
        form.append("file", file);
        form.append("filename", props.hint.filename);

        const res = await fetch("/api/ip-ranges/upload", { method: "POST", body: form });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
          setUploadErr(data?.error ?? "Upload failed");
          return;
        }
        setUploadSuccess(true);
        // Signal the parent to retry enabling the app route.
        setTimeout(() => {
          props.onUploaded();
        }, 1000);
      } catch {
        setUploadErr("Upload failed. Check backend connectivity.");
      } finally {
        setUploading(false);
      }
    };
    input.click();
  };

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={(e) => { if (e.target === e.currentTarget) props.onClose(); }}>
      <div class="mx-4 w-full max-w-lg rounded-2xl border border-[var(--border-default)] bg-[var(--bg-secondary)] p-6 shadow-2xl">
        <h2 class="text-lg font-bold text-[var(--text-primary)]">IP Ranges Required</h2>
        <p class="mt-1 text-sm text-[var(--text-tertiary)]">
          <span class="font-medium text-[var(--text-secondary)]">{props.appName}</span> uses a large number of IP ranges that need to be loaded from a file for precise routing.
        </p>

        <div class="mt-4 rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)]/50 p-4 space-y-3">
          <div>
            <p class="text-xs font-medium text-[var(--text-tertiary)]">Step 1: Download the IP ranges file</p>
            <a
              href={props.hint.download_url}
              target="_blank"
              rel="noopener noreferrer"
              class="mt-1 inline-flex items-center gap-1.5 text-sm text-[var(--status-success)] hover:text-[var(--status-success)] underline"
            >
              <svg class="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor">
                <path d="M10.75 2.75a.75.75 0 00-1.5 0v8.614L6.295 8.235a.75.75 0 10-1.09 1.03l4.25 4.5a.75.75 0 001.09 0l4.25-4.5a.75.75 0 00-1.09-1.03l-2.955 3.129V2.75z"/>
                <path d="M3.5 12.75a.75.75 0 00-1.5 0v2.5A2.75 2.75 0 004.75 18h10.5A2.75 2.75 0 0018 15.25v-2.5a.75.75 0 00-1.5 0v2.5c0 .69-.56 1.25-1.25 1.25H4.75c-.69 0-1.25-.56-1.25-1.25v-2.5z"/>
              </svg>
              {props.hint.download_url}
            </a>
          </div>

          <div>
            <p class="text-xs font-medium text-[var(--text-tertiary)]">Step 2: Upload to Gator</p>
            <p class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">
              The file will be stored locally and served to OPNsense. No cloud dependency.
            </p>
          </div>
        </div>

        <Show when={props.hint.description}>
          <p class="mt-3 text-xs text-[var(--text-tertiary)]">{props.hint.description}</p>
        </Show>

        <Show when={uploadErr()}>
          <div class="mt-3 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-300">
            {uploadErr()}
          </div>
        </Show>
        <Show when={uploadSuccess()}>
          <div class="mt-3 rounded-lg border border-[var(--status-success)]/30 bg-[var(--status-success)]/10 px-3 py-2 text-xs text-[var(--status-success)]">
            File uploaded. Enabling route...
          </div>
        </Show>

        <div class="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={props.onClose}
            class="rounded-lg border border-[var(--border-default)] bg-[var(--bg-tertiary)] px-4 py-2 text-[13px] font-medium text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleUploadFile()}
            disabled={uploading()}
            class="rounded-lg bg-[var(--accent-primary)] px-4 py-2 text-[13px] font-semibold text-[var(--bg-primary)] shadow-lg shadow-[var(--accent-primary)]/20 transition-all hover:brightness-110 disabled:opacity-50"
          >
            {uploading() ? "Uploading..." : "Choose File & Upload"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default IpRangeUploadModal;
