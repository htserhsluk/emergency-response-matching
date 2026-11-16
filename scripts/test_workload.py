"""Guard against misleading workload success and percentile reports."""
import unittest
from workload import summarize, wait_assignment


class WorkloadReportTests(unittest.TestCase):
    def test_assigning_is_an_intermediate_state(self):
        states = iter([{"status": "ASSIGNING"},
                       {"status": "ASSIGNED", "assignedResourceId": "resource-1"}])
        self.assertEqual(wait_assignment(lambda: next(states), 1, 0)["assignedResourceId"], "resource-1")

    def test_errors_and_missing_assignments_cannot_pass(self):
        result = summarize([
            {"latencyMs": 10, "assignmentMs": 100, "resourceId": "r1"},
            {"latencyMs": 20, "error": "HTTP 503"},
            {"latencyMs": 30},
        ], expected=3)
        self.assertFalse(result["passed"])
        self.assertEqual(result["errors"], 2)
        self.assertEqual(result["assigned"], 1)

    def test_duplicate_ownership_cannot_pass(self):
        result = summarize([
            {"latencyMs": 10, "assignmentMs": 100, "resourceId": "same"},
            {"latencyMs": 20, "assignmentMs": 200, "resourceId": "same"},
        ], expected=2)
        self.assertFalse(result["passed"])
        self.assertEqual(result["uniqueResources"], 1)

    def test_nearest_rank_percentile_and_count(self):
        rows = [{"latencyMs": i, "assignmentMs": i * 10, "resourceId": str(i)}
                for i in range(1, 21)]
        result = summarize(rows, expected=20)
        self.assertTrue(result["passed"])
        self.assertEqual(result["createLatencyMs"]["p95"], 19)
        self.assertEqual(result["assignmentObservedMs"]["p95"], 190)
        self.assertFalse(summarize(rows, expected=21)["passed"])


if __name__ == "__main__":
    unittest.main()
