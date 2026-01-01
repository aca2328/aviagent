package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aviagent/internal/config"
	"aviagent/internal/web"

	"go.uber.org/zap"
)

// Application version
const (
	AppName    = "VMware Avi LLM Agent"
	AppVersion = "1.0.0"
	BuildDate  = "2026-01-01"
)

func main() {
	// Parse command line flags
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize web server
	server, err := web.NewServer(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize web server", zap.Error(err))
	}

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      server.Router(),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting VMware Avi LLM Agent",
			zap.String("address", httpServer.Addr),
			zap.String("ollama_host", cfg.LLM.OllamaHost),
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start HTTP server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exiting")
}