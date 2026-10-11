# Reverse proxy

The app serves plain HTTP on `:8080` and expects to sit behind a proxy that terminates TLS. Set `SECURE_COOKIES=true` once you do, and `BASE_URL` to the public URL.

## Caddy

```
habits.example.com {
	reverse_proxy pocket-habits:8080
}
```

## nginx

```nginx
server {
    listen 443 ssl http2;
    server_name habits.example.com;
    location / {
        proxy_pass http://pocket-habits:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Traefik labels

```yaml
labels:
  - traefik.enable=true
  - traefik.http.routers.habits.rule=Host(`habits.example.com`)
  - traefik.http.routers.habits.entrypoints=websecure
  - traefik.http.routers.habits.tls.certresolver=le
  - traefik.http.services.habits.loadbalancer.server.port=8080
```

## Single sign-on

Set `AUTH_MODE=proxy` and let the proxy inject the user:

- **Tinyauth / Authelia / Authentik (Caddy):** `forward_auth … { copy_headers Remote-User Remote-Email }`
- **Cloudflare Access:** `AUTH_PROXY_HEADER=Cf-Access-Authenticated-User-Email`

The first request creates the user automatically. API tokens still work for automations that bypass the proxy (give them a path exemption or a second internal hostname).

## Mobile

Add the site to the iOS/Android home screen — it ships a web manifest and opens on the Today page in standalone mode.
