import { createFileRoute, Outlet, redirect, Link, useRouterState } from "@tanstack/react-router";
import { useAuth } from "@/features/auth/auth-context";
import { TopNav } from "@hire-flow/ui";
import { LayoutDashboard, Briefcase, FileText, Wallet } from "lucide-react";

const navItems = [
  { to: "/", label: "Dashboard", icon: <LayoutDashboard size={16} /> },
  { to: "/jobs", label: "Jobs", icon: <Briefcase size={16} /> },
  { to: "/contracts", label: "Contracts", icon: <FileText size={16} /> },
  { to: "/wallets", label: "Wallets", icon: <Wallet size={16} /> },
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
        appName="admin"
        navItems={navItems}
        currentPath={pathname}
        avatar="AD"
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
