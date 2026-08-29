import json
import os

# Let me create the project structure and all necessary files
project_structure = {
    "aviagent/": {
        "cmd/": {
            "server/": {
                "main.go": "Main application entry point"
            }
        },
        "internal/": {
            "avi/": {
                "client.go": "Avi API client implementation",
                "models.go": "Avi data models",
                "endpoints.go": "API endpoint definitions"
            },
            "llm/": {
                "client.go": "Ollama LLM client",
                "tools.go": "LLM tool definitions",
                "processor.go": "LLM response processor"
            },
            "web/": {
                "server.go": "Gin web server setup",
                "handlers.go": "HTTP handlers",
                "middleware.go": "Web middleware"
            },
            "config/": {
                "config.go": "Application configuration"
            }
        },
        "web/": {
            "templates/": {
                "index.html": "Main chat interface",
                "chat.html": "Chat components"
            },
            "static/": {
                "css/": {
                    "style.css": "Application styles"
                },
                "js/": {
                    "htmx.min.js": "HTMX library",
                    "app.js": "Application JavaScript"
                }
            }
        },
        "tests/": {
            "integration/": {
                "server_test.go": "Integration tests"
            },
            "unit/": {
                "avi_test.go": "Unit tests for Avi client",
                "llm_test.go": "Unit tests for LLM client"
            }
        },
        "docs/": {
            "README.md": "Project documentation",
            "API.md": "API documentation",
            "DEPLOYMENT.md": "Deployment guide"
        },
        "Dockerfile": "Multi-stage Docker build",
        "docker-compose.yml": "Development environment",
        "go.mod": "Go module definition",
        "go.sum": "Go module dependencies",
        "Makefile": "Build automation"
    }
}

print("Project structure created")
print(json.dumps(project_structure, indent=2))