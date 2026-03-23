import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider, type AuthState } from "@hire-flow/ui";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

interface RouterContext {
  auth: AuthState;
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ context }) => {
    await context.auth.restore();
  },
  component: function RootLayout() {
    const { auth } = Route.useRouteContext();
    return (
      <QueryClientProvider client={queryClient}>
        <AuthProvider auth={auth}>
          <Outlet />
        </AuthProvider>
      </QueryClientProvider>
    );
  },
});
