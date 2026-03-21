import { createFileRoute, Outlet, redirect, Link, useRouterState } from "@tanstack/react-router";
import { useAuth } from "@/features/auth/auth-context";
import { TopNav } from "@hire-flow/ui";
import { Briefcase, FileText, Wallet } from "lucide-react";

const navItems = [
  { to: "/jobs", label: "Jobs", icon: <Briefcase size={16} /> },
  { to: "/contracts", label: "Contracts", icon: <FileText size={16} /> },
  { to: "/wallet", label: "Wallet", icon: <Wallet size={16} /> },
];

export const Route = createFileRoute("/_auth")({
  beforeLoad: ({ context }) => {
    if (!context.auth.user) {
      throw redirect({ to: "/login" });
    }
  },
  component: AuthLayout,
});

function AuthLayout() {
  const { user, logout } = useAuth();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  if (!user) return null;

  return (
    <div className="min-h-screen bg-background-subtle">
      <TopNav
        appName="flow"
        navItems={navItems}
        currentPath={pathname}
        avatar={user.user_id.slice(0, 2).toUpperCase()}
        onLogout={logout}
        renderLink={({ to, className, children }) => (
          <Link to={to} className={className}>{children}</Link>
        )}
      />
      <main className="mx-auto max-w-7xl px-8 py-12">
        <Outlet />
      </main>
    </div>
  );
}
