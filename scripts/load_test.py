#!/usr/bin/env python3
"""Teste de carga local do endpoint público atrás do Nginx."""

import argparse
import concurrent.futures
import json
import math
import time
import urllib.error
import urllib.request
from collections import Counter


PAYLOAD = {
    "id": "tx-benchmark",
    "transaction": {
        "amount": 41.12,
        "installments": 2,
        "requested_at": "2026-03-11T18:45:53Z",
    },
    "customer": {
        "avg_amount": 82.24,
        "tx_count_24h": 3,
        "known_merchants": ["MERC-003", "MERC-016"],
    },
    "merchant": {"id": "MERC-016", "mcc": "5411", "avg_amount": 60.25},
    "terminal": {
        "is_online": False,
        "card_present": True,
        "km_from_home": 29.23,
    },
    "last_transaction": None,
}


def percentile(values, percentage):
    """Calcula um percentil sem dependência externa."""
    ordered = sorted(values)
    position = max(0, math.ceil(len(ordered) * percentage / 100) - 1)
    return ordered[position]


def request_once(url, body, timeout):
    started = time.perf_counter()
    request = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            response.read()
            return (
                (time.perf_counter() - started) * 1000,
                response.status,
                response.headers.get("X-API-Instance", "unknown"),
                None,
            )
    except urllib.error.HTTPError as error:
        return (
            (time.perf_counter() - started) * 1000,
            error.code,
            "error",
            str(error),
        )
    except Exception as error:  # noqa: BLE001 - o script precisa reportar falhas de rede.
        return (
            (time.perf_counter() - started) * 1000,
            0,
            "error",
            str(error),
        )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://127.0.0.1:9999/fraud-score")
    parser.add_argument("--requests", type=int, default=100)
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--timeout", type=float, default=10)
    args = parser.parse_args()

    if args.requests <= 0 or args.concurrency <= 0:
        parser.error("--requests e --concurrency devem ser positivos")

    body = json.dumps(PAYLOAD).encode()
    started = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as executor:
        futures = [
            executor.submit(request_once, args.url, body, args.timeout)
            for _ in range(args.requests)
        ]
        results = [future.result() for future in futures]
    elapsed = time.perf_counter() - started

    latencies = [result[0] for result in results]
    successful = [result for result in results if result[1] == 200]
    errors = [result for result in results if result[1] != 200]
    instances = Counter(result[2] for result in successful)

    print(f"requests={len(results)} success={len(successful)} errors={len(errors)}")
    print(f"throughput={len(results) / elapsed:.2f} req/s elapsed={elapsed:.3f}s")
    print(
        "latency_ms "
        f"min={min(latencies):.2f} "
        f"p50={percentile(latencies, 50):.2f} "
        f"p95={percentile(latencies, 95):.2f} "
        f"p99={percentile(latencies, 99):.2f} "
        f"max={max(latencies):.2f}"
    )
    print(f"instances={dict(instances)}")
    if errors:
        print(f"first_error={errors[0][3]}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
