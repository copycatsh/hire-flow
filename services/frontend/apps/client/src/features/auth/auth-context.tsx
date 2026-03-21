import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import type { AuthUser } from "./types";

interface AuthContextValue {
  user: AuthUser | null;
  setUser: (user: AuthUser) => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUserState] = useState<AuthUser | null>(null);

  const setUser = useCallback((u: AuthUser) => {
    setUserState(u);
  }, []);

  const logout = useCallback(() => {
    setUserState(null);
    window.location.assign("/login");
  }, []);

  return (
    <AuthContext.Provider value={{ user, setUser, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
