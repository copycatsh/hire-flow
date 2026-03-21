import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Wallet } from "./types";

export function useWallet() {
  return useQuery({
    queryKey: ["wallet"],
    queryFn: () => apiClient.get<Wallet>("/api/v1/wallet"),
  });
}
