import { useQueryClient } from "@tanstack/react-query";
import { authApi } from "@/lib/api/admin";

/** Ends the session and forgets everything cached for this user. */
export function useLogout() {
  const qc = useQueryClient();
  return async () => {
    try {
      await authApi.logout();
    } finally {
      qc.setQueryData(["me"], null);
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== "me" });
    }
  };
}
