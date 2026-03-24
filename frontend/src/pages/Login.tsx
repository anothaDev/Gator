import { createSignal } from "solid-js";

import Button from "../components/Button";
import Input from "../components/Input";
import { apiPost } from "../lib/api";

type Props = {
  onComplete: () => void;
};

export default function Login(props: Props) {
  const [password, setPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [loading, setLoading] = createSignal(false);

  const handleLogin = async () => {
    setError("");

    if (password().trim().length < 8) {
      setError("Enter your admin password.");
      return;
    }

    setLoading(true);

    try {
      const { ok, data } = await apiPost<{ error?: string }>("/api/auth/login", {
        password: password(),
      });

      if (!ok) {
        setError(data.error ?? "Login failed.");
        return;
      }

      props.onComplete();
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class="flex min-h-screen items-center justify-center bg-[var(--bg-primary)] px-4">
      <div class="w-full max-w-md rounded-2xl border border-[var(--border-strong)] bg-[var(--bg-secondary)] p-8 shadow-2xl">
        <div class="mb-8 text-center">
          <img src="/gator-logo.svg" alt="Gator logo" class="mx-auto h-16 w-16 scale-[3.2] object-contain" />
          <h1 class="mt-4 text-2xl font-semibold tracking-tight text-[var(--text-primary)]">Sign in to Gator</h1>
          <p class="mt-2 text-sm text-[var(--text-tertiary)]">Enter the local admin password to open the control center.</p>
        </div>

        <div class="space-y-4">
          <Input
            label="Admin password"
            type="password"
            value={password()}
            onInput={setPassword}
            placeholder="Enter password"
          />

          {error() !== "" && (
            <div class="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{error()}</div>
          )}

          <Button variant="primary" size="lg" loading={loading()} class="w-full" onClick={() => void handleLogin()}>
            Sign in
          </Button>
        </div>
      </div>
    </div>
  );
}
