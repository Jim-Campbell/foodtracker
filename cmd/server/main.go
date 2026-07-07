package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jimgcampbell/food/internal/api"
	"github.com/jimgcampbell/food/internal/db"
	"github.com/jimgcampbell/food/internal/food"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(log); err != nil {
		log.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := struct {
		DatabaseURL string
		APIKey      string
		Port        string
		MigrDir     string
		PWADir      string
		// Optional — used only to report configured/not-configured in health
		AnthropicKey string
		FDCKey       string
		R2AccountID  string
		R2AccessKey  string
		R2SecretKey  string
		R2Bucket     string
		R2PublicURL  string
	}{
		DatabaseURL:  requireEnv("DATABASE_URL"),
		APIKey:       requireEnv("FOOD_API_KEY"),
		Port:         getEnv("PORT", "8082"),
		MigrDir:      getEnv("MIGRATIONS_DIR", "internal/db/migrations"),
		PWADir:       getEnv("PWA_DIR", "pwa"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		FDCKey:       os.Getenv("FDC_API_KEY"),
		R2AccountID:  os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKey:  os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretKey:  os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:     os.Getenv("R2_BUCKET"),
		R2PublicURL:  os.Getenv("R2_PUBLIC_URL"),
	}

	photosEnabled := cfg.R2AccountID != "" && cfg.R2AccessKey != "" &&
		cfg.R2SecretKey != "" && cfg.R2Bucket != "" && cfg.R2PublicURL != ""
	aiEnabled := cfg.AnthropicKey != ""

	if cfg.AnthropicKey != "" {
		log.Info("AI configured", "key_set", true)
	} else {
		log.Info("AI not configured (set ANTHROPIC_API_KEY to enable)")
	}
	if cfg.FDCKey != "" {
		log.Info("FDC API configured", "key_set", true)
	} else {
		log.Info("FDC API not configured (set FDC_API_KEY to enable)")
	}
	if photosEnabled {
		log.Info("R2 photo storage configured")
	} else {
		log.Info("R2 photo storage not configured (set R2_* vars to enable)")
	}

	ctx := context.Background()

	log.Info("connecting to database")
	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database init: %w", err)
	}
	defer database.Close()
	log.Info("database connected")

	log.Info("running migrations", "dir", cfg.MigrDir)
	if err := db.RunMigrations(ctx, database, cfg.MigrDir); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Info("migrations complete")

	svc := food.NewService(database, log)
	handler := api.NewHandler(svc, log)
	r := api.NewRouter(api.Config{
		APIKey: cfg.APIKey,
		Photos: photosEnabled,
		AI:     aiEnabled,
		PWADir: cfg.PWADir,
	}, handler, log)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-stop
	log.Info("shutting down gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
