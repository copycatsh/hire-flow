import { useLogin as useLoginBase } from "@hire-flow/ui";
import { apiClient } from "@/lib/api-client";

export function useLogin() {
  return useLoginBase(apiClient);
}
