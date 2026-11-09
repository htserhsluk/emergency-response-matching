package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequiredFieldsPreserveZero(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"type":"WATER","location":{"latitude":0,"longitude":0},"details":"water needed"}`, true},
		{`{"type":"WATER","location":{"longitude":0},"details":"water needed"}`, false},
		{`{"type":"WATER","location":null,"details":"water needed"}`, false},
		{`{"type":"WATER","location":{"latitude":91,"longitude":0},"details":"water needed"}`, false},
		{`{"type":"UNKNOWN","location":{"latitude":0,"longitude":0},"details":"water needed"}`, false},
		{`{"type":"WATER","location":{"latitude":0,"longitude":0},"details":" "}`, false},
	} {
		var body CreateRequest
		if err := json.Unmarshal([]byte(tc.body), &body); err != nil {
			t.Fatal(err)
		}
		if err := body.Validate(); (err == nil) != tc.valid {
			t.Errorf("%s: validation=%v", tc.body, err)
		}
	}
}

func TestCanonicalPayloadReplay(t *testing.T) {
	var a, b CreateRequest
	json.Unmarshal([]byte(`{"type":"WATER","location":{"latitude":0,"longitude":1},"details":"need water"}`), &a)
	json.Unmarshal([]byte(`{"details":"need water", "location":{"longitude":1.0,"latitude":0.0},"type":"WATER"}`), &b)
	if PayloadHash(a) != PayloadHash(b) {
		t.Fatal("equivalent JSON must hash identically")
	}
	b.Details = "different"
	if PayloadHash(a) == PayloadHash(b) {
		t.Fatal("changed body must conflict")
	}
}

func TestOperationKeyValidation(t *testing.T) {
	for _, k := range []string{"", "short", strings.Repeat("x", 129), strings.Repeat("x", 16) + "\n"} {
		if ValidateKey(k) == nil {
			t.Errorf("accepted %q", k)
		}
	}
	if err := ValidateKey("request-key-00001"); err != nil {
		t.Fatal(err)
	}
	if EntityID("create-request", "request-key-00001") == EntityID("register-resource", "request-key-00001") {
		t.Fatal("entity kinds must be separated")
	}
}
