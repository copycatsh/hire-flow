import { Link } from "@tanstack/react-router";
import { useContract } from "./queries";
import type { Contract } from "./types";

interface Props {
  id: string;
}

function statusColor(status: Contract["status"]) {
  switch (status) {
    case "ACTIVE":
    case "COMPLETED":
      return "bg-success-bg text-success";
    case "PENDING":
    case "HOLD_PENDING":
    case "AWAITING_ACCEPT":
    case "COMPLETING":
      return "bg-warning-bg text-warning";
    case "DECLINED":
    case "CANCELLED":
    case "DECLINING":
      return "bg-error-bg text-error";
  }
}

function formatCurrency(cents: number) {
  return `$${(cents / 100).toFixed(2)}`;
}

export function ContractDetail({ id }: Props) {
  const { data: contract, isLoading, isError, error } = useContract(id);

  if (isLoading) {
    return <p className="text-sm text-foreground-secondary">Loading contract...</p>;
  }

  if (isError) {
    return (
      <div className="rounded-sm bg-error-bg px-4 py-2 text-sm text-error">
        {error.message}
      </div>
    );
  }

  if (!contract) return null;

  return (
    <div>
      <Link to="/contracts" className="text-sm text-primary hover:underline">
        &larr; Back to contracts
      </Link>

      <div className="mt-4 rounded-sm border border-border bg-white p-6">
        <div className="flex items-start justify-between gap-4">
          <h1 className="font-display text-xl font-semibold tracking-tight">{contract.title}</h1>
          <span
            className={`inline-block rounded-sm px-2 py-0.5 text-xs font-medium ${statusColor(contract.status)}`}
          >
            {contract.status}
          </span>
        </div>

        <p className="mt-3 font-mono text-lg">{formatCurrency(contract.amount)}</p>

        <p className="mt-3 text-sm text-foreground-secondary">
          Freelancer: {contract.freelancer_id.slice(0, 8)}...
        </p>

        <p className="mt-4 text-sm">{contract.description}</p>

        <div className="mt-6 flex gap-6 text-xs text-foreground-secondary">
          <span>
            Created: <span className="font-mono">{new Date(contract.created_at).toLocaleDateString()}</span>
          </span>
          <span>
            Updated: <span className="font-mono">{new Date(contract.updated_at).toLocaleDateString()}</span>
          </span>
        </div>
      </div>
    </div>
  );
}
