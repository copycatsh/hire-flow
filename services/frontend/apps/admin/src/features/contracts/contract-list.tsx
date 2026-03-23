import { useContracts } from "./queries";

const STATUS_BADGE: Record<string, string> = {
  PENDING: "bg-background-muted text-foreground-secondary",
  HOLD_PENDING: "bg-warning-bg text-warning",
  AWAITING_ACCEPT: "bg-info-bg text-info",
  ACTIVE: "bg-success-bg text-success",
  COMPLETING: "bg-warning-bg text-warning",
  COMPLETED: "bg-success-bg text-success",
  DECLINING: "bg-error-bg text-error",
  DECLINED: "bg-background-muted text-foreground-secondary",
  CANCELLED: "bg-background-muted text-foreground-secondary",
};

function formatAmount(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(cents / 100);
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
    return <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">{error.message}</div>;
  }

  const contracts = data?.items ?? [];

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <h1 className="font-display text-2xl font-semibold tracking-tight">All Contracts</h1>
        <p className="text-sm text-foreground-secondary">{data?.total ?? 0} total</p>
      </div>

      {contracts.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No contracts found.</p>
      ) : (
        <div className="overflow-hidden rounded-md border border-border bg-background shadow-sm">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border bg-background-muted">
              <tr>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Title</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Client</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Freelancer</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Amount</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {contracts.map((c) => (
                <tr key={c.id} className="hover:bg-background-muted/50">
                  <td className="px-6 py-4 font-medium text-foreground">{c.title}</td>
                  <td className="px-6 py-4 font-mono text-xs text-foreground-tertiary">{c.client_id.slice(0, 8)}</td>
                  <td className="px-6 py-4 font-mono text-xs text-foreground-tertiary">{c.freelancer_id.slice(0, 8)}</td>
                  <td className="px-6 py-4 font-mono text-foreground">{formatAmount(c.amount)}</td>
                  <td className="px-6 py-4">
                    <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_BADGE[c.status] || "bg-background-muted text-foreground-secondary"}`}>
                      {c.status.replace("_", " ")}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
