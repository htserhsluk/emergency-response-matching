package contracts

import (
	"encoding/json"
	"testing"
)

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
