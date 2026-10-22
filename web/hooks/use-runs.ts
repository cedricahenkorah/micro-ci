"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { cancelRun, getRun, listRuns, retryRun } from "@/lib/api/client";
import { runKeys } from "@/lib/queries";

export function useRuns(page: number) {
  return useQuery({
    queryKey: runKeys.list(page),
    queryFn: ({ signal }) => listRuns(page, 20, signal),
    refetchInterval: 5_000,
    placeholderData: keepPreviousData,
  });
}

export function useRun(id: string) {
  return useQuery({
    queryKey: runKeys.detail(id),
    queryFn: ({ signal }) => getRun(id, signal),
    refetchInterval: (query) => {
      const status = query.state.data?.status;

      return status === "queued" || status === "running" ? 5_000 : false;
    },
  });
}

export function useCancelRun() {
  const client = useQueryClient();

  return useMutation({
    mutationFn: cancelRun,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: runKeys.all });
    },
  });
}

export function useRetryRun() {
  const client = useQueryClient();

  return useMutation({
    mutationFn: retryRun,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: runKeys.lists });
    },
  });
}
