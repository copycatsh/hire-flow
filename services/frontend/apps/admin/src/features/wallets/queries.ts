import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Wallet, ListResponse } from "./types";

export function useWallets() {
  return useQuery({
    queryKey: ["wallets"],
    queryFn: () => apiClient.get<ListResponse<Wallet>>("/api/v1/wallets"),
  });
}
