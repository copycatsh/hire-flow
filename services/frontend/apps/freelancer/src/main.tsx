import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { AuthState } from "@hire-flow/ui";
import { routeTree } from "./routeTree.gen";
import "@hire-flow/ui/globals.css";

const auth = new AuthState("/freelancer");

const router = createRouter({
  routeTree,
  context: { auth },
});

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
