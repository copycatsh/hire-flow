import { createFileRoute } from "@tanstack/react-router";
import { MatchList } from "@/features/matches/match-list";

export const Route = createFileRoute("/_auth/matches")({
  component: MatchesPage,
});

function MatchesPage() {
  return <MatchList />;
}
