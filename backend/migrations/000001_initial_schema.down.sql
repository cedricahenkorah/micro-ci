BEGIN;

DROP TABLE events;
DROP TABLE steps;
ALTER TABLE runs DROP CONSTRAINT runs_active_attempt_fkey;
DROP TABLE attempts;
DROP TABLE runs;

COMMIT;
