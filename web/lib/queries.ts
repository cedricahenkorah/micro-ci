import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api/client";

export const runKeys = {
  all: ["runs"] as const,
  lists: ["runs", "list"] as const,
  list: (page: number) => ["runs", "list", page] as const,
  detail: (id: number) => ["runs", "detail", id] as const,
};

export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 2_000,
        retry: (count, error) =>
          count < 2 &&
          !(
            error instanceof ApiError &&
            error.status >= 400 &&
            error.status < 500
          ),
      },
      mutations: { retry: false },
    },
  });
}
