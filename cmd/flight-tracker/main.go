// Command flight-tracker runs the flight price tracker web server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"flight-price-tracker/internal/api"
	"flight-price-tracker/internal/config"
	"flight-price-tracker/internal/demo"
	"flight-price-tracker/internal/model"
	"flight-price-tracker/internal/notify"
	"flight-price-tracker/internal/secrets"
	"flight-price-tracker/internal/serpapi"
	"flight-price-tracker/internal/store"
	"flight-price-tracker/internal/tracker"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	defaultConfig := os.Getenv("CONFIG_FILE")
	if defaultConfig == "" {
		defaultConfig = "config.yaml"
	}
	configPath := flag.String("config", defaultConfig, "path to the YAML config file (env CONFIG_FILE)")
	printSpec := flag.Bool("openapi", false, "print the OpenAPI document as JSON and exit")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel()}))
	if err := run(*configPath, *printSpec, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(configPath string, printSpec bool, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	seed := make([]model.Destination, 0, len(cfg.Destinations))
	for _, d := range cfg.Destinations {
		seed = append(seed, model.Destination{IATA: d.IATA, City: d.City, MaxPrice: d.MaxPrice, Months: d.Months})
	}
	added, err := st.SeedDestinations(ctx, seed)
	if err != nil {
		return fmt.Errorf("seed destinations: %w", err)
	}
	log.Info("database ready", "path", cfg.Database.Path, "seeded_destinations", added)

	var searcher tracker.Searcher
	switch key := secrets.Lookup("SERPAPI_KEY", cfg.SecretsFile); {
	case cfg.Search.Provider == "demo":
		searcher = demo.New()
		log.Warn("using the demo search provider: prices are made up")
	case key != "":
		searcher = serpapi.New(key)
	default:
		log.Warn("SERPAPI_KEY not set in environment or secrets file; searches are disabled", "secrets_file", cfg.SecretsFile)
	}

	var notifier notify.Notifier = notify.Noop{}
	if cfg.Notify.Enabled {
		pw := secrets.Lookup("EMAIL_PASSWORD", cfg.SecretsFile)
		if pw == "" {
			return errors.New("notify.enabled is true but EMAIL_PASSWORD is not set in environment or secrets file")
		}
		notifier = &notify.Email{Host: cfg.Notify.SMTPHost, Port: cfg.Notify.SMTPPort, From: cfg.Notify.From,
			Password: pw, Recipients: cfg.Notify.Recipients}
	}

	tr := tracker.New(st, searcher, notifier, cfg.Search, cfg.Database.HistoryLimit, log)
	handler, humaAPI := api.New(ctx, cfg, st, tr, version, log)

	if printSpec {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(humaAPI.OpenAPI())
	}

	if searcher != nil {
		go tr.Schedule(ctx)
		log.Info("search scheduler started", "interval", cfg.Search.Interval, "run_on_startup", cfg.Search.RunOnStartup)
	}

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      5 * time.Minute, // a single destination search can take a while
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Server.Addr, "docs", "/docs", "openapi", "/openapi.yaml", "version", version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func logLevel() slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.ToUpper(os.Getenv("LOG_LEVEL")))); err != nil {
		return slog.LevelInfo
	}
	return l
}
