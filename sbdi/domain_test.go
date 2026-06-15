package sbdi

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// The client's HTTP behaviour is covered in sbdi_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "sbdi" {
		t.Errorf("Scheme = %q, want sbdi", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "sbdi" {
		t.Errorf("Identity.Binary = %q, want sbdi", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	typ, id, err := Domain{}.Classify("abc-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if typ != "occurrence" {
		t.Errorf("type = %q, want occurrence", typ)
	}
	if id != "abc-uuid" {
		t.Errorf("id = %q, want abc-uuid", id)
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("occurrence", "abc-uuid-123")
	want := "https://records.biodiversitydata.se/occurrences/abc-uuid-123"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknown(t *testing.T) {
	_, err := Domain{}.Locate("species", "abc")
	if err == nil {
		t.Error("expected error for unknown resource type")
	}
}
