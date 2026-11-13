package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"flight-price-tracker/internal/config"
	"flight-price-tracker/internal/store"
	"flight-price-tracker/internal/tracker"
)

func TestDestinationsCRUD(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tr := tracker.New(st, nil, nil, config.Default().Search, 10, log)

	huma.DefaultArrayNullable = false
	_, api := humatest.New(t)
	(&Server{cfg: config.Default(), store: st, tracker: tr, log: log, bgCtx: ctx}).register(api)

	body := map[string]any{"iata": "bcn", "city": "Barcelona", "max_price": 1500, "months": []string{"jun"}}
	if r := api.Post("/destinations", body); r.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	if r := api.Post("/destinations", body); r.Code != http.StatusConflict {
		t.Fatalf("duplicate create: %d", r.Code)
	}
	bad := map[string]any{"iata": "BCN", "city": "x", "max_price": 1, "months": []string{"foo"}}
	if r := api.Post("/destinations", bad); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid month: %d %s", r.Code, r.Body)
	}
	if r := api.Get("/destinations/BCN"); r.Code != http.StatusOK {
		t.Fatalf("get: %d", r.Code)
	}
	if r := api.Get("/destinations/BCN/prices"); r.Code != http.StatusOK {
		t.Fatalf("prices: %d %s", r.Code, r.Body)
	}
	if r := api.Post("/destinations/BCN/search"); r.Code != http.StatusServiceUnavailable {
		t.Fatalf("search without key: %d", r.Code)
	}
	if r := api.Delete("/destinations/BCN"); r.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", r.Code)
	}
	if r := api.Get("/destinations/BCN"); r.Code != http.StatusNotFound {
		t.Fatalf("get after delete: %d", r.Code)
	}
}
