# Stage 1: Build
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o portal-api .

# Stage 2: Runtime
FROM alpine:3.19
RUN adduser -D -s /bin/false appuser && \
    apk add --no-cache curl
WORKDIR /app
COPY --from=builder /app/portal-api .
RUN chown -R appuser:appuser /app
USER appuser

EXPOSE 5000 9999

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:5000/health || exit 1

CMD ["./portal-api"]
