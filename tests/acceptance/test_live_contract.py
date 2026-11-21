"""Black-box regression cases. Set ERM_AUDIT_URL to a running test deployment.

Successful lifecycle cases retain allocated records. Each run uses fresh operation keys.
Run with python -m unittest discover -s tests/acceptance -p test_live_contract.py -v.
"""
import json
import os
import time
import unittest
import uuid
from datetime import datetime, timezone
from urllib.error import HTTPError
from urllib.request import Request, urlopen


BASE = os.environ.get("ERM_AUDIT_URL", "").rstrip("/")
TYPES = ("WATER", "FOOD", "MEDICINE", "TRANSPORTATION", "SHELTER")
MISSING = "0" * 32


def operation_key():
    return uuid.uuid4().hex


@unittest.skipUnless(BASE, "Set ERM_AUDIT_URL to opt into retained test records")
class LiveContract(unittest.TestCase):
    def call(self, method, path, expected, body=None, key=None):
        headers = {"Content-Type": "application/json"}
        if key is not None:
            headers["Idempotency-Key"] = key
        data = None if body is None else json.dumps(body, ensure_ascii=False).encode()
        request = Request(BASE + path, data=data, headers=headers, method=method)
        try:
            response = urlopen(request, timeout=12)
        except HTTPError as error:
            response = error
        with response:
            raw = response.read()
            self.assertEqual(response.status, expected, (method, path, raw[:500]))
            self.assertEqual(response.headers.get_content_type(), "application/json")
            return json.loads(raw)

    def pages(self, path):
        cursor = ""
        seen = set()
        while True:
            page = self.call("GET", path + "&after=" + cursor, 200)
            yield from page["items"]
            cursor = page.get("next")
            if not cursor:
                return
            self.assertTrue(page["items"])
            self.assertNotIn(cursor, seen, "pagination loop")
            seen.add(cursor)

    def check_directory(self, status):
        overview = self.pages("/map/locations?type=ALL")
        expected = []
        for item in overview:
            if item["kind"] != "resource" or item["status"] != status:
                continue
            record = self.call("GET", "/resources/" + item["id"], 200)
            stamp = record.get("registeredAt")
            if status == "ALLOCATED":
                stamp = record.get("allocatedAt") or record.get("allocationConfirmedAt")
            when = datetime.fromisoformat(stamp) if stamp else datetime.min.replace(tzinfo=timezone.utc)
            expected.append((when, item["id"]))
        expected.sort(reverse=True)
        sort = "newest"
        if status == "ALLOCATED":
            expected = expected[:50]
            sort = "allocated"
        rows = self.pages(f"/resources?sort={sort}&status={status}&limit=7")
        self.assertEqual([r["id"] for r in rows], [identity for _, identity in expected])

    def test_live_allocation_window(self):
        self.check_directory("ALLOCATED")

    def test_live_available_pages(self):
        self.check_directory("AVAILABLE")

    def test_live_unavailable_pages(self):
        self.check_directory("UNAVAILABLE")

    def wait_status(self, path, status):
        deadline = time.monotonic() + 90
        while True:
            record = self.call("GET", path, 200)
            if record["status"] == status:
                return record
            if time.monotonic() >= deadline:
                self.fail(f"{path} expected {status}, last state {record['status']}")
            time.sleep(0.5)

    def test_radius_cap_and_automatic_availability_retry(self):
        # On the WGS84 equator the distance is longitude separation in radians
        # times the semi-major axis. Seed one metre on either side of the cap.
        from math import degrees

        origin = {"latitude": 0, "longitude": -120}
        def supply(metres, available):
            location = dict(origin, longitude=origin["longitude"] + degrees(metres / 6378137))
            return self.call("POST", "/resources", 201, {
                "location": location, "type": "TRANSPORTATION", "availability": available
            }, operation_key())

        far = supply(4000001, True)
        far_status = "/resources/" + far["id"] + "/status"
        self.addCleanup(self.call, "PUT", far_status, 200, {"status": "UNAVAILABLE"}, operation_key())
        near = supply(3999999, False)
        base = "/resources/" + near["id"]
        created = self.call("POST", "/requests", 201, {
            "location": origin, "type": "TRANSPORTATION", "details": "audit radius boundary"
        }, operation_key())
        path = "/requests/" + created["id"]
        self.wait_status(path, "UNMATCHED")
        preview = self.call("GET", base + "/candidates", 200)
        self.assertIn(created["id"], [row["id"] for row in preview["items"]])
        self.assertFalse(preview.get("assignedRequest"))
        self.call("PUT", base + "/status", 200, {"status": "AVAILABLE"}, operation_key())
        assigned = self.wait_status(path, "ASSIGNED")
        self.assertEqual(assigned["assignedResourceId"], near["id"])
        self.assertAlmostEqual(assigned["assignment"]["distanceMeters"], 3999999, delta=0.1)

    def lifecycle(self, kind, supply, demand, min_distance, max_distance):
        def location(pair):
            return dict(zip(("latitude", "longitude"), pair))

        res = self.call("POST", "/resources", 201, {
            "type": kind, "location": location(supply), "availability": True
        }, operation_key())
        key = operation_key()
        payload = {"type": kind, "location": location(demand), "details": "\u6c34" * 2000}
        created = self.call("POST", "/requests", 201, payload, key)
        request = "/requests/" + created["id"]
        replay = self.call("POST", "/requests", 201, payload, key)
        self.assertEqual((replay["id"], replay["createdAt"]), (created["id"], created["createdAt"]))
        self.call("POST", "/requests", 409, dict(payload, details="changed"), key)
        record = self.wait_status(request, "ASSIGNED")
        # Earlier runs can leave equally near supplies at the poles. Follow the
        # confirmed owner and verify its type, coordinates and reciprocal link.
        owner = record["assignedResourceId"]
        resource = "/resources/" + owner
        if owner != res["id"]:
            self.call("PUT", "/resources/" + res["id"] + "/status", 200,
                      {"status": "UNAVAILABLE"}, operation_key())

        self.assertEqual((record["type"], record["details"], record["location"]),
                         (kind, payload["details"], location(demand)))
        assigned = record["assignment"]
        self.assertEqual(assigned["location"], location(supply))
        self.assertGreaterEqual(assigned["distanceMeters"], min_distance)
        self.assertLessEqual(assigned["distanceMeters"], max_distance)
        preview = self.call("GET", resource + "/candidates", 200)
        self.assertEqual(preview["assignedRequest"]["id"], created["id"])
        self.assertFalse(preview["items"])
        inbox = request + "/notifications"
        notes = self.call("GET", inbox + "?limit=1", 200)["items"]
        self.assertEqual([n["resourceId"] for n in notes], [owner])
        read = inbox + "/" + notes[0]["id"] + "/read"
        first = self.call("PUT", read, 200, {}, operation_key())
        second = self.call("PUT", read, 200, {}, operation_key())
        self.assertTrue(first["readAt"])
        self.assertEqual(first["readAt"], second["readAt"])
        self.assertEqual(self.call("POST", "/match/" + created["id"], 200, {}, operation_key())["status"], "ASSIGNED")
        self.assertEqual(self.call("PUT", request + "/status", 200, {"status": "COMPLETED"}, operation_key())["status"], "COMPLETED")
        self.call("PUT", resource + "/status", 409, {"status": "AVAILABLE"}, operation_key())
        final = self.call("GET", resource, 200)
        self.assertEqual((final["type"], final["location"]), (kind, location(supply)))
        self.assertEqual((final["status"], final["availability"], final["assignedRequestId"]),
                         ("ALLOCATED", False, created["id"]))


def install_case(name, check):
    setattr(LiveContract, "test_" + name, check)


def query_case(path, expected):
    def check(self):
        self.call("GET", path, expected)
    return check


def invalid_body(path, body, key):
    def check(self):
        self.call("POST", path, 400, body, key)
    return check


for label, query in {
    "zero_limit": "limit=0", "negative_limit": "limit=-1", "large_limit": "limit=101",
    "fractional_limit": "limit=1.5", "invalid_status": "status=PENDING",
    "invalid_type": "type=water", "invalid_sort": "sort=created",
    "allocated_sort_available": "sort=allocated&status=AVAILABLE",
    "invalid_cursor": "after=short", "newest_id_only": "sort=newest&after=" + MISSING,
    "bad_timestamp": "sort=newest&after=2026-02-30/" + MISSING,
    "submicrosecond_cursor": "sort=newest&after=2026-01-01T00:00:00.000000001Z/" + MISSING,
}.items():
    install_case("directory_" + label, query_case("/resources?" + query, 400))

for kind in ("ALL", *TYPES):
    def filtered(self, kind=kind):
        seen = set()
        for item in self.pages("/map/locations?type=" + kind):
            identity = (item["kind"], item["id"])
            self.assertNotIn(identity, seen)
            seen.add(identity)
            self.assertIn(item["type"], TYPES if kind == "ALL" else (kind,))
    install_case("map_pages_" + kind.lower(), filtered)

for label, path, expected in [
    ("map_bad_kind", "/map/locations?type=all", 400),
    ("map_bad_cursor", "/map/locations?type=ALL&after=unknown:" + MISSING, 400),
    ("map_missing_type", "/map/locations", 400),
    ("missing_request", "/requests/" + MISSING, 404),
    ("missing_resource", "/resources/" + MISSING, 404),
    ("missing_preview", "/resources/" + MISSING + "/candidates", 404),
    ("missing_inbox", "/requests/" + MISSING + "/notifications", 404),
    ("inbox_bad_limit", "/requests/" + MISSING + "/notifications?limit=101", 400),
    ("inbox_bad_cursor", "/requests/" + MISSING + "/notifications?after=short", 400),
]:
    install_case(label, query_case(path, expected))

for path in ("/requests", "/resources"):
    base = {"type": "WATER", "location": {"latitude": 0, "longitude": 0}}
    base.update({"details": "boundary"} if path == "/requests" else {"availability": False})
    for label, change in {
        "null_location": {"location": None}, "missing_longitude": {"location": {"latitude": 0}},
        "beyond_pole": {"location": {"latitude": -90.000001, "longitude": 0}},
        "beyond_dateline": {"location": {"latitude": 0, "longitude": 180.000001}},
        "unknown_type": {"type": "ICE"}, "empty_type": {"type": ""},
        "string_latitude": {"location": {"latitude": "0", "longitude": 0}},
    }.items():
        install_case(path[1:] + "_" + label, invalid_body(path, dict(base, **change), operation_key()))
    for label, key in (("absent", None), ("short", "x" * 15), ("long", "x" * 129)):
        install_case(path[1:] + "_key_" + label, invalid_body(path, base, key))
    changes = (("unicode_over_limit", {"details": "\u6c34" * 2001}), ("blank", {"details": " \t "})) if path == "/requests" else (
        ("null_availability", {"availability": None}), ("string_availability", {"availability": "false"}))
    for label, change in changes:
        install_case(path[1:] + "_" + label, invalid_body(path, dict(base, **change), operation_key()))

# Geographic coordinates exercise the public API, PostgreSQL geography and saved assignment.
for kind, supply, demand, minimum, maximum in (
    ("WATER", (0, 179.999), (0, -179.999), 200, 250),
    ("FOOD", (90, 0), (90, 0), 0, 0),
    ("MEDICINE", (-90, 0), (-90, 0), 0, 0),
    ("TRANSPORTATION", (0, 0), (0, 0), 0, 0),
    ("SHELTER", (-22, -180), (-22, 180), 0, 0.01),
):
    def lifecycle_case(self, args=(kind, supply, demand, minimum, maximum)):
        self.lifecycle(*args)
    install_case("assignment_" + kind.lower(), lifecycle_case)
