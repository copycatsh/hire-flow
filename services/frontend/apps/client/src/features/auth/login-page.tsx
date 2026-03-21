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
    <div className="grid h-screen grid-cols-1 md:grid-cols-2">
      <div className="hidden flex-col justify-center bg-sidebar-bg px-16 md:flex">
        <h2 className="font-display text-[32px] font-bold leading-tight tracking-tight text-sidebar-active">
          The smarter way
          <br />
          to hire talent.
        </h2>
        <p className="mt-4 text-sm leading-relaxed text-sidebar-text">
          AI-powered matching connects you with the right freelancers in seconds,
          not weeks. Post a job, review matches, and get to work.
        </p>
      </div>
      <div className="flex flex-col justify-center px-16">
        <h3 className="font-display text-[22px] font-semibold tracking-tight">Sign in</h3>
        <p className="mb-8 text-sm text-foreground-secondary">
          Enter your credentials to continue
        </p>
        {login.isError && (
          <div className="mb-4 rounded-sm bg-error-bg px-4 py-2 text-sm text-error">
            {login.error.message}
          </div>
        )}
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label className="mb-1 block text-xs font-medium">Email</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@company.com"
              className="w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
              required
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium">Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter your password"
              className="w-full rounded-sm border border-border-strong bg-background px-3 py-2 text-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary-light"
              required
            />
          </div>
          <button
            type="submit"
            disabled={login.isPending}
            className="mt-1 w-full rounded-md bg-primary py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:opacity-50"
          >
            {login.isPending ? "Signing in..." : "Sign in"}
          </button>
        </form>
      </div>
    </div>
  );
}
