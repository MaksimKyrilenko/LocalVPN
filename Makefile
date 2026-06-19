.PHONY: all build server client clean test install

# Variables
SERVER_DIR=server
CLIENT_DIR=client
DEPLOY_DIR=deploy

# Default target
all: server client

# Build both
build: server client

# Build server
server:
	@echo "Building server..."
	cd $(SERVER_DIR) && go build -o bin/meshvpn-server cmd/server/main.go

# Build client CLI
client-cli:
	@echo "Building client CLI..."
	cd $(CLIENT_DIR)/cmd/cli && go build -o ../../../bin/meshvpn-cli main.go

# Build client GUI (Windows only)
client-gui:
	@echo "Building client GUI..."
	cd $(CLIENT_DIR) && wails build -o bin/MeshVPN.exe

# Build all clients
client: client-cli
ifeq ($(OS),Windows_NT)
	$(MAKE) client-gui
endif

# Run server locally
run-server:
	cd $(SERVER_DIR) && go run cmd/server/main.go -debug

# Run client CLI locally
run-cli:
	cd $(CLIENT_DIR)/cmd/cli && go run main.go

# Run tests
test:
	cd $(SERVER_DIR) && go test ./...
	cd $(CLIENT_DIR) && go test ./...

# Clean build artifacts
clean:
	rm -rf $(SERVER_DIR)/bin
	rm -rf $(CLIENT_DIR)/bin
	rm -rf bin/
	docker-compose -f $(DEPLOY_DIR)/docker-compose.yml down

# Install dependencies
deps:
	cd $(SERVER_DIR) && go mod download
	cd $(SERVER_DIR) && go mod tidy
	cd $(CLIENT_DIR) && go mod download
	cd $(CLIENT_DIR) && go mod tidy

# Docker build
docker-build:
	docker build -t meshvpn-server -f $(DEPLOY_DIR)/Dockerfile .

# Docker run
docker-run:
	cd $(DEPLOY_DIR) && docker-compose up -d

# Docker stop
docker-stop:
	cd $(DEPLOY_DIR) && docker-compose down

# Docker logs
docker-logs:
	cd $(DEPLOY_DIR) && docker-compose logs -f

# Quick install on VPS
install-remote:
	@echo "Usage: make install-remote HOST=user@server-ip"
	ssh $(HOST) 'bash -c "$(curl -fsSL https://raw.githubusercontent.com/yourusername/meshvpn/main/deploy/scripts/install.sh)"'

# Release build
release:
	@echo "Building release binaries..."
	mkdir -p release
	
	# Server
	cd $(SERVER_DIR) && GOOS=linux GOARCH=amd64 go build -ldflags "-w -s" -o ../release/meshvpn-server-linux-amd64 cmd/server/main.go
	
	# Client CLI
	cd $(CLIENT_DIR)/cmd/cli && GOOS=linux GOARCH=amd64 go build -ldflags "-w -s" -o ../../../release/meshvpn-cli-linux-amd64 main.go
	cd $(CLIENT_DIR)/cmd/cli && GOOS=windows GOARCH=amd64 go build -ldflags "-w -s" -o ../../../release/meshvpn-cli-windows-amd64.exe main.go
	
	@echo "Release binaries in ./release/"

# Help
help:
	@echo "MeshVPN Build System"
	@echo ""
	@echo "Targets:"
	@echo "  make server       - Build server binary"
	@echo "  make client-cli   - Build CLI client"
	@echo "  make client-gui   - Build GUI client (Windows)"
	@echo "  make run-server   - Run server locally"
	@echo "  make test         - Run tests"
	@echo "  make deps         - Install dependencies"
	@echo "  make docker-build - Build Docker image"
	@echo "  make docker-run   - Run with Docker Compose"
	@echo "  make release      - Build release binaries"
	@echo "  make clean        - Clean build artifacts"
