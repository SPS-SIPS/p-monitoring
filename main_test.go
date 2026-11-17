package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
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

	healthHandler(statusMap, "").ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var hr HealthResponse
	json.NewDecoder(resp.Body).Decode(&hr)
	if hr.Status != "ok" {
		t.Errorf("expected status ok, got %s", hr.Status)
	}
	if len(hr.Components) != 0 {
		t.Fatal("public health must not disclose components")
	}
}

func TestDiagnosticsAuthorization(t *testing.T) {
	status := NewStatusMap()
	status.Components["internal-db"] = HealthComponent{Name: "internal-db", Status: "ok", Error: "private diagnostic"}
	token := strings.Repeat("s", 32)
	for _, tc := range []struct {
		configured, supplied string
		code                 int
	}{
		{"", "", 503}, {token, "", 401}, {token, "Bearer wrong", 401}, {token, "Bearer " + token, 200},
	} {
		request := httptest.NewRequest("GET", "/diagnostics", nil)
		request.Header.Set("Authorization", tc.supplied)
		response := httptest.NewRecorder()
		healthHandler(status, tc.configured).ServeHTTP(response, request)
		if response.Code != tc.code {
			t.Fatalf("got %d, want %d", response.Code, tc.code)
		}
		if tc.code != 200 && strings.Contains(response.Body.String(), "internal-db") {
			t.Fatal("anonymous diagnostic disclosure")
		}
		if tc.code == 200 && !strings.Contains(response.Body.String(), "internal-db") {
			t.Fatal("authorized diagnostic missing")
		}
	}
}

func TestNonLoopbackListenerRequiresTLS(t *testing.T) {
	for _, address := range []string{":8080", "0.0.0.0:8080", "[::]:8080", "localhost:8080"} {
		if err := serveHealth(&http.Server{Addr: address}, "", ""); err == nil {
			t.Fatalf("accepted insecure listener %s", address)
		}
	}
}
