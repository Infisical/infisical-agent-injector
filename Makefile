.PHONY: all registry build push install clean uninstall

# Variables
REGISTRY_NAME := local-registry
REGISTRY_PORT := 8447
RANDOM_STRING := $(shell date +%s | sha256sum | base64 | head -c 8 | tr '[:upper:]' '[:lower:]')
IMAGE_NAME := agent-injector-$(RANDOM_STRING)
LOCAL_IMAGE := localhost:$(REGISTRY_PORT)/$(IMAGE_NAME)
CHART_DIR := ./helm
RELEASE_NAME := agent-injector

# ? Note(Daniel): If need be, you can change this to windows/amd64 to build the windows image.
PLATFORM := linux/amd64 # linux/amd64|windows/amd64
BUILD_TARGET := linux # linux|windows2019|windows2022


# Default target
run-dev: uninstall registry build push install

# Create local docker registry if it doesn't exist
registry:
	@echo "Checking for local registry..."
	@if [ ! "$$(docker ps -q -f name=$(REGISTRY_NAME))" ]; then \
		if [ "$$(docker ps -aq -f status=exited -f name=$(REGISTRY_NAME))" ]; then \
			echo "Starting existing registry container..."; \
			docker start $(REGISTRY_NAME); \
		else \
			echo "Creating new local registry on port $(REGISTRY_PORT)..."; \
			docker run -d \
				--name $(REGISTRY_NAME) \
				--restart=always \
				-p $(REGISTRY_PORT):5000 \
				registry:2; \
		fi \
	else \
		echo "Registry already running"; \
	fi

# Build the docker image
build:
	@echo "Building Docker image: $(IMAGE_NAME) for $(PLATFORM)..."
	docker build --target $(BUILD_TARGET) --platform $(PLATFORM) -t $(IMAGE_NAME):latest -f Dockerfile . 
	docker tag $(IMAGE_NAME):latest $(LOCAL_IMAGE):latest

# Push image to local registry
push:
	@echo "Pushing image to local registry..."
	docker push $(LOCAL_IMAGE):latest

# Install helm chart directly from directory
install:
	@echo "Installing Helm chart from $(CHART_DIR)..."
	helm upgrade --install $(RELEASE_NAME) $(CHART_DIR) \
		--set image.repository=localhost:$(REGISTRY_PORT)/$(IMAGE_NAME) \
		--set image.tag=latest \
		--wait

# Uninstall the helm release
uninstall:
	@echo "Uninstalling Helm release $(RELEASE_NAME)..."
	helm uninstall $(RELEASE_NAME) || true

# Clean up everything including the local registry
clean: uninstall
	@echo "Stopping and removing local registry..."
	docker stop $(REGISTRY_NAME) || true
	docker rm $(REGISTRY_NAME) || true
	@echo "Cleanup complete"