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
    return (
      <div className="flex items-center gap-2 text-foreground-secondary">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        Loading wallet...
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-md border border-border bg-error-bg p-4 text-sm text-error shadow-sm">
        {error.message.includes("not found")
          ? "Wallet not found. Run the seed command to create test wallets: docker compose exec payments /seed"
          : `Failed to load wallet: ${error.message}`}
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

      <div className="mt-6 rounded-md border border-border bg-background p-6 shadow-sm">
        <p className="text-xs uppercase tracking-wider text-foreground-secondary">Total Balance</p>
        <div className="mt-1 flex items-baseline gap-2">
          <span className="font-mono text-3xl font-bold tracking-tight">
            {formatCurrency(wallet.balance)}
          </span>
          <span className="text-sm text-foreground-secondary">{wallet.currency}</span>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-4">
        <div className="rounded-md border border-border bg-background p-6 shadow-sm">
          <p className="text-xs uppercase tracking-wider text-foreground-secondary">Available</p>
          <p
            className={`mt-1 font-mono text-xl font-bold tracking-tight ${
              wallet.available_balance > 0 ? "text-success" : ""
            }`}
          >
            {formatCurrency(wallet.available_balance)}
          </p>
        </div>
        <div className="rounded-md border border-border bg-background p-6 shadow-sm">
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
