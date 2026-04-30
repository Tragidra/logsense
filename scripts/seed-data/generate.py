#!/usr/bin/env python3
"""Generate deterministic seed log fixtures for logsense demos.

Produces:
  - scripts/seed-data/nginx-access.log (1000 lines, combined log format)
  - scripts/seed-data/app-json.log     (500 lines, structured JSON)

Run:
  python3 scripts/seed-data/generate.py

The RNG is seeded — output is reproducible and safe to commit.
"""
from __future__ import annotations

import json
import os
import random
from datetime import datetime, timedelta, timezone

SEED = 20260420
OUT_DIR = os.path.dirname(os.path.abspath(__file__))

NGINX_OUT = os.path.join(OUT_DIR, "nginx-access.log")
APP_OUT = os.path.join(OUT_DIR, "app-json.log")

START_TS = datetime(2026, 4, 19, 12, 0, 0, tzinfo=timezone.utc)


def generate_nginx(path: str, n_lines: int = 1000) -> None:
    rng = random.Random(SEED)

    paths_normal = [
        "/", "/about", "/pricing", "/docs",
        "/api/users", "/api/users/{id}", "/api/orders", "/api/orders/{id}",
        "/api/products", "/api/products/{id}", "/api/search",
        "/static/css/app.css", "/static/js/app.js", "/favicon.ico",
    ]
    paths_rare = [
        "/api/admin/stats", "/api/internal/flush",
        "/robots.txt", "/sitemap.xml",
    ]
    methods = ["GET"] * 8 + ["POST"] * 2 + ["PUT", "DELETE"]
    user_agents = [
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
        "curl/8.4.0",
        "Go-http-client/1.1",
        "kube-probe/1.29",
    ]

    with open(path, "w", encoding="utf-8") as f:
        ts = START_TS
        for i in range(n_lines):
            ts = ts + timedelta(seconds=rng.randint(0, 3))

            burst = 400 <= i < 460
            is_rare = rng.random() < 0.04

            if is_rare:
                url_tmpl = rng.choice(paths_rare)
                status = rng.choice([404, 404, 404, 200])
            else:
                url_tmpl = rng.choice(paths_normal)
                if burst:
                    status = rng.choice([500, 502, 503, 504, 200])
                else:
                    r = rng.random()
                    if r < 0.85:
                        status = 200
                    elif r < 0.92:
                        status = 301
                    elif r < 0.97:
                        status = 404
                    else:
                        status = 500

            path_req = url_tmpl.replace("{id}", str(rng.randint(1, 9999)))
            method = rng.choice(methods)
            ip = f"10.0.{rng.randint(0, 255)}.{rng.randint(1, 254)}"
            size = 0 if status >= 500 else rng.randint(120, 20_000)
            ua = rng.choice(user_agents)
            referer = "-" if rng.random() < 0.7 else "https://example.com" + rng.choice(paths_normal).replace("{id}", "42")
            rtime = rng.uniform(0.002, 0.8)
            if status >= 500:
                rtime = rng.uniform(0.4, 3.0)

            line = (
                f'{ip} - - [{ts.strftime("%d/%b/%Y:%H:%M:%S +0000")}] '
                f'"{method} {path_req} HTTP/1.1" {status} {size} '
                f'"{referer}" "{ua}" {rtime:.3f}'
            )
            f.write(line + "\n")


def generate_app_json(path: str, n_lines: int = 500) -> None:
    rng = random.Random(SEED + 1)

    services = ["api", "auth", "payment", "order", "search", "worker"]
    info_templates = [
        ("request received", {"method": "GET", "path": "/api/users/{id}", "duration_ms": "{lat}"}),
        ("request received", {"method": "POST", "path": "/api/orders", "duration_ms": "{lat}"}),
        ("user logged in", {"user_id": "{id}"}),
        ("cache hit", {"key": "user:{id}"}),
        ("job completed", {"job_id": "{id}", "duration_ms": "{lat}"}),
        ("health check ok", {}),
    ]
    warn_templates = [
        ("slow query detected", {"query": "SELECT * FROM orders WHERE user_id = {id}", "duration_ms": "{slow}"}),
        ("retry attempted", {"attempt": "{retry}", "upstream": "payment-service"}),
        ("circuit breaker half-open", {"upstream": "payment-service"}),
    ]
    error_templates = [
        ("connection to payment-gateway:8080 refused: no route to host", {}),
        ("failed to process order {id}: payment service unavailable", {}),
        ("database deadlock on table orders", {"user_id": "{id}"}),
        ("request failed with status 503", {"method": "POST", "path": "/api/orders/submit", "upstream": "payment-service"}),
    ]
    fatal_templates = [
        ("out of memory: killing worker", {"pid": "{pid}"}),
    ]

    def fill(tmpl, fields):
        out = {}
        for k, v in fields.items():
            if isinstance(v, str):
                v2 = v.replace("{id}", str(rng.randint(1, 99999)))
                v2 = v2.replace("{lat}", str(rng.randint(5, 800)))
                v2 = v2.replace("{slow}", str(rng.randint(900, 3500)))
                v2 = v2.replace("{retry}", str(rng.randint(1, 5)))
                v2 = v2.replace("{pid}", str(rng.randint(1000, 9999)))
                out[k] = v2
            else:
                out[k] = v
        msg = tmpl.replace("{id}", str(rng.randint(1000, 9999)))
        return msg, out

    with open(path, "w", encoding="utf-8") as f:
        ts = START_TS
        for i in range(n_lines):
            ts = ts + timedelta(seconds=rng.randint(0, 2))

            burst = 180 <= i < 240
            fatal = i == 475

            if fatal:
                level = "FATAL"
                service = "worker"
                tmpl, extra = rng.choice(fatal_templates)
                msg, fields = fill(tmpl, extra)
            elif burst:
                level = "ERROR"
                service = rng.choice(["payment", "order", "api"])
                tmpl, extra = rng.choice(error_templates)
                msg, fields = fill(tmpl, extra)
            else:
                r = rng.random()
                if r < 0.80:
                    level = "INFO"
                    service = rng.choice(services)
                    tmpl, extra = rng.choice(info_templates)
                elif r < 0.93:
                    level = "WARN"
                    service = rng.choice(services)
                    tmpl, extra = rng.choice(warn_templates)
                else:
                    level = "ERROR"
                    service = rng.choice(services)
                    tmpl, extra = rng.choice(error_templates)
                msg, fields = fill(tmpl, extra)

            rec = {
                "ts": ts.strftime("%Y-%m-%dT%H:%M:%SZ"),
                "level": level,
                "service": service,
                "msg": msg,
            }
            rec.update(fields)
            f.write(json.dumps(rec, separators=(",", ":")) + "\n")


def main() -> None:
    generate_nginx(NGINX_OUT, 1000)
    generate_app_json(APP_OUT, 500)
    print(f"wrote {NGINX_OUT}")
    print(f"wrote {APP_OUT}")


if __name__ == "__main__":
    main()
