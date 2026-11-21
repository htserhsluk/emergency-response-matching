package contracts

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// Duplicate rows represent a resource shard and a separate request shard carrying
// historical confirmation. Equal timestamps must paginate without losing records.
func TestAuditAllocationHistoryPageBoundaries(t *testing.T) {
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)
	for _, total := range []int{0, 1, 49, 50, 51, 101} {
		for _, limit := range []int{1, 7, 50, 100} {
			t.Run(fmt.Sprintf("rows_%d_page_%d", total, limit), func(t *testing.T) {
				var rows []Resource
				for i := range total {
					id := fmt.Sprintf("%032x", i+1)
					full := Resource{ID: id, Location: &Location{}, RegisteredAt: stamp.Add(time.Hour)}
					evidence := Resource{ID: id, AllocationConfirmedAt: stamp}
					if i%2 == 0 {
						rows = append(rows, full, evidence)
					} else {
						rows = append(rows, evidence, full)
					}
				}
				q := ListQuery{Limit: limit, Status: "ALLOCATED", Sort: "allocated"}
				var ids []string
				for pages := 0; ; pages++ {
					if pages > 50 {
						t.Fatal("cursor did not terminate")
					}
					page := AllocationPage(rows, q)
					if len(page.Items) > limit {
						t.Fatal("page limit exceeded")
					}
					for _, r := range page.Items {
						if r.Location == nil || !r.AllocationConfirmedAt.Equal(stamp) {
							t.Fatalf("lost details for %s", r.ID)
						}
						ids = append(ids, r.ID)
					}
					if page.Next == "" {
						break
					}
					q.After = page.Next
				}
				var expected []string
				for i := total; i > max(0, total-50); i-- {
					expected = append(expected, fmt.Sprintf("%032x", i))
				}
				if !slices.Equal(ids, expected) {
					t.Fatalf("got %v; want %v", ids, expected)
				}
			})
		}
	}
}
