# ORmonitor Go Backend

Go implementation of the existing ORmonitor API. It keeps the current route
paths and `data/db.json` layout so the legacy HTML SPA or the Vue frontend can
use the same backend contract.

## Run

```bash
cd backend
go run .
```

Environment variables:

- `PORT`: HTTP port, default `3000`.
- `DATA_PATH`: JSON database path, default `../data/db.json`.
- `STATIC_DIR`: optional static asset directory. If unset, the server tries
  `../frontend/dist`, then `../public`.

The server enables CORS for local frontend development and accepts sessions via
`X-Session-ID` or the existing `?sessionId=` query parameter.
