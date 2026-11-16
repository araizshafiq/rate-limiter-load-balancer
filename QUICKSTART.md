# Quick Start Guide

Get the system up and running in 5 minutes.

## Prerequisites

- Docker & Docker Compose installed
- Bash shell
- curl or another HTTP client

## Step 1: Clone/Navigate to Project

```bash
cd rate-limiting-load-balancing
```

## Step 2: Start All Services

```bash
docker-compose up --build
```

Wait for output showing:
```
research-nginx    | [notice] ... worker processes started
research-rate-limiter | Rate Limiter listening on :8080
research-load-balancer | Load Balancer listening on :8081
```

## Step 3: Test the System

Open another terminal:

```bash
# Test 1: First request (should assign cookie)
curl -v http://localhost/api/test

# You should see:
# Set-Cookie: backend_assignment=A (or B or C)
# 200 OK response
```

```bash
# Test 2: Repeat requests with cookie
curl -v -b "backend_assignment=A" http://localhost/api/test

# Should go to same backend (sticky)
```

```bash
# Test 3: Exceed rate limit (10 req/sec)
for i in {1..15}; do
  curl -s -w "Request $i: %{http_code}\n" http://localhost/api/test
done

# You should see:
# Requests 1-10: 200 OK
# Requests 11-15: 429 Too Many Requests
```

## Step 4: Check Logs

```bash
# View logs for each backend
cat logs/cookie-A.log
cat logs/cookie-B.log
cat logs/cookie-C.log

# Follow logs in real-time
tail -f logs/cookie-A.log
```

## Step 5: Monitor Services

```bash
# Check service health
curl http://localhost:8080/health | jq
curl http://localhost:8081/health | jq

# Check Redis
docker exec research-redis redis-cli
# Type: KEYS *
# Type: exit
```

## Stopping Services

```bash
# Stop and remove containers
docker-compose down

# Keep volumes (data)
docker-compose down -v
```

## Common Issues

### Services won't start
```bash
docker-compose logs rate-limiter
docker-compose logs load-balancer
docker-compose down && docker-compose up --build
```

### Port already in use
```bash
# Change published ports in docker-compose.yml
# Or kill process using port:
sudo lsof -i :80   # Check port 80
kill -9 <PID>      # Kill process
```

### Logs directory permissions
```bash
sudo mkdir -p logs
sudo chmod 777 logs
docker-compose up --build
```

## Next Steps

1. **Read README.md** - Understand architecture
2. **Read DEVELOPMENT.md** - Customize for your needs
3. **Run test.sh** - Run comprehensive tests
4. **Adjust parameters** - Tune for your use case

## Configuration Changes (Without Rebuilding)

### Change global rate limit
Edit `nginx/nginx.conf`:
```nginx
rate=10r/s;  # Change this number
```
Then restart:
```bash
docker-compose restart nginx
```

### Change per-cookie parameters
Edit `rate-limiter/main.go`:
```go
config := Config{
    MaxTokens:  100,   // Change
    RefillRate: 10,    // Change
}
```
Then rebuild:
```bash
docker-compose down && docker-compose up --build
```

## Key Endpoints

| Service | Port | Endpoint | Purpose |
|---------|------|----------|---------|
| Nginx | 80 | / | Entry point |
| Rate Limiter | 8080 | /health | Health check |
| Load Balancer | 8081 | /health | Health + load info |
| Redis | 6379 | - | Data storage |

## Example Requests

**Basic request**:
```bash
curl http://localhost/api/endpoint
```

**With specific cookie**:
```bash
curl -b "backend_assignment=A" http://localhost/api/endpoint
```

**With custom headers**:
```bash
curl -H "X-User-ID: user123" http://localhost/api/endpoint
```

**Verbose output**:
```bash
curl -v http://localhost/api/endpoint
```

**Check response headers**:
```bash
curl -i http://localhost/api/endpoint
```

## Performance Testing

```bash
# Light load (100 requests, 1 concurrent)
ab -n 100 -c 1 http://localhost/api/test

# Medium load (1000 requests, 10 concurrent)
ab -n 1000 -c 10 http://localhost/api/test

# Heavy load (10000 requests, 50 concurrent)
ab -n 10000 -c 50 http://localhost/api/test
```

## Reset Everything

```bash
# Stop services
docker-compose down -v

# Remove logs
rm -rf logs/

# Clean Docker
docker system prune

# Start fresh
docker-compose up --build
```

---

**Help**: Check README.md and DEVELOPMENT.md for detailed documentation.
