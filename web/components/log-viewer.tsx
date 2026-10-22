"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowDownToLine, Check, Copy, Download, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { LogEvent } from "@/lib/api/types";
import type { ConnectionState } from "@/lib/api/events";

const ansi = new RegExp(String.fromCharCode(27) + "\\[[0-9;]*[A-Za-z]", "g");

const connectionLabels: Record<ConnectionState, string> = {
  connected: "Connected",
  connecting: "Connecting",
  reconnecting: "Reconnecting",
  closed: "Disconnected",
};

export function LogViewer({
  logs,
  attemptId,
  stepIndex,
  connection,
  active,
}: {
  logs: LogEvent[];
  attemptId: string;
  stepIndex?: number;
  connection: ConnectionState;
  active: boolean;
}) {
  const [follow, setFollow] = useState(true);
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState(false);
  const viewport = useRef<HTMLPreElement>(null);

  const text = useMemo(
    () =>
      logs
        .filter(
          (event) =>
            event.attempt_id === attemptId &&
            (stepIndex === undefined || event.step_index === stepIndex),
        )
        .map((event) => event.payload.text)
        .join("")
        .replace(ansi, ""),
    [logs, attemptId, stepIndex],
  );

  useEffect(() => {
    if (follow && viewport.current) {
      viewport.current.scrollTop = viewport.current.scrollHeight;
    }
  }, [text, follow]);

  useEffect(() => {
    if (!copied) {
      return;
    }

    const timer = setTimeout(() => setCopied(false), 2000);

    return () => clearTimeout(timer);
  }, [copied]);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setCopyError(false);
    } catch {
      setCopyError(true);
    }
  };

  const download = () => {
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
    const anchor = document.createElement("a");
    const stepSuffix = stepIndex === undefined ? "" : `-step-${stepIndex + 1}`;

    anchor.href = url;
    anchor.download = `attempt-${attemptId}${stepSuffix}.log`;
    anchor.click();

    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  const label =
    connection === "connected" && active
      ? "Live"
      : connectionLabels[connection];

  const lineCount = text ? text.split("\n").length : 0;

  return (
    <section
      className="log-panel"
      aria-label="Command output"
    >
      <header className="log-header">
        <span>
          <Terminal size={14} />
          Output{" "}
          <span
            className={`stream-state ${connection === "connected" ? "stream-connected" : ""}`}
          >
            <span className="status-dot" />
            {label}
          </span>
        </span>

        <div>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={follow ? "Pause auto-scroll" : "Follow output"}
            aria-pressed={follow}
            title={follow ? "Pause auto-scroll" : "Follow output"}
            onClick={() => setFollow(!follow)}
            className={follow ? "text-foreground" : "text-muted-foreground"}
          >
            <ArrowDownToLine />
          </Button>

          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Copy output"
            title="Copy output"
            disabled={!text}
            onClick={copy}
          >
            {copied ? <Check /> : <Copy />}
          </Button>

          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Download output"
            title="Download output"
            disabled={!text}
            onClick={download}
          >
            <Download />
          </Button>
        </div>
      </header>

      {copyError && (
        <p
          className="inline-notice"
          role="status"
        >
          Could not copy. You can select the output or download it.
        </p>
      )}

      {connection === "reconnecting" && (
        <p
          className="inline-notice"
          role="status"
        >
          Connection interrupted. Reconnecting from the last saved event…
        </p>
      )}

      {connection === "closed" && (
        <p
          className="inline-notice"
          role="status"
        >
          The log connection closed. Reload the page to reconnect.
        </p>
      )}

      <pre
        ref={viewport}
        className="log-output"
        tabIndex={0}
        aria-label="Log text"
        onScroll={() => {
          const el = viewport.current;

          if (
            el &&
            follow &&
            el.scrollHeight - el.scrollTop - el.clientHeight > 50
          ) {
            setFollow(false);
          }
        }}
      >
        {text || (
          <span className="log-placeholder">
            {connection === "connecting"
              ? "Connecting to saved output…"
              : active
                ? "Waiting for command output…"
                : "No command output recorded for this selection."}
          </span>
        )}
      </pre>

      <footer className="log-footer">
        <span>
          {lineCount} lines ·{" "}
          {follow ? "Following output" : "Auto-scroll paused"}
        </span>
      </footer>
    </section>
  );
}
