import { createFileRoute, redirect } from "@tanstack/react-router";

// Renamed: chunking joined the retrieval settings.
export const Route = createFileRoute("/settings/retrieval")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/knowledge", replace: true });
  },
});
