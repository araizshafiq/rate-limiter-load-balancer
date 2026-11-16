# Development Guide

This guide explains how to customize and extend the Rate Limiting & Load Balancing system for research purposes.

## Project Structure

```
.
├── docker-compose.yml          # Docker setup
├── nginx/
│   └── nginx.conf             # Nginx configuration
├── rate-limiter/
│   ├── main.go               # Rate limiter service
│   ├── go.mod
│   ├── go.sum
│   └── Dockerfile
├── load-balancer/
│   ├── main.go               # Load balancer service
│   ├── go.mod
│   ├── go.sum
│   └── Dockerfile
├── logs/                       # Generated log files
├── README.md                   # User guide
└── DEVELOPMENT.md             # This file
```

## Customization Points

### 1. Rate Limiting Algorithm

**Current**: Token Bucket with fixed refill rate

**Location**: `rate-limiter/main.go` - `checkTokenBucket()` function

**To experiment with other algorithms** (Leaky Bucket, Sliding Window, etc.):

```go
// Replace checkTokenBucket() with your algorithm
func (s *RateLimiterService) checkTokenBucket(ctx context.Context, assignment string) (bool, error) {
    // Your custom algorithm here
    
    // Must return: (allowed bool, error)
}
```

**Example: Implement Sliding Window**
```go
func (s *RateLimiterService) checkSlidingWindow(ctx context.Context, assignment string) (bool, error) {
    key := fmt.Sprintf("window:%s", assignment)
    
    now := time.Now().Unix()
    windowStart := now - 60 // 60 second window
    
    // Count requests in window using Redis ZSET
    count, _ := s.redis.ZCount(ctx, key, 
        fmt.Sprintf("%d", windowStart), 
        fmt.Sprintf("%d", now)).Result()
    
    if count >= 100 { // Max 100 per minute
        return false, nil
    }
    
    // Add current request to set
    s.redis.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: uuid.New().String()})
    
    return true, nil
}
```

### 2. Load Balancing Strategy

**Current**: Least-load (pick backend with fewest active connections)

**Location**: `load-balancer/main.go` - `assignBasedOnLoad()` function

**To change strategy** (Round-robin, Random, Response-time based, etc.):

```go
func (s *LoadBalancerService) assignBasedOnLoad() string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    // Your custom strategy here
    // Must return: one of "A", "B", "C"
}
```

**Example: Round-robin**
```go
func (s *LoadBalancerService) assignRoundRobin() string {
    ctx := context.Background()
    
    // Get current counter
    counter, _ := s.redis.Incr(ctx, "round_robin_counter").Result()
    
    backends := []string{"A", "B", "C"}
    return backends[counter % 3]
}
```

**Example: Weighted**
```go
func (s *LoadBalancerService) assignWeighted() string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    // Weighted: A gets 50%, B gets 30%, C gets 20%
    weights := map[string]int{"A": 50, "B": 30, "C": 20}
    
    random := rand.Intn(100)
    if random < 50 { return "A" }
    if random < 80 { return "B" }
    return "C"
}
```

### 3. Health Check Metrics

**Current**: Active connections count

**Location**: `load-balancer/main.go` - `checkBackendLoad()` function

**To add custom metrics**:

```go
type HealthResponse struct {
    Status               string    `json:"status"`
    ActiveConnections    int       `json:"active_connections"`
    AvailableConnections int       `json:"available_connections"`
    
    // Add new fields
    CPUUsage            float64   `json:"cpu_usage"`
    MemoryUsage         float64   `json:"memory_usage"`
    ResponseTime        int       `json:"response_time_ms"`
    ErrorRate           float64   `json:"error_rate"`
}
```

Then modify load calculation:
```go
func (s *LoadBalancerService) calculateSmartLoad(health HealthResponse) int {
    // Combine multiple metrics
    connectionScore := health.ActiveConnections * 2
    cpuScore := int(health.CPUUsage * 100)
    errorScore := int(health.ErrorRate * 1000)
    
    totalScore := connectionScore + cpuScore + errorScore
    return totalScore
}
```

### 4. Logging Details

**Current**: Basic request logging

**Location**: `rate-limiter/main.go` and `load-balancer/main.go` - `logDeniedRequest()` and `logRequest()` functions

**To add more details**:

```go
type DetailedLogEntry struct {
    Timestamp       string
    IP              string
    UserAgent       string
    Method          string
    Path            string
    RequestHeaders  map[string]string
    ResponseStatus  int
    ResponseTime    int // milliseconds
    BackendUsed     string
    RateLimitBucket string
    TokensRemaining int
}

func (s *LoadBalancerService) logDetailedRequest(entry DetailedLogEntry) {
    jsonEntry, _ := json.MarshalIndent(entry, "", "  ")
    // Write to log
}
```

### 5. Rate Limit Parameters (Easy Changes)

**Location**: `rate-limiter/main.go` - `main()` function

```go
config := Config{
    MaxTokens:      100,        // Change this
    RefillRate:     10,         // Change this
    RefillInterval: 1 * time.Second,
}
```

**Scenario Examples**:

**Conservative (protect backend)**:
```go
MaxTokens:  50,   // Lower
RefillRate: 5,    // Slower refill
```

**Generous (user-friendly)**:
```go
MaxTokens:  1000,  // Higher
RefillRate: 100,   // Faster refill
```

**Burst-friendly**:
```go
MaxTokens:  200,   // High burst capacity
RefillRate: 5,     // Slow refill (bursts allowed)
```

### 6. Global Rate Limit (Nginx)

**Location**: `nginx/nginx.conf`

```nginx
# Current: 10 requests per second
limit_req_zone $binary_remote_addr zone=global_limit:10m rate=10r/s;

# For production:
limit_req_zone $binary_remote_addr zone=global_limit:10m rate=10000r/s;

# Advanced: Per-URI limits
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;
limit_req_zone $binary_remote_addr zone=upload_limit:10m rate=10r/s;
```

Then apply different limits:
```nginx
location /api/ {
    limit_req zone=api_limit burst=50;
    proxy_pass http://rate_limiter;
}

location /upload/ {
    limit_req zone=upload_limit burst=5;
    proxy_pass http://rate_limiter;
}
```

### 7. Failover Strategy

**Current**: Try primary, then fallback to others

**Location**: `load-balancer/main.go` - `routeWithFailover()` function

**To customize**:

```go
// Current: Try primary first, then alphabetical fallback
backends := []struct{name, url string}{
    {assignment, primaryURL},  // Try assigned first
}

// Alternative: Smart failover based on load
func (s *LoadBalancerService) getFailoverOrder(primary string) []string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    // Return backends sorted by load (lightest first)
    backends := []string{"A", "B", "C"}
    sort.Slice(backends, func(i, j int) bool {
        return s.loads[backends[i]] < s.loads[backends[j]]
    })
    
    // Put primary first
    result := []string{primary}
    for _, b := range backends {
        if b != primary {
            result = append(result, b)
        }
    }
    
    return result
}
```

### 8. Add Monitoring/Metrics

Create new file: `rate-limiter/metrics.go`

```go
package main

import (
    "sync"
    "time"
)

type Metrics struct {
    mu                  sync.RWMutex
    TotalRequests       int64
    AllowedRequests     int64
    DeniedRequests      int64
    DenialsByBucket     map[string]int64
    AverageResponseTime time.Duration
}

func (m *Metrics) RecordRequest(allowed bool, bucket string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.TotalRequests++
    if allowed {
        m.AllowedRequests++
    } else {
        m.DeniedRequests++
        m.DenialsByBucket[bucket]++
    }
}

func (m *Metrics) String() string {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    return fmt.Sprintf(
        "Total: %d, Allowed: %d, Denied: %d (%.1f%%)",
        m.TotalRequests,
        m.AllowedRequests,
        m.DeniedRequests,
        float64(m.DeniedRequests) / float64(m.TotalRequests) * 100,
    )
}
```

### 9. Add Request Prioritization

Example: Prioritize certain requests over others

```go
func (s *RateLimiterService) getPriority(r *http.Request) int {
    // High priority: admin users, specific paths
    if r.Header.Get("X-Priority") == "high" {
        return 10
    }
    
    if strings.HasPrefix(r.URL.Path, "/api/critical") {
        return 8
    }
    
    return 1 // Default priority
}

func (s *RateLimiterService) checkTokenBucketWithPriority(
    ctx context.Context, 
    assignment string, 
    priority int) (bool, error) {
    
    // High priority requests consume fewer tokens
    tokensCost := 1
    if priority > 5 {
        tokensCost = 1 // Or even: tokensCost = 0 (free)
    }
    
    // Rest of logic...
}
```

## Testing Custom Changes

### 1. Unit Testing

Create `rate-limiter/main_test.go`:

```go
package main

import (
    "context"
    "testing"
)

func TestTokenBucket(t *testing.T) {
    service := &RateLimiterService{
        config: Config{
            MaxTokens:  100,
            RefillRate: 10,
        },
    }
    
    // Initialize mock Redis...
    
    allowed, _ := service.checkTokenBucket(context.Background(), "A")
    if !allowed {
        t.Fail()
    }
}
```

### 2. Integration Testing

```bash
# Start services
docker-compose up -d

# Run test script
bash test.sh

# Monitor logs
tail -f logs/cookie-*.log
```

### 3. Load Testing

Use Apache Bench or similar:

```bash
# Single threaded
ab -n 100 -c 1 http://localhost/api/test

# Multi-threaded (50 concurrent)
ab -n 1000 -c 50 http://localhost/api/test

# Watch rate limiting
ab -n 100 -c 10 http://localhost/api/test 2>&1 | grep "429"
```

## Adding New Features

### Example: Add User-based Rate Limiting

1. **Extract user ID from request**:
```go
func getUserID(r *http.Request) string {
    // From header
    if id := r.Header.Get("X-User-ID"); id != "" {
        return id
    }
    
    // From cookie
    if cookie, _ := r.Cookie("user_id"); cookie != nil {
        return cookie.Value
    }
    
    // Default to IP
    return r.RemoteAddr
}
```

2. **Modify rate limiter**:
```go
func (s *RateLimiterService) rateLimitMiddleware(w http.ResponseWriter, r *http.Request) {
    userID := getUserID(r)
    
    // Check per-user limit
    allowed := s.checkUserRateLimit(r.Context(), userID)
    
    if !allowed {
        s.respondWithError(w, 429, "User rate limit exceeded")
        return
    }
    
    // Continue as normal
}
```

3. **Test**:
```bash
curl -H "X-User-ID: user123" http://localhost/api/test
```

## Debugging

### Enable verbose logging

Add to services:

```go
import "log"

// In functions
log.Printf("DEBUG: Cookie=%s, Load=%d", assignment, load)
```

### Redis inspection

```bash
docker exec research-redis redis-cli

# In redis-cli:
MONITOR              # Watch all commands
KEYS rate_limit:*    # See all rate limit buckets
GET rate_limit:A     # See specific bucket
FLUSHDB              # Clear all data
```

### Network inspection

```bash
# Watch Nginx
docker logs research-nginx -f

# Watch Rate Limiter
docker logs research-rate-limiter -f

# Watch Load Balancer
docker logs research-load-balancer -f
```

## Performance Tuning

### Redis Connection Pooling
```go
opt := redis.Options{
    Addr: "redis:6379",
    PoolSize: 100,  // Increase from default
}
```

### Request Buffering
```go
// In Nginx
proxy_buffering off;  # Lower latency
proxy_buffering on;   # Better throughput
```

### Backend Health Check Frequency
```yaml
# In docker-compose.yml
LOAD_CHECK_INTERVAL=10  # Check every 10 seconds (less overhead)
```

## Questions for Seniors

When customizing, consider:

1. **What are we optimizing for?** (Throughput, Latency, Fairness, Reliability)
2. **What are realistic traffic patterns?** (Burst? Steady? Peak hours?)
3. **What backend constraints exist?** (Max connections? Memory? CPU?)
4. **How important is stickiness?** (Session state vs load distribution)
5. **What's acceptable denial rate?** (1%? 0.1%?)

---

**Next Steps**: Pick a customization, implement it, run tests, compare results!
