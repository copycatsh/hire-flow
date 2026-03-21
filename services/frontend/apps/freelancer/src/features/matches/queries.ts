import { useMutation } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { MatchResponse } from "./types";

export function useFindJobMatches() {
  return useMutation({
    mutationFn: () => apiClient.post<MatchResponse>("/api/v1/matches"),
  });
}
