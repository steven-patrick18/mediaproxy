# HOMER on the base-app

This directory is the docker-compose stack for the HOMER 7 SIP-ladder
inspection server that lives on the base-app at `/opt/homer/`.

Three containers on a private docker network:

| Container | Image | Exposed on host | Purpose |
|---|---|---|---|
| `homer-db` | `postgres:15-alpine` | nothing | HOMER's metadata + capture storage |
| `heplify-server` | `sipcapture/heplify-server:latest` | UDP 9060 | Receives HEP3 from Kamailio's `siptrace` module |
| `homer-app` | `sipcapture/homer-app:latest` | 127.0.0.1:9080 | Web UI (fronted by nginx at `/homer/`) |

## First-time setup on a new base-app

```bash
# 1. Install docker
sudo apt-get update && sudo apt-get install -y docker.io docker-compose-plugin

# 2. Copy this directory into /opt/homer/
sudo mkdir -p /opt/homer && sudo cp -a deploy/homer/. /opt/homer/

# 3. Generate the DB password and write the .env
sudo bash -c 'echo "HOMER_DB_PASSWORD=$(openssl rand -base64 24 | tr -d =+/ | cut -c1-32)" > /opt/homer/.env'
sudo chmod 600 /opt/homer/.env

# 4. Bring it up
cd /opt/homer && sudo docker compose up -d
```

## Default admin login

HOMER ships with a default `admin / sipcapture` account.
**Change the password immediately** after first login at `/homer/`:
*Preferences → Users → admin → set new password*.

## nginx config

The base-app's main nginx site has a `location /homer/ { proxy_pass http://127.0.0.1:9080/; ... }`
block that fronts the homer-app UI. See `deploy/nginx/mediaproxy.conf` in the
repo root if you ever need to recreate it.

## UFW

Only UDP/9060 from the SipProxy IP is allowed in:

```bash
sudo ufw allow from 45.76.7.40 to any port 9060 proto udp comment 'HEP from SipProxy'
```

## How Kamailio pushes SIP into HOMER

The Kamailio template at `internal/agent/kamailio.go` loads the `siptrace`
module and configures it with HEP3 mode pointing at the base-app's public
IP on UDP/9060. Every SIP message that flows through Kamailio gets
duplicated as a HEP3 packet — original processing is unaffected.

## Operations

| What | Command |
|---|---|
| Status | `sudo docker compose ps` |
| Logs (live) | `sudo docker compose logs -f heplify-server` |
| Restart | `sudo docker compose restart homer-app` |
| Stop everything | `sudo docker compose down` |
| Wipe and start fresh (loses all SIP capture data!) | `sudo docker compose down -v` |

## Backups

The capture tables can grow fast. Add `homer-db-data` volume backups to the
nightly off-site routine if/when you start storing meaningful SIP history.
