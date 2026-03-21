import { afterEach, describe, it, expect, vi, beforeEach } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MatchList } from "./match-list";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, ...props }: { children: React.ReactNode; to: string; [key: string]: unknown }) => (
    <a href={props.to as string}>{children}</a>
  ),
}));

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
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
  it("renders match results with scores", async () => {
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

    render(<MatchList jobId="job-1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("92%")).toBeTruthy();
      expect(screen.getByText("71%")).toBeTruthy();
    });

    expect(screen.getByText("Candidate #1")).toBeTruthy();
    expect(screen.getByText("Candidate #2")).toBeTruthy();
  });

  it("shows empty state", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ matches: [], total: 0 }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    render(<MatchList jobId="job-1" />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("No matches found for this job.")).toBeTruthy();
    });
  });
});
