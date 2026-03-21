import { createFileRoute } from "@tanstack/react-router";
import { ContractList } from "@/features/contracts/contract-list";

export const Route = createFileRoute("/_auth/contracts/")({
  component: ContractList,
});
