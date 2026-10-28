import { createFileRoute, redirect } from "@tanstack/react-router";

// The first settings page.
export const Route = createFileRoute("/settings/")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/general", replace: true });
  },
});
