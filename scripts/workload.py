"""Repeatable local acceptance workload; uses only Python's standard library."""
import argparse
import concurrent.futures
import datetime
import json
import math
import platform
import time
import urllib.request
import uuid
from pathlib import Path


def summarize(rows, expected):
    assigned = [r for r in rows if r.get("resourceId") and
                "assignmentMs" in r and not r.get("error")]
    def stats(values):
        values = sorted(values)
        if not values:
            return None
        return {"p50": values[math.ceil(len(values) * .5) - 1],
                "p95": values[math.ceil(len(values) * .95) - 1],
                "max": values[-1]}
    unique = len({r["resourceId"] for r in assigned})
    return {"passed": len(rows) == expected == len(assigned) == unique,
            "attempted": len(rows), "assigned": len(assigned),
            "errors": len(rows) - len(assigned), "uniqueResources": unique,
            "createLatencyMs": stats([r["latencyMs"] for r in rows if "latencyMs" in r]),
            "assignmentObservedMs": stats([r["assignmentMs"] for r in assigned])}


def wait_assignment(read, seconds=120, interval=.25):
    deadline = time.perf_counter() + seconds
    while time.perf_counter() < deadline:
        current = read()
        if current["status"] == "ASSIGNED":
            return current
        if current["status"] not in ("PENDING", "ASSIGNING"):
            raise RuntimeError("unexpected request status " + current["status"])
        time.sleep(interval)
    raise TimeoutError("assignment not observed within deadline")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="http://127.0.0.1:18080")
    parser.add_argument("--count", type=int, default=30)
    parser.add_argument("--concurrency", type=int, default=5)
    parser.add_argument("--output", default=".local/workload.json")
    args = parser.parse_args()
    if not 1 <= args.concurrency <= args.count <= 1000:
        parser.error("require 1 <= concurrency <= count <= 1000")
    tag = uuid.uuid4().hex
    location = {"latitude": -20, "longitude": 30}

    def http(path, payload=None, key=None):
        headers = {"Content-Type": "application/json"}
        if key:
            headers["Idempotency-Key"] = key
        req = urllib.request.Request(args.base.rstrip("/") + path,
                                     data=json.dumps(payload).encode() if payload is not None else None,
                                     headers=headers)
        with urllib.request.urlopen(req, timeout=10) as response:
            expected = 201 if payload is not None else 200
            if response.status != expected:
                raise RuntimeError(f"unexpected HTTP status {response.status}")
            return json.load(response)

    # Setup is outside the measurement. Fresh keys preserve existing demonstration data.
    resources = [http("/resources", {"type": "MEDICINE", "location": location,
                                    "availability": True}, f"load-resource-{tag}-{i}")["id"]
                 for i in range(args.count)]

    def attempt(i):
        started = time.perf_counter()
        row = {"index": i, "operationKey": f"load-request-{tag}-{i}"}
        try:
            request = http("/requests", {"type": "MEDICINE", "location": location,
                                        "details": f"Measured workload {tag} {i}"}, row["operationKey"])
            row.update(requestId=request["id"], latencyMs=round((time.perf_counter()-started)*1000, 3))
            current = wait_assignment(lambda: http("/requests/" + request["id"]))
            row.update(resourceId=current["assignedResourceId"],
                       assignmentMs=round((time.perf_counter()-started)*1000, 3))
            return row
        except Exception as error:
            row["error"] = str(error)
            row.setdefault("latencyMs", round((time.perf_counter()-started)*1000, 3))
            return row

    started = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        rows = list(pool.map(attempt, range(args.count)))
    report = {"utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
              "client": platform.platform(), "concurrency": args.concurrency,
              "mode": "closed-loop create then poll, 250ms poll interval, no retries",
              "elapsedSeconds": round(time.perf_counter()-started, 3),
              "location": location, "seededResourceIds": resources,
              "summary": summarize(rows, args.count), "requests": rows}
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report["summary"], indent=2))
    return 0 if report["summary"]["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
