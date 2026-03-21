import { describe, it, expect, vi, beforeEach } from "vitest";
import { apiClient } from "./api-client";

const BASE = "/client";

describe("apiClient", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("sends GET request with credentials", async () => {
    const mockResponse = { id: "123", title: "Test Job" };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockResponse), { status: 200 }),
    );

    const result = await apiClient.get("/api/v1/jobs/123");
    expect(result).toEqual(mockResponse);
    expect(fetch).toHaveBeenCalledWith(
      `${BASE}/api/v1/jobs/123`,
      expect.objectContaining({ credentials: "include", method: "GET" }),
    );
  });

  it("sends POST request with JSON body", async () => {
    const body = { title: "New Job", description: "desc" };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ id: "456" }), { status: 201 }),
    );

    await apiClient.post("/api/v1/jobs", body);
    expect(fetch).toHaveBeenCalledWith(
      `${BASE}/api/v1/jobs`,
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify(body),
        headers: expect.objectContaining({ "Content-Type": "application/json" }),
      }),
    );
  });

  it("throws ApiError on non-2xx response", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "not found" }), { status: 404 }),
    );

    await expect(apiClient.get("/api/v1/jobs/999")).rejects.toThrow("not found");
  });

  it("retries once on 401 after successful refresh", async () => {
    let callCount = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url) => {
      const urlStr = typeof url === "string" ? url : url.toString();
      if (urlStr.includes("/auth/refresh")) {
        return new Response(JSON.stringify({ user_id: "1", role: "client" }), { status: 200 });
      }
      callCount++;
      if (callCount === 1) {
        return new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 });
      }
      return new Response(JSON.stringify({ id: "123" }), { status: 200 });
    });

    const result = await apiClient.get("/api/v1/jobs/123");
    expect(result).toEqual({ id: "123" });
    expect(callCount).toBe(2);
  });

  it("redirects to /login when refresh fails", async () => {
    const assignMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { assign: assignMock, pathname: "/jobs" },
      writable: true,
    });

    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }),
    );

    await expect(apiClient.get("/api/v1/jobs")).rejects.toThrow();
    expect(assignMock).toHaveBeenCalledWith("/login");
  });

  it("deduplicates concurrent refresh calls", async () => {
    let refreshCount = 0;
    let apiCallCount = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url) => {
      const urlStr = typeof url === "string" ? url : url.toString();
      if (urlStr.includes("/auth/refresh")) {
        refreshCount++;
        await new Promise((r) => setTimeout(r, 50));
        return new Response(JSON.stringify({ user_id: "1", role: "client" }), { status: 200 });
      }
      apiCallCount++;
      if (apiCallCount <= 2) {
        return new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 });
      }
      return new Response(JSON.stringify({ ok: true }), { status: 200 });
    });

    const results = await Promise.all([
      apiClient.get("/api/v1/jobs"),
      apiClient.get("/api/v1/wallet"),
    ]);
    expect(refreshCount).toBe(1);
    expect(results).toEqual([{ ok: true }, { ok: true }]);
  });
});
