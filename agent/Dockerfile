# Stage 1: Build
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o agent-mock .

# Stage 2: Runtime
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/agent-mock .

CMD ["./agent-mock"]
