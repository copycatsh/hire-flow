import { useEffect } from "react";
import { useFindJobMatches } from "./queries";
import type { JobMatch } from "./types";

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

function MatchCard({ match }: { match: JobMatch }) {
  const pct = Math.round(match.score * 100);

  return (
    <div className="rounded-md border border-border bg-background p-6 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:shadow-[var(--shadow-card-hover)]">
      <p className="text-sm font-medium text-foreground">
        Job {match.id.slice(0, 8)}...
      </p>
      <div className="mt-3 flex items-center gap-2">
        <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${scoreBg(match.score)} ${scoreColor(match.score)}`}>
          Match
        </span>
        <span className={`font-mono text-2xl font-semibold ${scoreColor(match.score)}`}>
          {pct}%
        </span>
      </div>
    </div>
  );
}

export function MatchList() {
  const findMatches = useFindJobMatches();

  useEffect(() => {
    findMatches.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div>
      <h1 className="font-display text-2xl font-semibold tracking-tight">Job Matches</h1>
      <p className="mt-1 text-sm text-foreground-secondary">
        Jobs that match your profile
      </p>

      {findMatches.isPending && (
        <div className="mt-6 flex items-center gap-2 text-foreground-secondary">
          <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          Finding matches...
        </div>
      )}

      {findMatches.isError && (
        <div className="mt-6 rounded-md bg-warning-bg px-4 py-3 text-sm text-warning">
          {findMatches.error.message.includes("404")
            ? "Your profile hasn't been indexed yet. Matches will appear once your profile is processed by our AI matching system."
            : `Failed to find matches: ${findMatches.error.message}`}
        </div>
      )}

      {findMatches.isSuccess && findMatches.data.matches.length === 0 && (
        <p className="mt-6 text-foreground-secondary">No job matches found yet.</p>
      )}

      {findMatches.isSuccess && findMatches.data.matches.length > 0 && (
        <div className="mt-6 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {findMatches.data.matches.map((match) => (
            <MatchCard key={match.id} match={match} />
          ))}
        </div>
      )}
    </div>
  );
}
