import { QueryClient } from "@tanstack/react-query";
import type { User } from "./api/admin";

export const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
});

/**
 * Called by every API helper on a 401: the session ended (expired, logged out
 * in another tab, disabled by an admin), so the app shows the login screen.
 */
export function sessionEnded() {
  queryClient.setQueryData<User | null>(["me"], null);
}
