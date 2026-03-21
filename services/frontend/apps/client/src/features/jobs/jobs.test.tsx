import { afterEach, describe, it, expect, vi, beforeEach } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { JobList } from "./job-list";
import { CreateJobForm } from "./create-job-form";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...props }: { children: ReactNode; to: string; [key: string]: unknown }) => (
    <a href={to} {...props}>{children}</a>
  ),
  useNavigate: () => vi.fn(),
}));

vi.mock("@/features/auth/auth-context", () => ({
  useAuth: () => ({
    user: { user_id: "11111111-1111-1111-1111-111111111111", role: "client" },
    setUser: vi.fn(),
    logout: vi.fn(),
  }),
}));

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function TestWrapper({ children }: { children: ReactNode }) {
  const queryClient = createTestQueryClient();
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

const mockJobs = [
  {
    id: "job-1",
    title: "React Developer",
    description: "Build React apps",
    budget_min: 100000,
    budget_max: 200000,
    status: "open" as const,
    client_id: "client-1",
    created_at: "2026-01-15T00:00:00Z",
    updated_at: "2026-01-15T00:00:00Z",
  },
  {
    id: "job-2",
    title: "Go Backend Engineer",
    description: "Build Go microservices",
    budget_min: 150000,
    budget_max: 300000,
    status: "draft" as const,
    client_id: "client-1",
    created_at: "2026-02-01T00:00:00Z",
    updated_at: "2026-02-01T00:00:00Z",
  },
];

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("JobList", () => {
  it("shows loading state", () => {
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));

    render(
      <TestWrapper>
        <JobList />
      </TestWrapper>,
    );

    expect(screen.getByText("Loading jobs...")).toBeInTheDocument();
  });

  it("renders job cards with links to detail", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(mockJobs), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    render(
      <TestWrapper>
        <JobList />
      </TestWrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText("React Developer")).toBeInTheDocument();
    });
    expect(screen.getByText("Go Backend Engineer")).toBeInTheDocument();

    const jobLinks = screen.getAllByRole("link").filter((a) => {
      const href = a.getAttribute("href");
      return href?.startsWith("/jobs/") && href !== "/jobs/new";
    });
    expect(jobLinks).toHaveLength(2);
  });

  it("shows empty state", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    render(
      <TestWrapper>
        <JobList />
      </TestWrapper>,
    );

    await waitFor(() => {
      expect(
        screen.getByText("No jobs yet. Create your first job to get started."),
      ).toBeInTheDocument();
    });
  });
});

describe("CreateJobForm", () => {
  beforeEach(() => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({}), { status: 200 }),
    );
  });

  it("validates required fields on submit", async () => {
    const user = userEvent.setup();

    render(
      <TestWrapper>
        <CreateJobForm />
      </TestWrapper>,
    );

    await user.click(screen.getByRole("button", { name: "Post Job" }));

    await waitFor(() => {
      expect(
        screen.getByText("Title must be at least 3 characters"),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText("Description must be at least 10 characters"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Minimum budget must be greater than 0"),
    ).toBeInTheDocument();
  });

  it("submits valid form with client_id", async () => {
    const createdJob = { id: "new-job-1", title: "Test Job" };
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(createdJob), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const user = userEvent.setup();

    render(
      <TestWrapper>
        <CreateJobForm />
      </TestWrapper>,
    );

    await user.type(screen.getByPlaceholderText("e.g. Senior React Developer"), "Test Job Title");
    await user.type(
      screen.getByPlaceholderText("Describe the role, requirements, and expectations..."),
      "This is a detailed job description for testing",
    );
    await user.type(screen.getByPlaceholderText("1000"), "1000");
    await user.type(screen.getByPlaceholderText("5000"), "5000");

    await user.click(screen.getByRole("button", { name: "Post Job" }));

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        expect.stringContaining("/api/v1/jobs"),
        expect.objectContaining({
          method: "POST",
          body: expect.stringContaining('"client_id":"11111111-1111-1111-1111-111111111111"'),
        }),
      );
    });
  });
});
