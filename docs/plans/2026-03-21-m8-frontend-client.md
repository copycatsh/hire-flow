# M8 — Frontend Client App Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the client-facing React SPA — login, post jobs, view AI matches, propose contracts, check wallet balance.

**Architecture:** pnpm monorepo (`services/frontend/`) with shared UI package (`packages/ui/`) and client app (`apps/client/`). SPA at root `/`, BFF API at `/client/*` via Traefik. Vite dev server proxies `/client/*` to Traefik for same-origin cookies. Native `fetch` with TanStack Query for server state, React Context for auth. Vitest + React Testing Library + msw for tests.

**Tech Stack:** React 19, Vite, TanStack Router (file-based), TanStack Query v5, shadcn/ui, Tailwind CSS v4, Zod, Vitest, msw

**Eng Review Decisions (2026-03-21):**
- Full pnpm monorepo from day one (packages/ui/ + apps/client/)
- SPA at root `/`, API at `/client/*` — separate Traefik paths
- CORS in Traefik (dynamic.yml middleware), not BFF code
- Native fetch (no axios)
- COOKIE_SECURE env var in BFF for dev mode
- React Context for auth (no Zustand)
- Feature-first code organization (features/ + routes/)
- Vitest + RTL + msw (no Playwright for now)
- Refresh mutex in API client (prevent concurrent 401 race)

**Design System:** See `DESIGN.md` — industrial/utilitarian, Instrument Sans + DM Sans + Geist Mono, #0F62FE blue primary, 4px spacing, 4-6px border-radius, dark sidebar.

**BFF Endpoints (all prefixed with `/client` in Traefik):**
```
POST /auth/login          → { email, password } → { user_id, role } + sets cookies
POST /auth/refresh        → (refresh_token cookie) → { user_id, role } + new cookies
GET  /api/v1/jobs         → Job[]
GET  /api/v1/jobs/{id}    → Job
POST /api/v1/jobs         → { title, description, budget_min, budget_max } → Job
POST /api/v1/jobs/{id}/matches → MatchResponse { matches: [{id, score}], total }
POST /api/v1/contracts    → { client_id, freelancer_id, title, description, amount, currency, client_wallet_id, freelancer_wallet_id } → Contract
GET  /api/v1/contracts/{id}  → Contract
GET  /api/v1/wallet       → Wallet { id, user_id, balance, currency, available_balance }
```

**Test Users:** `client@example.com` / `password` (user_id: `11111111-...`)

---

## Phase 1: Infrastructure & Scaffold

### Task 1: Add CORS middleware to Traefik

**Files:**
- Modify: `infra/traefik/dynamic.yml`

**Step 1: Add CORS middleware and apply to all BFF routers**

```yaml
# In infra/traefik/dynamic.yml, add to middlewares section:
    cors-headers:
      headers:
        accessControlAllowMethods:
          - GET
          - POST
          - PUT
          - DELETE
          - OPTIONS
        accessControlAllowOriginList:
          - "http://localhost:5173"
        accessControlAllowHeaders:
          - Content-Type
          - Authorization
        accessControlAllowCredentials: true
        accessControlMaxAge: 3600
```

Apply `cors-headers` middleware to all three BFF routers by adding it to their `middlewares` list (e.g., `middlewares: [cors-headers, strip-client]`).

**Step 2: Commit**

```bash
git add infra/traefik/dynamic.yml
git commit -m "feat(m8): add CORS middleware to Traefik for frontend dev"
```

---

### Task 2: Add COOKIE_SECURE env var to BFF client

**Files:**
- Modify: `services/bff/client/cmd/server/main.go`
- Modify: `services/bff/client/cmd/server/auth_handler.go`

**Step 1: Add cookieSecure flag to main.go**

In `main.go`, after the existing env var reads (~line 27), add:

```go
cookieSecure := os.Getenv("COOKIE_SECURE") != "false"
```

Pass it to `AuthConfig`:

```go
auth := &AuthConfig{
    Secret:          []byte(jwtSecret),
    AccessTokenTTL:  15 * time.Minute,
    RefreshTokenTTL: 7 * 24 * time.Hour,
    CookieSecure:    cookieSecure,
}
```

**Step 2: Add CookieSecure field to AuthConfig and use in setTokenCookies**

In `middleware.go`, add `CookieSecure bool` to `AuthConfig` struct.

In `auth_handler.go`, update `setTokenCookies` to use `h.auth.CookieSecure`:

```go
func (h *AuthHandler) setTokenCookies(w http.ResponseWriter, accessToken, refreshToken string) {
    sameSite := http.SameSiteStrictMode
    if !h.auth.CookieSecure {
        sameSite = http.SameSiteLaxMode
    }
    http.SetCookie(w, &http.Cookie{
        Name:     "access_token",
        Value:    accessToken,
        Path:     "/",
        HttpOnly: true,
        Secure:   h.auth.CookieSecure,
        SameSite: sameSite,
        MaxAge:   int(h.auth.AccessTokenTTL.Seconds()),
    })
    http.SetCookie(w, &http.Cookie{
        Name:     "refresh_token",
        Value:    refreshToken,
        Path:     "/auth/refresh",
        HttpOnly: true,
        Secure:   h.auth.CookieSecure,
        SameSite: sameSite,
        MaxAge:   int(h.auth.RefreshTokenTTL.Seconds()),
    })
}
```

**Step 3: Add COOKIE_SECURE=false to compose.yaml for bff-client**

In `compose.yaml`, add to the bff-client service environment:

```yaml
      COOKIE_SECURE: "false"
```

**Step 4: Run existing BFF tests**

Run: `cd services/bff/client && go test ./...`
Expected: All existing tests pass.

**Step 5: Commit**

```bash
git add services/bff/client/cmd/server/main.go services/bff/client/cmd/server/auth_handler.go services/bff/client/cmd/server/middleware.go compose.yaml
git commit -m "feat(m8): add COOKIE_SECURE env var for dev-mode cookies"
```

---

### Task 3: Scaffold pnpm monorepo

**Files:**
- Create: `services/frontend/package.json`
- Create: `services/frontend/pnpm-workspace.yaml`
- Create: `services/frontend/biome.json`
- Create: `services/frontend/.gitignore`

**Step 1: Create root workspace config**

`services/frontend/package.json`:
```json
{
  "name": "hire-flow-frontend",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "pnpm --filter @hire-flow/client dev",
    "build": "pnpm --filter @hire-flow/client build",
    "test": "pnpm --filter @hire-flow/client test",
    "lint": "biome check .",
    "lint:fix": "biome check --write ."
  }
}
```

`services/frontend/pnpm-workspace.yaml`:
```yaml
packages:
  - "apps/*"
  - "packages/*"
```

`services/frontend/biome.json`:
```json
{
  "$schema": "https://biomejs.dev/schemas/2.0/schema.json",
  "organizeImports": { "enabled": true },
  "linter": {
    "enabled": true,
    "rules": {
      "recommended": true
    }
  },
  "formatter": {
    "enabled": true,
    "indentStyle": "tab"
  }
}
```

`services/frontend/.gitignore`:
```
node_modules
dist
.vite
*.local
```

**Step 2: Initialize pnpm workspace**

Run:
```bash
cd services/frontend && pnpm install
```

**Step 3: Commit**

```bash
git add services/frontend/package.json services/frontend/pnpm-workspace.yaml services/frontend/biome.json services/frontend/.gitignore
git commit -m "feat(m8): scaffold pnpm monorepo for frontend"
```

---

### Task 4: Scaffold shared UI package (packages/ui)

**Files:**
- Create: `services/frontend/packages/ui/package.json`
- Create: `services/frontend/packages/ui/tsconfig.json`
- Create: `services/frontend/packages/ui/src/index.ts`
- Create: `services/frontend/packages/ui/src/lib/utils.ts`
- Create: `services/frontend/packages/ui/tailwind.config.ts`
- Create: `services/frontend/packages/ui/src/styles/globals.css`

**Step 1: Create package.json for UI package**

`services/frontend/packages/ui/package.json`:
```json
{
  "name": "@hire-flow/ui",
  "private": true,
  "type": "module",
  "exports": {
    ".": "./src/index.ts",
    "./globals.css": "./src/styles/globals.css",
    "./lib/utils": "./src/lib/utils.ts"
  },
  "dependencies": {
    "class-variance-authority": "^0.7.1",
    "clsx": "^2.1.1",
    "tailwind-merge": "^3.0.2"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "tailwindcss": "^4.0.0",
    "typescript": "^5.7.0"
  },
  "peerDependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  }
}
```

**Step 2: Create Tailwind config with DESIGN.md tokens**

`services/frontend/packages/ui/tailwind.config.ts`:
```typescript
import type { Config } from "tailwindcss";

export default {
  content: [
    "./src/**/*.{ts,tsx}",
    "../../apps/*/src/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: "#0F62FE",
          hover: "#0043CE",
          light: "#EDF5FF",
        },
        success: { DEFAULT: "#198038", bg: "#DEFBE6" },
        warning: { DEFAULT: "#B28600", bg: "#FFF8E1" },
        error: { DEFAULT: "#DA1E28", bg: "#FFF1F1" },
        info: { DEFAULT: "#0043CE", bg: "#EDF5FF" },
        sidebar: {
          bg: "#161616",
          text: "#C6C6C6",
          active: "#FFFFFF",
          hover: "#262626",
        },
        border: { DEFAULT: "#E0E0E0", strong: "#C6C6C6" },
        background: { DEFAULT: "#FFFFFF", subtle: "#FAFAFA", muted: "#F4F4F4" },
        foreground: { DEFAULT: "#161616", secondary: "#525252", placeholder: "#A8A8A8" },
      },
      fontFamily: {
        display: ["Instrument Sans", "sans-serif"],
        body: ["DM Sans", "sans-serif"],
        mono: ["Geist Mono", "monospace"],
      },
      borderRadius: {
        sm: "4px",
        md: "6px",
        lg: "8px",
        full: "9999px",
      },
      spacing: {
        "2xs": "2px",
        xs: "4px",
        sm: "8px",
        md: "16px",
        lg: "24px",
        xl: "32px",
        "2xl": "48px",
        "3xl": "64px",
      },
    },
  },
  plugins: [],
} satisfies Config;
```

**Step 3: Create globals.css with font imports and base styles**

`services/frontend/packages/ui/src/styles/globals.css`:
```css
@import "tailwindcss";

@theme {
  --font-display: "Instrument Sans", sans-serif;
  --font-body: "DM Sans", sans-serif;
  --font-mono: "Geist Mono", monospace;
}
```

**Step 4: Create utils (cn helper)**

`services/frontend/packages/ui/src/lib/utils.ts`:
```typescript
import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

**Step 5: Create barrel export**

`services/frontend/packages/ui/src/index.ts`:
```typescript
export { cn } from "./lib/utils";
```

**Step 6: Create tsconfig**

`services/frontend/packages/ui/tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "declaration": true,
    "declarationMap": true,
    "outDir": "dist",
    "paths": {
      "@/*": ["./src/*"]
    }
  },
  "include": ["src"]
}
```

**Step 7: Install dependencies**

Run:
```bash
cd services/frontend && pnpm install
```

**Step 8: Commit**

```bash
git add services/frontend/packages/
git commit -m "feat(m8): scaffold shared UI package with design tokens"
```

---

### Task 5: Scaffold client app (apps/client)

**Files:**
- Create: `services/frontend/apps/client/package.json`
- Create: `services/frontend/apps/client/tsconfig.json`
- Create: `services/frontend/apps/client/tsconfig.app.json`
- Create: `services/frontend/apps/client/vite.config.ts`
- Create: `services/frontend/apps/client/index.html`
- Create: `services/frontend/apps/client/src/main.tsx`
- Create: `services/frontend/apps/client/src/vite-env.d.ts`

**Step 1: Create package.json**

`services/frontend/apps/client/package.json`:
```json
{
  "name": "@hire-flow/client",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "dependencies": {
    "@hire-flow/ui": "workspace:*",
    "@tanstack/react-query": "^5.64.0",
    "@tanstack/react-router": "^1.95.0",
    "@tanstack/router-devtools": "^1.95.0",
    "@tanstack/router-plugin": "^1.95.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "zod": "^3.24.0"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.6.0",
    "@testing-library/react": "^16.1.0",
    "@testing-library/user-event": "^14.5.0",
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.3.0",
    "jsdom": "^25.0.0",
    "msw": "^2.7.0",
    "tailwindcss": "^4.0.0",
    "typescript": "^5.7.0",
    "vite": "^6.0.0",
    "vitest": "^3.0.0"
  }
}
```

**Step 2: Create vite.config.ts with proxy**

`services/frontend/apps/client/vite.config.ts`:
```typescript
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { TanStackRouterVite } from "@tanstack/router-plugin/vite";
import path from "node:path";

export default defineConfig({
  plugins: [TanStackRouterVite({ quoteStyle: "double" }), react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/client": {
        target: "http://localhost:80",
        changeOrigin: true,
      },
    },
  },
});
```

**Step 3: Create index.html**

`services/frontend/apps/client/index.html`:
```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>hire-flow</title>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,300;0,9..40,400;0,9..40,500;0,9..40,600;0,9..40,700;1,9..40,400&family=Instrument+Sans:wght@400;500;600;700&family=Geist+Mono:wght@400;500;600&display=swap" rel="stylesheet" />
</head>
<body>
  <div id="root"></div>
  <script type="module" src="/src/main.tsx"></script>
</body>
</html>
```

**Step 4: Create main.tsx (minimal — just React mount)**

`services/frontend/apps/client/src/main.tsx`:
```typescript
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@hire-flow/ui/globals.css";

const root = document.getElementById("root");
if (!root) throw new Error("Root element not found");

createRoot(root).render(
  <StrictMode>
    <div>hire-flow client app</div>
  </StrictMode>,
);
```

`services/frontend/apps/client/src/vite-env.d.ts`:
```typescript
/// <reference types="vite/client" />
```

**Step 5: Create tsconfig files**

`services/frontend/apps/client/tsconfig.json`:
```json
{
  "files": [],
  "references": [{ "path": "./tsconfig.app.json" }]
}
```

`services/frontend/apps/client/tsconfig.app.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "paths": {
      "@/*": ["./src/*"]
    }
  },
  "include": ["src"]
}
```

**Step 6: Install all dependencies**

Run:
```bash
cd services/frontend && pnpm install
```

**Step 7: Verify dev server starts**

Run:
```bash
cd services/frontend && pnpm dev
```
Expected: Vite dev server at http://localhost:5173 showing "hire-flow client app".

**Step 8: Commit**

```bash
git add services/frontend/apps/client/ services/frontend/pnpm-lock.yaml
git commit -m "feat(m8): scaffold Vite client app with TanStack + proxy"
```

---

## Phase 2: API Client & Auth

### Task 6: API client with refresh mutex

**Files:**
- Create: `services/frontend/apps/client/src/lib/api-client.ts`
- Create: `services/frontend/apps/client/src/lib/api-client.test.ts`

**Step 1: Write the failing tests**

`services/frontend/apps/client/src/lib/api-client.test.ts`:
```typescript
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
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
```

**Step 2: Set up vitest config**

Create `services/frontend/apps/client/vitest.config.ts`:
```typescript
import { defineConfig } from "vitest/config";
import path from "node:path";

export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    globals: true,
  },
});
```

Create `services/frontend/apps/client/src/test/setup.ts`:
```typescript
import "@testing-library/jest-dom/vitest";
```

**Step 3: Run tests to verify they fail**

Run: `cd services/frontend/apps/client && pnpm test`
Expected: FAIL — module `./api-client` not found.

**Step 4: Implement api-client**

`services/frontend/apps/client/src/lib/api-client.ts`:
```typescript
const BASE_URL = "/client";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

let refreshPromise: Promise<void> | null = null;

async function refreshToken(): Promise<void> {
  const res = await fetch(`${BASE_URL}/auth/refresh`, {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) {
    throw new ApiError(res.status, "refresh failed");
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const opts: RequestInit = {
    method,
    credentials: "include",
    headers: {} as Record<string, string>,
  };

  if (body !== undefined) {
    (opts.headers as Record<string, string>)["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }

  let res = await fetch(`${BASE_URL}${path}`, opts);

  if (res.status === 401) {
    try {
      if (!refreshPromise) {
        refreshPromise = refreshToken();
      }
      await refreshPromise;
      refreshPromise = null;
      res = await fetch(`${BASE_URL}${path}`, opts);
    } catch {
      refreshPromise = null;
      window.location.assign("/login");
      throw new ApiError(401, "session expired");
    }
  }

  if (!res.ok) {
    const data = await res.json().catch(() => ({ error: "request failed" }));
    throw new ApiError(res.status, data.error || `HTTP ${res.status}`);
  }

  return res.json() as Promise<T>;
}

export const apiClient = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  put: <T>(path: string, body?: unknown) => request<T>("PUT", path, body),
};
```

**Step 5: Run tests**

Run: `cd services/frontend/apps/client && pnpm test`
Expected: All 6 tests pass.

**Step 6: Commit**

```bash
git add services/frontend/apps/client/src/lib/ services/frontend/apps/client/src/test/ services/frontend/apps/client/vitest.config.ts
git commit -m "feat(m8): API client with fetch, 401 refresh mutex, tests"
```

---

### Task 7: Auth context & login mutation

**Files:**
- Create: `services/frontend/apps/client/src/features/auth/auth-context.tsx`
- Create: `services/frontend/apps/client/src/features/auth/use-login.ts`
- Create: `services/frontend/apps/client/src/features/auth/types.ts`
- Create: `services/frontend/apps/client/src/features/auth/auth-context.test.tsx`

**Step 1: Create auth types**

`services/frontend/apps/client/src/features/auth/types.ts`:
```typescript
export interface AuthUser {
  user_id: string;
  role: string;
}
```

**Step 2: Write failing tests for auth context**

`services/frontend/apps/client/src/features/auth/auth-context.test.tsx`:
```typescript
import { describe, it, expect, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
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
});
```

**Step 3: Run to verify failure**

Run: `cd services/frontend/apps/client && pnpm test`
Expected: FAIL — module not found.

**Step 4: Implement auth context**

`services/frontend/apps/client/src/features/auth/auth-context.tsx`:
```typescript
import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import type { AuthUser } from "./types";

interface AuthContextValue {
  user: AuthUser | null;
  setUser: (user: AuthUser) => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUserState] = useState<AuthUser | null>(null);

  const setUser = useCallback((u: AuthUser) => {
    setUserState(u);
  }, []);

  const logout = useCallback(() => {
    setUserState(null);
    window.location.assign("/login");
  }, []);

  return (
    <AuthContext.Provider value={{ user, setUser, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
```

**Step 5: Implement login mutation hook**

`services/frontend/apps/client/src/features/auth/use-login.ts`:
```typescript
import { useMutation } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import { useAuth } from "./auth-context";
import type { AuthUser } from "./types";

interface LoginRequest {
  email: string;
  password: string;
}

export function useLogin() {
  const { setUser } = useAuth();

  return useMutation({
    mutationFn: (data: LoginRequest) =>
      apiClient.post<AuthUser>("/auth/login", data),
    onSuccess: (data) => {
      setUser(data);
    },
  });
}
```

**Step 6: Run tests**

Run: `cd services/frontend/apps/client && pnpm test`
Expected: All tests pass.

**Step 7: Commit**

```bash
git add services/frontend/apps/client/src/features/auth/
git commit -m "feat(m8): auth context, login mutation hook with tests"
```

---

## Phase 3: TanStack Router & Layouts

### Task 8: Router setup with auth guard

**Files:**
- Create: `services/frontend/apps/client/src/routes/__root.tsx`
- Create: `services/frontend/apps/client/src/routes/_auth.tsx`
- Create: `services/frontend/apps/client/src/routes/login.tsx`
- Create: `services/frontend/apps/client/src/routes/_auth/jobs.index.tsx`
- Modify: `services/frontend/apps/client/src/main.tsx`

**Step 1: Create root route with providers**

`services/frontend/apps/client/src/routes/__root.tsx`:
```typescript
import { createRootRoute, Outlet } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "@/features/auth/auth-context";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

export const Route = createRootRoute({
  component: function RootLayout() {
    return (
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <Outlet />
        </AuthProvider>
      </QueryClientProvider>
    );
  },
});
```

**Step 2: Create auth layout (protected routes)**

`services/frontend/apps/client/src/routes/_auth.tsx`:
```typescript
import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuth } from "@/features/auth/auth-context";
import { Sidebar } from "@/features/layout/sidebar";

export const Route = createFileRoute("/_auth")({
  component: function AuthLayout() {
    const { user } = useAuth();
    if (!user) {
      throw redirect({ to: "/login" });
    }
    return (
      <div className="flex h-screen">
        <Sidebar />
        <main className="flex-1 overflow-auto bg-background-subtle p-lg">
          <Outlet />
        </main>
      </div>
    );
  },
});
```

**Step 3: Create login route**

`services/frontend/apps/client/src/routes/login.tsx`:
```typescript
import { createFileRoute, redirect } from "@tanstack/react-router";
import { LoginPage } from "@/features/auth/login-page";

export const Route = createFileRoute("/login")({
  component: LoginPage,
});
```

**Step 4: Create placeholder jobs index route**

`services/frontend/apps/client/src/routes/_auth/jobs.index.tsx`:
```typescript
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/_auth/jobs/")({
  component: function JobsPage() {
    return <div>Jobs page — coming in Task 10</div>;
  },
});
```

**Step 5: Update main.tsx with router**

`services/frontend/apps/client/src/main.tsx`:
```typescript
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { routeTree } from "./routeTree.gen";
import "@hire-flow/ui/globals.css";

const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const root = document.getElementById("root");
if (!root) throw new Error("Root element not found");

createRoot(root).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
);
```

**Step 6: Create sidebar component**

Create `services/frontend/apps/client/src/features/layout/sidebar.tsx` — a dark sidebar with nav items for Jobs, Matches, Contracts, Wallet per DESIGN.md. Links use TanStack Router `<Link>`. Active state highlighted. 240px wide. Logo "hire-flow" at top. Use Lucide icons (`Briefcase`, `Users`, `FileText`, `Wallet`, `Settings`).

Add `lucide-react` to client app dependencies.

**Step 7: Create login page component**

Create `services/frontend/apps/client/src/features/auth/login-page.tsx` — split layout per DESIGN.md preview (dark branded left, form right). Email + password inputs, submit button, error alert. Uses `useLogin` hook. On success, navigate to `/jobs`.

**Step 8: Run dev server and verify**

Run: `cd services/frontend && pnpm dev`
Expected: Login page renders at http://localhost:5173/login.

**Step 9: Commit**

```bash
git add services/frontend/apps/client/src/
git commit -m "feat(m8): TanStack Router setup, auth layout, login page, sidebar"
```

---

## Phase 4: Feature Pages

### Task 9: TypeScript types & query hooks

**Files:**
- Create: `services/frontend/apps/client/src/features/jobs/types.ts`
- Create: `services/frontend/apps/client/src/features/jobs/queries.ts`
- Create: `services/frontend/apps/client/src/features/matches/types.ts`
- Create: `services/frontend/apps/client/src/features/matches/queries.ts`
- Create: `services/frontend/apps/client/src/features/contracts/types.ts`
- Create: `services/frontend/apps/client/src/features/contracts/queries.ts`
- Create: `services/frontend/apps/client/src/features/wallet/types.ts`
- Create: `services/frontend/apps/client/src/features/wallet/queries.ts`

**Step 1: Create types matching backend JSON shapes exactly**

`services/frontend/apps/client/src/features/jobs/types.ts`:
```typescript
export interface Job {
  id: string;
  title: string;
  description: string;
  budget_min: number;
  budget_max: number;
  status: "draft" | "open" | "in_progress" | "closed";
  client_id: string;
  created_at: string;
  updated_at: string;
}

export interface CreateJobRequest {
  title: string;
  description: string;
  budget_min: number;
  budget_max: number;
}
```

`services/frontend/apps/client/src/features/matches/types.ts`:
```typescript
export interface MatchResult {
  id: string;
  score: number;
}

export interface MatchResponse {
  matches: MatchResult[];
  total: number;
}
```

`services/frontend/apps/client/src/features/contracts/types.ts`:
```typescript
export interface Contract {
  id: string;
  client_id: string;
  freelancer_id: string;
  title: string;
  description: string;
  amount: number;
  currency: string;
  status: "PENDING" | "HOLD_PENDING" | "AWAITING_ACCEPT" | "ACTIVE" | "COMPLETING" | "COMPLETED" | "DECLINING" | "DECLINED" | "CANCELLED";
  client_wallet_id: string;
  freelancer_wallet_id: string;
  hold_id?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateContractRequest {
  client_id: string;
  freelancer_id: string;
  title: string;
  description: string;
  amount: number;
  currency: string;
  client_wallet_id: string;
  freelancer_wallet_id: string;
}
```

`services/frontend/apps/client/src/features/wallet/types.ts`:
```typescript
export interface Wallet {
  id: string;
  user_id: string;
  balance: number;
  currency: string;
  available_balance: number;
  created_at: string;
  updated_at: string;
}
```

**Step 2: Create TanStack Query hooks for each feature**

Each queries file exports `useX` hooks wrapping `apiClient` with `useQuery`/`useMutation`. Query keys: `["jobs"]`, `["jobs", id]`, `["matches", jobId]`, `["contracts", id]`, `["wallet"]`.

Example — `services/frontend/apps/client/src/features/jobs/queries.ts`:
```typescript
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";
import type { Job, CreateJobRequest } from "./types";

export function useJobs() {
  return useQuery({
    queryKey: ["jobs"],
    queryFn: () => apiClient.get<Job[]>("/api/v1/jobs"),
  });
}

export function useJob(id: string) {
  return useQuery({
    queryKey: ["jobs", id],
    queryFn: () => apiClient.get<Job>(`/api/v1/jobs/${id}`),
    enabled: !!id,
  });
}

export function useCreateJob() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateJobRequest) =>
      apiClient.post<Job>("/api/v1/jobs", data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });
}
```

Same pattern for matches (`useMatches(jobId)` → POST), contracts (`useContract(id)`, `useCreateContract()`), wallet (`useWallet()` → GET).

**Step 3: Commit**

```bash
git add services/frontend/apps/client/src/features/
git commit -m "feat(m8): TypeScript types and TanStack Query hooks for all features"
```

---

### Task 10: Jobs pages (list, detail, create)

**Files:**
- Modify: `services/frontend/apps/client/src/routes/_auth/jobs.index.tsx`
- Create: `services/frontend/apps/client/src/routes/_auth/jobs.$id.tsx`
- Create: `services/frontend/apps/client/src/routes/_auth/jobs.new.tsx`
- Create: `services/frontend/apps/client/src/features/jobs/job-list.tsx`
- Create: `services/frontend/apps/client/src/features/jobs/job-detail.tsx`
- Create: `services/frontend/apps/client/src/features/jobs/create-job-form.tsx`

**Step 1: Write component tests for job list**

Test that the job list renders table rows from mocked data, shows empty state, and shows loading state. Use msw to mock `GET /client/api/v1/jobs`.

**Step 2: Implement job list page**

A table matching DESIGN.md: title (font-weight-500), status (badge), budget (mono font, formatted as `$X,XXX`), created date (mono, relative). Header with "Your Jobs" title and "+ New Job" primary button linking to `/jobs/new`. Use `useJobs()` hook.

**Step 3: Write component tests for job detail**

Test that job detail renders title, description, budget range, status. Include "Find Matches" primary button. Test 404 state.

**Step 4: Implement job detail page**

Card layout with job info. "Find Matches" button triggers match search (Task 11). Status badge. Budget displayed as range.

**Step 5: Write component tests for create job form**

Test form validation (empty title, invalid budget), successful submission redirects to detail page.

**Step 6: Implement create job form**

Form with: title (input), description (textarea), budget_min (number input), budget_max (number input). Zod validation schema. Uses `useCreateJob()`. On success, navigate to `/jobs/${newJob.id}`.

**Step 7: Run all tests**

Run: `cd services/frontend/apps/client && pnpm test`
Expected: All tests pass.

**Step 8: Commit**

```bash
git add services/frontend/apps/client/src/
git commit -m "feat(m8): jobs pages — list, detail, create with tests"
```

---

### Task 11: Matches page

**Files:**
- Create: `services/frontend/apps/client/src/routes/_auth/jobs.$id.matches.tsx`
- Create: `services/frontend/apps/client/src/features/matches/match-list.tsx`

**Step 1: Write component tests**

Test that match results render with score (percentage), freelancer ID, and "Propose Contract" button. Test empty state.

**Step 2: Implement matches page**

Route: `/jobs/:id/matches`. Uses `useMatches(jobId)`. Displays match results as cards with:
- Freelancer ID (truncated UUID)
- Score as percentage with color coding (>80% green, >60% yellow, <60% gray)
- "Propose Contract" button linking to contract creation

**Step 3: Run tests**

Run: `cd services/frontend/apps/client && pnpm test`
Expected: Pass.

**Step 4: Commit**

```bash
git add services/frontend/apps/client/src/
git commit -m "feat(m8): matches page with score display and propose action"
```

---

### Task 12: Contracts page

**Files:**
- Create: `services/frontend/apps/client/src/routes/_auth/contracts.tsx`
- Create: `services/frontend/apps/client/src/features/contracts/contract-detail.tsx`
- Create: `services/frontend/apps/client/src/features/contracts/propose-contract-form.tsx`

**Step 1: Write component tests**

Test contract detail renders all fields (title, amount in mono font, status badge, dates). Test propose form submits with correct payload.

**Step 2: Implement contract proposal**

The "Propose Contract" flow: user clicks from matches → form pre-fills freelancer_id and client_id from auth context. User enters title, description, amount. Submits via `useCreateContract()`. Shows resulting contract detail.

Route: `/contracts` shows contract detail by ID (passed via search params or navigation state).

**Step 3: Run tests and commit**

```bash
git add services/frontend/apps/client/src/
git commit -m "feat(m8): contract proposal and detail page with tests"
```

---

### Task 13: Wallet page

**Files:**
- Create: `services/frontend/apps/client/src/routes/_auth/wallet.tsx`
- Create: `services/frontend/apps/client/src/features/wallet/wallet-page.tsx`

**Step 1: Write component tests**

Test wallet renders balance and available balance in mono font, formatted as currency. Test loading state.

**Step 2: Implement wallet page**

Simple card showing:
- Total balance (large, mono font)
- Available balance (smaller, with "held" amount calculated)
- Currency

Uses `useWallet()` hook. Amounts divided by 100 for display (backend stores cents).

**Step 3: Run tests and commit**

```bash
git add services/frontend/apps/client/src/
git commit -m "feat(m8): wallet balance page with tests"
```

---

## Phase 5: Integration & Polish

### Task 14: Index redirect & route completion

**Files:**
- Create: `services/frontend/apps/client/src/routes/index.tsx`

**Step 1: Create index redirect**

`services/frontend/apps/client/src/routes/index.tsx`:
```typescript
import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
  beforeLoad: () => {
    throw redirect({ to: "/login" });
  },
});
```

**Step 2: Verify all routes work**

Manual verification checklist:
1. `/` redirects to `/login`
2. `/login` shows login form
3. After login → redirects to `/jobs`
4. `/jobs` shows job list
5. `/jobs/new` shows create form
6. `/jobs/:id` shows detail
7. `/jobs/:id/matches` shows matches
8. `/contracts` shows contract detail
9. `/wallet` shows balance

**Step 3: Commit**

```bash
git add services/frontend/apps/client/src/routes/
git commit -m "feat(m8): index redirect, route completion"
```

---

### Task 15: Update TODOS.md & CLAUDE.md

**Files:**
- Modify: `TODOS.md`
- Modify: `CLAUDE.md`

**Step 1: Mark CORS TODO as completed**

In `TODOS.md`, move "Add CORS middleware to BFFs" to Completed section with resolution: "CORS handled in Traefik middleware (dynamic.yml), not BFF layer."

**Step 2: Add Playwright e2e TODO**

Add to Pending in `TODOS.md`:
```markdown
### Add Playwright e2e tests for client frontend
- **What:** End-to-end browser tests for login → create job → view matches → propose contract flow
- **Why:** Vitest + RTL tests mock the API. E2e tests verify the real stack works together (CORS, cookies, Traefik routing)
- **Context:** M8 ships with Vitest + RTL + msw. E2e tests require Docker Compose running.
- **Depends on:** M8 complete
```

**Step 3: Update CLAUDE.md ports table**

Add frontend dev server to the ports table:
```
| frontend-client  | 5173 | Vite    |
```

**Step 4: Commit**

```bash
git add TODOS.md CLAUDE.md
git commit -m "docs(m8): update TODOS (CORS done, Playwright TODO), add frontend port"
```

---

### Task 16: Final test run & verification

**Step 1: Run all frontend tests**

Run: `cd services/frontend/apps/client && pnpm test`
Expected: All tests pass (18+ test cases covering all codepaths from eng review test diagram).

**Step 2: Run lint**

Run: `cd services/frontend && pnpm lint`
Expected: No errors.

**Step 3: Run BFF tests (regression check)**

Run: `cd services/bff/client && go test ./...`
Expected: All existing tests pass.

**Step 4: Build check**

Run: `cd services/frontend/apps/client && pnpm build`
Expected: Vite builds successfully, output in `dist/`.

**Step 5: Commit any fixes from verification**

If any issues found, fix and commit individually.

---

## Summary

| Phase | Tasks | What it delivers |
|-------|-------|-----------------|
| 1. Infrastructure | 1-5 | CORS, cookie fix, pnpm monorepo, UI package, Vite app scaffold |
| 2. API & Auth | 6-7 | Fetch client with refresh mutex, auth context, login hook |
| 3. Router & Layout | 8 | TanStack Router, auth guard, sidebar, login page |
| 4. Feature Pages | 9-13 | Types, query hooks, jobs CRUD, matches, contracts, wallet |
| 5. Integration | 14-16 | Redirects, docs, final verification |

**Total: 16 tasks, ~18 test cases, 0 new Go abstractions, 1 small BFF change (cookie env var)**
