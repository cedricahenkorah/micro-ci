"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import {
  ArrowLeft,
  Check,
  ChevronRight,
  Clock3,
  GitBranch,
  GitCommitHorizontal,
  Loader2,
  RotateCcw,
  Square,
  Terminal,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useCancelRun, useRetryRun, useRun } from "@/hooks/use-runs";
import { useRunEvents } from "@/hooks/use-run-events";
import { useNow } from "@/hooks/use-now";
import { dateTime, duration, repositoryName } from "@/lib/format";
import type { Attempt, FailureKind, RunDetails } from "@/lib/api/types";
import { StatusBadge } from "./status-badge";
import { LoadingRows, QueryError } from "./query-state";
import { LogViewer } from "./log-viewer";

const failureReasons: Record<FailureKind, string> = {
  config: "The pipeline configuration could not be loaded or validated.",
  checkout: "The repository could not be checked out at the requested commit.",
  command:
    "A pipeline command failed. Check the step output and exit code below.",
  infrastructure:
    "Execution was interrupted by a worker or infrastructure failure.",
  timeout: "The attempt exceeded its execution deadline.",
  cancelled: "Execution was cancelled.",
};

export function RunDetail({ id }: { id: number }) {
  const query = useRun(id);

  return (
    <>
      <Link
        className="back-link"
        href="/runs"
      >
        <ArrowLeft size={14} />
        All runs
      </Link>

      {query.isError && (
        <QueryError
          error={query.error}
          retry={() => query.refetch()}
        />
      )}

      {query.isPending ? (
        <LoadingRows />
      ) : (
        query.data && (
          <RunContent
            key={id}
            run={query.data}
          />
        )
      )}
    </>
  );
}

function RunContent({ run }: { run: RunDetails }) {
  const router = useRouter();
  const cancel = useCancelRun();
  const retry = useRetryRun();
  const stream = useRunEvents(run.id);
  const now = useNow();
  const [selectedAttempt, setSelectedAttempt] = useState<number>();

  const attempt =
    run.attempts.find((attempt) => attempt.id === selectedAttempt) ??
    run.attempts.at(-1);

  const active = run.status === "queued" || run.status === "running";
  const mutationError = cancel.error ?? retry.error;
  const start = run.attempts[0]?.started_at;
  const end = active ? undefined : run.attempts.at(-1)?.finished_at;

  return (
    <>
      <header className="page-header detail-heading">
        <div>
          <h1>{repositoryName(run.repository)}</h1>

          <div className="detail-subtitle">
            <span>Run #{run.id}</span>
            <StatusBadge status={run.status} />

            <span>
              <GitBranch size={13} />
              {run.branch}
            </span>

            <code>
              <GitCommitHorizontal size={14} />
              {run.commit_sha.slice(0, 7)}
            </code>
          </div>
        </div>

        <div className="run-actions">
          {active ? (
            <Button
              variant="outline"
              disabled={cancel.isPending || !!run.cancel_requested_at}
              onClick={() => cancel.mutate(run.id)}
            >
              {cancel.isPending ? (
                <Loader2 className="animate-spin" />
              ) : (
                <Square />
              )}

              {run.cancel_requested_at
                ? "Cancelling…"
                : cancel.isPending
                  ? "Requesting…"
                  : "Cancel run"}
            </Button>
          ) : (
            <Button
              variant="outline"
              disabled={retry.isPending}
              onClick={() =>
                retry.mutate(run.id, {
                  onSuccess: (newRun) => router.push(`/runs/${newRun.id}`),
                })
              }
            >
              <RotateCcw className={retry.isPending ? "animate-spin" : ""} />
              {retry.isPending ? "Retrying…" : "Retry run"}
            </Button>
          )}
        </div>
      </header>

      {mutationError && (
        <div
          className="error-panel"
          role="alert"
        >
          {mutationError.message}
        </div>
      )}

      {stream.error && (
        <div
          className="error-panel"
          role="alert"
        >
          {stream.error}
        </div>
      )}

      <dl className="run-metadata">
        <div>
          <dt>Repository</dt>
          <dd title={run.repository}>{run.repository}</dd>
        </div>

        <div>
          <dt>Created</dt>

          <dd>
            <time dateTime={run.created_at}>{dateTime(run.created_at)}</time>
          </dd>
        </div>

        <div>
          <dt>Duration</dt>

          <dd>
            <Clock3 size={13} />
            {duration(start, end, now)}
          </dd>
        </div>

        <div>
          <dt>Commit</dt>

          <dd>
            <code title={run.commit_sha}>{run.commit_sha.slice(0, 12)}</code>
          </dd>
        </div>
      </dl>

      {run.retry_of_run_id && (
        <p className="retry-note">
          Retry of{" "}
          <Link href={`/runs/${run.retry_of_run_id}`}>
            run #{run.retry_of_run_id}
            <ChevronRight size={12} />
          </Link>
        </p>
      )}

      <div className="section-bar attempt-bar">
        <span className="section-tab">
          Execution{" "}
          <span className="count">
            {run.attempts.length}{" "}
            {run.attempts.length === 1 ? "attempt" : "attempts"}
          </span>
        </span>

        {attempt && (
          <Select
            value={String(attempt.id)}
            onValueChange={(value) => setSelectedAttempt(Number(value))}
          >
            <SelectTrigger
              aria-label="Select attempt"
              className="attempt-select"
            >
              <SelectValue />
            </SelectTrigger>

            <SelectContent>
              {run.attempts.map((attempt) => (
                <SelectItem
                  key={attempt.id}
                  value={String(attempt.id)}
                >
                  Attempt {attempt.number} · {attempt.status.replace("_", " ")}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>

      {attempt ? (
        <AttemptView
          key={attempt.id}
          attempt={attempt}
          stream={stream}
          now={now}
        />
      ) : (
        <div className="queued-state">
          <span className="empty-icon">
            <Clock3 size={24} />
          </span>

          <h2>
            {run.status === "cancelled"
              ? "Cancelled before execution"
              : "Waiting for a worker"}
          </h2>

          <p>
            {run.status === "cancelled"
              ? "This run was cancelled before an attempt started."
              : "Your run is queued. Steps and output will appear here when execution begins."}
          </p>
        </div>
      )}
    </>
  );
}

function AttemptView({
  attempt,
  stream,
  now,
}: {
  attempt: Attempt;
  stream: ReturnType<typeof useRunEvents>;
  now: number;
}) {
  const [stepIndex, setStepIndex] = useState<number>();
  const step = attempt.steps.find((step) => step.index === stepIndex);

  return (
    <>
      {attempt.failure_kind && (
        <div
          className={`failure-notice ${attempt.status === "lost" ? "lost-notice" : ""}`}
        >
          <span className="status-dot" />

          <div>
            <strong>
              {attempt.status === "lost"
                ? "Worker connection lost"
                : attempt.failure_kind === "cancelled"
                  ? "Attempt cancelled"
                  : "Attempt did not complete successfully"}
            </strong>

            <p>
              {failureReasons[attempt.failure_kind]}

              {attempt.status === "lost" &&
                " This attempt's saved steps and logs remain available."}
            </p>
          </div>
        </div>
      )}

      <div className="execution-grid">
        <aside className="steps-panel">
          <div className="steps-title">
            <span>Attempt {attempt.number}</span>
            <StatusBadge status={attempt.status} />
          </div>

          <Button
            variant="ghost"
            className={`all-output ${stepIndex === undefined ? "selected" : ""}`}
            onClick={() => setStepIndex(undefined)}
          >
            <Terminal size={14} />
            All output
            <span>{attempt.steps.length} steps</span>
          </Button>

          <div className="step-list">
            {attempt.steps.map((step) => (
              <button
                key={step.index}
                className={`step-row ${stepIndex === step.index ? "selected" : ""}`}
                onClick={() => setStepIndex(step.index)}
                aria-pressed={stepIndex === step.index}
              >
                <span className={`step-symbol status-${step.status}`}>
                  {step.status === "running" ? (
                    <Loader2
                      size={14}
                      className="animate-spin"
                    />
                  ) : step.status === "succeeded" ? (
                    <Check size={14} />
                  ) : (
                    <span>{String(step.index + 1).padStart(2, "0")}</span>
                  )}
                </span>

                <span className="step-description">
                  <strong>{step.name}</strong>

                  <span>
                    {step.status.replace("_", " ")}
                    {step.exit_code !== undefined
                      ? ` · exit ${step.exit_code}`
                      : ""}
                  </span>
                </span>

                <span className="step-duration">
                  {duration(step.started_at, step.finished_at, now)}
                </span>
              </button>
            ))}
          </div>

          {!attempt.steps.length && (
            <p className="steps-empty">
              {attempt.status === "running"
                ? "Preparing the pipeline…"
                : "No steps were created."}
            </p>
          )}

          <div className="worker-info">
            <span>Worker</span>

            <code title={attempt.worker_id}>
              {attempt.worker_id.slice(0, 12)}
            </code>

            <span>Started</span>
            <span>{dateTime(attempt.started_at)}</span>

            {attempt.finished_at && (
              <>
                <span>Finished</span>
                <span>{dateTime(attempt.finished_at)}</span>
              </>
            )}
          </div>
        </aside>

        <div className="output-column">
          {step && (
            <div className="step-command">
              <span>$</span>
              <code>{step.command}</code>
            </div>
          )}

          <LogViewer
            key={`${attempt.id}-${stepIndex ?? "all"}`}
            logs={stream.logs}
            attemptId={attempt.id}
            stepIndex={stepIndex}
            connection={stream.connection}
            active={attempt.status === "running"}
          />
        </div>
      </div>
    </>
  );
}
