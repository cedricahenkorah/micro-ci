export type RunStatus =
  "queued" | "running" | "succeeded" | "failed" | "timed_out" | "cancelled";

export type AttemptStatus = Exclude<RunStatus, "queued"> | "lost";

export type StepStatus = AttemptStatus | "pending" | "skipped";

export type FailureKind =
  | "config"
  | "checkout"
  | "command"
  | "infrastructure"
  | "timeout"
  | "cancelled";

export interface Run {
  id: string;
  repository: string;
  branch: string;
  commit_sha: string;
  status: RunStatus;
  active_attempt_id?: string;
  cancel_requested_at?: string;
  retry_of_run_id?: string;
  created_at: string;
}

export interface RunSummary extends Run {
  started_at?: string;
  finished_at?: string;
}

export interface Attempt {
  id: string;
  run_id: string;
  number: number;
  worker_id: string;
  lease_expires_at: string;
  started_at: string;
  finished_at?: string;
  status: AttemptStatus;
  failure_kind?: FailureKind;
  steps: Step[];
}

export interface Step {
  attempt_id: string;
  index: number;
  name: string;
  command: string;
  status: StepStatus;
  started_at?: string;
  finished_at?: string;
  exit_code?: number;
}

export interface RunDetails extends Run {
  attempts: Attempt[];
}

export interface Page<T> {
  data: T[];
  totalCount: number;
  totalPages: number;
  currentPage: number;
  hasMore: boolean;
  itemsPerPage: number;
}

export interface ApiResponse<T> {
  status: "success";
  code: number;
  message?: string;
  data: T;
}

export interface ApiErrorResponse {
  status: "error";
  code: number;
  message?: string;
}

interface EventBase {
  run_id: string;
  attempt_id?: string;
  sequence: number;
  step_index?: number;
  timestamp: string;
}

export type RunEvent = EventBase &
  (
    | {
        type: "run.status";
        payload: { status: RunStatus; cancel_requested_at?: string | null };
      }
    | {
        type: "attempt.status";
        payload: { status: AttemptStatus; failure_kind?: FailureKind };
      }
    | {
        type: "step.status";
        payload: { status: StepStatus; exit_code?: number | null };
      }
    | { type: "log"; payload: { text: string } }
  );

export type LogEvent = Extract<RunEvent, { type: "log" }>;
