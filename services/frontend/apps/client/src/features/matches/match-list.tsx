import { useEffect } from "react";
import { Link } from "@tanstack/react-router";
import { useFindMatches } from "@/features/matches/queries";
import type { MatchResult } from "@/features/matches/types";

function scoreColor(score: number): string {
  if (score >= 0.8) return "text-success";
  if (score >= 0.6) return "text-warning";
  return "text-foreground-secondary";
}

function MatchCard({ match, jobId }: { match: MatchResult; jobId: string }) {
  const pct = Math.round(match.score * 100);

  return (
    <div className="rounded-sm border border-border bg-background p-4">
      <p className="text-sm text-foreground-secondary">
        Freelancer {match.id.slice(0, 8)}…
      </p>
      <p className={`mt-2 font-mono text-2xl font-semibold ${scoreColor(match.score)}`}>
        {pct}%
      </p>
      <Link
        to="/contracts"
        search={{ freelancer_id: match.id, job_id: jobId }}
        className="mt-3 inline-block rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary/90"
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
        className="text-sm text-foreground-secondary hover:text-foreground"
      >
        ← Back to job
      </Link>

      <h1 className="mt-4 font-display text-xl font-semibold">Matching Freelancers</h1>
      <p className="mt-1 text-sm text-foreground-secondary">
        for Job {jobId.slice(0, 8)}…
      </p>

      {findMatches.isPending && (
        <p className="mt-6 animate-pulse text-foreground-secondary">Finding matches…</p>
      )}

      {findMatches.isError && (
        <p className="mt-6 text-error">Failed to find matches. Please try again.</p>
      )}

      {findMatches.isSuccess && findMatches.data.matches.length === 0 && (
        <p className="mt-6 text-foreground-secondary">No matches found for this job.</p>
      )}

      {findMatches.isSuccess && findMatches.data.matches.length > 0 && (
        <div className="mt-6 grid gap-4 sm:grid-cols-2">
          {findMatches.data.matches.map((match) => (
            <MatchCard key={match.id} match={match} jobId={jobId} />
          ))}
        </div>
      )}
    </div>
  );
}
