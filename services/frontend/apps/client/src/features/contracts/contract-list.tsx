import { Link } from "@tanstack/react-router";
import { useContracts } from "./queries";
import type { Contract } from "./types";

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
    default:
      return "bg-background-muted text-foreground-secondary";
  }
}

function formatCurrency(cents: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
  }).format(cents / 100);
}

export function ContractList() {
  const { data, isLoading, isError, error } = useContracts();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading contracts...
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">
        {error.message}
      </div>
    );
  }

  return (
    <div>
      <h1 className="mb-8 font-display text-2xl font-semibold tracking-tight">
        Your Contracts
      </h1>

      {!data?.items?.length ? (
        <p className="text-sm text-foreground-secondary">
          No contracts yet. Propose a contract from a matched freelancer.
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
          {data!.items.map((contract) => (
            <Link
              key={contract.id}
              to="/contracts/$id"
              params={{ id: contract.id }}
              className="rounded-md border border-border bg-background p-6 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:shadow-[var(--shadow-card-hover)] hover:border-primary-500"
            >
              <div className="flex items-start justify-between gap-2">
                <h3 className="font-display text-base font-semibold tracking-tight text-foreground">
                  {contract.title}
                </h3>
                <span
                  className={`shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusColor(contract.status)}`}
                >
                  {contract.status}
                </span>
              </div>
              <p className="mt-2 line-clamp-2 text-sm text-foreground-secondary">
                {contract.description}
              </p>
              <div className="mt-4 flex items-center justify-between">
                <span className="font-mono text-lg font-semibold">
                  {formatCurrency(contract.amount)}
                </span>
                <span className="text-xs text-foreground-tertiary">
                  {new Date(contract.created_at).toLocaleDateString("en-US", {
                    month: "short",
                    day: "numeric",
                  })}
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
