import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Contract, ListResponse } from "./types";

export function useContracts() {
  return useQuery({
    queryKey: ["contracts"],
    queryFn: () => apiClient.get<ListResponse<Contract>>("/api/v1/contracts"),
  });
}
