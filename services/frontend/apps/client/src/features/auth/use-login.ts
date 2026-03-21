import { useMutation } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { useAuth } from "./auth-context";
import type { AuthUser } from "./types";

interface LoginRequest {
  email: string;
  password: string;
}

export function useLogin() {
  const { setUser } = useAuth();

  return useMutation({
    mutationFn: (data: LoginRequest) =>
      apiClient.post<AuthUser>("/auth/login", data),
    onSuccess: (data) => {
      setUser(data);
    },
  });
}
