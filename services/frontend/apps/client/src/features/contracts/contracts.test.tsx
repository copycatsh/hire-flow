import { afterEach, describe, it, expect, vi, beforeEach } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ContractDetail } from "./contract-detail";
import { ProposeContractForm } from "./propose-contract-form";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, ...props }: any) => <a {...props}>{children}</a>,
  useNavigate: () => vi.fn(),
}));

vi.mock("@/features/auth/auth-context", () => ({
  useAuth: () => ({
    user: { user_id: "client-abc-123", role: "client" },
    setUser: vi.fn(),
    logout: vi.fn(),
  }),
}));

const mockContract = {
  id: "contract-001",
  client_id: "client-abc-123",
  freelancer_id: "freelancer-xyz-456",
  title: "Build Landing Page",
  description: "Design and implement a responsive landing page",
  amount: 250000,
  currency: "USD",
  status: "ACTIVE" as const,
  client_wallet_id: "client-abc-123",
  freelancer_wallet_id: "freelancer-xyz-456",
  created_at: "2026-01-15T10:00:00Z",
  updated_at: "2026-01-16T12:00:00Z",
};

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ContractDetail", () => {
  beforeEach(() => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockContract), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });

  it("renders contract title, amount, and status badge", async () => {
    render(<ContractDetail id="contract-001" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Build Landing Page")).toBeInTheDocument();
    });

    expect(screen.getByText("$2500.00")).toBeInTheDocument();
    expect(screen.getByText("ACTIVE")).toBeInTheDocument();
  });

  it("shows truncated freelancer id", async () => {
    render(<ContractDetail id="contract-001" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/freelanc/)).toBeInTheDocument();
    });
  });
});

describe("ProposeContractForm", () => {
  it("renders with pre-filled freelancer id", () => {
    render(<ProposeContractForm freelancerId="freelancer-xyz-456" />, {
      wrapper: createWrapper(),
    });

    expect(screen.getByText("Propose a Contract")).toBeInTheDocument();
    expect(screen.getByText(/freelanc/)).toBeInTheDocument();
  });

  it("submits the form with correct data", async () => {
    const mockResponse = { ...mockContract, status: "PENDING" as const };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockResponse), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const user = userEvent.setup();

    render(<ProposeContractForm freelancerId="freelancer-xyz-456" />, {
      wrapper: createWrapper(),
    });

    await user.type(screen.getByPlaceholderText("Contract title"), "Build Landing Page");
    await user.type(screen.getByPlaceholderText("Describe the scope of work"), "Full design and dev");
    await user.type(screen.getByPlaceholderText("0.00"), "2500");

    await user.click(screen.getByRole("button", { name: "Submit Proposal" }));

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        "/client/api/v1/contracts",
        expect.objectContaining({ method: "POST" }),
      );
    });
  });
});
