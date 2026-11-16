package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds all configurable parameters
type Config struct {
	BackendA           string
	BackendB           string
	BackendC           string
	RedisURL           string
	LogDir             string
	LoadCheckInterval  time.Duration
	HealthCheckTimeout time.Duration
	FailoverRetries    int
}

// LoadBalancerService handles load balancing and routing
type LoadBalancerService struct {
	redis    *redis.Client
	config   Config
	backends map[string]string
	mu       sync.RWMutex
	loads    map[string]int // Current load per backend
}

// BackendLoad represents backend load information
type BackendLoad struct {
	Backend           string `json:"backend"`
	ActiveConnections int    `json:"active_connections"`
	LastChecked       string `json:"last_checked"`
}

// HealthResponse is what backends return from /health endpoint
type HealthResponse struct {
	Status               string `json:"status"`
	ActiveConnections    int    `json:"active_connections"`
	AvailableConnections int    `json:"available_connections"`
}

func main() {
	// Configuration
	config := Config{
		BackendA:           os.Getenv("BACKEND_A"),
		BackendB:           os.Getenv("BACKEND_B"),
		BackendC:           os.Getenv("BACKEND_C"),
		RedisURL:           os.Getenv("REDIS_URL"),
		LogDir:             os.Getenv("LOG_DIR"),
		LoadCheckInterval:  3 * time.Second,
		HealthCheckTimeout: 2 * time.Second,
		FailoverRetries:    2,
	}

	// Default backends for testing
	if config.BackendA == "" {
		config.BackendA = "http://localhost:9001"
	}
	if config.BackendB == "" {
		config.BackendB = "http://localhost:9002"
	}
	if config.BackendC == "" {
		config.BackendC = "http://localhost:9003"
	}
	if config.RedisURL == "" {
		config.RedisURL = "redis://localhost:6379"
	}
	if config.LogDir == "" {
		config.LogDir = "./logs"
	}

	// Parse load check interval from env if provided
	if interval := os.Getenv("LOAD_CHECK_INTERVAL"); interval != "" {
		if seconds, err := strconv.Atoi(interval); err == nil {
			config.LoadCheckInterval = time.Duration(seconds) * time.Second
		}
	}

	// Create logs directory
	if err := os.MkdirAll(config.LogDir, 0755); err != nil {
		log.Fatalf("Failed to create log directory: %v", err)
	}

	// Initialize Redis client
	opt, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}

	redisClient := redis.NewClient(opt)
	defer redisClient.Close()

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("✓ Connected to Redis")

	// Initialize load balancer service
	service := &LoadBalancerService{
		redis:  redisClient,
		config: config,
		backends: map[string]string{
			"A": config.BackendA,
			"B": config.BackendB,
			"C": config.BackendC,
		},
		loads: map[string]int{
			"A": 0,
			"B": 0,
			"C": 0,
		},
	}

	// Start periodic load checking
	go service.startLoadChecker(context.Background())

	// Setup routes
	http.HandleFunc("/health", service.healthHandler)
	http.HandleFunc("/", service.loadBalanceHandler)

	// Start server
	port := ":8081"
	log.Printf("Load Balancer listening on %s", port)
	log.Printf("Backends: A=%s, B=%s, C=%s", config.BackendA, config.BackendB, config.BackendC)
	log.Printf("Load check interval: %v", config.LoadCheckInterval)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// healthHandler returns service health status
func (s *LoadBalancerService) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.mu.RLock()
	loads := map[string]int{
		"A": s.loads["A"],
		"B": s.loads["B"],
		"C": s.loads["C"],
	}
	s.mu.RUnlock()

	response := map[string]interface{}{
		"status":    "ok",
		"service":   "load-balancer",
		"loads":     loads,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}

// loadBalanceHandler handles incoming requests
func (s *LoadBalancerService) loadBalanceHandler(w http.ResponseWriter, r *http.Request) {
	// Check for existing cookie
	cookie, err := r.Cookie("backend_assignment")
	var assignment string

	if err == http.ErrNoCookie {
		// No cookie - assign based on current load
		assignment = s.assignBasedOnLoad()
		log.Printf("→ No cookie found, assigning to backend %s", assignment)

		// Set cookie in response
		http.SetCookie(w, &http.Cookie{
			Name:     "backend_assignment",
			Value:    assignment,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   86400, // 24 hours
			SameSite: http.SameSiteLaxMode,
		})
	} else if err != nil {
		// Cookie error
		log.Printf("✗ Cookie error: %v", err)
		s.respondWithError(w, 400, "Invalid cookie")
		return
	} else {
		// Has cookie
		assignment = cookie.Value
		if !isValidAssignment(assignment) {
			log.Printf("✗ Invalid cookie value: %s", assignment)
			s.respondWithError(w, 400, "Invalid cookie value")
			return
		}
		log.Printf("✓ Existing cookie found: %s (sticky routing)", assignment)
	}

	// Route to backend with retries
	backendURL := s.backends[assignment]
	success := s.routeWithFailover(w, r, assignment, backendURL)

	if success {
		// Log successful routing
		s.logRequest(assignment, r, http.StatusOK)
	} else {
		// All backends failed
		s.respondWithError(w, 503, "All backends unavailable")
		s.logRequest(assignment, r, 503)
	}
}

// assignBasedOnLoad selects backend with least load
func (s *LoadBalancerService) assignBasedOnLoad() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find backend with minimum load
	minLoad := s.loads["A"]
	assignment := "A"

	if s.loads["B"] < minLoad {
		minLoad = s.loads["B"]
		assignment = "B"
	}

	if s.loads["C"] < minLoad {
		assignment = "C"
	}

	log.Printf("Load based assignment - A:%d, B:%d, C:%d → Selected: %s",
		s.loads["A"], s.loads["B"], s.loads["C"], assignment)

	return assignment
}

// routeWithFailover routes request with automatic failover
func (s *LoadBalancerService) routeWithFailover(w http.ResponseWriter, r *http.Request, assignment string, primaryURL string) bool {
	// Get backend list in failover order
	backends := []struct{ name, url string }{
		{assignment, primaryURL},
	}

	// Add other backends for failover
	for backend, url := range s.backends {
		if backend != assignment {
			backends = append(backends, struct{ name, url string }{backend, url})
		}
	}

	// Try each backend
	for attempt, backend := range backends {
		success := s.proxyRequest(w, r, backend.name, backend.url)
		if success {
			return true
		}

		// If primary failed and we have retries left
		if attempt == 0 && attempt < len(backends)-1 {
			log.Printf("✗ Backend %s failed, attempting failover to %s", backend.name, backends[attempt+1].name)
			w.Header().Set("X-Failover", fmt.Sprintf("Original: %s, Retry: %s", backend.name, backends[attempt+1].name))
		}
	}

	return false
}

// proxyRequest proxies the request to a backend
func (s *LoadBalancerService) proxyRequest(w http.ResponseWriter, r *http.Request, backend string, backendURL string) bool {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Create proxy request
	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, backendURL+r.RequestURI, r.Body)
	if err != nil {
		log.Printf("✗ Failed to create proxy request for %s: %v", backend, err)
		return false
	}

	// Copy headers (except hop-by-hop)
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Make the request
	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		log.Printf("✗ Backend %s request failed: %v", backend, err)
		return false
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("✗ Failed to copy response body from %s: %v", backend, err)
		return false
	}

	log.Printf("✓ Successfully routed to backend %s", backend)
	return true
}

// respondWithError sends error response
func (s *LoadBalancerService) respondWithError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"status":    statusCode,
		"message":   message,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}

// logRequest logs all requests to cookie-specific files
func (s *LoadBalancerService) logRequest(assignment string, r *http.Request, statusCode int) {
	logFile := fmt.Sprintf("%s/cookie-%s.log", s.config.LogDir, assignment)

	logEntry := fmt.Sprintf(
		"[%s] ROUTED | IP: %s | Method: %s | Path: %s | Status: %d\n",
		time.Now().Format(time.RFC3339),
		r.RemoteAddr,
		r.Method,
		r.RequestURI,
		statusCode,
	)

	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("✗ Failed to open log file %s: %v", logFile, err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(logEntry); err != nil {
		log.Printf("✗ Failed to write to log file: %v", err)
	}
}

// startLoadChecker periodically checks backend loads
func (s *LoadBalancerService) startLoadChecker(ctx context.Context) {
	ticker := time.NewTicker(s.config.LoadCheckInterval)
	defer ticker.Stop()

	// Do first check immediately
	s.checkBackendLoads(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkBackendLoads(ctx)
		}
	}
}

// checkBackendLoads queries backends for their current load
func (s *LoadBalancerService) checkBackendLoads(ctx context.Context) {
	var wg sync.WaitGroup

	for backend, backendURL := range s.backends {
		wg.Add(1)
		go func(backend, url string) {
			defer wg.Done()
			s.checkBackendLoad(ctx, backend, url)
		}(backend, backendURL)
	}

	wg.Wait()

	// Store loads in Redis
	s.storeLoadsInRedis(ctx)
}

// checkBackendLoad queries a specific backend's load
func (s *LoadBalancerService) checkBackendLoad(ctx context.Context, backend string, backendURL string) {
	checkCtx, cancel := context.WithTimeout(ctx, s.config.HealthCheckTimeout)
	defer cancel()

	// Make health check request
	req, _ := http.NewRequestWithContext(checkCtx, "GET", backendURL+"/health", nil)
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Printf("✗ Health check failed for backend %s: %v", backend, err)
		s.mu.Lock()
		s.loads[backend] = 999 // High load to avoid routing to dead backend
		s.mu.Unlock()
		return
	}
	defer resp.Body.Close()

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var health HealthResponse

	if err := json.Unmarshal(body, &health); err != nil {
		log.Printf("✗ Failed to parse health response from %s: %v", backend, err)
		return
	}

	// Update load
	s.mu.Lock()
	s.loads[backend] = health.ActiveConnections
	s.mu.Unlock()

	log.Printf("✓ Backend %s health check - Active connections: %d", backend, health.ActiveConnections)
}

// storeLoadsInRedis stores current loads in Redis for monitoring
func (s *LoadBalancerService) storeLoadsInRedis(ctx context.Context) {
	s.mu.RLock()
	loads := map[string]int{
		"A": s.loads["A"],
		"B": s.loads["B"],
		"C": s.loads["C"],
	}
	s.mu.RUnlock()

	for backend, load := range loads {
		key := fmt.Sprintf("backend_load:%s", backend)
		loadData := BackendLoad{
			Backend:           backend,
			ActiveConnections: load,
			LastChecked:       time.Now().Format(time.RFC3339),
		}

		data, _ := json.Marshal(loadData)
		if err := s.redis.Set(ctx, key, string(data), 5*time.Minute).Err(); err != nil {
			log.Printf("✗ Failed to store load in Redis: %v", err)
		}
	}
}

// isValidAssignment checks if assignment is A, B, or C
func isValidAssignment(assignment string) bool {
	return assignment == "A" || assignment == "B" || assignment == "C"
}
