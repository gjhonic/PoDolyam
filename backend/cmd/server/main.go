package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"podolyam/internal/httpapi"
	"podolyam/internal/storage"
	"syscall"
	"time"
)

func run() error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("задайте DATABASE_URL или выполните make db-start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	db, err := storage.Open(ctx, url)
	cancel()
	if err != nil {
		return err
	}
	defer db.Close()
	origin := os.Getenv("APP_ORIGIN")
	if origin == "" {
		origin = "http://localhost:5173"
	}
	handler, err := httpapi.NewApp(db, origin)
	if err != nil {
		return err
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	shutdownDone := make(chan struct{})
	// Одна горутина нужна для завершения HTTP-сервера по сигналу.
	// Она не участвует в расчётах и имеет явное условие выхода.
	go func() {
		defer close(shutdownDone)
		select {
		case <-signals.Done():
		case <-done:
			return
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			slog.Error("Ошибка завершения HTTP", "error", err)
		}
	}()
	slog.Info("Сервер PoDolyam запущен", "address", addr)
	err = server.ListenAndServe()
	close(done)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("HTTP-сервер: %w", err)
}
func main() {
	if err := run(); err != nil {
		slog.Error("Не удалось запустить сервер", "error", err)
		os.Exit(1)
	}
}
