FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o demo-backend .

FROM alpine:3.21
RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /app/demo-backend /usr/local/bin/demo-backend
USER app
EXPOSE 9001
ENTRYPOINT ["/usr/local/bin/demo-backend"]
