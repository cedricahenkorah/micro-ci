# Micro CI

A small CI runner with a Go backend and Next.js dashboard.

## Local setup

Requires Go 1.26.3+, Node.js/npm, Make, PostgreSQL, and [golang-migrate](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate).

From the repository root:

```sh
cp -n backend/.env.example backend/.env
# Set DATABASE_URL in backend/.env to an existing PostgreSQL database.
npm --prefix web ci
make -C backend migrate-up
```

Run in separate terminals:

```sh
make -C backend api       # API: http://localhost:4000
make -C backend worker
npm --prefix web run dev  # Frontend: printed local URL
```

## Checks

```sh
make -C backend health            # API must be running
make -C backend check             # Tests + go vet
make -C backend test-integration  # PostgreSQL required
make -C backend help              # All commands, including migrations
```

Example pipeline: [examples/.micro-ci.yml](examples/.micro-ci.yml).
