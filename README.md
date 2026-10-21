# Micro CI

A small CI runner with a Go backend and Next.js dashboard.

## Usage

Requires Go 1.26.3+, Git, Make, Docker, PostgreSQL, and [golang-migrate](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate).

1. Clone micro-ci:

   ```sh
   git clone https://github.com/cedricahenkorah/micro-ci.git
   cd micro-ci
   ```

2. Configure the database:

   ```sh
   cp -n backend/.env.example backend/.env
   ```

   Set `DATABASE_URL` in `backend/.env` to an existing database. Leave GitHub settings blank for local runs.

3. Start PostgreSQL and Docker, then apply migrations and install the CLI:

   ```sh
   make -C backend migrate-up
   make -C backend install
   ```

   If `micro-ci --help` reports `command not found`, add Go's binary directory to `PATH` (usually `~/go/bin`):

   ```sh
   export PATH="$HOME/go/bin:$PATH"
   ```

   Put this line in your shell configuration to keep it across terminals.

4. Start the API and worker in separate terminals, from the micro-ci directory:

   ```sh
   make -C backend api
   ```

   ```sh
   make -C backend worker
   ```

5. Add and commit a `.micro-ci.yml` in your project's root. See the [example pipeline](examples/.micro-ci.yml) and adjust its commands for your project.

6. Run your project's current commit:

   ```sh
   cd /path/to/your-project
   micro-ci run .
   ```

   Stay on a branch. Uncommitted changes are excluded. View the run and logs in the dashboard.

The API defaults to `http://localhost:4000`. Override it with `micro-ci run . --api-url http://localhost:4001`.

## Uninstall

From the micro-ci directory:

```sh
make -C backend uninstall
```

## Frontend development

Requires Node.js/npm. From the micro-ci directory:

```sh
npm --prefix web ci
cp -n web/.env.example web/.env.local
npm --prefix web run dev
```

Open [localhost:3000/runs](http://localhost:3000/runs).

For custom ports, update `NEXT_PUBLIC_API_URL` in `web/.env.local` and `CORS_ALLOWED_ORIGINS` in `backend/.env`, then restart the servers.

## Checks

```sh
npm --prefix web run lint
npm --prefix web run typecheck
npm --prefix web run build
make -C backend health            # API must be running
make -C backend check             # Tests + go vet
make -C backend test-integration  # PostgreSQL required
make -C backend help              # All commands, including migrations
```
