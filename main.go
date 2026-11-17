package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config structure for internal components
type ComponentConfig struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

type AppConfig struct {
	Components       []ComponentConfig `json:"components"`
	CheckInterval    int               `json:"check_interval_seconds"`
	LogDirectory     string            `json:"log_directory"`
	LogRetentionDays int               `json:"log_retention_days"`
	ListenAddress    string            `json:"listen_address"`
}

// Health response structure
type HealthComponent struct {
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	EndpointStatus string    `json:"endpoint_status"`
	HTTPResult     string    `json:"http_result"`
	LastChecked    time.Time `json:"last_checked"`
	Error          string    `json:"error,omitempty"`
}

type HealthResponse struct {
	Status     string            `json:"status"`
	Components []HealthComponent `json:"components"`
	Timestamp  string            `json:"timestamp"`
}

// In-memory status for components
type StatusMap struct {
	mu         sync.RWMutex
	Components map[string]HealthComponent
}

func NewStatusMap() *StatusMap {
	return &StatusMap{Components: make(map[string]HealthComponent)}
}

func (s *StatusMap) Update(name string, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Components[name] = HealthComponent{Name: name, Status: status, LastChecked: time.Now()}
}

func (s *StatusMap) GetAll() []HealthComponent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	components := make([]HealthComponent, 0, len(s.Components))
	for _, v := range s.Components {
		components = append(components, v)
	}
	return components
}

// Logging

func setupLogger(logDir string) *log.Logger {
	os.MkdirAll(logDir, 0700)
	logFile := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	return log.New(f, "", log.LstdFlags|log.LUTC|log.Lmsgprefix)
}

// Config loading

func loadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Health check logic for components with endpoints
func checkComponents(cfg *AppConfig, statusMap *StatusMap, logger *log.Logger) {
	client := &http.Client{Timeout: 5 * time.Second}
	for _, c := range cfg.Components {
		status := "unknown"
		endpointStatus := "not ok"
		httpResult := ""
		errMsg := ""
		resp, err := client.Get(c.Endpoint)
		if err != nil {
			status = "unreachable"
			httpResult = err.Error()
			errMsg = err.Error()
		} else {
			httpResult = resp.Status
			defer resp.Body.Close()
			// Accept HTTP 200 as healthy, or plain text 'ok' (case-insensitive, trimmed)
			var health struct {
				Status string `json:"status"`
			}
			bodyBytes, _ := io.ReadAll(resp.Body)
			decErr := json.Unmarshal(bodyBytes, &health)
			if decErr == nil && health.Status == "ok" {
				status = "ok"
				endpointStatus = "ok"
			} else {
				// Try plain text
				bodyStr := string(bodyBytes)
				if resp.StatusCode == 200 && (len(bodyStr) == 0 || trimToOk(bodyStr)) {
					status = "ok"
					endpointStatus = "ok"
				} else if resp.StatusCode == 200 {
					status = "ok"
					endpointStatus = "ok"
				} else {
					status = "invalid_response"
					errMsg = decErr.Error()
				}
			}
		}
		statusMap.mu.Lock()
		statusMap.Components[c.Name] = HealthComponent{
			Name:           c.Name,
			Status:         status,
			EndpointStatus: endpointStatus,
			HTTPResult:     httpResult,
			LastChecked:    time.Now(),
			Error:          errMsg,
		}
		statusMap.mu.Unlock()
		logEntry := struct {
			Time           time.Time `json:"time"`
			Component      string    `json:"component"`
			Status         string    `json:"status"`
			EndpointStatus string    `json:"endpoint_status"`
			HTTPResult     string    `json:"http_result"`
			Error          string    `json:"error,omitempty"`
		}{
			Time:           time.Now().UTC(),
			Component:      c.Name,
			Status:         status,
			EndpointStatus: endpointStatus,
			HTTPResult:     httpResult,
			Error:          errMsg,
		}
		b, _ := json.Marshal(logEntry)
		logger.Println(string(b))
	}
}

// Helper to check if a string is 'ok' (case-insensitive, trimmed)
func trimToOk(s string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(s))
	return trimmed == "ok"
}

// Periodic health check for internal components
func startHealthChecks(cfg *AppConfig, statusMap *StatusMap, logger *log.Logger) {
	interval := time.Duration(cfg.CheckInterval) * time.Second
	go func() {
		for {
			checkComponents(cfg, statusMap, logger)
			time.Sleep(interval)
		}
	}()
}

// HTTP server with /health endpoint
func overallStatus(components []HealthComponent) string {
	for _, c := range components {
		if c.Status != "ok" {
			return "degraded"
		}
	}
	return "ok"
}

func healthHandler(statusMap *StatusMap, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		status := overallStatus(statusMap.GetAll())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		json.NewEncoder(w).Encode(map[string]string{"status": status})
	})
	mux.HandleFunc("/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if len(token) < 32 {
			http.Error(w, "Diagnostics are not configured", http.StatusServiceUnavailable)
			return
		}
		expected := sha256.Sum256([]byte("Bearer " + token))
		actual := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		components := statusMap.GetAll()
		json.NewEncoder(w).Encode(HealthResponse{Status: overallStatus(components), Components: components, Timestamp: time.Now().UTC().Format(time.RFC3339)})
	})
	return mux
}

func serveHealth(server *http.Server, cert, key string) error {
	host, _, err := net.SplitHostPort(server.Addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	loopback := ip != nil && ip.IsLoopback()
	if (cert == "") != (key == "") {
		return fmt.Errorf("both TLS_CERT_FILE and TLS_KEY_FILE are required")
	}
	if cert != "" {
		return server.ListenAndServeTLS(cert, key)
	}
	if !loopback {
		return fmt.Errorf("non-loopback listeners require TLS_CERT_FILE and TLS_KEY_FILE")
	}
	return server.ListenAndServe()
}

func startHTTPServer(statusMap *StatusMap, addr string, logger *log.Logger) {
	server := &http.Server{Addr: addr, Handler: healthHandler(statusMap, os.Getenv("STATUS_API_TOKEN")), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	logger.Printf("HTTP server listening on %s", addr)
	log.Fatal(serveHealth(server, os.Getenv("TLS_CERT_FILE"), os.Getenv("TLS_KEY_FILE")))
}

// Log retention cleanup

func cleanupLogs(logDir string, retentionDays int, logger *log.Logger) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	files, err := os.ReadDir(logDir)
	if err != nil {
		logger.Printf("Failed to read log dir: %v", err)
		return
	}
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(logDir, f.Name()))
		}
	}
}

func main() {
	configPath := flag.String("config", "config.json", "Path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config from %s: %v", *configPath, err)
	}
	logger := setupLogger(cfg.LogDirectory)
	statusMap := NewStatusMap()

	cleanupLogs(cfg.LogDirectory, cfg.LogRetentionDays, logger)
	startHealthChecks(cfg, statusMap, logger)
	addr := cfg.ListenAddress
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	startHTTPServer(statusMap, addr, logger)
}
