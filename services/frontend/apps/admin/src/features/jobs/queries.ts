import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Job, ListResponse } from "./types";

export function useJobs() {
  return useQuery({
    queryKey: ["jobs"],
    queryFn: () => apiClient.get<ListResponse<Job>>("/api/v1/jobs"),
  });
}
