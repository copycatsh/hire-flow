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

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function JobList() {
  const { data: jobs, isLoading, isError, error } = useJobs();

  if (isLoading) {
    return <p>Loading jobs...</p>;
  }

  if (isError) {
    return (
      <div className="rounded-sm bg-error-bg px-4 py-2 text-sm text-error">
        {error.message}
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="font-display text-2xl font-semibold tracking-tight">
          Your Jobs
        </h1>
        <Link
          to="/jobs/new"
          className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover"
        >
          + New Job
        </Link>
      </div>

      {!jobs || jobs.length === 0 ? (
        <p className="text-sm text-foreground-secondary">
          No jobs yet. Create your first job to get started.
        </p>
      ) : (
        <div className="overflow-hidden rounded-sm border border-border bg-white">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-background-muted text-left text-xs uppercase tracking-wider text-foreground-secondary">
                <th className="px-4 py-3 font-medium">Title</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Budget</th>
                <th className="px-4 py-3 font-medium">Created</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.id} className="border-b border-border">
                  <td className="px-4 py-3 font-medium">
                    <Link
                      to="/jobs/$id"
                      params={{ id: job.id }}
                      className="hover:text-primary"
                    >
                      {job.title}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={`rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_BADGE[job.status]}`}
                    >
                      {job.status.replace("_", " ")}
                    </span>
                  </td>
                  <td className="px-4 py-3 font-mono">
                    {formatBudget(job.budget_min)} &ndash;{" "}
                    {formatBudget(job.budget_max)}
                  </td>
                  <td className="px-4 py-3 font-mono">
                    {formatDate(job.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
