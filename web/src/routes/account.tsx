import { createFileRoute, redirect } from "@tanstack/react-router";

// Moved into the settings.
export const Route = createFileRoute("/account")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/account", replace: true });
  },
});
