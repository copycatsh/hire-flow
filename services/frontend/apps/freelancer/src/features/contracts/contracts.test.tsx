import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { ContractList } from "./contract-list";
import { ContractDetail } from "./contract-detail";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...props }: { children: ReactNode; to: string; [key: string]: unknown }) => (
    <a href={to} {...props}>{children}</a>
  ),
}));

const mockContracts = [
  {
    id: "contract-001",
    client_id: "client-abc-123",
    freelancer_id: "freelancer-xyz-456",
    title: "Build Landing Page",
    description: "Design and implement a responsive landing page",
    amount: 250000,
    currency: "USD",
    status: "ACTIVE",
    client_wallet_id: "client-abc-123",
    freelancer_wallet_id: "freelancer-xyz-456",
    created_at: "2026-01-15T10:00:00Z",
    updated_at: "2026-01-16T12:00:00Z",
  },
  {
    id: "contract-002",
    client_id: "client-abc-123",
    freelancer_id: "freelancer-xyz-456",
    title: "API Integration",
    description: "Integrate third-party APIs",
    amount: 500000,
    currency: "USD",
    status: "AWAITING_ACCEPT",
    client_wallet_id: "client-abc-123",
    freelancer_wallet_id: "freelancer-xyz-456",
    created_at: "2026-02-01T10:00:00Z",
    updated_at: "2026-02-01T10:00:00Z",
  },
];

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ContractList", () => {
  it("shows loading state", () => {
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));

    render(<ContractList />, { wrapper: createWrapper() });

    expect(screen.getByText("Loading contracts...")).toBeTruthy();
  });

  it("renders contract cards with status badges", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(mockContracts), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    render(<ContractList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Build Landing Page")).toBeInTheDocument();
    });
    expect(screen.getByText("API Integration")).toBeInTheDocument();
    expect(screen.getByText("ACTIVE")).toBeInTheDocument();
    expect(screen.getByText("AWAITING_ACCEPT")).toBeInTheDocument();
  });

  it("shows Action Required badge for AWAITING_ACCEPT contracts", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(mockContracts), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    render(<ContractList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Action Required")).toBeInTheDocument();
    });
  });

  it("renders empty state", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    render(<ContractList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(
        screen.getByText("No contracts yet. Contracts will appear here when a client sends you a proposal."),
      ).toBeInTheDocument();
    });
  });
});

describe("ContractDetail", () => {
  it("shows Accept Contract button for AWAITING_ACCEPT status", async () => {
    const awaitingContract = { ...mockContracts[1] };
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(awaitingContract), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    render(<ContractDetail id="contract-002" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Accept Contract" })).toBeInTheDocument();
    });
  });

  it("does not show Accept button for ACTIVE contracts", async () => {
    const activeContract = { ...mockContracts[0] };
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(activeContract), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    render(<ContractDetail id="contract-001" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Build Landing Page")).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: "Accept Contract" })).not.toBeInTheDocument();
  });
});
