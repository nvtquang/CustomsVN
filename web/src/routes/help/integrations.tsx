import { createFileRoute, redirect } from "@tanstack/react-router";

// Moved into the settings.
export const Route = createFileRoute("/help/integrations")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/integrations", replace: true });
  },
});
