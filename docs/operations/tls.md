# TLS Posture

This document describes how TLS is expected to be deployed in front of
the sofascore-scrapper binary, and how to verify a fresh deployment is
actually serving HTTPS end-to-end.

## Current model: TLS terminated at the load balancer

The backend binary (`cmd/server`) speaks plain HTTP. TLS is terminated
at the load balancer (nginx, Traefik, AWS ALB, etc.) in front of the
backend pool. The backend receives `X-Forwarded-Proto: https` from the
LB and uses it for any code path that needs to know the original
protocol (redirects, absolute URL construction, etc.).

This is the standard cloud-native pattern and is what we deploy in
`deployments/docker/compose.multi.yml` (nginx in front of 3 backends).

## Why we don't terminate TLS in the binary

1. **Cert rotation**: LB-managed certs rotate automatically (AWS ACM,
   GCP Managed Certificates, Caddy on-prem). In-binary certs would
   require a hot-reload mechanism and an HSM integration.
2. **mTLS to backends**: most cloud LBs already do mTLS to backends
   when the backends are in a private subnet; replicating this in Go
   adds operational complexity.
3. **Operational simplicity**: one place to debug TLS issues, not N.

## How to generate a cert for dev / staging

### Option A: `mkcert` (local development only)

The fastest way to get a trusted local cert that browsers and `curl`
accept without warnings.

```bash
# Install mkcert (one-time)
brew install mkcert        # macOS
scoop install mkcert        # Windows
# or: https://github.com/FiloSottile/mkcert#installation

# Install a local CA in your system trust store
mkcert -install

# Generate a cert for the local dev hostname
cd deployments/docker
mkcert -cert-file nginx.crt -key-file nginx.key "localhost" "127.0.0.1" "backend-1"

# Mount the certs in compose.dev.yml (or compose.multi.yml for staging):
#   - ./nginx.crt:/etc/nginx/nginx.crt:ro
#   - ./nginx.key:/etc/nginx/nginx.key:ro
# And add a TLS server block listening on 443 to nginx.conf.
```

### Option B: Let's Encrypt (staging / production)

For staging and production, use certbot with the DNS-01 challenge so
the cert can be issued without exposing port 80 to the internet.

```bash
# Install certbot
apt-get install certbot

# Issue a cert for sofascore.example.com
certbot certonly --dns-cloudflare \
  --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
  -d sofascore.example.com \
  -d "*.sofascore.example.com"

# Certs land in /etc/letsencrypt/live/sofascore.example.com/{fullchain.pem,privkey.pem}
# Mount them in nginx (see Option A for the volume syntax) and add
# a TLS server block with `ssl_certificate` directives.

# Auto-renewal is via the certbot systemd timer; verify with
certbot renew --dry-run
```

## Verifying a deployment

After applying certs and restarting the LB, the following checks should
all pass:

```bash
# 1. TLS handshake works
openssl s_client -connect sofascore.example.com:443 -servername sofascore.example.com < /dev/null

# 2. Security headers present
curl -I https://sofascore.example.com/api/app/v1/current-events
# expect: Strict-Transport-Security, X-Content-Type-Options,
#          Content-Security-Policy, Referrer-Policy, Permissions-Policy

# 3. HSTS preload list (after the operator opts in to preload)
# https://hstspreload.org/?domain=sofascore.example.com

# 4. SSL Labs grade
# https://www.ssllabs.com/ssltest/analyze.html?d=sofascore.example.com
# Target: A or A+
```

## What to do if TLS is misconfigured

1. Check the cert expiry: `echo | openssl s_client -connect ... 2>/dev/null | openssl x509 -noout -dates`
2. Check the cert SAN matches the hostname: `echo | openssl s_client -connect ... 2>/dev/null | openssl x509 -noout -ext subjectAltName`
3. Check the nginx access log for TLS handshake errors: `docker logs <nginx-container> 2>&1 | grep -i "ssl\|tls\|handshake"`
4. If HSTS is breaking a dev flow, use a non-HSTS browser profile or temporarily disable HSTS in nginx.conf.

## Future work: enforce TLS in the binary (Issue #60 Issue 1)

The current model assumes the operator deploys the LB correctly. The
issue tracker tracks an opt-in `APP_REQUIRE_TLS=1` env var that would
make the backend binary reject requests without `X-Forwarded-Proto:
https`, for operators who want belt-and-suspenders enforcement. This
is deferred — nginx + the LB already cover the threat model, and
in-binary enforcement breaks the local-dev flow (where TLS is not used
on the loopback). The plan is to ship the doc + nginx headers in this
PR and revisit binary enforcement after we have a way to disable it
for tests.
