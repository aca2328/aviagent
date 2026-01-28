"""
Python Mistral Client - Configuration Handling
VMware Avi LLM Agent - Python Implementation
"""

import os
from typing import Optional
from pydantic import BaseModel, Field
from pydantic_settings import BaseSettings, SettingsConfigDict
from dotenv import load_dotenv

# Load environment variables
load_dotenv()


class MistralConfig(BaseModel):
    """Mistral AI configuration"""
    api_base_url: str = Field(default="https://api.mistral.ai")
    api_key: str
    default_model: str = Field(default="mistral-medium")
    models: list[str] = Field(default_factory=lambda: ["mistral-tiny", "mistral-small", "mistral-medium"])
    timeout: int = Field(default=60)
    temperature: float = Field(default=0.7)
    max_tokens: int = Field(default=2048)
    debug: bool = Field(default=False)
    
    @classmethod
    def from_env_and_data(cls, **data):
        """
        Create config from environment variables and explicit data
        Environment variables take precedence over explicit data for security-sensitive fields
        """
        # Load environment variables
        env_config = {
            "api_base_url": os.environ.get("MISTRAL_API_BASE_URL", data.get("api_base_url", "https://api.mistral.ai")),
            "api_key": os.environ.get("MISTRAL_API_KEY") or data.get("api_key", ""),
            "default_model": os.environ.get("MISTRAL_DEFAULT_MODEL", data.get("default_model", "mistral-medium")),
            "timeout": int(os.environ.get("MISTRAL_TIMEOUT", data.get("timeout", 60))),
            "temperature": float(os.environ.get("MISTRAL_TEMPERATURE", data.get("temperature", 0.7))),
            "max_tokens": int(os.environ.get("MISTRAL_MAX_TOKENS", data.get("max_tokens", 2048))),
            "debug": os.environ.get("MISTRAL_DEBUG", data.get("debug", "false")).lower() in ("true", "1", "t"),
        }
        
        # Parse models from environment variable or use provided data
        if "MISTRAL_MODELS" in os.environ:
            models_str = os.environ["MISTRAL_MODELS"]
            env_config["models"] = [model.strip() for model in models_str.split(",") if model.strip()]
        elif "models" in data:
            env_config["models"] = data["models"]
        
        return cls(**env_config)


class AviConfig(BaseSettings):
    """VMware Avi Load Balancer configuration with environment variable support"""
    host: str = Field(..., env="AVI_HOST")
    username: str = Field(..., env="AVI_USERNAME")
    password: str = Field(..., env="AVI_PASSWORD")
    version: str = Field(default="31.2.1", env="AVI_VERSION")
    tenant: str = Field(default="admin", env="AVI_TENANT")
    timeout: int = Field(default=30, env="AVI_TIMEOUT")
    insecure: bool = Field(default=False, env="AVI_INSECURE")
    auth_method: str = Field(default="session", env="AVI_AUTH_METHOD")

    model_config = SettingsConfigDict(env_prefix="AVI_")


class AppConfig(BaseSettings):
    """Application configuration with environment variable support"""
    server_port: int = Field(default=8080, env="SERVER_PORT")
    provider: str = Field(default="mistral", env="LLM_PROVIDER")
    mistral: MistralConfig = Field(default_factory=MistralConfig)
    avi: AviConfig = Field(default_factory=AviConfig)
    log_level: str = Field(default="info", env="LOG_LEVEL")

    model_config = SettingsConfigDict(env_prefix="AVI_AGENT_")


def load_config(config_path: Optional[str] = None) -> AppConfig:
    """
    Load configuration from file and environment variables
    
    Args:
        config_path: Optional path to YAML config file
        
    Returns:
        AppConfig: Loaded configuration
    """
    if config_path and os.path.exists(config_path):
        # TODO: Implement YAML config file loading
        # For now, just use environment variables
        pass

    return AppConfig()


def parse_environment_models(env_var: str) -> list[str]:
    """
    Parse comma-separated environment variable into list
    
    Args:
        env_var: Environment variable name
        
    Returns:
        list[str]: Parsed model names
    """
    models_str = os.environ.get(env_var, "")
    if not models_str:
        return []
    
    return [model.strip() for model in models_str.split(",") if model.strip()]