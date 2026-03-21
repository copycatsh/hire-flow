import { useMutation } from "@tanstack/react-query";
import { useAuth } from "./auth-context";

interface LoginResponse {
  user_id: string;
  role: string;
}

interface ApiClient {
  post: <T>(path: string, body?: unknown) => Promise<T>;
}

export function useLogin(apiClient: ApiClient) {
  const { setUser } = useAuth();

  return useMutation({
    mutationFn: (data: { email: string; password: string }) =>
      apiClient.post<LoginResponse>("/auth/login", data),
    onSuccess: (data) => {
      setUser(data);
    },
  });
}
