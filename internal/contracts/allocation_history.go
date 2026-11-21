package contracts

import (
	"sort"
	"time"
)

const AllocationHistoryLimit = 50

func (r Resource) AllocationOrderTime() time.Time {
	if !r.AllocatedAt.IsZero() {
		return r.AllocatedAt
	}
	return r.AllocationConfirmedAt
}

// Apply the cursor to the current top 50. An old cursor cannot reach records
// that have since fallen outside that window.
func AllocationPage(rows []Resource, q ListQuery) ResourcePage {
	byID := make(map[string]Resource, len(rows))
	for _, row := range rows {
		if prev, found := byID[row.ID]; found {
			if row.Location == nil {
				prev.AllocationConfirmedAt = row.AllocationConfirmedAt
				row = prev
			} else if row.AllocationConfirmedAt.IsZero() {
				row.AllocationConfirmedAt = prev.AllocationConfirmedAt
			}
		}
		byID[row.ID] = row
	}
	ranked := make([]Resource, 0, len(byID))
	for _, row := range byID {
		ranked = append(ranked, row)
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.AllocationOrderTime().Equal(b.AllocationOrderTime()) {
			return a.ID > b.ID
		}
		return a.AllocationOrderTime().After(b.AllocationOrderTime())
	})
	ranked = ranked[:min(len(ranked), AllocationHistoryLimit)]
	if q.After != "" {
		at, id, _ := q.NewestCursor()
		i := sort.Search(len(ranked), func(i int) bool {
			stamp := ranked[i].AllocationOrderTime()
			return stamp.Before(at) || stamp.Equal(at) && ranked[i].ID < id
		})
		ranked = ranked[i:]
	}
	page := ResourcePage{Items: ranked}
	if len(ranked) > q.Limit {
		page.Items = ranked[:q.Limit]
		page.Next = q.CursorFor(page.Items[len(page.Items)-1])
	}
	return page
}
