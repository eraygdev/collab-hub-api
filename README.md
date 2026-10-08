# RepoReef API

> REST API for [RepoReef](https://reporeef.com) — a platform where developers share open source projects, collaborate with teams, and find teammates.

[![MIT License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Gin](https://img.shields.io/badge/Gin-1.12-00ADD8?logo=gin&logoColor=white)](https://gin-gonic.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Neon-4169E1?logo=postgresql&logoColor=white)](https://neon.tech/)
[![Docker](https://img.shields.io/badge/Docker-multi--stage-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)
[![CI](https://github.com/eraygdev/reporeef-api/actions/workflows/ci.yml/badge.svg)](https://github.com/eraygdev/reporeef-api/actions)

**Frontend:** [reporeef.com](https://reporeef.com) · [Frontend repo](https://github.com/eraygdev/reporeef)

---

## About

RepoReef API is the backend powering [RepoReef](https://reporeef.com) — a platform where developers publish open source projects, receive join requests from contributors, and build teams.

It handles:

- Authentication via **GitHub OAuth 2.0** + **JWT** (HS256, 7-day expiry)
- User profiles, project CRUD, categories, stars, contributor workflow
- GitHub repo validation (public check via `git-upload-pack`)
- Cover image validation (GitHub raw URLs only, 2 MB max)
- Security: rate limiting, security headers, XSS sanitization, structured error codes

The frontend consumes this API over HTTPS and translates error codes into localized messages — **no raw server messages ever reach the user**.

## Features

### Authentication

- GitHub OAuth 2.0 flow with state-based CSRF protection
- JWT issued after successful login — **contains no email** (privacy-first)
- 7-day expiry, auto-refresh on frontend
- Optional auth middleware (guest browse supported)

### Projects

- Full CRUD with ownership checks
- Category assignment (up to 5 per project, AND/OR filter modes)
- Contributor limit chosen at creation (5 / 10 / 20 / 50, immutable after)
- **Cover images: GitHub-only** — URL is normalized to `raw.githubusercontent.com`, format-validated (no SVG — XSS risk), size-checked via HEAD request (2 MB max)
- **Repo validation: public-only** — checked via `git-upload-pack` endpoint (no GitHub API rate limit)

### Contributors

- Join request flow (pending / approved / rejected)
- Premium users can attach a message (up to 300 chars)
- Project owners approve, reject, or remove contributors
- Self-leave support

### Platform

- Rate limiting (per-IP, in-memory with cleanup)
- Security headers (OWASP recommendations)
- Audit log for sensitive actions
- Structured error codes (`username_taken`, `github_repo_not_accessible`, etc.)
- Multi-stage Docker build (alpine, non-root user, healthcheck)

## API Overview

All endpoints are prefixed with `/api`.

### Auth

| Method | Path                    | Auth | Description                                 |
| ------ | ----------------------- | ---- | ------------------------------------------- |
| GET    | `/auth/github/login`    | —    | Redirect to GitHub OAuth                    |
| GET    | `/auth/github/callback` | —    | OAuth callback → JWT → redirect to frontend |
| GET    | `/auth/me`              | JWT  | Current user info + project count           |

### Projects

| Method | Path                                 | Auth        | Description                                   |
| ------ | ------------------------------------ | ----------- | --------------------------------------------- |
| GET    | `/projects`                          | optional    | List with search, category filter, pagination |
| GET    | `/projects/:id`                      | optional    | Project detail + contributors                 |
| POST   | `/projects`                          | JWT         | Create project                                |
| PUT    | `/projects/:id`                      | JWT (owner) | Update project                                |
| DELETE | `/projects/:id`                      | JWT (owner) | Delete project                                |
| POST   | `/projects/:id/star`                 | JWT         | Star                                          |
| DELETE | `/projects/:id/star`                 | JWT         | Unstar                                        |
| POST   | `/projects/:id/join`                 | JWT         | Send join request                             |
| DELETE | `/projects/:id/leave`                | JWT         | Leave project                                 |
| GET    | `/projects/:id/my-join-status`       | JWT         | My join status                                |
| DELETE | `/projects/:id/contributors/:userId` | JWT (owner) | Remove contributor                            |

### Me

| Method | Path                       | Auth | Description              |
| ------ | -------------------------- | ---- | ------------------------ |
| PUT    | `/me`                      | JWT  | Update username + bio    |
| GET    | `/me/projects`             | JWT  | My projects with stats   |
| GET    | `/me/contributions`        | JWT  | Projects I contribute to |
| GET    | `/me/contributor-requests` | JWT  | Pending join requests    |

### Users

| Method | Path               | Auth     | Description               |
| ------ | ------------------ | -------- | ------------------------- |
| GET    | `/users/search?q=` | —        | Search users by username  |
| GET    | `/users/:username` | optional | Public profile + projects |

### Misc

| Method | Path                                | Description                    |
| ------ | ----------------------------------- | ------------------------------ |
| GET    | `/categories`                       | List categories                |
| GET    | `/config`                           | Limits and allowed values      |
| PUT    | `/contributor-requests/:id/approve` | Approve request (owner)        |
| PUT    | `/contributor-requests/:id/reject`  | Reject request (owner)         |
| GET    | `/ping`                             | Health check (DB connectivity) |

## Tech Stack

- **Go 1.26** — language
- **Gin 1.12** — HTTP framework
- **pgx/v5** — PostgreSQL driver (connection pool, no ORM)
- **Neon** — serverless PostgreSQL
- **golang-jwt/jwt/v5** — JWT signing (HS256)
- **golang.org/x/oauth2** — GitHub OAuth flow
- **golang.org/x/time/rate** — token bucket rate limiter
- **godotenv** — `.env` loading in development
- **Docker** — multi-stage build, alpine runtime, non-root user
- **GitHub Actions** — CI (build + vet + golangci-lint)

## Getting Started

### Prerequisites

- Go **1.26+**
- A PostgreSQL database (Neon, local, or any Postgres 14+)
- A GitHub OAuth App — [create one here](https://github.com/settings/developers)
  - **Homepage URL:** `http://localhost:5173`
  - **Authorization callback URL:** `http://localhost:8080/api/auth/github/callback`

### Installation

```bash
# Clone the repository
git clone https://github.com/eraygdev/reporeef-api.git
cd reporeef-api

# Copy environment template
cp .env.example .env
# Now edit .env and fill in your values
```

### Environment Variables

Create a `.env` file in the project root:

```env
PORT=8080
ENV=development

DATABASE_URL=postgresql://user:password@host/db?sslmode=require
JWT_SECRET=change-me-to-a-long-random-string

GITHUB_CLIENT_ID=your_github_oauth_client_id
GITHUB_CLIENT_SECRET=your_github_oauth_client_secret
GITHUB_OAUTH_REDIRECT_URL=http://localhost:8080/api/auth/github/callback

FRONTEND_URL=http://localhost:5173
```

| Variable                    | Required | Description                                            |
| --------------------------- | -------- | ------------------------------------------------------ |
| `PORT`                      | No       | Server port (default: `8080`)                          |
| `ENV`                       | No       | Set to `development` to allow `localhost:5173` in CORS |
| `DATABASE_URL`              | **Yes**  | PostgreSQL connection string                           |
| `JWT_SECRET`                | **Yes**  | Long random string for signing JWTs                    |
| `GITHUB_CLIENT_ID`          | **Yes**  | From your GitHub OAuth App                             |
| `GITHUB_CLIENT_SECRET`      | **Yes**  | From your GitHub OAuth App                             |
| `GITHUB_OAUTH_REDIRECT_URL` | **Yes**  | Must match GitHub App callback URL exactly             |
| `FRONTEND_URL`              | **Yes**  | Frontend origin (CORS + OAuth redirect target)         |

### Database Setup

The API expects the following tables (create via your migration tool or manually):

- `users` — id, username, email, avatar_url, bio, is_premium, github_id, google_id, created_at
- `projects` — id, title, description, long_description, github_url, demo_url, image_url, author_id, max_contributors, status, created_at, updated_at
- `categories` — id, name, slug
- `project_categories` — project_id, category_id
- `project_stars` — project_id, user_id
- `project_contributors` — id, project_id, user_id, status, message, created_at, approved_at
- `audit_logs` — id, user_id, action, entity_type, entity_id, ip_address, user_agent, created_at

### Run Locally

```bash
go run .
```

Server starts at `http://localhost:8080`. Check with:

```bash
curl http://localhost:8080/ping
# → {"status":"healthy","db":"up"}
```

### Build for Production

```bash
go build -o reporeef-api .
./reporeef-api
```

## Docker

### Build

```bash
docker build -t reporeef-backend:latest .
```

### Run with Docker Compose

```bash
docker compose up -d
```

The Compose file uses `.env` for configuration, exposes port `8080`, and includes a healthcheck against `/ping`.

### Docker Image Details

- **Stage 1 (builder):** `golang:1.26-alpine` — static binary with `CGO_ENABLED=0`, stripped with `-ldflags="-s -w"`
- **Stage 2 (runtime):** `alpine:3.20` — CA certs, tzdata, non-root `appuser`
- **Healthcheck:** `wget --spider http://localhost:8080/ping` every 30s
- **Size:** ~20 MB final image

## Project Structure

```
.
├── main.go                  # Entry point, graceful shutdown
├── routes.go                # Route registration + CORS setup
├── db.go                    # pgxpool connection management
├── auth.go                  # GitHub OAuth config + JWT + middleware
├── audit.go                 # Audit log helper
├── errors.go                # Server error handling + unique violation check
├── constants.go             # All limits, regexes, allowed values
├── validation.go            # Text/URL validation, GitHub repo/image normalizers
├── sanitize.go              # XSS text sanitizer
├── image_check.go           # HEAD request for image size validation
├── handlers_auth.go         # GitHub OAuth handlers
├── handlers_user.go         # Profile, search, ping
├── handlers_project.go      # Projects CRUD, stars
├── handlers_contributor.go  # Join/leave/approve/reject/remove flow
├── handlers_category.go     # Categories list + validation
├── handlers_config.go       # Limits endpoint for frontend
├── middleware_headers.go    # Security headers
├── middleware_ratelimit.go  # Per-IP rate limiter
├── Dockerfile               # Multi-stage build
├── docker-compose.yml       # Single-service compose
└── .github/workflows/ci.yml # CI pipeline
```

## Architecture Notes

- **No ORM** — all queries are hand-written SQL via `pgx/v5`. Prepared statements, parameterized queries, no string concatenation.
- **Advisory locks** for project count enforcement (`pg_advisory_xact_lock`).
- **Fail-open NSFW check (deferred)** — future Sightengine integration will fail-open on API errors to keep UX unaffected; the critical safety net is the **GitHub-only image source** and **public repo requirement**.
- **GitHub repo visibility** is checked via `GET https://github.com/{user}/{repo}/info/refs?service=git-upload-pack`:
  - `200` → public ✓
  - `401` → private ✗
  - `404` → not found ✗
  - No GitHub API rate limit involved (unlike `api.github.com`)
- **Cover image size** is checked via a `HEAD` request with a 5-second timeout — fail-open on network errors.
- **JWT contains no email** — only `user_id`, `username`, `is_premium`, `exp`, `iat`. This is deliberate: even if a token leaks, no PII is exposed.
- **Error codes are stable strings** (e.g. `username_taken`, `github_repo_not_accessible`) — the frontend maps them to localized messages. Raw server errors (SQL errors, panics) never reach the client.
- **Rate limiter is in-memory** — fine for single-instance deployments. Redis migration is on the roadmap for horizontal scaling.
- **Audit log** captures login, create, update, delete, leave, and remove-contributor actions with IP + user agent.

## Security

- Rate limiting: **5 req/s per IP, burst 10** (token bucket)
- Security headers: `X-Content-Type-Options`, `X-Frame-Options`, `X-XSS-Protection`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy`
- CORS: strict origin allowlist (`FRONTEND_URL` + `localhost:5173` in dev)
- Input sanitization: HTML tag stripping, `javascript:` / `data:` scheme removal, event handler attribute removal
- SQL: fully parameterized queries, no string concat
- JWT: HS256 with secret from env, 7-day expiry, no PII in claims
- Regex-based input validation with character allowlists
- Multi-stage Docker build → non-root runtime user
- Structured error codes only — no stack traces in responses

## Roadmap

### Done

- [x] GitHub OAuth 2.0 + JWT auth
- [x] Project CRUD with ownership checks
- [x] Contributor workflow (join/approve/reject/leave/remove)
- [x] Category system with AND/OR filter modes
- [x] Star system
- [x] User profile + search
- [x] Config endpoint for frontend limits
- [x] Rate limiting + security headers
- [x] XSS sanitization
- [x] GitHub repo public check (`git-upload-pack`)
- [x] Cover image validation (GitHub-only, 2 MB max)
- [x] Audit logging
- [x] Structured error codes
- [x] Multi-stage Docker + healthcheck
- [x] GitHub Actions CI

### In Progress

- [ ] VPS deployment (Keyubu + Docker Compose)
- [ ] Custom domain (`api.reporeef.com`)
- [ ] Redis-backed rate limiting

### Planned

- [ ] AI-powered category suggestion (reads project README via LLM)
- [ ] NSFW image moderation (Sightengine integration)
- [ ] Notification system (email + in-app)
- [ ] Comment system
- [ ] Full-text search with `pg_trgm`
- [ ] GitHub webhook-based contributor verification
- [ ] 2FA for sensitive actions
- [ ] Prometheus metrics endpoint
- [ ] OpenAPI / Swagger spec

## Contributing

Contributions are welcome. If you'd like to help:

1. Fork the repo
2. Create a feature branch (`git checkout -b feat/your-feature`)
3. Commit your changes (`git commit -m "feat: add something"`)
4. Push to your branch (`git push origin feat/your-feature`)
5. Open a Pull Request

For larger changes, please open an issue first to discuss what you'd like to change.

## License

This project is licensed under the **MIT License** — see the [LICENSE](LICENSE) file for details.

## Contact

**Eray** — [@eraygdev](https://github.com/eraygdev)
Email: hello@reporeef.com
