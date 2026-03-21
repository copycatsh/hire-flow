import { type ReactNode } from "react";
import { cn } from "../lib/utils";

export interface NavItem {
  to: string;
  label: string;
  icon: ReactNode;
}

interface TopNavProps {
  appName: string;
  navItems: NavItem[];
  currentPath: string;
  avatar?: string;
  onLogout?: () => void;
  renderLink: (props: { to: string; className: string; children: ReactNode }) => ReactNode;
}

export function TopNav({ appName, navItems, currentPath, avatar, onLogout, renderLink }: TopNavProps) {
  return (
    <nav className="sticky top-0 z-50 flex h-[72px] items-center justify-between border-b border-border bg-background px-8">
      <div className="flex items-center gap-10">
        <span className="font-display text-xl font-bold tracking-tight">
          hire<span className="text-primary">{appName}</span>
        </span>
        <div className="flex items-center gap-1">
          {navItems.map((item, i) => (
            <span key={item.to}>
              {renderLink({
                to: item.to,
                className: cn(
                  "flex items-center gap-2 rounded-sm px-4 py-2.5 text-sm font-medium text-foreground-secondary transition-colors",
                  "hover:bg-background-muted hover:text-foreground",
                  currentPath.startsWith(item.to) && "bg-primary-light text-primary",
                ),
                children: (
                  <>
                    {item.icon}
                    {item.label}
                  </>
                ),
              })}
            </span>
          ))}
        </div>
      </div>
      <div className="flex items-center gap-3">
        {onLogout && (
          <button
            type="button"
            onClick={onLogout}
            className="rounded-sm px-3 py-1.5 text-sm text-foreground-secondary transition-colors hover:bg-background-muted hover:text-foreground"
          >
            Logout
          </button>
        )}
        {avatar && (
          <div className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-primary to-accent text-xs font-semibold text-white">
            {avatar}
          </div>
        )}
      </div>
    </nav>
  );
}
