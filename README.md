# Micro CI

A lightweight CI runner for executing pipelines in disposable containers, with live logs and automatic recovery from worker failures.

## Local setup

Requires Go 1.26.3+, Node.js/npm, Make, and a running PostgreSQL instance. Run these commands from the repository root.

```sh
cp -n backend/.env.example backend/.env
npm --prefix web ci
```

Set `DATABASE_URL` in `backend/.env` to your PostgreSQL credentials, port, and an existing database.

Install the [golang-migrate CLI](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate) (`brew install golang-migrate` on macOS), then apply the database schema:

```sh
make -C backend migrate-up
```

Start each process in a separate terminal:

```sh
make -C backend api       # API: http://localhost:4000
make -C backend worker    # Currently waits for shutdown
npm --prefix web run dev  # Frontend: use the printed local URL
```

Check the running API and run backend tests/static checks:

```sh
make -C backend health    # Expect HTTP 200 and status "available"
make -C backend check
```

Press Ctrl+C in each process terminal to stop it. Use `make -C backend help` for all backend shortcuts.

An example pipeline is in [examples/.micro-ci.yml](examples/.micro-ci.yml).

## Database migrations

```sh
make -C backend migrate-version
make -C backend migrate-new NAME=add_run_priority
make -C backend migrate-up
make -C backend migrate-down  # Undo only the latest migration
```

`migrate-new` creates numbered, empty `.up.sql` and `.down.sql` files in `backend/migrations/`; fill in the SQL before applying them. It does not infer changes from Go structs. Applied versions are tracked in `schema_migrations`. Rollback runs the down SQL; rolling back the initial migration deletes all four application tables and their data.

Database commands load `backend/.env`; exported environment variables take precedence. Once a migration is applied to a shared database, add a new migration instead of editing the old one.
