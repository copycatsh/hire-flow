import { Link } from "@tanstack/react-router";
import { Briefcase, FileText, Settings, Users, Wallet } from "lucide-react";
import { cn } from "@hire-flow/ui";

const navItems = [
  { to: "/jobs", label: "Jobs", icon: Briefcase },
  { to: "/jobs", label: "Matches", icon: Users },
  { to: "/contracts", label: "Contracts", icon: FileText },
  { to: "/wallet", label: "Wallet", icon: Wallet },
] as const;

export function Sidebar() {
  return (
    <aside className="flex h-screen w-60 flex-col bg-sidebar-bg p-4">
      <div className="mb-6 px-2 font-display text-base font-bold text-sidebar-active">
        hire-flow
      </div>
      <nav className="flex flex-1 flex-col gap-1">
        {navItems.map((item) => (
          <Link
            key={item.label}
            to={item.to}
            className={cn(
              "flex items-center gap-2 rounded-sm px-3 py-2 text-sm text-sidebar-text transition-colors",
              "hover:bg-sidebar-hover hover:text-sidebar-active",
              "[&.active]:bg-sidebar-hover [&.active]:font-medium [&.active]:text-sidebar-active",
            )}
          >
            <item.icon size={16} />
            {item.label}
          </Link>
        ))}
        <div className="my-2 h-px bg-[#2A2A2A]" />
        <Link
          to="/settings"
          className={cn(
            "flex items-center gap-2 rounded-sm px-3 py-2 text-sm text-sidebar-text transition-colors",
            "hover:bg-sidebar-hover hover:text-sidebar-active",
            "[&.active]:bg-sidebar-hover [&.active]:font-medium [&.active]:text-sidebar-active",
          )}
        >
          <Settings size={16} />
          Settings
        </Link>
      </nav>
    </aside>
  );
}
