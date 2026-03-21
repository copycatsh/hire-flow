import { createFileRoute } from "@tanstack/react-router";
import { MatchList } from "@/features/matches/match-list";

export const Route = createFileRoute("/_auth/jobs_/$id/matches")({
  component: function MatchesPage() {
    const { id } = Route.useParams();
    return <MatchList jobId={id} />;
  },
});
