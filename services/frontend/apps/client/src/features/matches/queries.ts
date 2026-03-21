import { useMutation } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { MatchResponse } from "./types";

export function useFindMatches() {
  return useMutation({
    mutationFn: (jobId: string) =>
      apiClient.post<MatchResponse>(`/api/v1/jobs/${jobId}/matches`),
  });
}
