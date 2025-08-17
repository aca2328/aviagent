# VMware Avi LLM Agent Makefile

# Variables
APP_NAME := vmware-avi-llm-agent
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=${BUILD_DATE} -w -s"

# Go variables
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
GO_VERSION := $(shell go version | cut -d ' ' -f 3)

# Docker variables
REGISTRY ?= localhost
IMAGE_NAME := ${REGISTRY}/${APP_NAME}
DOCKER_TAG ?= ${VERSION}

# Directories
BUILD_DIR := build
BIN_DIR := ${BUILD_DIR}/bin
COVERAGE_DIR := ${BUILD_DIR}/coverage

.PHONY: help
help: ## Display this help screen
	@echo "VMware Avi LLM Agent - Makefile Help"
	@echo "===================================="
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: clean
clean: ## Clean build artifacts
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf ${BUILD_DIR}
	@go clean -cache
	@go clean -testcache
	@docker system prune -f --filter "label=app=${APP_NAME}" 2>/dev/null || true

.PHONY: deps
deps: ## Download dependencies
	@echo "📦 Downloading dependencies..."
	@go mod download
	@go mod tidy

.PHONY: deps-update
deps-update: ## Update dependencies
	@echo "🔄 Updating dependencies..."
	@go get -u ./...
	@go mod tidy

.PHONY: fmt
fmt: ## Format Go code
	@echo "🎨 Formatting Go code..."
	@go fmt ./...
	@goimports -w . 2>/dev/null || echo "goimports not found, skipping..."

.PHONY: lint
lint: ## Lint Go code
	@echo "🔍 Linting Go code..."
	@golangci-lint run ./... || echo "golangci-lint not found, install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"

.PHONY: vet
vet: ## Vet Go code
	@echo "🩺 Vetting Go code..."
	@go vet ./...

.PHONY: security
security: ## Run security checks
	@echo "🔒 Running security checks..."
	@gosec ./... || echo "gosec not found, install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"

.PHONY: test
test: ## Run tests
	@echo "🧪 Running tests..."
	@mkdir -p ${COVERAGE_DIR}
	@go test -v -race -coverprofile=${COVERAGE_DIR}/coverage.out ./...

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo "🧪 Running integration tests..."
	@go test -v -tags=integration ./tests/integration/...

.PHONY: test-coverage
test-coverage: test ## Generate test coverage report
	@echo "📊 Generating coverage report..."
	@go tool cover -html=${COVERAGE_DIR}/coverage.out -o ${COVERAGE_DIR}/coverage.html
	@go tool cover -func=${COVERAGE_DIR}/coverage.out | tail -1
	@echo "Coverage report generated: ${COVERAGE_DIR}/coverage.html"

.PHONY: benchmark
benchmark: ## Run benchmarks
	@echo "⚡ Running benchmarks..."
	@go test -bench=. -benchmem ./...

.PHONY: build
build: deps ## Build the application
	@echo "🔨 Building ${APP_NAME}..."
	@mkdir -p ${BIN_DIR}
	@CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build ${LDFLAGS} -o ${BIN_DIR}/${APP_NAME} ./cmd/server

.PHONY: build-all
build-all: deps ## Build for all platforms
	@echo "🔨 Building for all platforms..."
	@mkdir -p ${BIN_DIR}
	@for os in linux windows darwin; do \
		for arch in amd64 arm64; do \
			if [ "$$os" = "windows" ]; then \
				ext=".exe"; \
			else \
				ext=""; \
			fi; \
			echo "Building $$os/$$arch..."; \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build ${LDFLAGS} -o ${BIN_DIR}/${APP_NAME}-$$os-$$arch$$ext ./cmd/server; \
		done \
	done

.PHONY: run
run: build ## Build and run the application
	@echo "🚀 Starting ${APP_NAME}..."
	@./${BIN_DIR}/${APP_NAME} -config config.yaml

.PHONY: run-dev
run-dev: ## Run in development mode
	@echo "🚀 Starting ${APP_NAME} in development mode..."
	@go run ./cmd/server -config config.yaml

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	@docker build -t ${IMAGE_NAME}:${DOCKER_TAG} -t ${IMAGE_NAME}:latest .
	@echo "Built: ${IMAGE_NAME}:${DOCKER_TAG}"

.PHONY: docker-run
docker-run: docker-build ## Build and run Docker container
	@echo "🐳 Running Docker container..."
	@docker run --rm -p 8080:8080 \
		-e AVI_HOST=${AVI_HOST} \
		-e AVI_USERNAME=${AVI_USERNAME} \
		-e AVI_PASSWORD=${AVI_PASSWORD} \
		-e OLLAMA_HOST=${OLLAMA_HOST} \
		${IMAGE_NAME}:${DOCKER_TAG}

.PHONY: docker-push
docker-push: docker-build ## Push Docker image to registry
	@echo "🐳 Pushing Docker image to registry..."
	@docker push ${IMAGE_NAME}:${DOCKER_TAG}
	@docker push ${IMAGE_NAME}:latest

.PHONY: docker-compose-up
docker-compose-up: ## Start all services with Docker Compose
	@echo "🐳 Starting services with Docker Compose..."
	@docker-compose up -d

.PHONY: docker-compose-down
docker-compose-down: ## Stop all services with Docker Compose
	@echo "🐳 Stopping services with Docker Compose..."
	@docker-compose down

.PHONY: docker-compose-logs
docker-compose-logs: ## View Docker Compose logs
	@docker-compose logs -f

.PHONY: setup-dev
setup-dev: ## Setup development environment
	@echo "🛠 Setting up development environment..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	@echo "✅ Development environment setup complete!"

.PHONY: pre-commit
pre-commit: fmt lint vet security test ## Run all pre-commit checks
	@echo "✅ All pre-commit checks passed!"

.PHONY: release-check
release-check: clean deps pre-commit build docker-build ## Run all release checks
	@echo "✅ All release checks passed!"

.PHONY: install
install: build ## Install the binary
	@echo "📦 Installing ${APP_NAME}..."
	@sudo cp ${BIN_DIR}/${APP_NAME} /usr/local/bin/
	@echo "✅ Installed to /usr/local/bin/${APP_NAME}"

.PHONY: uninstall
uninstall: ## Uninstall the binary
	@echo "🗑 Uninstalling ${APP_NAME}..."
	@sudo rm -f /usr/local/bin/${APP_NAME}
	@echo "✅ Uninstalled from /usr/local/bin/${APP_NAME}"

.PHONY: version
version: ## Show version information
	@echo "App Name:     ${APP_NAME}"
	@echo "Version:      ${VERSION}"
	@echo "Commit:       ${COMMIT}"
	@echo "Build Date:   ${BUILD_DATE}"
	@echo "Go Version:   ${GO_VERSION}"
	@echo "OS/Arch:      ${GOOS}/${GOARCH}"

.PHONY: health-check
health-check: ## Check if the application is running
	@echo "🔍 Checking application health..."
	@curl -f http://localhost:8080/api/health || echo "❌ Application not responding"

.PHONY: demo
demo: ## Run a demo environment
	@echo "🎬 Starting demo environment..."
	@docker-compose -f docker-compose.yml -f docker-compose.demo.yml up -d
	@echo "✅ Demo environment started!"
	@echo "🌐 Open http://localhost:8080 in your browser"
	@echo "📊 Monitoring available at http://localhost:3000 (admin/admin)"

.PHONY: docs-serve
docs-serve: ## Serve documentation locally
	@echo "📚 Serving documentation..."
	@python3 -m http.server 8000 --directory docs/ || echo "Python3 not found, install to serve docs"

.PHONY: generate
generate: ## Generate code (if using code generation)
	@echo "🔄 Generating code..."
	@go generate ./...

# Release targets
.PHONY: tag
tag: ## Create a new git tag (usage: make tag VERSION=v1.0.0)
	@if [ -z "${VERSION}" ]; then echo "Usage: make tag VERSION=v1.0.0"; exit 1; fi
	@git tag -a ${VERSION} -m "Release ${VERSION}"
	@git push origin ${VERSION}
	@echo "✅ Tagged and pushed ${VERSION}"

.PHONY: changelog
changelog: ## Generate changelog
	@echo "📝 Generating changelog..."
	@git log --oneline --decorate --graph --all > CHANGELOG.txt
	@echo "✅ Changelog generated in CHANGELOG.txt"

# Default target
.DEFAULT_GOAL := help