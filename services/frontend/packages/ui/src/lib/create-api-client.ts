export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function createApiClient(baseUrl: string) {
  let refreshPromise: Promise<void> | null = null;

  async function refreshToken(): Promise<void> {
    const res = await fetch(`${baseUrl}/auth/refresh`, {
      method: "POST",
      credentials: "include",
    });
    if (!res.ok) {
      throw new ApiError(res.status, "refresh failed");
    }
  }

  async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const opts: RequestInit = {
      method,
      credentials: "include",
      headers: {} as Record<string, string>,
    };

    if (body !== undefined) {
      (opts.headers as Record<string, string>)["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }

    let res = await fetch(`${baseUrl}${path}`, opts);

    if (res.status === 401) {
      try {
        if (!refreshPromise) {
          refreshPromise = refreshToken();
        }
        await refreshPromise;
        refreshPromise = null;
        res = await fetch(`${baseUrl}${path}`, opts);
      } catch (err) {
        refreshPromise = null;
        if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
          window.location.assign("/login");
          throw new ApiError(401, "session expired");
        }
        throw err instanceof ApiError ? err : new ApiError(0, "network error");
      }
    }

    if (!res.ok) {
      const data = await res.json().catch(() => ({ error: "request failed" }));
      throw new ApiError(res.status, data.error || `HTTP ${res.status}`);
    }

    return res.json() as Promise<T>;
  }

  return {
    get: <T>(path: string) => request<T>("GET", path),
    post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
    put: <T>(path: string, body?: unknown) => request<T>("PUT", path, body),
  };
}
