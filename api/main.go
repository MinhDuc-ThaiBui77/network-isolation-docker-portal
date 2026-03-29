package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags)

	cfg := LoadConfig()

	// Connect to PostgreSQL
	if cfg.DatabaseURL != "" {
		if err := ConnectDB(cfg.DatabaseURL); err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		log.Println("Connected to PostgreSQL")
	}

	// Connect to Redis
	if cfg.RedisURL != "" {
		if err := ConnectRedis(cfg.RedisURL); err != nil {
			log.Fatalf("Failed to connect to Redis: %v", err)
		}
		log.Println("Connected to Redis")
	}

	// Start TCP server for agent connections
	p := NewPortal(cfg.TCPPort)
	if err := p.Start(); err != nil {
		log.Fatalf("Failed to start TCP server: %v", err)
	}

	// Set up HTTP server
	mux := http.NewServeMux()
	registerRoutes(mux, p)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: mux,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("HTTP server listening on 0.0.0.0:%d", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
	p.Shutdown()
	CloseRedis()
	CloseDB()
}
