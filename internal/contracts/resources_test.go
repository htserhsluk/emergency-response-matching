package contracts

import (
	"encoding/json"
	"testing"
)

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
