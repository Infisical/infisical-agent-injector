# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o infisical-agent-injector

# Final stage
FROM alpine:latest

# Install tini
RUN apk add --no-cache tini

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/infisical-agent-injector /app/

# Use tini as init process
ENTRYPOINT ["/sbin/tini", "--", "/app/infisical-agent-injector"]