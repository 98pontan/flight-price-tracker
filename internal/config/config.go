// Package config loads the YAML configuration used to initialize the server.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"flight-price-tracker/internal/months"
)

type Config struct {
	Server       Server        `yaml:"server"`
	Database     Database      `yaml:"database"`
	Search       Search        `yaml:"search"`
	Notify       Notify        `yaml:"notify"`
	SecretsFile  string        `yaml:"secrets_file"`
	Destinations []Destination `yaml:"destinations"`
}

type Server struct {
	Addr string `yaml:"addr"`
}

type Database struct {
	Path string `yaml:"path"`
	// HistoryLimit is how many search results are kept per destination.
	HistoryLimit int `yaml:"history_limit"`
}

type Search struct {
	// Provider is "serpapi" (real prices, needs SERPAPI_KEY) or "demo" (made-up prices).
	Provider       string        `yaml:"provider"`
	DefaultAirport string        `yaml:"default_airport"`
	Currency       string        `yaml:"currency"`
	OneWay         bool          `yaml:"one_way"`
	MaxMonthsAhead int           `yaml:"max_months_ahead"`
	Interval       time.Duration `yaml:"interval"`
	RunOnStartup   bool          `yaml:"run_on_startup"`
}

type Notify struct {
	Enabled    bool     `yaml:"enabled"`
	SMTPHost   string   `yaml:"smtp_host"`
	SMTPPort   int      `yaml:"smtp_port"`
	From       string   `yaml:"from"`
	Recipients []string `yaml:"recipients"`
}

type Destination struct {
	IATA     string   `yaml:"iata"`
	City     string   `yaml:"city"`
	MaxPrice int      `yaml:"max_price"`
	Months   []string `yaml:"months"`
}

var iataRe = regexp.MustCompile(`^[A-Z]{3}$`)

// Default returns the configuration used for any value missing from the file.
func Default() Config {
	return Config{
		Server:   Server{Addr: ":8080"},
		Database: Database{Path: "flights.db", HistoryLimit: 10},
		Search: Search{
			Provider:       "serpapi",
			DefaultAirport: "ARN",
			Currency:       "SEK",
			MaxMonthsAhead: 6,
			Interval:       24 * time.Hour,
		},
		Notify:      Notify{SMTPHost: "smtp.gmail.com", SMTPPort: 587},
		SecretsFile: "/vault/secrets/authentication",
	}
}

// Load reads the YAML file at path, applies environment overrides and validates the result.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.applyEnv(); err != nil {
		return cfg, err
	}
	return cfg, cfg.Validate()
}

// applyEnv lets containers override common settings without editing the YAML.
func (c *Config) applyEnv() error {
	str := map[string]*string{
		"LISTEN_ADDR":     &c.Server.Addr,
		"DATABASE_PATH":   &c.Database.Path,
		"DEFAULT_AIRPORT": &c.Search.DefaultAirport,
		"CURRENCY":        &c.Search.Currency,
		"SECRETS_FILE":    &c.SecretsFile,
		"SEARCH_PROVIDER": &c.Search.Provider,
	}
	for k, p := range str {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			*p = v
		}
	}
	if v, ok := os.LookupEnv("ONE_WAY"); ok && v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("ONE_WAY: %w", err)
		}
		c.Search.OneWay = b
	}
	if v, ok := os.LookupEnv("SEARCH_INTERVAL"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("SEARCH_INTERVAL: %w", err)
		}
		c.Search.Interval = d
	}
	return nil
}

func (c *Config) Validate() error {
	var errs []error
	if c.Database.HistoryLimit < 1 {
		errs = append(errs, errors.New("database.history_limit must be >= 1"))
	}
	if c.Search.MaxMonthsAhead < 1 || c.Search.MaxMonthsAhead > 12 {
		errs = append(errs, errors.New("search.max_months_ahead must be between 1 and 12"))
	}
	if c.Search.Interval < time.Minute {
		errs = append(errs, errors.New("search.interval must be at least 1m"))
	}
	if c.Search.Provider != "serpapi" && c.Search.Provider != "demo" {
		errs = append(errs, fmt.Errorf("search.provider must be serpapi or demo, got %q", c.Search.Provider))
	}
	c.Search.DefaultAirport = strings.ToUpper(c.Search.DefaultAirport)
	if !iataRe.MatchString(c.Search.DefaultAirport) {
		errs = append(errs, fmt.Errorf("search.default_airport %q is not a 3-letter IATA code", c.Search.DefaultAirport))
	}
	if c.Notify.Enabled && (c.Notify.From == "" || len(c.Notify.Recipients) == 0) {
		errs = append(errs, errors.New("notify.from and notify.recipients are required when notify.enabled is true"))
	}
	seen := map[string]bool{}
	for i := range c.Destinations {
		d := &c.Destinations[i]
		d.IATA = strings.ToUpper(strings.TrimSpace(d.IATA))
		if !iataRe.MatchString(d.IATA) {
			errs = append(errs, fmt.Errorf("destinations[%d]: iata %q is not a 3-letter code", i, d.IATA))
		}
		if seen[d.IATA] {
			errs = append(errs, fmt.Errorf("destinations[%d]: duplicate iata %q", i, d.IATA))
		}
		seen[d.IATA] = true
		if d.MaxPrice <= 0 {
			errs = append(errs, fmt.Errorf("destinations[%d] (%s): max_price must be > 0", i, d.IATA))
		}
		if len(d.Months) == 0 {
			errs = append(errs, fmt.Errorf("destinations[%d] (%s): months must not be empty", i, d.IATA))
		}
		norm, bad, ok := months.Normalize(d.Months)
		if !ok {
			errs = append(errs, fmt.Errorf("destinations[%d] (%s): invalid month %q", i, d.IATA, bad))
		}
		d.Months = norm
	}
	return errors.Join(errs...)
}
