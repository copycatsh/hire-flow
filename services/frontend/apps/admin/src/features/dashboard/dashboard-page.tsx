import { useDashboardStats } from "./queries";
import type { ServiceStats } from "./types";

function StatCard({ label, stats }: { label: string; stats: ServiceStats }) {
  return (
    <div className="rounded-md border border-border bg-background p-6 shadow-sm">
      <p className="text-sm font-medium text-foreground-secondary">{label}</p>
      {stats.error ? (
        <p className="mt-2 text-sm text-error">{stats.error}</p>
      ) : (
        <p className="mt-2 font-mono text-3xl font-semibold text-foreground">{stats.total}</p>
      )}
    </div>
  );
}

export function DashboardPage() {
  const { data, isLoading, isError, error } = useDashboardStats();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading dashboard...
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">
        {error.message}
      </div>
    );
  }

  if (!data) return null;

  return (
    <div>
      <h1 className="mb-8 font-display text-2xl font-semibold tracking-tight">
        Platform Overview
      </h1>
      <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
        <StatCard label="Total Jobs" stats={data.jobs} />
        <StatCard label="Total Contracts" stats={data.contracts} />
        <StatCard label="Total Wallets" stats={data.wallets} />
      </div>
    </div>
  );
}
