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

# Windows 2022 stage
FROM mcr.microsoft.com/powershell:nanoserver-ltsc2022 AS windows2022
WORKDIR /app
COPY --from=builder /app/infisical-agent-injector.exe /app/
ENTRYPOINT ["C:\\app\\infisical-agent-injector.exe"]

# Windows 2019 stage
FROM mcr.microsoft.com/powershell:nanoserver-1809 AS windows2019
WORKDIR /app
COPY --from=builder /app/infisical-agent-injector.exe /app/
ENTRYPOINT ["C:\\app\\infisical-agent-injector.exe"]
