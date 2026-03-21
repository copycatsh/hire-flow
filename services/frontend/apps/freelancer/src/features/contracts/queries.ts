import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Contract } from "./types";

export function useContracts() {
  return useQuery({
    queryKey: ["contracts"],
    queryFn: () => apiClient.get<Contract[]>("/api/v1/contracts"),
  });
}

export function useContract(id: string) {
  return useQuery({
    queryKey: ["contracts", id],
    queryFn: () => apiClient.get<Contract>(`/api/v1/contracts/${id}`),
    enabled: !!id,
  });
}

export function useAcceptContract() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiClient.put<Contract>(`/api/v1/contracts/${id}/accept`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["contracts"] });
    },
  });
}
