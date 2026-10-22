BEGIN;

CREATE TABLE runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'timed_out', 'cancelled')),
    active_attempt_id UUID,
    cancel_requested_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    webhook_delivery_id TEXT UNIQUE
);

CREATE INDEX runs_created_idx ON runs (created_at DESC, id DESC);

-- Workers claim the oldest queued run, using id to break timestamp ties.
CREATE INDEX runs_queue_idx ON runs (created_at, id)
    WHERE status = 'queued';

CREATE TABLE attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES runs (id),
    number INTEGER NOT NULL CHECK (number > 0),
    worker_id TEXT NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'succeeded', 'failed', 'timed_out', 'cancelled', 'lost')),
    failure_kind TEXT
        CHECK (failure_kind IN ('config', 'checkout', 'command', 'infrastructure', 'timeout', 'cancelled')),
    CONSTRAINT attempts_run_number_key UNIQUE (run_id, number),
    -- Supports foreign keys that also verify which run owns the attempt.
    CONSTRAINT attempts_run_id_id_key UNIQUE (run_id, id)
);

CREATE INDEX attempts_running_lease_idx ON attempts (lease_expires_at, id)
    WHERE status = 'running';

-- Added after attempts exists because the two tables reference each other.
ALTER TABLE runs ADD CONSTRAINT runs_active_attempt_fkey
    FOREIGN KEY (id, active_attempt_id) REFERENCES attempts (run_id, id);

CREATE TABLE steps (
    attempt_id UUID NOT NULL REFERENCES attempts (id),
    step_index INTEGER NOT NULL CHECK (step_index >= 0),
    name TEXT NOT NULL,
    command TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'skipped', 'cancelled', 'timed_out', 'lost')),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    exit_code INTEGER,
    PRIMARY KEY (attempt_id, step_index)
);

CREATE TABLE events (
    run_id UUID NOT NULL REFERENCES runs (id),
    attempt_id UUID,
    sequence BIGINT NOT NULL CHECK (sequence > 0),
    type TEXT NOT NULL
        CHECK (type IN ('run.status', 'attempt.status', 'step.status', 'log', 'log.truncated')),
    step_index INTEGER CHECK (step_index >= 0),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload JSONB NOT NULL,
    -- This primary key also supports ordered event reads for a run.
    PRIMARY KEY (run_id, sequence),
    CONSTRAINT events_attempt_fkey
        FOREIGN KEY (run_id, attempt_id) REFERENCES attempts (run_id, id),
    CONSTRAINT events_step_fkey
        FOREIGN KEY (attempt_id, step_index) REFERENCES steps (attempt_id, step_index),
    CONSTRAINT events_step_requires_attempt
        CHECK (step_index IS NULL OR attempt_id IS NOT NULL)
);

COMMIT;
