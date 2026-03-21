import { useWallet } from "@/features/wallet/queries";

function formatCurrency(cents: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
  }).format(cents / 100);
}

export function WalletPage() {
  const { data: wallet, isLoading, error } = useWallet();

  if (isLoading) {
    return <p>Loading wallet...</p>;
  }

  if (error) {
    return (
      <div className="rounded-sm border border-red-300 bg-red-50 p-4 text-sm text-red-700">
        Failed to load wallet: {error.message}
      </div>
    );
  }

  if (!wallet) {
    return null;
  }

  const held = wallet.balance - wallet.available_balance;

  return (
    <div>
      <h1 className="font-display text-2xl font-semibold tracking-tight">Wallet</h1>

      <div className="mt-6 rounded-sm border border-border bg-white p-6">
        <p className="text-xs uppercase tracking-wider text-foreground-secondary">Total Balance</p>
        <div className="mt-1 flex items-baseline gap-2">
          <span className="font-mono text-3xl font-bold tracking-tight">
            {formatCurrency(wallet.balance)}
          </span>
          <span className="text-sm text-foreground-secondary">{wallet.currency}</span>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-4">
        <div className="rounded-sm border border-border bg-white p-6">
          <p className="text-xs uppercase tracking-wider text-foreground-secondary">Available</p>
          <p
            className={`mt-1 font-mono text-xl font-bold tracking-tight ${
              wallet.available_balance > 0 ? "text-success" : ""
            }`}
          >
            {formatCurrency(wallet.available_balance)}
          </p>
        </div>
        <div className="rounded-sm border border-border bg-white p-6">
          <p className="text-xs uppercase tracking-wider text-foreground-secondary">Held</p>
          <p
            className={`mt-1 font-mono text-xl font-bold tracking-tight ${
              held > 0 ? "text-warning" : ""
            }`}
          >
            {formatCurrency(held)}
          </p>
        </div>
      </div>
    </div>
  );
}
