import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { ProposeContractForm } from "@/features/contracts/propose-contract-form";

const searchSchema = z.object({
  freelancer_id: z.string().optional(),
  job_id: z.string().optional(),
});

export const Route = createFileRoute("/_auth/contracts/")({
  validateSearch: searchSchema,
  component: function ContractsPage() {
    const { freelancer_id, job_id } = Route.useSearch();
    return <ProposeContractForm freelancerId={freelancer_id} jobId={job_id} />;
  },
});
