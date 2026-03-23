import { useWallets } from "./queries";

function formatBalance(cents: number) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(cents / 100);
}

export function WalletList() {
  const { data, isLoading, isError, error } = useWallets();

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading wallets...
      </div>
    );
  }

  if (isError) {
    return <div className="rounded-md bg-error-bg px-4 py-3 text-sm text-error">{error.message}</div>;
  }

  const wallets = data?.items ?? [];

  return (
    <div>
      <div className="mb-8 flex items-center justify-between">
        <h1 className="font-display text-2xl font-semibold tracking-tight">All Wallets</h1>
        <p className="text-sm text-foreground-secondary">{data?.total ?? 0} total</p>
      </div>

      {wallets.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No wallets found.</p>
      ) : (
        <div className="overflow-hidden rounded-md border border-border bg-background shadow-sm">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-border bg-background-muted">
              <tr>
                <th className="px-6 py-3 font-medium text-foreground-secondary">User</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Balance</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Available</th>
                <th className="px-6 py-3 font-medium text-foreground-secondary">Currency</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {wallets.map((w) => (
                <tr key={w.id} className="hover:bg-background-muted/50">
                  <td className="px-6 py-4 font-mono text-xs text-foreground-tertiary">{w.user_id.slice(0, 8)}</td>
                  <td className="px-6 py-4 font-mono text-foreground">{formatBalance(w.balance)}</td>
                  <td className="px-6 py-4 font-mono text-foreground">{formatBalance(w.available_balance)}</td>
                  <td className="px-6 py-4 text-foreground-secondary">{w.currency}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
