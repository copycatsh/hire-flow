import { useQuery, useMutation } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Contract, CreateContractRequest } from "./types";

export function useContract(id: string) {
  return useQuery({
    queryKey: ["contracts", id],
    queryFn: () => apiClient.get<Contract>(`/api/v1/contracts/${id}`),
    enabled: !!id,
  });
}

export function useCreateContract() {
  return useMutation({
    mutationFn: (data: CreateContractRequest) =>
      apiClient.post<Contract>("/api/v1/contracts", data),
  });
}
