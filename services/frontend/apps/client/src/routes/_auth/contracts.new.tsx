import { createFileRoute } from "@tanstack/react-router";
import { ProposeContractForm } from "@/features/contracts/propose-contract-form";
import { z } from "zod";

const searchSchema = z.object({
  freelancer_id: z.string().optional(),
  job_id: z.string().optional(),
});

export const Route = createFileRoute("/_auth/contracts/new")({
  validateSearch: searchSchema,
  component: ProposeContractPage,
});

function ProposeContractPage() {
  const { freelancer_id, job_id } = Route.useSearch();
  return <ProposeContractForm freelancerId={freelancer_id} jobId={job_id} />;
}
