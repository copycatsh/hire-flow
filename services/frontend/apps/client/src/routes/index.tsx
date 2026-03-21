import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/features/auth/auth-context";
import { useEffect } from "react";

export const Route = createFileRoute("/")({
  component: IndexRedirect,
});

function IndexRedirect() {
  const { user } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    navigate({ to: user ? "/jobs" : "/login" });
  }, [user, navigate]);

  return null;
}
