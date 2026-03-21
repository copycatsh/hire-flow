import { useEffect } from "react";
import { Link } from "@tanstack/react-router";
import { useFindMatches } from "@/features/matches/queries";
import type { MatchResult } from "@/features/matches/types";

function scoreColor(score: number): string {
  if (score >= 0.8) return "text-success";
  if (score >= 0.6) return "text-accent";
  return "text-foreground-secondary";
}

function scoreBg(score: number): string {
  if (score >= 0.8) return "bg-success-bg";
  if (score >= 0.6) return "bg-accent-light";
  return "bg-background-muted";
}

function MatchCard({ match, jobId, rank }: { match: MatchResult; jobId: string; rank: number }) {
  const pct = Math.round(match.score * 100);

  return (
    <div className="rounded-md border border-border bg-background p-6 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:shadow-[var(--shadow-card-hover)]">
      <p className="text-sm font-medium text-foreground">
        Candidate #{rank}
      </p>
      <div className="mt-3 flex items-center gap-2">
        <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${scoreBg(match.score)} ${scoreColor(match.score)}`}>
          Match
        </span>
        <span className={`font-mono text-2xl font-semibold ${scoreColor(match.score)}`}>
          {pct}%
        </span>
      </div>
      <Link
        to="/contracts/new"
        search={{ freelancer_id: match.id, job_id: jobId }}
        className="mt-4 inline-block rounded-md bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:bg-primary-hover"
      >
        Propose Contract
      </Link>
    </div>
  );
}

export function MatchList({ jobId }: { jobId: string }) {
  const findMatches = useFindMatches();

  useEffect(() => {
    findMatches.mutate(jobId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jobId]);

  return (
    <div>
      <Link
        to="/jobs/$id"
        params={{ id: jobId }}
        className="text-sm text-primary hover:underline"
      >
        &larr; Back to job
      </Link>

      <h1 className="mt-4 font-display text-xl font-semibold tracking-tight">Matching Freelancers</h1>
      <p className="mt-1 text-sm text-foreground-secondary">
        for Job {jobId.slice(0, 8)}...
      </p>

      {findMatches.isPending && (
        <div className="mt-6 flex items-center gap-2 text-foreground-secondary">
          <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          Finding matches...
        </div>
      )}

      {findMatches.isError && (
        <p className="mt-6 rounded-md bg-error-bg px-4 py-3 text-sm text-error">
          Failed to find matches. Please try again.
        </p>
      )}

      {findMatches.isSuccess && findMatches.data.matches.length === 0 && (
        <p className="mt-6 text-foreground-secondary">No matches found for this job.</p>
      )}

      {findMatches.isSuccess && findMatches.data.matches.length > 0 && (
        <div className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {findMatches.data.matches.map((match, i) => (
            <MatchCard key={match.id} match={match} jobId={jobId} rank={i + 1} />
          ))}
        </div>
      )}
    </div>
  );
}
