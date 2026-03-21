import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MatchList } from "./match-list";

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

describe("MatchList", () => {
  it("shows loading state", async () => {
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));

    render(<MatchList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Finding matches...")).toBeTruthy();
    });
  });

  it("renders match cards with scores", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          matches: [
            { id: "abc12345-0000-0000-0000-000000000000", score: 0.92 },
            { id: "def67890-0000-0000-0000-000000000000", score: 0.71 },
          ],
          total: 2,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    render(<MatchList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("92%")).toBeTruthy();
    });
    expect(screen.getByText("71%")).toBeTruthy();
    expect(screen.getByText(/abc12345/)).toBeTruthy();
    expect(screen.getByText(/def67890/)).toBeTruthy();
  });

  it("renders empty state", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ matches: [], total: 0 }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    render(<MatchList />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("No job matches found yet.")).toBeTruthy();
    });
  });
});
