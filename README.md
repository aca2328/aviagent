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

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Access to a VMware Avi Load Balancer controller
- Ollama running locally or remotely

### 1. Clone the Repository
```bash
git clone <repository-url>
cd aviagent
```

### 2. Configure Environment
Create a `.env` file:
```env
AVI_HOST=your-avi-controller.example.com
AVI_USERNAME=admin
AVI_PASSWORD=your-password
OLLAMA_HOST=http://localhost:11434
```

### 3. Start Services
```bash
# Start all services
docker-compose up -d

# Or with monitoring stack
docker-compose --profile monitoring up -d
```

### 4. Pull LLM Models
```bash
# Pull required models
docker-compose exec ollama ollama pull llama3.2
docker-compose exec ollama ollama pull mistral
```

### 5. Access the Application
Open your browser and navigate to: `http://localhost:8080`

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