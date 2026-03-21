import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { WalletPage } from "./wallet-page";

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("WalletPage", () => {
  it("renders wallet balance", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "w1",
          user_id: "u1",
          balance: 5000000,
          currency: "USD",
          available_balance: 4000000,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    render(<WalletPage />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("$50,000.00")).toBeTruthy();
    });

    expect(screen.getByText("$40,000.00")).toBeTruthy();
    expect(screen.getByText("$10,000.00")).toBeTruthy();
  });

  it("shows loading state", () => {
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));

    render(<WalletPage />, { wrapper: createWrapper() });

    expect(screen.getByText("Loading wallet...")).toBeTruthy();
  });
});
