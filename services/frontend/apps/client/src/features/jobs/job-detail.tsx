import { Link } from "@tanstack/react-router";
import { useJob } from "./queries";
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

export function JobDetail({ id }: { id: string }) {
  const { data: job, isLoading, isError } = useJob(id);

  if (isLoading) {
    return <p>Loading job...</p>;
  }

  if (isError || !job) {
    return (
      <div>
        <Link to="/jobs" className="text-sm text-primary hover:underline">
          &larr; Back to Jobs
        </Link>
        <p className="mt-4 text-sm text-foreground-secondary">Job not found</p>
      </div>
    );
  }

  return (
    <div>
      <Link to="/jobs" className="text-sm text-primary hover:underline">
        &larr; Back to Jobs
      </Link>

      <div className="mt-4 rounded-sm border border-border bg-white p-6">
        <div className="flex items-start justify-between">
          <div>
            <h1 className="font-display text-xl font-semibold">{job.title}</h1>
            <span
              className={`mt-2 inline-block rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_BADGE[job.status]}`}
            >
              {job.status.replace("_", " ")}
            </span>
          </div>
          <Link
            to="/jobs/$id/matches"
            params={{ id: job.id }}
            className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover"
          >
            Find Matches
          </Link>
        </div>

        <p className="mt-4 text-sm leading-relaxed text-foreground-secondary">
          {job.description}
        </p>

        <div className="mt-6 grid grid-cols-2 gap-4 text-sm">
          <div>
            <span className="text-foreground-secondary">Budget</span>
            <p className="mt-1 font-mono font-medium">
              {formatBudget(job.budget_min)} &ndash;{" "}
              {formatBudget(job.budget_max)}
            </p>
          </div>
          <div>
            <span className="text-foreground-secondary">Created</span>
            <p className="mt-1 font-mono font-medium">
              {formatDate(job.created_at)}
            </p>
          </div>
          <div>
            <span className="text-foreground-secondary">Last Updated</span>
            <p className="mt-1 font-mono font-medium">
              {formatDate(job.updated_at)}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
