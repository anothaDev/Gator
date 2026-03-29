import { createSignal, Show } from "solid-js";

import Button from "../components/Button";
import Input from "../components/Input";
import { apiPost } from "../lib/api";

type Props = {
  onComplete: () => void;
};

export default function AuthBootstrap(props: Props) {
  const [password, setPassword] = createSignal("");
  const [confirmPassword, setConfirmPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [loading, setLoading] = createSignal(false);

  const handleBootstrap = async () => {
    setError("");

    if (password().trim().length < 8) {
      setError("Password must be at least 8 characters.");
      return;
    }

    if (password() !== confirmPassword()) {
      setError("Passwords do not match.");
      return;
    }

    setLoading(true);

    try {
      const { ok, data } = await apiPost<{ error?: string }>("/api/auth/bootstrap", {
        password: password(),
      });

      if (!ok) {
        setError(data.error ?? "Failed to enable authentication.");
        return;
      }

      props.onComplete();
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-surface px-4">
      <div class="w-full max-w-xl rounded-xl border border-border bg-surface p-8 shadow-2xl">
        <div class="mb-8 text-center">
          <img src="/gator64px.svg" alt="Gator logo" class="mx-auto h-16 w-16 scale-[3.2] object-contain" />
          <h1 class="mt-4 text-title-h2 font-semibold tracking-tight text-fg">Secure this Gator instance</h1>
          <p class="mt-2 text-body-sm text-fg-muted">Before opening the app, create a local admin password. All routes will require authentication after this step.</p>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <Input
            label="Admin password"
            type="password"
            value={password()}
            onInput={setPassword}
            placeholder="Minimum 8 characters"
          />
          <Input
            label="Confirm password"
            type="password"
            value={confirmPassword()}
            onInput={setConfirmPassword}
            placeholder="Re-enter password"
          />
        </div>

        <Show when={error() !== ""}>
          <div class="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{error()}</div>
        </Show>

        <Button variant="primary" size="lg" loading={loading()} class="mt-6 w-full" onClick={() => void handleBootstrap()}>
          Enable authentication
        </Button>
      </div>
    </div>
  );
}
