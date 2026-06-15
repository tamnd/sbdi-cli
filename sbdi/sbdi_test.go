package sbdi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClientAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalRecords":0,"startIndex":0,"pageSize":20,"occurrences":[]}`))
	}))
	defer srv.Close()

	c := NewClientAt(srv.URL)
	results, total, err := c.SearchAt(context.Background(), srv.URL, "test", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestSearchOccurrences(t *testing.T) {
	occs := []wireOccurrence{
		{
			UUID:           "uuid-001",
			OccurrenceID:   "occ-001",
			ScientificName: "Parus major",
			Country:        "Sweden",
			Kingdom:        "Animalia",
			Family:         "Paridae",
			Year:           2023,
			BasisOfRecord:  "HUMAN_OBSERVATION",
		},
		{
			UUID:           "uuid-002",
			OccurrenceID:   "occ-002",
			ScientificName: "Vulpes vulpes",
			Country:        "Sweden",
			Kingdom:        "Animalia",
			Family:         "Canidae",
			Year:           2022,
			BasisOfRecord:  "HUMAN_OBSERVATION",
		},
	}
	resp := wireSearchResp{
		TotalRecords: 1234,
		StartIndex:   0,
		PageSize:     20,
		Occurrences:  occs,
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClientAt(srv.URL)
	results, total, err := c.SearchAt(context.Background(), srv.URL, "Parus major", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1234 {
		t.Errorf("total = %d, want 1234", total)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].ID != "uuid-001" {
		t.Errorf("ID = %q, want uuid-001", results[0].ID)
	}
	if results[0].ScientificName != "Parus major" {
		t.Errorf("ScientificName = %q, want Parus major", results[0].ScientificName)
	}
	if results[0].Country != "Sweden" {
		t.Errorf("Country = %q, want Sweden", results[0].Country)
	}
}

func TestSearchOccurrencesTotal(t *testing.T) {
	resp := wireSearchResp{
		TotalRecords: 175383933,
		StartIndex:   0,
		PageSize:     1,
		Occurrences:  []wireOccurrence{},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClientAt(srv.URL)
	_, total, err := c.SearchAt(context.Background(), srv.URL, "*", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 175383933 {
		t.Errorf("total = %d, want 175383933", total)
	}
}

func TestRetryOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalRecords":1,"startIndex":0,"pageSize":20,"occurrences":[]}`))
	}))
	defer srv.Close()

	c := NewClientAt(srv.URL)
	c.cfg.Retries = 5

	start := time.Now()
	_, _, err := c.SearchAt(context.Background(), srv.URL, "test", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestBackoff(t *testing.T) {
	if d := backoff(1); d != 500*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 500ms", d)
	}
	if d := backoff(10); d != 5*time.Second {
		t.Errorf("backoff(10) = %v, want 5s", d)
	}
}
