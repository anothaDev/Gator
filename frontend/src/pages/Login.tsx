import { createSignal, Show } from "solid-js";

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
    <div class="flex min-h-screen items-center justify-center bg-surface px-4">
      <div class="w-full max-w-md rounded-xl border border-border bg-surface p-8 shadow-2xl">
        <div class="mb-8 text-center">
          <img src="/gator64px.svg" alt="Gator logo" class="mx-auto h-20 w-20 object-contain" />
          <h1 class="mt-4 text-title-h2 font-semibold tracking-tight text-fg">Sign in to Gator</h1>
          <p class="mt-2 text-body-sm text-fg-muted">Enter the local admin password to open the control center.</p>
        </div>

        <div class="space-y-4">
          <Input
            label="Admin password"
            type="password"
            value={password()}
            onInput={setPassword}
            placeholder="Enter password"
          />

          <Show when={error() !== ""}>
            <div class="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-body-sm text-red-300">{error()}</div>
          </Show>

          <Button variant="primary" size="lg" loading={loading()} class="w-full" onClick={() => void handleLogin()}>
            Sign in
          </Button>
        </div>
      </div>
    </div>
  );
}
