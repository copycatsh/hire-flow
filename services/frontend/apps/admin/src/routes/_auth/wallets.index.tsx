import { createFileRoute } from "@tanstack/react-router";
import { WalletList } from "@/features/wallets/wallet-list";

export const Route = createFileRoute("/_auth/wallets/")({
  component: WalletList,
});
