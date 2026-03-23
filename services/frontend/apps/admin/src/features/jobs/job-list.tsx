import { useJobs } from "./queries";
import type { Job } from "./types";

const STATUS_BADGE: Record<Job["status"], string> = {
  open: "bg-success-bg text-success",
  draft: "bg-background-muted text-foreground-secondary",
  in_progress: "bg-warning-bg text-warning",
  closed: "bg-background-muted text-foreground-secondary",
};

function formatBudget(amount: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(amount / 100);
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
    return <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">{error.message}</div>;
  }

  const jobs = data?.items ?? [];

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <h1 className="font-display text-2xl font-semibold tracking-tight">All Jobs</h1>
        <p className="text-sm text-foreground-secondary">{data?.total ?? 0} total</p>
      </div>

      {jobs.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No jobs found.</p>
      ) : (
        <div className="overflow-hidden rounded-md border border-border bg-background shadow-sm">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border bg-background-muted">
              <tr>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Title</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Client</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Budget</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Status</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {jobs.map((job) => (
                <tr key={job.id} className="hover:bg-background-muted/50">
                  <td className="px-6 py-4 font-medium text-foreground">{job.title}</td>
                  <td className="px-6 py-4 font-mono text-xs text-foreground-tertiary">{job.client_id.slice(0, 8)}</td>
                  <td className="px-6 py-4 font-mono text-foreground">{formatBudget(job.budget_min)} &ndash; {formatBudget(job.budget_max)}</td>
                  <td className="px-6 py-4">
                    <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_BADGE[job.status]}`}>
                      {job.status.replace("_", " ")}
                    </span>
                  </td>
                  <td className="px-6 py-4 text-foreground-secondary">{timeAgo(job.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
