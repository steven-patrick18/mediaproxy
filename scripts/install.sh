#!/usr/bin/env bash
#
# mediaproxy — fresh control-plane install.
#
# Run on a clean Ubuntu 24.04 VPS after `git clone`-ing this repo to
# /opt/mediaproxy. Idempotent: re-running picks up where it left off
# and skips steps that are already done.
#
# Usage:
#   sudo ./scripts/install.sh \
#       --domain mediaproxy.example.com \
#       --admin-email you@example.com \
#       [--admin-password 'pick-one']      # auto-generated if omitted
#
# What it does:
#   1. apt-get installs every system dep (postgres, redis, nginx, go,
#      certbot, docker)
#   2. Creates the `mediaproxy` postgres role + database
#   3. Generates a DB password, JWT secret, HOMER DB password (each
#      32+ random chars). Stores them in /opt/mediaproxy/.env and
#      /opt/homer/.env with mode 0600. If .env already exists, reuses
#      the existing values — re-runs never rotate secrets silently.
#   4. Installs golang-migrate, runs the DB schema migrations
#   5. Builds baseapp, seedadmin, node-agent (host + linux/amd64)
#   6. Seeds the admin user
#   7. Installs the mediaproxy-baseapp systemd unit + starts it
#   8. Drops the nginx site template, substitutes the domain, gets
#      a Let's Encrypt cert via certbot, reloads nginx
#   9. Brings up the HOMER docker stack at /opt/homer/
#
# Anything fails → set -e exits with a clear message + tells you what
# to fix. Re-running after the fix continues from there.

set -euo pipefail

DOMAIN=""
ADMIN_EMAIL=""
ADMIN_PASSWORD=""
SKIP_HOMER=0
SKIP_TLS=0

REPO_DIR="/opt/mediaproxy"
HOMER_DIR="/opt/homer"

log()  { printf "\033[1;36m[install]\033[0m %s\n" "$*"; }
warn() { printf "\033[1;33m[warn]\033[0m %s\n" "$*" >&2; }
die()  { printf "\033[1;31m[fatal]\033[0m %s\n" "$*" >&2; exit 1; }

# --- args -------------------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)         DOMAIN="$2"; shift 2 ;;
    --admin-email)    ADMIN_EMAIL="$2"; shift 2 ;;
    --admin-password) ADMIN_PASSWORD="$2"; shift 2 ;;
    --skip-homer)     SKIP_HOMER=1; shift ;;
    --skip-tls)       SKIP_TLS=1; shift ;;
    -h|--help)
      grep '^# ' "$0" | sed 's/^# //'; exit 0 ;;
    *) die "unknown flag: $1 (run with --help)" ;;
  esac
done

[[ $EUID -eq 0 ]]      || die "must run as root (try sudo)"
[[ -n "$DOMAIN" ]]     || die "--domain is required (e.g. mediaproxy.example.com)"
[[ -n "$ADMIN_EMAIL" ]] || die "--admin-email is required"
[[ -d "$REPO_DIR/.git" ]] || die "$REPO_DIR is not a git checkout; clone the repo there first"

if [[ -z "$ADMIN_PASSWORD" ]]; then
  ADMIN_PASSWORD=$(openssl rand -base64 24 | tr -d '=+/')
  log "generated admin password: $ADMIN_PASSWORD  (write this down NOW)"
fi

cd "$REPO_DIR"

# --- 1) system packages -----------------------------------------------------

log "1/9 installing system packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq \
  postgresql postgresql-contrib redis-server nginx certbot python3-certbot-nginx \
  build-essential git curl jq openssl ca-certificates \
  docker.io docker-compose-plugin

systemctl enable --now postgresql redis-server nginx docker

# Go: prefer the official tarball if `go` is missing or too old. We need 1.22+.
if ! command -v go >/dev/null || [[ $(go version | grep -oE 'go1\.[0-9]+' | sed 's/go1\.//') -lt 22 ]]; then
  log "    installing Go 1.23 (system go missing or too old)"
  GO_VER=1.23.4
  curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz
  rm /tmp/go.tgz
  # Make `go` resolvable for this script + future shells.
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
fi

# golang-migrate
if ! command -v migrate >/dev/null; then
  log "    installing golang-migrate"
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
  ln -sf "$(go env GOPATH)/bin/migrate" /usr/local/bin/migrate
fi

# --- 2) postgres role + DB --------------------------------------------------

log "2/9 ensuring postgres role + database"
PG_PASSWORD=""
if [[ -f "$REPO_DIR/.env" ]] && grep -q '^DATABASE_URL=' "$REPO_DIR/.env"; then
  PG_PASSWORD=$(grep '^DATABASE_URL=' "$REPO_DIR/.env" | sed -E 's|.*://[^:]+:([^@]+)@.*|\1|')
  log "    reusing existing DB password from $REPO_DIR/.env"
fi
[[ -z "$PG_PASSWORD" ]] && PG_PASSWORD=$(openssl rand -base64 24 | tr -d '=+/')

ROLE_EXISTS=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='mediaproxy'" || true)
if [[ -z "$ROLE_EXISTS" ]]; then
  sudo -u postgres psql -c "CREATE USER mediaproxy WITH PASSWORD '$PG_PASSWORD';"
else
  # Sync the password to whatever .env says (handles operator edits).
  sudo -u postgres psql -c "ALTER USER mediaproxy WITH PASSWORD '$PG_PASSWORD';"
fi
DB_EXISTS=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='mediaproxy'" || true)
[[ -z "$DB_EXISTS" ]] && sudo -u postgres psql -c "CREATE DATABASE mediaproxy OWNER mediaproxy;"

# --- 3) .env -----------------------------------------------------------------

log "3/9 writing $REPO_DIR/.env"
if [[ ! -f "$REPO_DIR/.env" ]]; then
  JWT_SECRET=$(openssl rand -hex 32)
  cat > "$REPO_DIR/.env" <<EOF
DATABASE_URL=postgres://mediaproxy:${PG_PASSWORD}@127.0.0.1:5432/mediaproxy?sslmode=disable
REDIS_ADDR=127.0.0.1:6379
HTTP_LISTEN=127.0.0.1:8080
JWT_SECRET=${JWT_SECRET}
LOG_LEVEL=info
EOF
  chmod 600 "$REPO_DIR/.env"
else
  log "    .env already exists, leaving it alone"
fi

# --- 4) migrations ----------------------------------------------------------

log "4/9 running DB migrations"
set -a; . "$REPO_DIR/.env"; set +a
migrate -path "$REPO_DIR/migrations" -database "$DATABASE_URL" up

# --- 5) build binaries ------------------------------------------------------

log "5/9 building baseapp + seedadmin + node-agent"
cd "$REPO_DIR"
make build
make build-agent-static

# --- 6) seed admin ----------------------------------------------------------

log "6/9 seeding admin user $ADMIN_EMAIL"
ADMIN_EXISTS=$(sudo -u postgres psql -d mediaproxy -tAc "SELECT 1 FROM admin_users WHERE email='$ADMIN_EMAIL'" 2>/dev/null || true)
if [[ -z "$ADMIN_EXISTS" ]]; then
  "$REPO_DIR/bin/seedadmin" --email "$ADMIN_EMAIL" --password "$ADMIN_PASSWORD"
else
  log "    admin $ADMIN_EMAIL already exists, skipping seed"
fi

# --- 7) systemd unit --------------------------------------------------------

log "7/9 installing mediaproxy-baseapp.service"
cat > /etc/systemd/system/mediaproxy-baseapp.service <<EOF
[Unit]
Description=Mediaproxy Base App (control plane)
After=network.target postgresql.service redis-server.service

[Service]
EnvironmentFile=$REPO_DIR/.env
ExecStart=$REPO_DIR/bin/baseapp
WorkingDirectory=$REPO_DIR
Restart=on-failure
RestartSec=5
User=root
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now mediaproxy-baseapp
sleep 2
if ! systemctl is-active --quiet mediaproxy-baseapp; then
  die "baseapp failed to start — check: journalctl -u mediaproxy-baseapp -n 50"
fi

# --- 8) nginx + TLS ---------------------------------------------------------

log "8/9 configuring nginx for $DOMAIN"
# Materialize the deploy template with the requested domain. The repo
# template ships with mediaproxy.voipzap.com — swap to the real domain.
sed "s/mediaproxy\.voipzap\.com/$DOMAIN/g" \
    "$REPO_DIR/deploy/nginx/mediaproxy.conf" \
    > /etc/nginx/sites-available/mediaproxy
ln -sf /etc/nginx/sites-available/mediaproxy /etc/nginx/sites-enabled/mediaproxy
rm -f /etc/nginx/sites-enabled/default

if [[ $SKIP_TLS -eq 0 ]]; then
  if [[ ! -d "/etc/letsencrypt/live/$DOMAIN" ]]; then
    # certbot needs a working HTTP block first — temporarily strip the
    # SSL stanza so port 80 is enough to respond to the ACME challenge.
    log "    obtaining TLS cert via certbot (this contacts Let's Encrypt)"
    # Run a bare http-only nginx config just for the ACME challenge.
    cat > /etc/nginx/sites-enabled/mediaproxy.acme <<EOF
server { listen 80; listen [::]:80; server_name $DOMAIN; root /var/www/html; }
EOF
    rm -f /etc/nginx/sites-enabled/mediaproxy
    nginx -t && systemctl reload nginx
    certbot certonly --webroot -w /var/www/html -d "$DOMAIN" \
      --non-interactive --agree-tos -m "$ADMIN_EMAIL"
    rm -f /etc/nginx/sites-enabled/mediaproxy.acme
    ln -sf /etc/nginx/sites-available/mediaproxy /etc/nginx/sites-enabled/mediaproxy
  else
    log "    TLS cert already exists for $DOMAIN, skipping certbot"
  fi
fi
nginx -t && systemctl reload nginx

# --- 9) HOMER ---------------------------------------------------------------

if [[ $SKIP_HOMER -eq 0 ]]; then
  log "9/9 bringing up HOMER stack at $HOMER_DIR"
  mkdir -p "$HOMER_DIR"
  cp -an "$REPO_DIR/deploy/homer/." "$HOMER_DIR/"
  if [[ ! -f "$HOMER_DIR/.env" ]]; then
    HOMER_PW=$(openssl rand -base64 24 | tr -d '=+/' | cut -c1-32)
    echo "HOMER_DB_PASSWORD=$HOMER_PW" > "$HOMER_DIR/.env"
    chmod 600 "$HOMER_DIR/.env"
  fi
  (cd "$HOMER_DIR" && docker compose up -d)
else
  log "9/9 skipping HOMER (--skip-homer)"
fi

# --- summary ----------------------------------------------------------------

cat <<EOF

================================================================
  ✅ mediaproxy is installed.
================================================================

  Panel:           https://$DOMAIN
  Admin login:     $ADMIN_EMAIL
  Admin password:  $ADMIN_PASSWORD       <-- save this

  baseapp logs:    journalctl -u mediaproxy-baseapp -f
  HOMER UI:        https://$DOMAIN/homer/ (admin / sipcapture)

Next:
  1. Log in. Add your first SipProxy / MediaNode under
     Infrastructure → Nodes → Add node, then click "Provision via SSH".
  2. Configure client dialer IPs, carrier hosts, IP groups, assignments.
  3. Point your dialer at https://$DOMAIN's signaling IP.

EOF
