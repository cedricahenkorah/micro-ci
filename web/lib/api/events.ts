import { API_URL } from "./client";
import type { RunEvent } from "./types";

export type ConnectionState =
  "connecting" | "connected" | "reconnecting" | "closed";

export function subscribeToRun(
  id: number,
  callbacks: {
    onEvent: (event: RunEvent) => void;
    onConnection: (state: ConnectionState) => void;
    onError: (message: string) => void;
  },
) {
  const source = new EventSource(`${API_URL}/runs/${id}/events`);
  let sequence = 0;

  source.onopen = () => callbacks.onConnection("connected");
  source.onerror = () =>
    callbacks.onConnection(
      source.readyState === EventSource.CLOSED ? "closed" : "reconnecting",
    );

  const receive = (message: MessageEvent<string>) => {
    let event: RunEvent;

    try {
      event = JSON.parse(message.data) as RunEvent;

      if (
        event.run_id !== id ||
        !Number.isSafeInteger(event.sequence) ||
        event.sequence < 1 ||
        !event.payload ||
        (event.type === "log" && typeof event.payload.text !== "string")
      ) {
        throw new Error("Invalid event");
      }
    } catch {
      source.close();
      callbacks.onConnection("closed");
      callbacks.onError(
        "The event stream returned invalid data. Reload to replay the saved logs.",
      );

      return;
    }

    if (event.sequence <= sequence) {
      return;
    }

    sequence = event.sequence;
    callbacks.onEvent(event);
  };

  for (const type of ["run.status", "attempt.status", "step.status", "log"]) {
    source.addEventListener(type, receive);
  }

  return () => source.close();
}
