package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/local/trade-discipline-desktop/backend/internal/api"
	"github.com/local/trade-discipline-desktop/backend/internal/service"
	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

type readyMessage struct {
	Event         string `json:"event"`
	Port          int    `json:"port"`
	SchemaVersion int    `json:"schemaVersion"`
}

func main() {
	if err := run(); err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"level": "ERROR", "message": "sidecar stopped", "error": err.Error()})
		os.Exit(1)
	}
}

func run() error {
	token := os.Getenv("DISCIPLINE_SESSION_TOKEN")
	dataDir := os.Getenv("DISCIPLINE_DATA_DIR")
	if len(token) < 32 {
		return fmt.Errorf("DISCIPLINE_SESSION_TOKEN is missing or too short")
	}
	if dataDir == "" {
		return fmt.Errorf("DISCIPLINE_DATA_DIR is required")
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0o700); err != nil {
		return fmt.Errorf("create data directories: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(dataDir, "logs", "go-sidecar.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	slog.SetDefault(slog.New(slog.NewJSONHandler(logFile, nil)))

	db, err := store.Open(filepath.Join(dataDir, "discipline.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen loopback: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{
		Handler:           api.NewRouter(service.New(db, nil), token),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	encoded, _ := json.Marshal(readyMessage{Event: "ready", Port: port, SchemaVersion: 1})
	fmt.Println(string(encoded))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case sig := <-stop:
		slog.Info("shutdown requested", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
