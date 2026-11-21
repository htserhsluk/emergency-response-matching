package contracts

import (
	"encoding/json"
	"testing"
)

func TestResourceListNewestCursorAndFilter(t *testing.T) {
	const id = "00000000000000000000000000000001"
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"limit":2,"type":"WATER","sort":"newest","after":"2025-03-29T10:11:12.123456Z/00000000000000000000000000000001"}`, true},
		{`{"limit":2,"type":"UNKNOWN"}`, false},
		{`{"limit":2,"sort":"other"}`, false},
		{`{"limit":20,"sort":"allocated","status":"ALLOCATED"}`, true},
		{`{"limit":20,"sort":"allocated","status":"AVAILABLE"}`, false},
		{`{"limit":2,"sort":"newest","after":"` + id + `"}`, false},
	} {
		var q ListQuery
		if err := json.Unmarshal([]byte(tc.body), &q); err != nil {
			t.Fatal(err)
		}
		if (q.Validate() == nil) != tc.valid {
			t.Errorf("validation: %s", tc.body)
		}
	}
}

func TestResourceListStatusFilter(t *testing.T) {
	for _, status := range []string{"", "UNALLOCATED", "AVAILABLE", "UNAVAILABLE", "ALLOCATED"} {
		var q ListQuery
		if err := json.Unmarshal([]byte(`{"limit":2,"status":"`+status+`"}`), &q); err != nil {
			t.Fatal(err)
		}
		if err := q.Validate(); err != nil {
			t.Fatalf("valid status %q: %v", status, err)
		}
	}
	var q ListQuery
	if err := json.Unmarshal([]byte(`{"limit":2,"status":"UNKNOWN"}`), &q); err != nil {
		t.Fatal(err)
	}
	if q.Validate() == nil {
		t.Fatal("unknown resource status filter accepted")
	}
}

func TestResourceRequiredFieldsPreserveFalse(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"type":"WATER","location":{"latitude":0,"longitude":0},"availability":false}`, true},
		{`{"type":"WATER","location":{"latitude":0,"longitude":0}}`, false},
		{`{"type":"WATER","location":{"latitude":0},"availability":true}`, false},
		{`{"type":"WATER","location":{"latitude":91,"longitude":0},"availability":true}`, false},
		{`{"type":"UNKNOWN","location":{"latitude":0,"longitude":0},"availability":true}`, false},
	} {
		var c RegisterResource
		if err := json.Unmarshal([]byte(tc.body), &c); err != nil {
			t.Fatal(err)
		}
		if (c.Validate() == nil) != tc.valid {
			t.Errorf("validation: %s", tc.body)
		}
	}
}
