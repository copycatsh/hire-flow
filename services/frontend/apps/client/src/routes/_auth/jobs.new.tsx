import { createFileRoute } from "@tanstack/react-router";
import { CreateJobForm } from "@/features/jobs/create-job-form";

export const Route = createFileRoute("/_auth/jobs/new")({
  component: CreateJobForm,
});
