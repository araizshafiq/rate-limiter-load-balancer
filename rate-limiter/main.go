package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds all configurable parameters
type Config struct {
	MaxTokens       int64
	RefillRate      int64 // tokens per second
	RefillInterval  time.Duration
	LoadBalancerURL string
	RedisURL        string
	LogDir          string
}

// RateLimiterService handles rate limiting logic
type RateLimiterService struct {
	redis  *redis.Client
	config Config
	proxy  *httputil.ReverseProxy
}

// ErrorResponse JSON format for errors
type ErrorResponse struct {
	Status    int    `json:"status"`
	Message   string `json:"message"`
	Cookie    string `json:"cookie,omitempty"`
	Timestamp string `json:"timestamp"`
}

// TokenBucket represents a rate limit bucket
type TokenBucket struct {
	Tokens       int64     `json:"tokens"`
	LastRefillAt time.Time `json:"last_refill_at"`
}

func main() {
	loadBalancerURL := os.Getenv("LOAD_BALANCER_URL")
	if loadBalancerURL == "" {
		loadBalancerURL = "http://localhost:8081"
	}

	// Configuration with changeable parameters
	config := Config{
		MaxTokens:       100,
		RefillRate:      10, // 10 tokens per second
		RefillInterval:  1 * time.Second,
		LoadBalancerURL: loadBalancerURL,
		RedisURL:        os.Getenv("REDIS_URL"),
		LogDir:          os.Getenv("LOG_DIR"),
	}

	if config.RedisURL == "" {
		config.RedisURL = "redis://localhost:6379"
	}
	if config.LogDir == "" {
		config.LogDir = "./logs"
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

	// Initialize rate limiter service
	service := &RateLimiterService{
		redis:  redisClient,
		config: config,
	}

	// Setup reverse proxy to load balancer
	loadBalancerURLParsed, err := url.Parse(config.LoadBalancerURL)
	if err != nil {
		log.Fatalf("Invalid load balancer URL: %v", err)
	}

	service.proxy = httputil.NewSingleHostReverseProxy(loadBalancerURLParsed)

	// Setup routes
	http.HandleFunc("/health", service.healthHandler)
	http.HandleFunc("/", service.rateLimitMiddleware)

	// Start server
	port := ":8080"
	log.Printf("Rate Limiter listening on %s", port)
	log.Printf("Configuration: MaxTokens=%d, RefillRate=%d/sec", config.MaxTokens, config.RefillRate)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// healthHandler returns service health status
func (s *RateLimiterService) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","service":"rate-limiter"}`)
}

// rateLimitMiddleware checks rate limits for requests with cookies
func (s *RateLimiterService) rateLimitMiddleware(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if request has a cookie
	cookie, err := r.Cookie("backend_assignment")

	if err == http.ErrNoCookie {
		// No cookie - pass to load balancer for assignment
		log.Printf("→ No cookie found, forwarding to Load Balancer")
		s.proxy.ServeHTTP(w, r)
		return
	}

	if err != nil {
		// Other cookie errors
		s.respondWithError(w, 400, "Invalid cookie", "", err.Error())
		return
	}

	// Extract cookie value (should be A, B, or C)
	assignment := cookie.Value

	// Validate cookie value
	if !isValidAssignment(assignment) {
		s.respondWithError(w, 400, "Invalid cookie value", assignment, "Cookie must be A, B, or C")
		return
	}

	// Check rate limit for this cookie
	allowed, err := s.checkTokenBucket(ctx, assignment)
	if err != nil {
		log.Printf("✗ Error checking rate limit for %s: %v", assignment, err)
		s.respondWithError(w, 500, "Rate limit check failed", assignment, err.Error())
		return
	}

	if !allowed {
		// Rate limit exceeded - log and deny
		log.Printf("✗ Rate limit exceeded for cookie=%s from %s", assignment, r.RemoteAddr)
		s.logDeniedRequest(assignment, r)
		s.respondWithError(w, 429, "Rate limit exceeded", assignment, fmt.Sprintf("Too many requests for cookie %s", assignment))
		return
	}

	// Rate limit OK - pass to load balancer
	log.Printf("✓ Rate limit OK for cookie=%s, forwarding to Load Balancer", assignment)
	s.proxy.ServeHTTP(w, r)
}

// checkTokenBucket implements token bucket algorithm
func (s *RateLimiterService) checkTokenBucket(ctx context.Context, assignment string) (bool, error) {
	bucketKey := fmt.Sprintf("rate_limit:%s", assignment)

	// Try to get existing bucket
	bucketJSON, err := s.redis.Get(ctx, bucketKey).Result()

	var bucket TokenBucket

	if err == redis.Nil {
		// New bucket - initialize with full tokens
		bucket = TokenBucket{
			Tokens:       s.config.MaxTokens,
			LastRefillAt: time.Now(),
		}
	} else if err != nil {
		return false, err
	} else {
		// Decode existing bucket
		if err := json.Unmarshal([]byte(bucketJSON), &bucket); err != nil {
			return false, err
		}
	}

	// Calculate tokens to refill
	now := time.Now()
	timePassed := now.Sub(bucket.LastRefillAt).Seconds()
	tokensToAdd := int64(timePassed * float64(s.config.RefillRate))

	// Add tokens (but don't exceed max)
	bucket.Tokens += tokensToAdd
	if bucket.Tokens > s.config.MaxTokens {
		bucket.Tokens = s.config.MaxTokens
	}

	bucket.LastRefillAt = now

	// Check if we have tokens
	if bucket.Tokens >= 1 {
		bucket.Tokens-- // Consume one token

		// Save updated bucket
		bucketJSON, _ := json.Marshal(bucket)
		if err := s.redis.Set(ctx, bucketKey, string(bucketJSON), 24*time.Hour).Err(); err != nil {
			return false, err
		}

		return true, nil
	}

	// Out of tokens
	return false, nil
}

// logDeniedRequest logs denied requests to cookie-specific files
func (s *RateLimiterService) logDeniedRequest(assignment string, r *http.Request) {
	logFile := fmt.Sprintf("%s/cookie-%s.log", s.config.LogDir, assignment)

	logEntry := fmt.Sprintf(
		"[%s] DENIED | IP: %s | Method: %s | Path: %s | Reason: Rate limit exceeded\n",
		time.Now().Format(time.RFC3339),
		r.RemoteAddr,
		r.Method,
		r.RequestURI,
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

// respondWithError sends JSON error response
func (s *RateLimiterService) respondWithError(w http.ResponseWriter, statusCode int, message, cookie, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Status:    statusCode,
		Message:   message,
		Cookie:    cookie,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if details != "" {
		response.Message = fmt.Sprintf("%s: %s", message, details)
	}

	json.NewEncoder(w).Encode(response)
}

// isValidAssignment checks if assignment is A, B, or C
func isValidAssignment(assignment string) bool {
	return assignment == "A" || assignment == "B" || assignment == "C"
}
