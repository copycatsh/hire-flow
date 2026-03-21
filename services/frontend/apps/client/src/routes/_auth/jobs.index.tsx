import { createFileRoute } from "@tanstack/react-router";
import { JobList } from "@/features/jobs/job-list";

export const Route = createFileRoute("/_auth/jobs/")({
  component: JobList,
});
