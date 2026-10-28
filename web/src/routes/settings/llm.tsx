import { createFileRoute, redirect } from "@tanstack/react-router";

// Renamed: the model settings manage every model type.
export const Route = createFileRoute("/settings/llm")({
  beforeLoad: () => {
    throw redirect({ to: "/settings/models", replace: true });
  },
});
