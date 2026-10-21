# Micro CI dashboard

Run from `web/`:

```sh
npm ci
cp -n .env.example .env.local
npm run dev
```

Open [localhost:3000/runs](http://localhost:3000/runs). Start the API and worker using the [project setup](../README.md).

## Configuration

`NEXT_PUBLIC_API_URL` in `.env.local` defaults to `http://localhost:4000/api/v1`.

For custom ports, also update `CORS_ALLOWED_ORIGINS` in `backend/.env` and restart the servers.

## Checks

```sh
npm run lint
npm run typecheck
npm run build
```
