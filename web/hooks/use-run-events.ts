"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { subscribeToRun, type ConnectionState } from "@/lib/api/events";
import type { LogEvent } from "@/lib/api/types";
import { runKeys } from "@/lib/queries";

export function useRunEvents(id: string) {
  const client = useQueryClient();
  const [logs, setLogs] = useState<LogEvent[]>([]);
  const [connection, setConnection] = useState<ConnectionState>("connecting");
  const [error, setError] = useState<string>();

  useEffect(() => {
    let pending: LogEvent[] = [];
    let refreshTimer: ReturnType<typeof setTimeout> | undefined;
    let firstBatch = true;

    const refresh = () => {
      if (refreshTimer) {
        return;
      }

      refreshTimer = setTimeout(() => {
        refreshTimer = undefined;
        void client.invalidateQueries({ queryKey: runKeys.detail(id) });
        void client.invalidateQueries({ queryKey: runKeys.lists });
      }, 150);
    };

    const flush = setInterval(() => {
      if (!pending.length) {
        return;
      }

      const batch = pending;
      const replace = firstBatch;

      pending = [];
      firstBatch = false;

      setLogs((previous) => (replace ? batch : [...previous, ...batch]));
    }, 100);

    const close = subscribeToRun(id, {
      onEvent: (event) => {
        if (event.type === "log") {
          pending.push(event);
        } else {
          refresh();
        }
      },
      onConnection: (state) => {
        setConnection(state);

        if (state === "connected") {
          refresh();
        }
      },
      onError: setError,
    });

    return () => {
      close();
      clearInterval(flush);
      clearTimeout(refreshTimer);
    };
  }, [id, client]);

  return { logs, connection, error };
}
