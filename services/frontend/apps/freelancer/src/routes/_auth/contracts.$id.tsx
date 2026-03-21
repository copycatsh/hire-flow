import { createFileRoute } from "@tanstack/react-router";
import { ContractDetail } from "@/features/contracts/contract-detail";

export const Route = createFileRoute("/_auth/contracts/$id")({
  component: function ContractDetailPage() {
    const { id } = Route.useParams();
    return <ContractDetail id={id} />;
  },
});
