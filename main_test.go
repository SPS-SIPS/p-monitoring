package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func TestStatusMap_UpdateAndGetAll(t *testing.T) {
	s := NewStatusMap()
	s.Components["db"] = HealthComponent{Name: "db", Status: "ok", EndpointStatus: "ok", HTTPResult: "200 OK"}
	s.Components["cache"] = HealthComponent{Name: "cache", Status: "degraded", EndpointStatus: "not ok", HTTPResult: "503 Service Unavailable"}
	all := s.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 components, got %d", len(all))
	}
	found := false
	for _, c := range all {
		if c.Name == "db" && c.Status == "ok" && c.EndpointStatus == "ok" && c.HTTPResult == "200 OK" {
			found = true
		}
	}
	if !found {
		t.Errorf("db component with status ok and endpoint ok not found")
	}
}

func BenchmarkStatusMap_Update(b *testing.B) {
	s := NewStatusMap()
	for i := 0; i < b.N; i++ {
		name := "comp" + strconv.Itoa(i%10)
		s.Update(name, "ok")
	}
}

func TestHealthEndpoint(t *testing.T) {
	// Mock HTTP servers for components
	dbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer dbSrv.Close()
	cacheSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer cacheSrv.Close()

	cfg := &AppConfig{
		Components: []ComponentConfig{
			{Name: "db", Endpoint: dbSrv.URL},
			{Name: "cache", Endpoint: cacheSrv.URL},
		},
		CheckInterval:    1,
		LogDirectory:     "/tmp",
		LogRetentionDays: 1,
	}
	statusMap := NewStatusMap()
	logger := log.New(os.Stdout, "", 0)
	checkComponents(cfg, statusMap, logger)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	// Directly test the handler logic
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		components := statusMap.GetAll()
		status := "ok"
		for _, c := range components {
			if c.Status != "ok" {
				status = "degraded"
				break
			}
		}
		resp := HealthResponse{
			Status:     status,
			Components: components,
		}
		json.NewEncoder(w).Encode(resp)
	}
	handler(w, req)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var hr HealthResponse
	json.NewDecoder(resp.Body).Decode(&hr)
	if hr.Status != "ok" {
		t.Errorf("expected status ok, got %s", hr.Status)
	}
	if len(hr.Components) != 2 {
		t.Errorf("expected 2 components, got %d", len(hr.Components))
	}
	for _, c := range hr.Components {
		if c.EndpointStatus != "ok" {
			t.Errorf("expected endpoint_status ok, got %s", c.EndpointStatus)
		}
		if c.HTTPResult != "200 OK" {
			t.Errorf("expected http_result 200 OK, got %s", c.HTTPResult)
		}
	}
}
