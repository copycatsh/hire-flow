import { useState, type FormEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useLogin } from "./use-login";

export function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const login = useLogin();
  const navigate = useNavigate();

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    login.mutate(
      { email, password },
      {
        onSuccess: () => navigate({ to: "/jobs" }),
      },
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background-subtle">
      <div className="w-full max-w-sm rounded-lg border border-border bg-background p-8 shadow-sm">
        <h1 className="font-display text-2xl font-bold tracking-tight">
          hire<span className="text-primary">flow</span>
        </h1>
        <p className="mt-1 text-sm text-foreground-secondary">Client Portal</p>

        {login.isError && (
          <div className="mt-4 rounded-md bg-error-bg px-4 py-2 text-sm text-error">
            {login.error.message}
          </div>
        )}

        <form onSubmit={handleSubmit} className="mt-6 space-y-4">
          <div>
            <label className="text-sm font-medium text-foreground" htmlFor="email">Email</label>
            <input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@company.com"
              className="mt-1 w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
              required
            />
          </div>
          <div>
            <label className="text-sm font-medium text-foreground" htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter your password"
              className="mt-1 w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
              required
            />
          </div>
          <button
            type="submit"
            disabled={login.isPending}
            className="w-full rounded-sm bg-primary px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:opacity-50"
          >
            {login.isPending ? "Signing in..." : "Sign In"}
          </button>
        </form>
      </div>
    </div>
  );
}
