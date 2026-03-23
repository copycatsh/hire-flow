import { Link } from "@tanstack/react-router";
import { useJobs } from "./queries";
import type { Job } from "./types";

const STATUS_BADGE: Record<Job["status"], string> = {
  open: "bg-success-bg text-success",
  draft: "bg-background-muted text-foreground-secondary",
  in_progress: "bg-warning-bg text-warning",
  closed: "bg-background-muted text-foreground-secondary",
};

function formatBudget(amount: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
  }).format(amount / 100);
}

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const days = Math.floor(diff / 86_400_000);
  if (days === 0) return "Today";
  if (days === 1) return "Yesterday";
  if (days < 30) return `${days}d ago`;
  return `${Math.floor(days / 30)}mo ago`;
}

export function JobList() {
  const { data, isLoading, isError, error } = useJobs();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading jobs...
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

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <h1 className="font-display text-2xl font-semibold tracking-tight">
          Your Jobs
        </h1>
        <Link
          to="/jobs/new"
          className="rounded-md bg-primary px-4 py-2.5 text-sm font-medium text-white shadow-sm transition-colors hover:bg-primary-hover"
        >
          + New Job
        </Link>
      </div>

      {!data?.items?.length ? (
        <p className="text-sm text-foreground-secondary">
          No jobs yet. Create your first job to get started.
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
          {data!.items.map((job) => (
            <Link
              key={job.id}
              to="/jobs/$id"
              params={{ id: job.id }}
              className="rounded-md border border-border bg-background p-6 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:shadow-[var(--shadow-card-hover)] hover:border-primary-500"
            >
              <div className="flex items-start justify-between gap-2">
                <h3 className="font-display text-base font-semibold tracking-tight text-foreground">
                  {job.title}
                </h3>
                <span
                  className={`shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_BADGE[job.status]}`}
                >
                  {job.status.replace("_", " ")}
                </span>
              </div>
              <p className="mt-4 text-sm text-foreground-secondary">
                {timeAgo(job.created_at)}
              </p>
              <p className="mt-3 font-mono text-lg font-semibold text-foreground">
                {formatBudget(job.budget_min)} &ndash; {formatBudget(job.budget_max)}
              </p>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
