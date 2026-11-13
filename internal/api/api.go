// Package api exposes the flight tracker over HTTP. The OpenAPI 3.1 document is generated
// from the operations registered here and served at /openapi.json and /openapi.yaml,
// with interactive docs at /docs.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"flight-price-tracker/internal/config"
	"flight-price-tracker/internal/model"
	"flight-price-tracker/internal/months"
	"flight-price-tracker/internal/store"
	"flight-price-tracker/internal/tracker"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	tracker *tracker.Tracker
	log     *slog.Logger
	// bgCtx is used for searches that outlive the request that started them.
	bgCtx context.Context
}

// New builds the HTTP handler. bgCtx bounds background search runs started through the API.
func New(bgCtx context.Context, cfg config.Config, st *store.Store, tr *tracker.Tracker, version string, log *slog.Logger) (http.Handler, huma.API) {
	// Every array the API returns is non-nil, so don't advertise them as nullable.
	huma.DefaultArrayNullable = false
	mux := http.NewServeMux()
	hcfg := huma.DefaultConfig("Flight Price Tracker API", version)
	hcfg.Info.Description = "Tracks flight prices to a set of destinations and keeps a short price history for each."
	api := humago.New(mux, hcfg)

	s := &Server{cfg: cfg, store: st, tracker: tr, log: log, bgCtx: bgCtx}
	s.register(api)
	return logRequests(log, mux), api
}

// ---- inputs / outputs ----

type iataParam struct {
	IATA string `path:"iata" pattern:"^[A-Za-z]{3}$" doc:"IATA code of the destination" example:"BCN"`
}

func (p iataParam) code() string { return strings.ToUpper(p.IATA) }

type destinationFields struct {
	City     string   `json:"city" minLength:"1" maxLength:"100" example:"Barcelona"`
	MaxPrice int      `json:"max_price" minimum:"1" doc:"Notify when a flight is cheaper than this" example:"1500"`
	Months   []string `json:"months" minItems:"1" maxItems:"12" uniqueItems:"true" enum:"jan,feb,mar,apr,may,jun,jul,aug,sep,oct,nov,dec" doc:"Months to search" example:"[\"jun\",\"jul\"]"`
}

type createDestinationInput struct {
	Body struct {
		IATA     string   `json:"iata" pattern:"^[A-Za-z]{3}$" doc:"IATA code of the destination" example:"BCN"`
		City     string   `json:"city" minLength:"1" maxLength:"100" example:"Barcelona"`
		MaxPrice int      `json:"max_price" minimum:"1" doc:"Notify when a flight is cheaper than this" example:"1500"`
		Months   []string `json:"months" minItems:"1" maxItems:"12" uniqueItems:"true" enum:"jan,feb,mar,apr,may,jun,jul,aug,sep,oct,nov,dec" doc:"Months to search" example:"[\"jun\",\"jul\"]"`
	}
}

type updateDestinationInput struct {
	IATA string `path:"iata" pattern:"^[A-Za-z]{3}$" doc:"IATA code of the destination" example:"BCN"`
	Body destinationFields
}

type destinationOutput struct {
	Body model.Destination
}

type destinationListOutput struct {
	Body struct {
		Destinations []model.Destination `json:"destinations"`
	}
}

type historyInput struct {
	IATA  string `path:"iata" pattern:"^[A-Za-z]{3}$" doc:"IATA code of the destination" example:"BCN"`
	Limit int    `query:"limit" minimum:"1" maximum:"100" default:"10" doc:"Maximum number of results to return"`
}

type historyOutput struct {
	Body struct {
		Destination model.Destination    `json:"destination"`
		Results     []model.SearchResult `json:"results" doc:"Most recent search results, oldest first"`
	}
}

type searchResultOutput struct {
	Body model.SearchResult
}

type runOutput struct {
	Body struct {
		Status string `json:"status" enum:"started" example:"started"`
	}
}

type statusOutput struct {
	Body struct {
		NextRunAt    *time.Time `json:"next_run_at,omitempty" doc:"When the scheduler searches all destinations next"`
		Running      bool       `json:"running" doc:"Whether a full search run is in progress"`
		Configured   bool       `json:"search_configured" doc:"Whether a SerpApi key is configured"`
		LastRunAt    *time.Time `json:"last_run_at,omitempty"`
		LastRunError string     `json:"last_run_error,omitempty"`
	}
}

type configOutput struct {
	Body struct {
		Provider       string `json:"provider" enum:"serpapi,demo" doc:"Flight search backend; demo makes up prices"`
		DefaultAirport string `json:"default_airport" example:"ARN"`
		Currency       string `json:"currency" example:"SEK"`
		OneWay         bool   `json:"one_way"`
		MaxMonthsAhead int    `json:"max_months_ahead" doc:"Only months this close are searched (current month included)"`
		Interval       string `json:"interval" doc:"How often all destinations are searched" example:"24h0m0s"`
		IntervalSecs   int64  `json:"interval_seconds"`
		RunOnStartup   bool   `json:"run_on_startup"`
		HistoryLimit   int    `json:"history_limit" doc:"Search results kept per destination"`
		NotifyEnabled  bool   `json:"notify_enabled"`
		NotifyCount    int    `json:"notify_recipients" doc:"Number of email recipients"`
	}
}

type overviewOutput struct {
	Body struct {
		Histories []priceHistory `json:"histories"`
	}
}

type priceHistory struct {
	Destination model.Destination    `json:"destination"`
	Results     []model.SearchResult `json:"results" doc:"Most recent search results, oldest first"`
}

type searchInput struct {
	Body struct {
		From   string   `json:"from,omitempty" pattern:"^[A-Za-z]{3}$" doc:"Departure airport, defaults to the configured one" example:"ARN"`
		To     string   `json:"to" pattern:"^[A-Za-z]{3}$" doc:"Destination IATA code" example:"BCN"`
		Months []string `json:"months" minItems:"1" maxItems:"12" uniqueItems:"true" enum:"jan,feb,mar,apr,may,jun,jul,aug,sep,oct,nov,dec" example:"[\"jun\"]"`
		OneWay *bool    `json:"one_way,omitempty" doc:"Defaults to the configured value"`
		Limit  int      `json:"limit,omitempty" minimum:"1" maximum:"100" default:"20"`
	}
}

type searchOutput struct {
	Body struct {
		From     string         `json:"from"`
		To       string         `json:"to"`
		Currency string         `json:"currency"`
		OneWay   bool           `json:"one_way"`
		Flights  []model.Flight `json:"flights" doc:"Cheapest first"`
	}
}

type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok"`
	}
}

// ---- routes ----

func (s *Server) register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "health", Method: http.MethodGet, Path: "/healthz",
		Summary: "Health check", Tags: []string{"System"},
	}, s.health)

	huma.Register(api, huma.Operation{
		OperationID: "get-config", Method: http.MethodGet, Path: "/config",
		Summary: "Effective configuration (no secrets)", Tags: []string{"System"},
	}, s.config)

	huma.Register(api, huma.Operation{
		OperationID: "list-destinations", Method: http.MethodGet, Path: "/destinations",
		Summary: "List destinations", Tags: []string{"Destinations"},
	}, s.listDestinations)

	huma.Register(api, huma.Operation{
		OperationID: "create-destination", Method: http.MethodPost, Path: "/destinations",
		Summary: "Add a destination", Tags: []string{"Destinations"},
		DefaultStatus: http.StatusCreated,
	}, s.createDestination)

	huma.Register(api, huma.Operation{
		OperationID: "get-destination", Method: http.MethodGet, Path: "/destinations/{iata}",
		Summary: "Get a destination", Tags: []string{"Destinations"},
	}, s.getDestination)

	huma.Register(api, huma.Operation{
		OperationID: "update-destination", Method: http.MethodPut, Path: "/destinations/{iata}",
		Summary: "Update a destination", Tags: []string{"Destinations"},
	}, s.updateDestination)

	huma.Register(api, huma.Operation{
		OperationID: "delete-destination", Method: http.MethodDelete, Path: "/destinations/{iata}",
		Summary: "Delete a destination and its price history", Tags: []string{"Destinations"},
		DefaultStatus: http.StatusNoContent,
	}, s.deleteDestination)

	huma.Register(api, huma.Operation{
		OperationID: "get-price-history", Method: http.MethodGet, Path: "/destinations/{iata}/prices",
		Summary:     "Price history for a destination",
		Description: "Returns the most recent search results (the database keeps the latest `history_limit`, 10 by default), oldest first, so the lowest price can be plotted over time.",
		Tags:        []string{"Prices"},
	}, s.priceHistory)

	huma.Register(api, huma.Operation{
		OperationID: "price-overview", Method: http.MethodGet, Path: "/prices",
		Summary: "Price history for every destination", Tags: []string{"Prices"},
	}, s.priceOverview)

	huma.Register(api, huma.Operation{
		OperationID: "search-flights", Method: http.MethodPost, Path: "/search",
		Summary:     "Search flights now",
		Description: "Runs a one-off search and returns the cheapest flights. Nothing is stored.",
		Tags:        []string{"Searches"},
	}, s.search)

	huma.Register(api, huma.Operation{
		OperationID: "search-destination", Method: http.MethodPost, Path: "/destinations/{iata}/search",
		Summary: "Search one destination now", Tags: []string{"Searches"},
		Description: "Runs a flight search immediately, stores the result and returns it.",
	}, s.searchDestination)

	huma.Register(api, huma.Operation{
		OperationID: "run-search", Method: http.MethodPost, Path: "/searches",
		Summary: "Search all destinations now", Tags: []string{"Searches"},
		Description:   "Starts a background search of every destination. Poll `GET /searches/status` for progress.",
		DefaultStatus: http.StatusAccepted,
	}, s.runAll)

	huma.Register(api, huma.Operation{
		OperationID: "search-status", Method: http.MethodGet, Path: "/searches/status",
		Summary: "Status of the search runner", Tags: []string{"Searches"},
	}, s.status)
}

// ---- handlers ----

func (s *Server) health(ctx context.Context, _ *struct{}) (*healthOutput, error) {
	if err := s.store.Ping(ctx); err != nil {
		return nil, huma.Error503ServiceUnavailable("database unavailable")
	}
	out := &healthOutput{}
	out.Body.Status = "ok"
	return out, nil
}

func (s *Server) config(_ context.Context, _ *struct{}) (*configOutput, error) {
	out := &configOutput{}
	c, b := s.cfg, &out.Body
	b.Provider, b.DefaultAirport, b.Currency, b.OneWay = c.Search.Provider, c.Search.DefaultAirport, c.Search.Currency, c.Search.OneWay
	b.MaxMonthsAhead, b.Interval, b.IntervalSecs = c.Search.MaxMonthsAhead, c.Search.Interval.String(), int64(c.Search.Interval.Seconds())
	b.RunOnStartup, b.HistoryLimit = c.Search.RunOnStartup, c.Database.HistoryLimit
	b.NotifyEnabled, b.NotifyCount = c.Notify.Enabled, len(c.Notify.Recipients)
	return out, nil
}

func (s *Server) priceOverview(ctx context.Context, _ *struct{}) (*overviewOutput, error) {
	ds, err := s.store.ListDestinations(ctx)
	if err != nil {
		return nil, s.internal(err)
	}
	out := &overviewOutput{}
	out.Body.Histories = make([]priceHistory, 0, len(ds))
	for _, d := range ds {
		rs, err := s.store.ListResults(ctx, d.IATA, s.cfg.Database.HistoryLimit)
		if err != nil {
			return nil, s.internal(err)
		}
		out.Body.Histories = append(out.Body.Histories, priceHistory{Destination: d, Results: rs})
	}
	return out, nil
}

func (s *Server) search(ctx context.Context, in *searchInput) (*searchOutput, error) {
	ms, bad, ok := months.Normalize(in.Body.Months)
	if !ok {
		return nil, huma.Error422UnprocessableEntity("invalid month " + bad)
	}
	q := tracker.SearchQuery{
		Departure: strings.ToUpper(in.Body.From),
		Arrival:   strings.ToUpper(in.Body.To),
		Months:    ms,
		OneWay:    s.cfg.Search.OneWay,
	}
	if q.Departure == "" {
		q.Departure = s.cfg.Search.DefaultAirport
	}
	if in.Body.OneWay != nil {
		q.OneWay = *in.Body.OneWay
	}
	limit := in.Body.Limit
	if limit == 0 {
		limit = 20
	}
	flights, err := s.tracker.Search(ctx, q, limit)
	if err != nil {
		return nil, s.searchErr(err, q.Arrival)
	}
	out := &searchOutput{}
	out.Body.From, out.Body.To, out.Body.Currency, out.Body.OneWay, out.Body.Flights =
		q.Departure, q.Arrival, s.cfg.Search.Currency, q.OneWay, flights
	return out, nil
}

func (s *Server) listDestinations(ctx context.Context, _ *struct{}) (*destinationListOutput, error) {
	ds, err := s.store.ListDestinations(ctx)
	if err != nil {
		return nil, s.internal(err)
	}
	out := &destinationListOutput{}
	out.Body.Destinations = ds
	return out, nil
}

func (s *Server) createDestination(ctx context.Context, in *createDestinationInput) (*destinationOutput, error) {
	d, err := toDestination(in.Body.IATA, destinationFields{City: in.Body.City, MaxPrice: in.Body.MaxPrice, Months: in.Body.Months})
	if err != nil {
		return nil, err
	}
	switch err := s.store.CreateDestination(ctx, &d); {
	case errors.Is(err, store.ErrExists):
		return nil, huma.Error409Conflict("destination " + d.IATA + " already exists")
	case err != nil:
		return nil, s.internal(err)
	}
	return &destinationOutput{Body: d}, nil
}

func (s *Server) getDestination(ctx context.Context, in *iataParam) (*destinationOutput, error) {
	d, err := s.store.GetDestination(ctx, in.code())
	if err != nil {
		return nil, s.storeErr(err, in.code())
	}
	return &destinationOutput{Body: d}, nil
}

func (s *Server) updateDestination(ctx context.Context, in *updateDestinationInput) (*destinationOutput, error) {
	d, err := toDestination(in.IATA, in.Body)
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateDestination(ctx, &d); err != nil {
		return nil, s.storeErr(err, d.IATA)
	}
	return &destinationOutput{Body: d}, nil
}

func (s *Server) deleteDestination(ctx context.Context, in *iataParam) (*struct{}, error) {
	if err := s.store.DeleteDestination(ctx, in.code()); err != nil {
		return nil, s.storeErr(err, in.code())
	}
	return nil, nil
}

func (s *Server) priceHistory(ctx context.Context, in *historyInput) (*historyOutput, error) {
	iata := strings.ToUpper(in.IATA)
	d, err := s.store.GetDestination(ctx, iata)
	if err != nil {
		return nil, s.storeErr(err, iata)
	}
	rs, err := s.store.ListResults(ctx, d.IATA, in.Limit)
	if err != nil {
		return nil, s.internal(err)
	}
	out := &historyOutput{}
	out.Body.Destination, out.Body.Results = d, rs
	return out, nil
}

func (s *Server) searchDestination(ctx context.Context, in *iataParam) (*searchResultOutput, error) {
	d, err := s.store.GetDestination(ctx, in.code())
	if err != nil {
		return nil, s.storeErr(err, in.code())
	}
	r, err := s.tracker.SearchDestination(ctx, d)
	if err != nil {
		return nil, s.searchErr(err, d.IATA)
	}
	return &searchResultOutput{Body: r}, nil
}

func (s *Server) searchErr(err error, iata string) error {
	switch {
	case errors.Is(err, tracker.ErrNotConfigured):
		return huma.Error503ServiceUnavailable(err.Error())
	case errors.Is(err, tracker.ErrNoMonthsInRange):
		return huma.Error422UnprocessableEntity(err.Error())
	default:
		s.log.Error("search failed", "iata", iata, "err", err)
		return huma.NewError(http.StatusBadGateway, "flight search failed", err)
	}
}

func (s *Server) runAll(_ context.Context, _ *struct{}) (*runOutput, error) {
	if !s.tracker.Configured() {
		return nil, huma.Error503ServiceUnavailable(tracker.ErrNotConfigured.Error())
	}
	if s.tracker.Running() {
		return nil, huma.Error409Conflict(tracker.ErrBusy.Error())
	}
	go func() {
		if err := s.tracker.RunAll(s.bgCtx); err != nil && !errors.Is(err, tracker.ErrBusy) {
			s.log.Error("search run failed", "err", err)
		}
	}()
	out := &runOutput{}
	out.Body.Status = "started"
	return out, nil
}

func (s *Server) status(_ context.Context, _ *struct{}) (*statusOutput, error) {
	out := &statusOutput{}
	out.Body.Running = s.tracker.Running()
	out.Body.Configured = s.tracker.Configured()
	if next := s.tracker.NextRun(); !next.IsZero() {
		out.Body.NextRunAt = &next
	}
	if at, err := s.tracker.LastRun(); !at.IsZero() {
		out.Body.LastRunAt = &at
		if err != nil {
			out.Body.LastRunError = err.Error()
		}
	}
	return out, nil
}

// ---- helpers ----

func toDestination(iata string, f destinationFields) (model.Destination, error) {
	ms, bad, ok := months.Normalize(f.Months)
	if !ok {
		return model.Destination{}, huma.Error422UnprocessableEntity("invalid month " + bad)
	}
	return model.Destination{
		IATA:     strings.ToUpper(iata),
		City:     strings.TrimSpace(f.City),
		MaxPrice: f.MaxPrice,
		Months:   ms,
	}, nil
}

func (s *Server) storeErr(err error, iata string) error {
	if errors.Is(err, store.ErrNotFound) {
		return huma.Error404NotFound("destination " + iata + " not found")
	}
	return s.internal(err)
}

func (s *Server) internal(err error) error {
	s.log.Error("internal error", "err", err)
	return huma.Error500InternalServerError("internal error")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Debug("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start))
	})
}
