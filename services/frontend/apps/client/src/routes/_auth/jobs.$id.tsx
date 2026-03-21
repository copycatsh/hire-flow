import { createFileRoute } from "@tanstack/react-router";
import { JobDetail } from "@/features/jobs/job-detail";

export const Route = createFileRoute("/_auth/jobs/$id")({
  component: function JobDetailPage() {
    const { id } = Route.useParams();
    return <JobDetail id={id} />;
  },
});
