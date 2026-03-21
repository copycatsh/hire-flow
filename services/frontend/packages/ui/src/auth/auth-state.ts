import type { AuthUser } from "./types";

export class AuthState {
  user: AuthUser | null = null;
  private baseUrl: string;
  private restorePromise: Promise<void> | null = null;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  async restore(): Promise<void> {
    if (this.restorePromise) return this.restorePromise;

    this.restorePromise = this.doRestore();
    return this.restorePromise;
  }

  private async doRestore(): Promise<void> {
    try {
      const res = await fetch(`${this.baseUrl}/auth/refresh`, {
        method: "POST",
        credentials: "include",
      });
      if (!res.ok) {
        this.user = null;
        return;
      }
      this.user = await res.json();
    } catch {
      this.user = null;
    }
  }

  setUser(user: AuthUser) {
    this.user = user;
  }

  logout() {
    this.user = null;
  }
}
