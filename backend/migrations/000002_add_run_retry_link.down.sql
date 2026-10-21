BEGIN;

ALTER TABLE runs DROP COLUMN retry_of_run_id;

COMMIT;
