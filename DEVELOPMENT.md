# Development Guide

Fast local development workflow using a local Docker registry.

## Quick Start

```bash
make up-dev
```

That's it! This will:
1. Create and start a local Docker registry (if not already running)
  a. This only happens once if you don't already have a local Docker registry.
2. Build the Agent Injector Docker image
3. Push the Agent Injector Docker image to your local Docker registry
4. Install the Agent Injector helm chart with the newly built Docker image

## How It Works

### Local Docker Registry

The Makefile spins up a local Docker registry container on port `8443`. This registry:
- Persists across runs (uses `--restart=always`)
- Is accessible from your cluster as `localhost:8443`
- Requires no authentication

### Fast Build Loop

Each time you run `make up-dev`:
1. A unique image name is generated (e.g., `agent-injector-a1b2c3d4`)
2. Your code is built into a Docker image
3. The image is pushed to your local registry
4. Helm upgrades the deployment with the new image

## Development Workflow

### Making Changes

```bash
# 1. Make your code changes
vim main.go

# 2. Test them
make up-dev

# 3. Watch the logs
kubectl logs -f deployment/infisical-agent-injector

# 4. Test with a pod
kubectl apply -f test/linux/linux-pod.yaml
```

### Building for Windows

To test Windows builds:

```bash
# Edit the Makefile variables
PLATFORM := windows/amd64
BUILD_TARGET := windows2022  # or windows2019

# Then build
make up-dev
```

Or temporarily override:

```bash
make up-dev PLATFORM=windows/amd64 BUILD_TARGET=windows2022
```

### Check what's running

```bash
# Get the current image
kubectl get deployment agent-injector -o jsonpath='{.spec.template.spec.containers[0].image}'

# Check pod status
kubectl get pods -l app.kubernetes.io/name=infisical-agent-injector

# View logs
kubectl logs -l app.kubernetes.io/name=infisical-agent-injector --tail=50
```

## Cleanup

When you run `make up-dev`, any previously created dev installations are automatically removed. However if you need to manually uninstall the agent injector you can do so by running `make uninstall`.

If you want to completely purge your development setup for the agent injector, you can run `make clean`