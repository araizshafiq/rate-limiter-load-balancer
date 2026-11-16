# University Research Assistant - Rate Limiting & Load Balancing System

A production-ready system for handling high-volume requests with smart rate limiting and intelligent load balancing.

## Architecture

```
┌──────────────────┐
│  Client Request  │
└────────┬─────────┘
         │
    ┌────▼─────────────────────────────────────┐
    │  Nginx - Global Rate Limiting             │
    │  (10 req/sec for testing, 10000 for prod)│
    │  - Fast C-based filtering                │
    │  - Protects all downstream services      │
    └────┬──────────────────────────────────────┘
         │
    ┌────▼────────────────────────────────────┐
    │  Rate Limiter Service (Go)               │
    │  - Per-cookie token bucket algorithm     │
    │  - Redis-backed state                    │
    │  - Logs denied requests                  │
    └────┬─────────────────────────────────────┘
         │
    ┌────▼────────────────────────────────────┐
    │  Load Balancer Service (Go)              │
    │  - Smart assignment based on load        │
    │  - Sticky routing (per-cookie)           │
    │  - Automatic failover                    │
    │  - Logs all routed requests              │
    └────┬─────────────────────────────────────┘
         │
    ┌────┴──────────────┬─────────────┬──────────────┐
    │                   │             │              │
┌───▼──────┐      ┌──────▼──┐   ┌─────▼──┐   ┌──────▼──┐
│ Backend A│      │Backend B │   │Backend C│   │  Redis  │
│ Port 9001│      │Port 9002 │   │Port 9003│   │Port 6379│
└──────────┘      └──────────┘   └─────────┘   └─────────┘
```

## Components

### 1. Nginx
- **Role**: Global rate limiting at entry point
- **Configuration**: `/nginx/nginx.conf`
- **Current limit**: 10 req/sec (testing), configurable

### 2. Rate Limiter Service
- **Port**: 8080
- **Algorithm**: Token Bucket
- **State**: Redis
- **Features**:
  - Per-cookie rate limiting
  - Configurable max tokens and refill rate
  - Detailed denial logging

### 3. Load Balancer Service
- **Port**: 8081
- **Features**:
  - Smart assignment based on backend load
  - Sticky routing (cookie-based)
  - Automatic failover
  - Health checks every 3 seconds
  - Request logging per backend

### 4. Redis
- **Port**: 6379
- **Stores**:
  - Token bucket states
  - Backend load information
  - Rate limit metrics

## Configuration Parameters

### Rate Limiter (rate-limiter/main.go)

```go
Config{
    MaxTokens:      100,        // Max tokens per bucket (A, B, or C)
    RefillRate:     10,         // Tokens added per second
    RefillInterval: 1 * time.Second,
}
```

**Example**:
- MaxTokens = 100
- RefillRate = 10
- Means: Max 100 requests per cookie, refill 10 per second
- Sustainable rate: 10 requests/sec per cookie

### Load Balancer (load-balancer/main.go)

```go
Config{
    LoadCheckInterval:   3 * time.Second,  // How often to check backend load
    HealthCheckTimeout:  2 * time.Second,  // Timeout for health checks
    FailoverRetries:     2,                // Failover attempt count
}
```

### Nginx (nginx/nginx.conf)

```nginx
# Global rate limit zone
limit_req_zone $binary_remote_addr zone=global_limit:10m rate=10r/s;

# For production, change to:
# rate=10000r/s;
```

## Usage

### Prerequisites

- Docker & Docker Compose
- Git

### Quick Start

1. **Clone/setup project**:
```bash
cd rate-limiting-load-balancing
```

2. **Start all services**:
```bash
docker-compose up --build
```

This will:
- Start Redis
- Build and start Rate Limiter (port 8080)
- Build and start Load Balancer (port 8081)
- Start three demo backends used to demonstrate routing and failover
- Start Nginx (port 80)

3. **Test the system**:

```bash
# First request (no cookie)
curl -v http://localhost/test1

# Should see:
# - Set-Cookie: backend_assignment=A
# - 200 OK response

# Subsequent requests (has cookie)
curl -v -b "backend_assignment=A" http://localhost/test2

# Exceed rate limit (more than 10 req/sec)
for i in {1..15}; do curl http://localhost/test; done

# Should see 429 responses after 10 requests
```

## Logging

Logs are written to `./logs/` directory:

- **cookie-A.log**: All requests for backend A (allowed and denied)
- **cookie-B.log**: All requests for backend B (allowed and denied)
- **cookie-C.log**: All requests for backend C (allowed and denied)

### Log Format

**Rate Limiter (Denied Requests)**:
```
[2026-07-03T10:30:45Z] DENIED | IP: 127.0.0.1:54321 | Method: GET | Path: /api/data | Reason: Rate limit exceeded
```

**Load Balancer (Routed Requests)**:
```
[2026-07-03T10:30:46Z] ROUTED | IP: 127.0.0.1:54322 | Method: POST | Path: /api/submit | Status: 200
```

## API Endpoints

### Rate Limiter

- `GET /health` - Health check
- `*` - All requests passed to Load Balancer if rate limit allows

### Load Balancer

- `GET /health` - Returns current load status:
```json
{
  "status": "ok",
  "service": "load-balancer",
  "loads": {
    "A": 5,
    "B": 3,
    "C": 7
  },
  "timestamp": "2026-07-03T10:30:45Z"
}
```

## Error Responses

### Rate Limit Exceeded (429)
```json
{
  "status": 429,
  "message": "Rate limit exceeded: Too many requests for cookie A",
  "cookie": "A",
  "timestamp": "2026-07-03T10:30:45Z"
}
```

### Backend Unavailable (503)
```json
{
  "status": 503,
  "message": "All backends unavailable",
  "timestamp": "2026-07-03T10:30:45Z"
}
```

## Cookie Management

### Cookie Details

- **Name**: `backend_assignment`
- **Value**: `A`, `B`, or `C`
- **Path**: `/`
- **HttpOnly**: Yes (security)
- **Max-Age**: 86400 (24 hours)
- **SameSite**: Lax

### How It Works

**First Request** (no cookie):
1. Nginx: Checks global rate limit ✓
2. Rate Limiter: No cookie, passes to Load Balancer
3. Load Balancer: Checks backend loads
4. Load Balancer: Assigns to least-loaded backend (e.g., A)
5. Load Balancer: Sets cookie `backend_assignment=A`
6. Request routed to Backend A

**Subsequent Requests** (has cookie):
1. Nginx: Checks global rate limit ✓
2. Rate Limiter: Checks per-cookie rate limit ✓
3. Rate Limiter: Passes to Load Balancer
4. Load Balancer: Reads cookie (sticky routing)
5. Load Balancer: Routes to same backend (A)
6. Request routed to Backend A

## Customization

### Adjust Rate Limits

**For testing** (current):
```
Global Nginx: 10 req/sec
Per-cookie: 100 max, 10 refill/sec
```

**For production**:

1. **Edit Nginx config** (`nginx/nginx.conf`):
```nginx
rate=10000r/s;  # Change from 10r/s
```

2. **Edit Rate Limiter** (`rate-limiter/main.go`):
```go
config := Config{
    MaxTokens:   1000,      // Increase from 100
    RefillRate:  100,       // Increase from 10
}
```

3. **Rebuild and restart**:
```bash
docker-compose down
docker-compose up --build
```

### Adjust Load Check Interval

Edit `docker-compose.yml`:
```yaml
environment:
  - LOAD_CHECK_INTERVAL=5  # Change from 3 seconds to 5
```

### Add More Backends

1. **Edit `docker-compose.yml`**: Add backend services
2. **Edit `load-balancer/main.go`**: Extend load balancer logic
3. **Rebuild and restart**

## Monitoring

### Check Service Health

```bash
# Rate Limiter
curl http://localhost:8080/health

# Load Balancer
curl http://localhost:8081/health

# Check logs
tail -f logs/cookie-A.log
tail -f logs/cookie-B.log
tail -f logs/cookie-C.log
```

### Monitor Redis

```bash
docker exec research-redis redis-cli

# In Redis CLI:
KEYS *
GET rate_limit:A
GET backend_load:A
```

## Troubleshooting

### Services won't start
```bash
# Check logs
docker-compose logs rate-limiter
docker-compose logs load-balancer

# Rebuild
docker-compose up --build
```

### Nginx rate limiting too strict
- Increase `rate` in `nginx/nginx.conf`
- Increase `burst` value
- Restart: `docker-compose restart nginx`

### Backend load not updating
- Check Redis connection: `docker exec research-redis redis-cli ping`
- Verify backend health endpoints are working
- Check logs: `docker-compose logs load-balancer`

### Requests always go to same backend
- This is expected (sticky routing)
- Cookie remains with same backend for 24 hours
- To test load distribution, use different IPs or clear cookies

## Performance Considerations

1. **Redis Operations**: Token bucket checks use simple SET/GET operations (fast)
2. **Health Checks**: Every 3 seconds (configurable, minimal overhead)
3. **Nginx Global Limit**: C-based, minimal overhead even at 10,000 req/sec
4. **Logging**: Async file writes, buffered

## For Research/Testing

Seniors can customize:

1. **Token Bucket Parameters**: Test different refill strategies
2. **Load Assignment Algorithm**: Change from least-load to round-robin, etc.
3. **Failover Behavior**: Test backend failure scenarios
4. **Logging Detail**: Add request bodies, response times, etc.
5. **Health Check Metrics**: Add custom metrics to backend health

## Future Enhancements

- [ ] Metrics collection (Prometheus)
- [ ] Advanced load prediction
- [ ] Adaptive rate limiting based on backend health
- [ ] Request prioritization
- [ ] User-based rate limiting
- [ ] Per-IP limits in addition to per-cookie

---

**Author**: Created for University Research Platform  
**Last Updated**: 2026-07-03  
**Status**: Ready for testing
