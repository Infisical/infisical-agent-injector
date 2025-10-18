FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build for target platform
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -o infisical-agent-injector$([ "$TARGETOS" = "windows" ] && echo ".exe" || echo "")

# Linux final stage
FROM alpine:latest AS linux
RUN apk add --no-cache tini
WORKDIR /app
COPY --from=builder /app/infisical-agent-injector /app/
ENTRYPOINT ["/sbin/tini", "--", "/app/infisical-agent-injector"]


# Windows final stage  
FROM mcr.microsoft.com/powershell:nanoserver-ltsc2022 AS windows
WORKDIR /app
COPY --from=builder /app/infisical-agent-injector.exe /app/
ENTRYPOINT ["C:\\app\\infisical-agent-injector.exe"]

# Select final stage based on OS
FROM ${TARGETOS} AS final