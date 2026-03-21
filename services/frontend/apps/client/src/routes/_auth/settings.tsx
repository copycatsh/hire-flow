import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_auth/settings")({
  component: function SettingsPage() {
    return (
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-2 text-sm text-foreground-secondary">
          Account settings coming soon.
        </p>
      </div>
    );
  },
});
