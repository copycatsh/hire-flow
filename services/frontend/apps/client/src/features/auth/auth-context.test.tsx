import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, screen, act } from "@testing-library/react";
import { AuthProvider, useAuth } from "./auth-context";

function TestConsumer() {
  const { user, setUser, logout } = useAuth();
  return (
    <div>
      <span data-testid="user">{user ? user.user_id : "null"}</span>
      <button onClick={() => setUser({ user_id: "123", role: "client" })}>login</button>
      <button onClick={logout}>logout</button>
    </div>
  );
}

afterEach(() => {
  cleanup();
});

describe("AuthContext", () => {
  it("starts with null user", () => {
    render(<AuthProvider><TestConsumer /></AuthProvider>);
    expect(screen.getByTestId("user").textContent).toBe("null");
  });

  it("sets user after login", () => {
    render(<AuthProvider><TestConsumer /></AuthProvider>);
    act(() => screen.getByText("login").click());
    expect(screen.getByTestId("user").textContent).toBe("123");
  });

  it("clears user on logout", () => {
    Object.defineProperty(window, "location", {
      value: { assign: vi.fn() },
      writable: true,
    });
    render(<AuthProvider><TestConsumer /></AuthProvider>);
    act(() => screen.getByText("login").click());
    act(() => screen.getByText("logout").click());
    expect(screen.getByTestId("user").textContent).toBe("null");
  });

  it("throws when used outside provider", () => {
    expect(() => render(<TestConsumer />)).toThrow("useAuth must be used within AuthProvider");
  });
});
