import { useQuery } from "@tanstack/react-query";
import { authApi, type User } from "@/lib/api/admin";
import { ApiError } from "@/lib/api/client";

/** The logged-in user: null when nobody is, undefined while it loads. */
export function useMe() {
  return useQuery<User | null>({
    queryKey: ["me"],
    queryFn: async () => {
      try {
        return await authApi.me();
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null;
        throw e;
      }
    },
    staleTime: Infinity,
  });
}
