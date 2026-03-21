import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import type { AuthUser } from "./types";
import type { AuthState } from "./auth-state";

interface AuthContextValue {
  user: AuthUser | null;
  setUser: (user: AuthUser) => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

interface AuthProviderProps {
  children: ReactNode;
  auth: AuthState;
}

export function AuthProvider({ children, auth }: AuthProviderProps) {
  const [user, setUserState] = useState<AuthUser | null>(auth.user);

  const setUser = useCallback(
    (u: AuthUser) => {
      auth.setUser(u);
      setUserState(u);
    },
    [auth],
  );

  const logout = useCallback(() => {
    auth.logout();
    setUserState(null);
    window.location.assign("/login");
  }, [auth]);

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
