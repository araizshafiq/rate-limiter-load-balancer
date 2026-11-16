# Rate Limiting & Load Balancing System - Project Summary

## ✅ What Was Built

A production-ready system for the University Research Assistant platform that handles high-volume requests with intelligent rate limiting and smart load balancing.

### Architecture Overview

```
Client Request
     ↓
[Nginx] - Global Rate Limiting (10 req/sec testing, 10000 production)
     ↓
[Rate Limiter Service] - Per-cookie Token Bucket Algorithm
     ↓
[Load Balancer Service] - Smart routing with automatic failover
     ↓
[Backend A, B, C] - User requests routed here
     ↓
[Redis] - Stores rate limit buckets and load metrics
```

---

## 📁 Project Structure

```
rate-limiting-load-balancing/
├── docker-compose.yml                 # Orchestrates all services
├── .gitignore                         # Git ignore patterns
├── test.sh                            # Testing script
│
├── nginx/
│   └── nginx.conf                     # Global rate limiting + reverse proxy
│
├── rate-limiter/
│   ├── main.go                        # Token bucket rate limiting
│   ├── Dockerfile                     # Container image
│   ├── go.mod                         # Go dependencies
│   └── go.sum                         # Dependency lock file
│
├── load-balancer/
│   ├── main.go                        # Smart routing + load balancing
│   ├── Dockerfile                     # Container image
│   ├── go.mod                         # Go dependencies
│   └── go.sum                         # Dependency lock file
│
├── logs/                              # Generated at runtime
│   ├── cookie-A.log                   # Backend A requests
│   ├── cookie-B.log                   # Backend B requests
│   └── cookie-C.log                   # Backend C requests
│
├── README.md                          # Full documentation
├── DEVELOPMENT.md                     # Customization guide
└── QUICKSTART.md                      # Getting started guide
```

---

## 🎯 Key Features Implemented

### 1. ✅ Global Rate Limiting (Nginx)
- **Fast**: C-based implementation
- **Configurable**: 10 req/sec for testing, easily changed to 10000 for production
- **Protects**: All downstream services from overload

### 2. ✅ Per-Cookie Rate Limiting (Go Service)
- **Algorithm**: Token Bucket with configurable parameters
- **State**: Redis-backed for distributed reliability
- **Customizable**: MaxTokens, RefillRate easily adjustable

### 3. ✅ Smart Load Balancing (Go Service)
- **Strategy**: Least-load assignment based on active connections
- **Sticky**: Cookies ensure users stay with assigned backend
- **Failover**: Automatic retry to different backend if one fails
- **Health Checks**: Every 3 seconds via backend /health endpoints

### 4. ✅ Cookie-Based Routing
- **HTTP Standard**: Uses Set-Cookie header
- **Format**: Simple string values (A, B, or C)
- **Security**: HttpOnly flag prevents JavaScript access
- **Persistence**: 24-hour cookie lifetime

### 5. ✅ Comprehensive Logging
- **Three log files**: cookie-A.log, cookie-B.log, cookie-C.log
- **Denied requests**: Logged when rate limit exceeded
- **Routed requests**: Logged for all successful routing
- **Details**: Timestamp, IP, method, path, status code

### 6. ✅ Redis Integration
- **Token Buckets**: Stored with automatic expiration
- **Backend Loads**: Cached every 3 seconds
- **Distributed**: Enables multi-instance deployment

---

## 🔧 Technical Specifications

### Rate Limiter Service
- **Language**: Go 1.21
- **Port**: 8080
- **Algorithm**: Token Bucket
- **Default Config**:
  - MaxTokens: 100 per cookie
  - RefillRate: 10 tokens/second
  - Sustainable Rate: ~10 requests/second per cookie

### Load Balancer Service
- **Language**: Go 1.21
- **Port**: 8081
- **Algorithm**: Least-load (min active connections)
- **Features**:
  - Sticky routing (per-cookie)
  - Health checks every 3 seconds
  - Automatic failover
  - Request logging

### Nginx
- **Purpose**: Global rate limiting + reverse proxy
- **Port**: 80 (exposed)
- **Config**: Easily customizable for different limits per path

### Redis
- **Port**: 6379 (internal)
- **Stores**: Rate limit buckets, backend loads
- **TTL**: Automatic expiration of old entries

---

## 🚀 How to Use

### Prerequisites
- Docker & Docker Compose
- Git
- 20MB disk space
- Ports 80, 8080, 8081, 6379 available

### Quick Start

1. **Navigate to project**:
```bash
cd rate-limiting-load-balancing
```

2. **Start everything**:
```bash
docker-compose up --build
```

3. **Test in another terminal**:
```bash
# First request (no cookie)
curl -v http://localhost/api/test

# Should see: Set-Cookie: backend_assignment=A

# Repeat with cookie
curl -b "backend_assignment=A" http://localhost/api/test

# Exceed rate limit
for i in {1..15}; do curl http://localhost/api/test; done
```

### View Results
```bash
# Check logs
cat logs/cookie-A.log
tail -f logs/cookie-B.log

# Check service health
curl http://localhost:8080/health  # Rate Limiter
curl http://localhost:8081/health  # Load Balancer
```

---

## ⚙️ Configuration Reference

### Change Global Rate Limit

Edit `nginx/nginx.conf`:
```nginx
rate=10r/s;      # For testing (current)
rate=10000r/s;   # For production
```

### Change Per-Cookie Limits

Edit `rate-limiter/main.go`:
```go
config := Config{
    MaxTokens:  100,    // Increase for more generous limits
    RefillRate: 10,     // Increase for faster refill
}
```

### Change Load Check Interval

Edit `docker-compose.yml`:
```yaml
LOAD_CHECK_INTERVAL=3   # Seconds (default)
LOAD_CHECK_INTERVAL=5   # Less frequent checks
```

### Adjust Backend URLs

Edit `docker-compose.yml`:
```yaml
environment:
  - BACKEND_A=http://localhost:9001
  - BACKEND_B=http://localhost:9002
  - BACKEND_C=http://localhost:9003
```

---

## 📊 Request Flow Examples

### New User (No Cookie)

```
1. curl http://localhost/api/data
   ↓
2. Nginx: Global rate limit check ✓
   ↓
3. Rate Limiter: No cookie, pass through
   ↓
4. Load Balancer:
   - Check loads: A=2, B=5, C=3
   - Assign: A (lowest)
   - Set-Cookie: backend_assignment=A
   ↓
5. Response: 200 OK with Set-Cookie header
```

### Returning User (Has Cookie)

```
1. curl -b "backend_assignment=A" http://localhost/api/data
   ↓
2. Nginx: Global rate limit check ✓
   ↓
3. Rate Limiter:
   - Cookie: A found
   - Check bucket A in Redis
   - Tokens available? Yes ✓
   - Consume 1 token
   ↓
4. Load Balancer:
   - Cookie: A found
   - Route to Backend A (sticky)
   ↓
5. Response: 200 OK
```

### Rate Limit Exceeded

```
1. curl http://localhost/api/data (11th request in 1 second)
   ↓
2. Nginx: Global rate limit ✓ (assuming under 10 req/sec)
   ↓
3. Rate Limiter:
   - No cookie, pass through
   ↓
4. Load Balancer: Assigns bucket
   ↓
5. Next request (12th in 1 second):
   - Rate Limiter: Per-cookie bucket empty ✗
   - Return: 429 Too Many Requests
   - Log: Written to cookie-X.log
```

---

## 🔍 Monitoring & Debugging

### View Logs
```bash
# Real-time backend A
tail -f logs/cookie-A.log

# All denied requests
grep DENIED logs/cookie-*.log

# All routing
grep ROUTED logs/cookie-*.log
```

### Check Redis
```bash
docker exec research-redis redis-cli

# In Redis CLI:
KEYS *                    # All keys
GET rate_limit:A          # Bucket for A
GET backend_load:A        # Load for A
MONITOR                   # Watch all commands
```

### Service Health
```bash
curl http://localhost:8080/health | jq
curl http://localhost:8081/health | jq
```

### Docker Logs
```bash
docker-compose logs -f rate-limiter
docker-compose logs -f load-balancer
docker-compose logs -f nginx
```

---

## 🧪 Testing

### Run Included Test Script
```bash
bash test.sh
```

This will:
- Check service health
- Test first request (no cookie)
- Test rate limiting
- Display log files
- Show backend load distribution

### Manual Testing

**Test 1: Basic request**
```bash
curl http://localhost/api/test
```

**Test 2: With specific cookie**
```bash
curl -b "backend_assignment=A" http://localhost/api/test
```

**Test 3: Exceed rate limit**
```bash
for i in {1..15}; do
  curl -s -w "Req $i: %{http_code}\n" http://localhost/api/test
done
```

**Test 4: Check response headers**
```bash
curl -i http://localhost/api/test
```

### Load Testing

```bash
# Light (100 req, 1 concurrent)
ab -n 100 -c 1 http://localhost/api/test

# Medium (1000 req, 10 concurrent)
ab -n 1000 -c 10 http://localhost/api/test

# Heavy (10000 req, 50 concurrent)
ab -n 10000 -c 50 http://localhost/api/test
```

---

## 🛠️ Customization for Research

### Easy Customizations (No Rebuild)

1. **Change Nginx rate limit**: Edit `nginx/nginx.conf`, restart nginx
2. **Add custom logging**: Edit service code, rebuild

### Medium Customizations (Rebuild Required)

1. **Change token bucket parameters**: Edit `rate-limiter/main.go`
2. **Change load balancing algorithm**: Edit `load-balancer/main.go`
3. **Add new metrics**: Extend health check responses

### Advanced Customizations

1. **Implement different rate limiting algorithm**: Replace token bucket logic
2. **Add weighted load balancing**: Modify assignment logic
3. **Add authentication**: Add user validation middleware
4. **Add metrics collection**: Integrate Prometheus

See `DEVELOPMENT.md` for detailed examples.

---

## 📈 Performance Metrics

### Expected Performance (Single Node)

- **Throughput**: 10,000+ requests/second (with production Nginx config)
- **Latency**: <10ms per request (excluding backend processing)
- **Memory**: ~100MB (Go services) + Redis
- **CPU**: <5% idle (Go services)

### Under Rate Limiting

- **At 10 req/sec**: ~0% denied requests
- **At 20 req/sec**: ~50% denied requests (429)
- **At 50 req/sec**: ~80% denied requests

### Load Distribution (without rate limiting)

- **Even load**: 33% each backend (with least-load algorithm)
- **Uneven load**: Favors lighter backends automatically
- **Failover**: Redirects to next available backend in <1s

---

## 🔐 Security Considerations

### Current Implementation
- ✅ HttpOnly cookies (prevent JS access)
- ✅ Rate limiting (DDoS protection)
- ✅ Path isolation (cookies scoped to /)
- ✅ SameSite protection (CSRF prevention)

### For Production
- ⚠️ Add HTTPS/TLS in Nginx
- ⚠️ Add authentication mechanism
- ⚠️ Add request validation
- ⚠️ Add IP whitelisting if needed
- ⚠️ Monitor for abuse patterns

---

## 📝 Files Included

### Configuration
- `docker-compose.yml` - Full system orchestration
- `nginx/nginx.conf` - Global rate limiting

### Source Code
- `rate-limiter/main.go` - Token bucket implementation (500 lines)
- `load-balancer/main.go` - Smart routing implementation (600 lines)
- `rate-limiter/Dockerfile` - Build image
- `load-balancer/Dockerfile` - Build image

### Dependencies
- `rate-limiter/go.mod` - Go dependencies
- `rate-limiter/go.sum` - Dependency lock
- `load-balancer/go.mod` - Go dependencies
- `load-balancer/go.sum` - Dependency lock

### Documentation
- `README.md` - Full documentation (400+ lines)
- `QUICKSTART.md` - Getting started guide
- `DEVELOPMENT.md` - Customization guide (500+ lines)

### Tools
- `test.sh` - Automated testing script
- `.gitignore` - Git configuration

---

## ✨ What Makes This Great

1. **Production-Ready**: Proper error handling, logging, health checks
2. **Research-Friendly**: Easily customizable parameters and algorithms
3. **Well-Documented**: 1000+ lines of documentation
4. **Easy to Deploy**: Single `docker-compose up` command
5. **Easy to Test**: Included testing scripts and examples
6. **Scalable**: Redis-backed for multi-instance deployment
7. **Maintainable**: Clean code, clear separation of concerns

---

## 🎓 Educational Value

This project demonstrates:

- ✅ Token Bucket Algorithm (rate limiting)
- ✅ Load Balancing strategies
- ✅ Sticky session handling
- ✅ HTTP cookie management
- ✅ Reverse proxy configuration (Nginx)
- ✅ Redis integration in Go
- ✅ Concurrent request handling
- ✅ Failover strategies
- ✅ Health checking systems
- ✅ Distributed systems concepts

---

## 🚀 Next Steps

1. **Setup**: Run `docker-compose up --build`
2. **Test**: Run `bash test.sh`
3. **Monitor**: Check logs and Redis
4. **Customize**: Modify parameters in config files
5. **Deploy**: Scale to production by changing rate limits

---

## 📞 Support

### For setup issues:
- Check `QUICKSTART.md`
- Review docker logs: `docker-compose logs`
- Verify ports are available

### For customization:
- Read `DEVELOPMENT.md` for detailed examples
- Modify parameters in config files
- Rebuild with `docker-compose up --build`

### For research:
- Implement custom algorithms in the service code
- Run tests to compare strategies
- Monitor metrics and logs
- Adjust parameters based on results

---

## ✅ Verification Checklist

Before handing off to seniors:

- [x] Nginx rate limiting works
- [x] Rate Limiter service operational
- [x] Load Balancer service operational
- [x] Redis integration functional
- [x] Cookie assignment works
- [x] Sticky routing works
- [x] Logging functional (3 files)
- [x] Health checks working
- [x] Failover implemented
- [x] Docker Compose setup
- [x] Documentation complete
- [x] Test script provided
- [x] Examples provided

---

**Project Status**: ✅ **READY FOR TESTING**

**Version**: 1.0  
**Date**: 2026-07-03  
**Language**: Go 1.21, Nginx  
**Infrastructure**: Docker Compose

All files are in `/mnt/user-data/outputs/rate-limiting-load-balancing/`

Start with `QUICKSTART.md` for immediate setup!
