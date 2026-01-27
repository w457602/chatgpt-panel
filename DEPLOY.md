# chatgpt-panel Deployment Guide

This document covers backend + frontend deployment and PostgreSQL setup for
`chatgpt-panel` on the server, including the Cloudflare tunnel mapping.

## Architecture

- Backend: Go (Gin + GORM) serving API + HTML templates.
- Frontend: `templates/index.html` (served by Gin).
- Database: PostgreSQL.
- Public access: Cloudflared tunnel (openai.tual.me -> localhost:9531).
- OAuth callback: http://localhost:1455/auth/callback (local only).

## Ports

- App service port: `9531` (internal).
- OAuth callback: `1455` (local, required for Codex PKCE).

## Build (local machine)

Use Aliyun GOPROXY to avoid upstream timeouts:

```bash
mkdir -p .cache/go-build .cache/go-mod dist
GOPROXY=https://mirrors.aliyun.com/goproxy/,direct \
GOSUMDB=off GONOSUMDB=* \
GOCACHE=$PWD/.cache/go-build \
GOMODCACHE=$PWD/.cache/go-mod \
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
go build -mod=mod -o dist/chatgpt-panel ./cmd
```

## Server prerequisites

- Ubuntu server with SSH access.
- PostgreSQL installed and running.
- Cloudflared running under systemd (`cloudflared.service`).

## Database setup

1) Find existing DB credentials on the server (example):

```bash
cat /home/ubuntu/mail-api.env
```

This file contains:
```
DATABASE_URL=postgres://<user>:<password>@localhost:5432/<db>?sslmode=disable
```

2) Create the database (example using `mailapi` role):

```bash
sudo -u postgres psql -c "CREATE DATABASE chatgpt_panel OWNER mailapi"
```

If you want a dedicated role instead:

```bash
sudo -u postgres psql <<'SQL'
CREATE USER chatgpt_panel WITH PASSWORD '<strong-password>';
CREATE DATABASE chatgpt_panel OWNER chatgpt_panel;
SQL
```

## Deploy (server)

### 1) Create directories

```bash
mkdir -p /home/ubuntu/chatgpt-panel/templates /home/ubuntu/chatgpt-panel/static
```

### 2) Upload binary + templates

From local machine:

```bash
rsync -avz -e "ssh -i ~/.ssh/codex_webauto" dist/chatgpt-panel \
  ubuntu@58.87.68.10:/home/ubuntu/chatgpt-panel/

rsync -avz -e "ssh -i ~/.ssh/codex_webauto" templates/ \
  ubuntu@58.87.68.10:/home/ubuntu/chatgpt-panel/templates/

rsync -avz -e "ssh -i ~/.ssh/codex_webauto" static/ \
  ubuntu@58.87.68.10:/home/ubuntu/chatgpt-panel/static/
```

### 3) Configure environment

Create `/home/ubuntu/chatgpt-panel/.env`:

```
DB_HOST=localhost
DB_PORT=5432
DB_USER=<db_user>
DB_PASSWORD=<db_password>
DB_NAME=chatgpt_panel
DB_SSLMODE=disable
SERVER_PORT=9531
GIN_MODE=release
JWT_SECRET=<strong-random-secret>
```

### 4) Systemd service

Create `/etc/systemd/system/chatgpt-panel.service`:

```
[Unit]
Description=ChatGPT Panel
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/chatgpt-panel
EnvironmentFile=/home/ubuntu/chatgpt-panel/.env
ExecStart=/home/ubuntu/chatgpt-panel/chatgpt-panel
Restart=always
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo chmod +x /home/ubuntu/chatgpt-panel/chatgpt-panel
sudo systemctl daemon-reload
sudo systemctl enable --now chatgpt-panel.service
```

### 5) Cloudflared tunnel mapping

Edit `/etc/cloudflared/config.yml` and add:

```
  - hostname: openai.tual.me
    service: http://localhost:9531
    originRequest:
      noTLSVerify: true
      connectTimeout: 30s
      tcpKeepAlive: 30s
      keepAliveTimeout: 90s
      keepAliveConnections: 100
```

Restart:

```bash
sudo systemctl restart cloudflared.service
```

## Verify

```bash
sudo systemctl status chatgpt-panel.service --no-pager -l
curl -I http://127.0.0.1:9531/
```

Open in browser:
```
https://openai.tual.me
```

Default admin (created by app on first run):
- Username: `admin`
- Password: `admin123`

## OAuth (Codex PKCE) note

The callback is fixed at `http://localhost:1455/auth/callback`. If you log in
from your local browser, use SSH port-forwarding so the callback hits the server:

```bash
ssh -L 1455:localhost:1455 -i ~/.ssh/codex_webauto ubuntu@58.87.68.10
```

Keep this tunnel open while completing the OAuth flow.

## Zeabur deployment (Docker, US region)

This section is for deploying to Zeabur using the Dockerfile and Zeabur Postgres.

### 1) Create services

- Region: select **US**.
- Add a Postgres service from Zeabur (any size).
- Add a new app service from Git repo.
  - Service root/working directory: `chatgpt-panel`
  - Build type: Docker (uses `chatgpt-panel/Dockerfile`)

### 2) Environment variables

Attach Postgres to the app so Zeabur injects `DATABASE_URL` automatically, then add:

```
GIN_MODE=release
JWT_SECRET=<strong-random-secret>
```

`PORT` is injected by Zeabur automatically (do not set `SERVER_PORT`).

### 3) Deploy + access

- Deploy the app and open the **Zeabur-generated URL** (no custom domain).
- Default admin: `admin` / `admin123` (change after first login).

### 4) OAuth note (Zeabur)

The redirect URI is still `http://localhost:1455/auth/callback`. After login,
copy the **full browser address bar URL** and paste it into the OAuth modal.

## Optional: Import accounts from JSON

If your source file is JSON Lines (one JSON object per line), you can import
directly into PostgreSQL:

```bash
awk 'NF' /home/ubuntu/chatgpt-panel/import/chatgpt_accounts_api.json \
  > /tmp/chatgpt_accounts_api.ndjson
```

Then run (example):

```sql
CREATE TEMP TABLE import_lines (payload jsonb);
COPY import_lines FROM '/tmp/chatgpt_accounts_api.ndjson';
-- upsert into accounts (map email/access_token/refresh_token/cookies/expired)
```

Keep passwords as a placeholder (e.g. `imported`) to satisfy NOT NULL.
