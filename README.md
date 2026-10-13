# Micro CI

A lightweight CI runner for executing pipelines in disposable containers, with live logs and automatic recovery from worker failures.

## Local setup

Requires Go 1.26.3+, Node.js/npm, Make, and a running PostgreSQL instance. Run these commands from the repository root.

```sh
cp -n backend/.env.example backend/.env
npm --prefix web ci
```

Set `DATABASE_URL` in `backend/.env` to your PostgreSQL credentials, port, and an existing database.

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
