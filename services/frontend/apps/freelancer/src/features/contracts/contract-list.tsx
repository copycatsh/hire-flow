import { Link } from "@tanstack/react-router";
import { useContracts } from "./queries";

function statusColor(status: string) {
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
  const contracts = data?.items;

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

      {!contracts || contracts.length === 0 ? (
        <p className="text-sm text-foreground-secondary">
          No contracts yet. Contracts will appear here when a client sends you a proposal.
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
          {contracts.map((contract) => (
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
                {contract.status === "AWAITING_ACCEPT" && (
                  <span className="rounded-full bg-accent-light px-2.5 py-0.5 text-xs font-medium text-accent">
                    Action Required
                  </span>
                )}
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
