# VMware Avi Load Balancer LLM Agent

A Go-based web agent that allows users to interact with the VMware Avi Load Balancer API via natural language, powered by an Ollama-hosted LLM. The solution is containerized in a single Docker image with a multi-stage build, using Gin for the web server and HTMX for the chat UI.

## Features

### 🚀 Core Functionality
- **Natural Language Interface**: Chat with your Avi Load Balancer using plain English
- **Complete API Coverage**: All VMware ALB SDK endpoints implemented
- **LLM Integration**: Powered by Ollama with support for multiple models (llama3, mistral, codellama, etc.)
- **Real-time Chat UI**: Modern web interface built with HTMX and Bootstrap
- **Multi-turn Conversations**: Maintains context for follow-up questions
- **Tool Definitions**: Comprehensive function definitions for accurate API mapping

### 🛠 Technical Features
- **Go Backend**: High-performance web server using Gin framework
- **Docker Containerized**: Multi-stage build for optimized production images
- **Authentication**: Secure session management with VMware Avi controllers
- **Error Handling**: Robust error handling and user-friendly error messages
- **Health Monitoring**: Built-in health checks and status monitoring
- **Logging**: Structured logging with configurable levels

### 📊 Supported Operations
- **Virtual Services**: List, create, update, delete, scale, migrate, switchover
- **Pools**: Manage backend pools, scale out/in, health monitoring
- **Service Engines**: Monitor status, performance, and configuration
- **Health Monitors**: Configure and manage health checks
- **Analytics**: Retrieve performance metrics and monitoring data

## 🚀 Quick Start

### 📋 Prerequisites
- **Docker** (v20.10+) and **Docker Compose** (v1.29+)
- Access to a **VMware Avi Load Balancer controller** (v21.1+ recommended)
- **Ollama** service (local or remote) for LLM processing
- Minimum **4GB RAM** and **2 CPU cores** for development

### 💻 System Requirements
| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 2 cores | 4+ cores |
| RAM | 4GB | 8GB+ |
| Disk | 10GB | 20GB+ (for LLM models) |
| Docker | v20.10+ | Latest stable |

### 1️⃣ Clone the Repository
```bash
# Clone the repository
git clone https://github.com/your-org/aviagent.git
cd aviagent

# Check out the latest stable version
git checkout v1.0.0
```

### 2️⃣ Configure Environment
Create a `.env` file with your configuration:
```bash
cp .env.example .env
nano .env
```

Example `.env` file:
```env
# Avi Load Balancer Configuration
AVI_HOST=avi-controller.example.com
AVI_USERNAME=admin
AVI_PASSWORD=your-secure-password
AVI_VERSION=31.2.1
AVI_TENANT=admin

# Ollama Configuration
OLLAMA_HOST=http://localhost:11434

# Application Configuration
LOG_LEVEL=info
SERVER_PORT=8080

# Security Settings
AVI_INSECURE=false  # Set to true only for testing with self-signed certs
```

**⚠️ Security Note:** Never commit your `.env` file with sensitive credentials!

### 3️⃣ Start Services
```bash
# Start core services (Avi Agent + Ollama)
docker-compose up -d

# Start with monitoring stack (Prometheus + Grafana)
docker-compose --profile monitoring up -d

# View service status
docker-compose ps
```

### 4️⃣ Pull LLM Models
```bash
# Pull required LLM models (this may take several minutes)
docker-compose exec ollama ollama pull llama3.2
docker-compose exec ollama ollama pull mistral
docker-compose exec ollama ollama pull codellama

# Verify models are available
docker-compose exec ollama ollama list
```

### 5️⃣ Access the Application
- **Web Interface**: `http://localhost:8080`
- **API Documentation**: `http://localhost:8080/api/docs`
- **Health Check**: `http://localhost:8080/api/health`
- **Monitoring** (if enabled): `http://localhost:3000` (Grafana)

### 6️⃣ Verify Installation
```bash
# Check application health
curl -s http://localhost:8080/api/health | jq .

# Check Avi controller connection
curl -s http://localhost:8080/api/health | jq .avi_status

# Check LLM service connection  
curl -s http://localhost:8080/api/health | jq .llm_status
```

## 📦 Installation Options

### Option A: Docker with Ollama (Recommended for Development)
```bash
# Build and run with Docker using Ollama
docker build -t aviagent:latest .
docker run -d -p 8080:8080 \
  -e AVI_HOST=your-avi-controller \
  -e AVI_USERNAME=admin \
  -e AVI_PASSWORD=your-password \
  -e LLM_PROVIDER=ollama \
  -e OLLAMA_HOST=http://host.docker.internal:11434 \
  --name aviagent \
  aviagent:latest
```

### Option B: Docker with Mistral AI (Recommended for Production)
```bash
# Build and run with Docker using Mistral AI
docker build -t aviagent:latest .
docker run -d -p 8080:8080 \
  -e AVI_HOST=your-avi-controller \
  -e AVI_USERNAME=admin \
  -e AVI_PASSWORD=your-password \
  -e LLM_PROVIDER=mistral \
  -e MISTRAL_API_KEY=your-mistral-api-key \
  --name aviagent \
  aviagent:latest
```

### Option B: Binary Installation
```bash
# Download pre-built binary (replace version as needed)
wget https://github.com/your-org/aviagent/releases/download/v1.0.0/aviagent-linux-amd64
chmod +x aviagent-linux-amd64
mv aviagent-linux-amd64 /usr/local/bin/aviagent

# Create configuration file
nano /etc/aviagent/config.yaml

# Start the service
aviagent -config /etc/aviagent/config.yaml
```

### Option C: Docker Compose (Full Stack)

#### Using Ollama (Development)
```bash
# Start with Ollama (includes Ollama service)
docker-compose up -d

# Or explicitly specify Ollama
docker-compose up -d -e LLM_PROVIDER=ollama
```

#### Using Mistral AI (Production)
```bash
# Start with Mistral AI (no Ollama service needed)
docker-compose up -d -e LLM_PROVIDER=mistral -e MISTRAL_API_KEY=your-api-key

# Scale the application
docker-compose up -d --scale avi-llm-agent=2 -e LLM_PROVIDER=mistral -e MISTRAL_API_KEY=your-api-key
```

### Option D: From Source
```bash
# Install Go 1.21+
sudo apt-get install golang-go

# Build from source
go mod download
go build -o aviagent ./cmd/server

# Run the application
./aviagent -config config.yaml
```

## 🎯 Usage Guide

### 🌐 Web Interface
1. **Login**: Access `http://localhost:8080` in your browser
2. **Model Selection**: Choose your preferred LLM model from the dropdown
3. **Quick Actions**: Use predefined queries from the sidebar
4. **Natural Language**: Type your questions in the chat input

### 💬 Example Queries

#### Basic Information
```
"What are the current virtual services?"
"Show me all pools with their health status"
"List service engines that are down"
"Get analytics for the last hour"
```

#### Management Operations
```
"Create a new virtual service for my web application"
"Scale out the backend pool for app1 to 5 servers"
"Add server 10.1.1.100 to the web-pool"
"Enable SSL for the api-virtualservice"
```

#### Monitoring and Troubleshooting
```
"Show me performance metrics for vs-web"
"Which pools have unhealthy servers?"
"Get connection statistics for the last 6 hours"
"Show me service engines with high CPU usage"
```

### 🔧 Advanced Usage

#### Direct API Access
```bash
# List virtual services via API
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "List all virtual services", "model": "llama3.2"}'

# Get specific virtual service details
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Show details for virtual service vs-web-01", "model": "mistral"}'
```

#### Avi API Proxy
```bash
# Direct Avi API access (for advanced users)
curl -X GET "http://localhost:8080/api/avi/virtualservice?limit_by=10" \
  -H "Authorization: Bearer your-token"

# Create virtual service via proxy
curl -X POST "http://localhost:8080/api/avi/virtualservice" \
  -H "Content-Type: application/json" \
  -d '{"name": "test-vs", "services": [{"port": 80}]}'
```

## 🛠 Configuration

### Configuration File
Create a `config.yaml` file:

```yaml
# Server Configuration
server:
  port: 8080
  read_timeout: 30
  write_timeout: 30
  idle_timeout: 60

# Avi Load Balancer Configuration
avi:
  host: "avi-controller.example.com"
  username: "admin"
  password: "your-secure-password"
  version: "31.2.1"
  tenant: "admin"
  timeout: 30
  insecure: false  # Set to true only for testing

# LLM Provider Configuration (choose one)
provider: "ollama"  # or "mistral"

# Ollama Configuration (used when provider: "ollama")
llm:
  ollama_host: "http://localhost:11434"
  default_model: "llama3.2"
  models:
    - "llama3.2"
    - "mistral"
    - "codellama"
  timeout: 60
  temperature: 0.7
  max_tokens: 2048

# Mistral AI Configuration (used when provider: "mistral")
mistral:
  api_base_url: "https://api.mistral.ai"
  api_key: "your-mistral-api-key"
  default_model: "mistral-tiny"
  models:
    - "mistral-tiny"
    - "mistral-small"
    - "mistral-medium"
    - "mistral-large"
  timeout: 60
  temperature: 0.7
  max_tokens: 2048

# Logging Configuration
log:
  level: "info"
  format: "json"
```

### Environment Variables
The application supports environment variables for configuration:

```bash
# Avi Load Balancer
export AVI_HOST="avi-controller.example.com"
export AVI_USERNAME="admin"
export AVI_PASSWORD="your-password"

# LLM Provider (choose one)
export LLM_PROVIDER="ollama"  # or "mistral"

# Ollama Configuration
export OLLAMA_HOST="http://localhost:11434"

# Mistral AI Configuration
export MISTRAL_API_KEY="your-mistral-api-key"

# Application Settings
export LOG_LEVEL="debug"
export GIN_MODE="release"
export SERVER_PORT=8080
```

### LLM Provider Selection

The application supports two LLM providers:

#### 🦙 Ollama (Default)
- **Local/self-hosted** LLM service
- **Open-source models** (Llama3, Mistral, CodeLlama, etc.)
- **No API costs** - run models locally
- **Best for**: Development, testing, private deployments

#### 🤖 Mistral AI
- **Cloud-based** LLM service
- **Managed API** with enterprise-grade models
- **Pay-as-you-go pricing**
- **Best for**: Production, scalability, enterprise use

#### Comparison

| Feature | Ollama | Mistral AI |
|---------|--------|------------|
| **Hosting** | Self-hosted | Cloud-managed |
| **Cost** | Free (local) | Pay-as-you-go |
| **Models** | Open-source | Proprietary + Open-source |
| **Setup** | Requires local setup | API key only |
| **Scalability** | Limited by hardware | Highly scalable |
| **Latency** | Low (local) | Higher (network) |
| **Privacy** | Full control | Cloud-based |

### Configuration Priority
1. **Environment Variables** (highest priority)
2. **Configuration File** (`config.yaml`)
3. **Default Values** (lowest priority)

## 🐳 Docker Administration

### Provider Switching

#### Switch from Ollama to Mistral AI
```bash
# Stop current services
docker-compose down

# Start with Mistral AI
docker-compose up -d -e LLM_PROVIDER=mistral -e MISTRAL_API_KEY=your-api-key

# Verify the switch
curl http://localhost:8080/api/health | jq .provider
```

#### Switch from Mistral AI to Ollama
```bash
# Stop current services
docker-compose down

# Start with Ollama
docker-compose up -d -e LLM_PROVIDER=ollama

# Pull required models
docker-compose exec ollama ollama pull llama3.2

# Verify the switch
curl http://localhost:8080/api/health | jq .provider
```

### Environment Variable Management

#### Using .env file
```bash
# Create .env file for Mistral AI
cat > .env << EOF
LLM_PROVIDER=mistral
MISTRAL_API_KEY=your-api-key
AVI_HOST=your-avi-controller
AVI_USERNAME=admin
AVI_PASSWORD=your-password
EOF

# Start services
docker-compose --env-file .env up -d
```

#### Using command line
```bash
# Start with environment variables
docker-compose up -d \\
  -e LLM_PROVIDER=mistral \\
  -e MISTRAL_API_KEY=your-api-key \\
  -e AVI_HOST=your-avi-controller
```

### Resource Management

#### Memory and CPU Limits
```bash
# Limit resources for Avi Agent
docker-compose up -d \\
  --compatibility \\
  -e LLM_PROVIDER=ollama \\
  -e AVI_HOST=your-avi-controller

# Or use docker run with limits
docker run -d \\
  --memory=2g \\
  --cpus=2 \\
  -p 8080:8080 \\
  -e LLM_PROVIDER=mistral \\
  -e MISTRAL_API_KEY=your-api-key \\
  aviagent:latest
```

## 🔧 Administration

### Model Management

#### Ollama Models
```bash
# List available Ollama models
curl http://localhost:8080/api/models

# Validate a specific Ollama model
curl -X POST http://localhost:8080/api/models/validate \
  -H "Content-Type: application/json" \
  -d '{"model": "llama3.2"}'

# Add a new model to Ollama
docker-compose exec ollama ollama pull new-model-name

# List Ollama models directly
docker-compose exec ollama ollama list
```

#### Mistral AI Models
```bash
# List available Mistral AI models
curl http://localhost:8080/api/models

# Validate a specific Mistral AI model
curl -X POST http://localhost:8080/api/models/validate \
  -H "Content-Type: application/json" \
  -d '{"model": "mistral-small"}'

# Get Mistral AI model information
curl https://api.mistral.ai/v1/models \
  -H "Authorization: Bearer $MISTRAL_API_KEY"
```

### Session Management
```bash
# Get chat history
curl http://localhost:8080/api/chat/history

# Clear chat history
curl -X DELETE http://localhost:8080/api/chat/history
```

### Health Monitoring
```bash
# Check application health
curl http://localhost:8080/api/health

# Check specific component health
curl http://localhost:8080/api/health?component=avi

# Get detailed status
curl -s http://localhost:8080/api/health | jq .
```

## 📊 Monitoring and Observability

### Built-in Monitoring
- **Health Endpoint**: `/api/health`
- **Metrics Endpoint**: `/api/metrics` (if enabled)
- **Logging**: Structured JSON logging to stdout

### Prometheus Integration
```yaml
# Add to your prometheus.yml
scrape_configs:
  - job_name: 'aviagent'
    scrape_interval: 15s
    static_configs:
      - targets: ['aviagent:8080']
```

### Grafana Dashboards
Import the provided Grafana dashboard:
```bash
# Import dashboard
curl -X POST http://localhost:3000/api/dashboards/import \
  -H "Authorization: Bearer your-grafana-api-key" \
  -H "Content-Type: application/json" \
  -d '{"dashboard": "$(cat monitoring/grafana/dashboard.json)", "overwrite": true}'
```

## 🐳 Docker Management

### Common Docker Commands
```bash
# View logs
docker-compose logs -f avi-llm-agent

# Restart services
docker-compose restart

# Scale services
docker-compose up -d --scale avi-llm-agent=2

# Update services
docker-compose pull
docker-compose up -d --build

# Cleanup
docker-compose down -v
```

### Docker Health Checks
```bash
# Check container health
docker inspect --format='{{json .State.Health}}' aviagent-avi-llm-agent-1

# View health status
docker ps --format "table {{.ID}}\t{{.Names}}\t{{.Status}}"
```

## 🔄 Upgrading

### Upgrade Procedure
```bash
# 1. Backup your configuration
cp config.yaml config.yaml.backup
cp .env .env.backup

# 2. Pull the latest code
git pull origin main
git checkout v1.1.0  # Check out specific version

# 3. Update dependencies
docker-compose pull

# 4. Rebuild and restart
docker-compose up -d --build

# 5. Verify upgrade
docker-compose logs -f --tail=50
curl http://localhost:8080/api/health
```

### Version Compatibility
| Avi Agent Version | Avi Controller Version | Ollama Version |
|-------------------|-----------------------|----------------|
| v1.0.x | 21.1 - 31.2.x | 0.1.0+ |
| v1.1.x | 22.1+ | 0.1.20+ |

## 🛑 Troubleshooting

### Common Issues

#### Connection to Avi Controller
```bash
# Test Avi controller connectivity
curl -k https://your-avi-controller.com/login

# Check SSL certificates
openssl s_client -connect your-avi-controller.com:443 -showcerts

# Verify credentials
export AVI_HOST=your-avi-controller.com
export AVI_USERNAME=admin
export AVI_PASSWORD=your-password
curl -u "$AVI_USERNAME:$AVI_PASSWORD" -k https://$AVI_HOST/login
```

#### Ollama Model Issues
```bash
# List available Ollama models
docker-compose exec ollama ollama list

# Pull missing Ollama models
docker-compose exec ollama ollama pull llama3.2

# Check Ollama logs
docker-compose logs ollama

# Restart Ollama service
docker-compose restart ollama

# Check Ollama resource usage
docker stats ollama
```

#### Mistral AI Connection Issues
```bash
# Test Mistral AI API connectivity
curl https://api.mistral.ai/v1/models \
  -H "Authorization: Bearer $MISTRAL_API_KEY"

# Check API key validity
curl https://api.mistral.ai/v1/models \
  -H "Authorization: Bearer $MISTRAL_API_KEY" -v

# Test with different model
curl -X POST https://api.mistral.ai/v1/chat/completions \
  -H "Authorization: Bearer $MISTRAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "mistral-tiny", "messages": [{"role": "user", "content": "Hello"}]}'

# Check rate limits
curl https://api.mistral.ai/v1/usage \
  -H "Authorization: Bearer $MISTRAL_API_KEY"
```

#### Application Errors
```bash
# View application logs
docker-compose logs avi-llm-agent

# Check application health
curl -v http://localhost:8080/api/health

# Test direct API access
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "test", "model": "llama3.2"}'
```

### Docker-Specific Issues

#### Ollama Container Issues
```bash
# Check Ollama container logs
docker-compose logs ollama

# Restart Ollama container
docker-compose restart ollama

# Check Ollama health
docker-compose exec ollama curl http://localhost:11434/api/tags

# Increase Ollama resources
docker-compose up -d --force-recreate ollama
```

#### Provider Configuration Issues
```bash
# Check which provider is being used
curl http://localhost:8080/api/health | jq .provider

# Verify environment variables
docker-compose exec avi-llm-agent env | grep LLM_PROVIDER

# Check Mistral API key
docker-compose exec avi-llm-agent env | grep MISTRAL_API_KEY
```

#### Network Connectivity Issues
```bash
# Test Ollama connectivity from Avi Agent container
docker-compose exec avi-llm-agent curl http://ollama:11434/api/tags

# Test Mistral AI connectivity
docker-compose exec avi-llm-agent curl https://api.mistral.ai/v1/models

# Check DNS resolution
docker-compose exec avi-llm-agent ping ollama
```

### Debug Mode
Enable debug logging:
```bash
# Update config.yaml
sed -i 's/level: "info"/level: "debug"/' config.yaml

# Or set environment variable
export LOG_LEVEL=debug

# Restart the application
docker-compose restart avi-llm-agent

# View debug logs
docker-compose logs -f --tail=100
```

## 📚 Advanced Configuration

### Customizing LLM Behavior
```yaml
# In config.yaml
llm:
  temperature: 0.3  # More deterministic (0.0 - 1.0)
  max_tokens: 4096  # Maximum response length
  timeout: 120     # Longer timeout for complex queries
```

### Performance Tuning
```yaml
# Optimize for high load
server:
  read_timeout: 60
  write_timeout: 60
  idle_timeout: 120

avi:
  timeout: 60
  # Enable caching (if supported)
```

### Security Hardening
```yaml
# Secure configuration
avi:
  insecure: false  # Always use for production
  # Consider using certificate-based authentication

# Enable rate limiting (requires reverse proxy)
# Example Nginx configuration:
location /api/ {
    limit_req zone=api burst=10 nodelay;
}
```

## 🎓 Learning Resources

### VMware Avi Documentation
- [Avi API Guide](https://avinetworks.com/docs/latest/api-guide/)
- [Avi REST API Reference](https://avinetworks.com/docs/latest/api-reference/)
- [Avi Architecture](https://avinetworks.com/docs/latest/architecture/)

### LLM and Ollama
- [Ollama Documentation](https://ollama.ai/docs)
- [Ollama Models](https://ollama.ai/library)
- [LLM Best Practices](https://platform.openai.com/docs/guides/best-practices)

### Mistral AI
- [Mistral AI Documentation](https://docs.mistral.ai/)
- [Mistral AI API Reference](https://docs.mistral.ai/api/)
- [Mistral AI Models](https://mistral.ai/technology/)
- [Mistral AI Pricing](https://mistral.ai/pricing/)

### Development
- [Go Documentation](https://go.dev/doc/)
- [Gin Web Framework](https://gin-gonic.com/docs/)
- [HTMX Documentation](https://htmx.org/docs/)

## 🤝 Community and Support

### Getting Help
- **GitHub Issues**: Report bugs and feature requests
- **GitHub Discussions**: Ask questions and share ideas
- **Community Forum**: Join our community discussions

### Contributing
```bash
# Fork the repository
# Create a feature branch
git checkout -b feature/your-feature

# Make changes and commit
git commit -m "Add your feature"

# Push to your fork
git push origin feature/your-feature

# Create a Pull Request
```

### Code Standards
- Follow Go formatting with `gofmt`
- Use `golangci-lint` for linting
- Maintain 80%+ test coverage
- Document all public APIs

## 📜 License

This project is licensed under the **Apache License 2.0** - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- VMware Avi Load Balancer team
- Ollama contributors
- Gin web framework developers
- HTMX community

---

**Built with ❤️ for the VMware community**

## Configuration

### Application Configuration
Create a `config.yaml` file:

```yaml
server:
  port: 8080
  read_timeout: 30
  write_timeout: 30
  idle_timeout: 60

avi:
  host: "avi-controller.example.com"
  username: "admin"
  password: "password"
  version: "31.2.1"
  tenant: "admin"
  timeout: 30
  insecure: true

llm:
  ollama_host: "http://localhost:11434"
  default_model: "llama3.2"
  models:
    - "llama3.2"
    - "mistral"
    - "codellama"
  timeout: 60
  temperature: 0.7
  max_tokens: 2048

log:
  level: "info"
  format: "json"
```

### Environment Variables
The application supports environment variables for sensitive configuration:

- `AVI_HOST` - Avi controller hostname
- `AVI_USERNAME` - Avi username
- `AVI_PASSWORD` - Avi password
- `OLLAMA_HOST` - Ollama server URL

## Usage Examples

### Basic Queries
- "What are the current virtual services?"
- "Show me all pools with their health status"
- "List service engines that are down"
- "Get analytics for the last hour"

### Management Operations  
- "Create a new virtual service for my web application"
- "Scale out the backend pool for app1"
- "Add a new server 10.1.1.100 to the web-pool"
- "Enable SSL for the api-virtualservice"

### Monitoring and Troubleshooting
- "Show me performance metrics for vs-web"
- "Which pools have unhealthy servers?"
- "Get connection statistics for the last 6 hours"
- "Show me service engines with high CPU usage"

## API Endpoints

### Chat API
- `POST /api/chat` - Send chat message
- `GET /api/chat/history` - Get conversation history
- `DELETE /api/chat/history` - Clear history

### Model Management  
- `GET /api/models` - List available models
- `POST /api/models/validate` - Validate model availability

### Health and Status
- `GET /api/health` - Application health check
- `GET /api/avi/*` - Direct Avi API proxy

### HTMX Endpoints
- `POST /htmx/chat` - HTMX chat interface
- `GET /htmx/models` - Model selection UI
- `GET /htmx/history` - Chat history UI

## Development

### Building from Source
```bash
# Install dependencies
go mod download

# Run tests
go test ./...

# Build binary
go build -o aviagent ./cmd/server

# Run application
./aviagent -config config.yaml
```

### Development with Docker
```bash
# Build development image
docker build -t aviagent:dev .

# Run with development configuration
docker run -p 8080:8080 \
  -v $(pwd)/config.yaml:/etc/aviagent/config.yaml \
  aviagent:dev
```

### Testing
```bash
# Run unit tests
go test ./internal/... -v

# Run integration tests  
go test ./tests/integration/... -v

# Run with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. ./...
```

## Architecture

### Project Structure
```
aviagent/
├── cmd/
│   └── server/          # Application entry point
├── internal/
│   ├── avi/            # Avi API client
│   ├── llm/            # LLM client and tools
│   ├── web/            # Web server and handlers
│   └── config/         # Configuration management
├── web/
│   ├── templates/      # HTML templates
│   └── static/         # Static assets (CSS, JS)
├── tests/
│   ├── unit/           # Unit tests
│   └── integration/    # Integration tests
├── docs/               # Documentation
├── Dockerfile          # Multi-stage Docker build
├── docker-compose.yml  # Development environment
└── README.md
```

### Component Architecture
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Client    │    │   LLM Service   │    │  Avi Controller │
│   (Browser)     │◄──►│    (Ollama)     │    │   (VMware)      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Avi LLM Agent (Go)                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │ Web Server  │  │ LLM Client  │  │     Avi API Client      │ │
│  │   (Gin)     │  │  (Ollama)   │  │   (VMware ALB SDK)      │ │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## LLM Tool Definitions

The agent provides comprehensive tool definitions for the LLM to understand available operations:

### Virtual Service Tools
- `list_virtual_services` - List and filter virtual services
- `get_virtual_service` - Get detailed VS information
- `create_virtual_service` - Create new virtual services
- `update_virtual_service` - Modify existing virtual services
- `delete_virtual_service` - Remove virtual services

### Pool Management Tools
- `list_pools` - List and filter backend pools
- `get_pool` - Get detailed pool information
- `create_pool` - Create new backend pools
- `scale_out_pool` - Add capacity to pools
- `scale_in_pool` - Remove capacity from pools

### Monitoring Tools
- `list_health_monitors` - List health monitors
- `get_health_monitor` - Get health monitor details
- `list_service_engines` - List service engines
- `get_service_engine` - Get service engine details
- `get_analytics` - Retrieve performance metrics

### Generic Operations
- `execute_generic_operation` - Execute any Avi API operation

## Security Considerations

### Authentication
- Secure session management with VMware Avi controllers
- CSRF token protection
- Session timeout handling

### Network Security
- TLS/SSL support for Avi controller connections
- Configurable certificate validation
- Network isolation with Docker

### Application Security
- Input validation and sanitization
- Rate limiting (recommended with reverse proxy)
- Non-root container execution
- Minimal container image with distroless base

## Monitoring and Observability

### Health Checks
- Application health endpoint: `/api/health`
- Docker health checks configured
- Kubernetes readiness/liveness probes supported

### Logging
- Structured JSON logging
- Configurable log levels
- Request/response logging
- Error tracking and alerting

### Metrics (Optional)
- Prometheus metrics endpoint
- Grafana dashboards
- Custom application metrics

## Troubleshooting

### Common Issues

#### Connection to Avi Controller
```bash
# Check connectivity
curl -k https://your-avi-controller.com/login

# Verify credentials
# Check firewall rules
# Validate SSL certificates
```

#### Ollama Model Issues
```bash
# List available models
docker-compose exec ollama ollama list

# Pull missing models
docker-compose exec ollama ollama pull llama3.2

# Check Ollama logs
docker-compose logs ollama
```

#### Application Logs
```bash
# View application logs
docker-compose logs avi-llm-agent

# Follow logs in real-time
docker-compose logs -f avi-llm-agent
```

### Debug Mode
Enable debug logging:
```yaml
log:
  level: "debug"
  format: "json"
```

## Production Deployment

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aviagent
spec:
  replicas: 2
  selector:
    matchLabels:
      app: aviagent
  template:
    metadata:
      labels:
        app: aviagent
    spec:
      containers:
      - name: app
        image: aviagent:latest
        ports:
        - containerPort: 8080
        env:
        - name: AVI_HOST
          valueFrom:
            secretKeyRef:
              name: avi-credentials
              key: host
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"  
            cpu: "500m"
```

### Scaling Considerations
- Horizontal scaling supported
- Session state is stateless
- Load balancer configuration needed
- Database for persistent chat history (optional)

## Contributing

### Development Setup
1. Fork the repository
2. Create a feature branch
3. Make changes and add tests
4. Run the full test suite
5. Submit a pull request

### Code Standards
- Go formatting with `gofmt`
- Linting with `golangci-lint`
- Test coverage > 80%
- Documentation for public APIs

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Support

### Documentation
- [API Documentation](docs/API.md)
- [Deployment Guide](docs/DEPLOYMENT.md)
- [Configuration Reference](docs/CONFIGURATION.md)

### Community
- GitHub Issues for bug reports
- GitHub Discussions for questions
- Contributing guidelines in [CONTRIBUTING.md](CONTRIBUTING.md)

## Roadmap

### Version 1.1
- [ ] Persistent chat history
- [ ] User authentication and authorization
- [ ] Advanced analytics dashboards
- [ ] Webhook support for notifications

### Version 1.2
- [ ] Multi-tenant support
- [ ] Custom tool definitions
- [ ] Integration with other VMware products
- [ ] Advanced monitoring and alerting

---

**Built with ❤️ for the VMware community**