// Package tracker runs flight searches for destinations and records the results.
package tracker

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"flight-price-tracker/internal/config"
	"flight-price-tracker/internal/model"
	"flight-price-tracker/internal/months"
	"flight-price-tracker/internal/notify"
	"flight-price-tracker/internal/serpapi"
	"flight-price-tracker/internal/store"
)

var (
	ErrNotConfigured   = errors.New("flight search is not configured: SERPAPI_KEY is missing")
	ErrNoMonthsInRange = errors.New("none of the destination's months are within the search window")
	ErrBusy            = errors.New("a search run is already in progress")
)

// Searcher is the flight search backend (SerpApi in production).
type Searcher interface {
	Explore(ctx context.Context, p serpapi.ExploreParams) (*serpapi.ExploreResult, error)
}

type Tracker struct {
	store        *store.Store
	searcher     Searcher // nil when no API key is configured
	notifier     notify.Notifier
	cfg          config.Search
	historyLimit int
	log          *slog.Logger
	now          func() time.Time

	running sync.Mutex
	lastRun struct {
		sync.Mutex
		at   time.Time
		err  error
		next time.Time
	}
}

func New(st *store.Store, s Searcher, n notify.Notifier, cfg config.Search, historyLimit int, log *slog.Logger) *Tracker {
	if n == nil {
		n = notify.Noop{}
	}
	return &Tracker{store: st, searcher: s, notifier: n, cfg: cfg, historyLimit: historyLimit, log: log, now: time.Now}
}

// SearchQuery is an ad-hoc search that is not stored.
type SearchQuery struct {
	Departure string
	Arrival   string
	Months    []string
	OneWay    bool
}

// Search runs an ad-hoc search and returns up to limit flights, cheapest first. Nothing is stored.
func (t *Tracker) Search(ctx context.Context, q SearchQuery, limit int) ([]model.Flight, error) {
	if q.Departure == "" {
		q.Departure = t.cfg.DefaultAirport
	}
	flights, err := t.explore(ctx, q.Departure, q.Arrival, q.OneWay, q.Months)
	if err != nil {
		return nil, err
	}
	if len(flights) > limit {
		flights = flights[:limit]
	}
	return flights, nil
}

// explore searches each month within the search window and returns all priced flights, cheapest
// first. It fails only if every month's search failed.
func (t *Tracker) explore(ctx context.Context, departure, arrival string, oneWay bool, names []string) ([]model.Flight, error) {
	if t.searcher == nil {
		return nil, ErrNotConfigured
	}
	current := int(t.now().Month())
	var targets []int
	for _, name := range names {
		if m, ok := months.Number(name); ok && months.Ahead(m, current) < t.cfg.MaxMonthsAhead {
			targets = append(targets, m)
		}
	}
	if len(targets) == 0 {
		return nil, ErrNoMonthsInRange
	}

	var (
		flights   = []model.Flight{}
		errs      []error
		succeeded int
	)
	for _, m := range targets {
		res, err := t.searcher.Explore(ctx, serpapi.ExploreParams{
			Departure: departure,
			Arrival:   arrival,
			Currency:  t.cfg.Currency,
			OneWay:    oneWay,
			Month:     m,
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("month %d: %w", m, err))
			continue
		}
		succeeded++
		for _, f := range res.Flights {
			flights = append(flights, model.Flight{
				Month:     months.Names[m-1],
				Price:     int(math.Round(f.Price)),
				StartDate: f.StartDate,
				EndDate:   f.EndDate,
				Airline:   f.Airline,
			})
		}
	}
	if succeeded == 0 {
		return nil, errors.Join(errs...)
	}
	for _, err := range errs {
		t.log.Warn("partial search failure", "arrival", arrival, "err", err)
	}
	slices.SortStableFunc(flights, func(a, b model.Flight) int { return cmp.Compare(a.Price, b.Price) })
	return flights, nil
}

// SearchDestination searches every configured month of d that falls within the search window,
// stores the cheapest flight found and sends a notification if it is below d.MaxPrice.
func (t *Tracker) SearchDestination(ctx context.Context, d model.Destination) (model.SearchResult, error) {
	flights, err := t.explore(ctx, t.cfg.DefaultAirport, d.IATA, t.cfg.OneWay, d.Months)
	if err != nil {
		return model.SearchResult{}, err
	}
	r := model.SearchResult{
		IATA:         d.IATA,
		SearchedAt:   t.now().UTC(),
		Departure:    t.cfg.DefaultAirport,
		Currency:     t.cfg.Currency,
		OneWay:       t.cfg.OneWay,
		FlightsFound: len(flights),
	}
	if len(flights) > 0 {
		best := flights[0]
		r.LowestPrice = &best.Price
		r.StartDate, r.EndDate, r.Airline = best.StartDate, best.EndDate, best.Airline
		r.BelowMax = best.Price < d.MaxPrice
	}
	if err := t.store.AddResult(ctx, &r, t.historyLimit); err != nil {
		return r, fmt.Errorf("store result: %w", err)
	}
	t.log.Info("search done", "iata", d.IATA, "lowest_price", r.LowestPrice, "flights", len(flights), "below_max", r.BelowMax)

	if r.BelowMax {
		if err := t.notifier.NotifyDeal(ctx, d, r); err != nil {
			t.log.Error("notification failed", "iata", d.IATA, "err", err)
		}
	}
	return r, nil
}

// RunAll searches every destination. It returns ErrBusy if a run is already in progress.
func (t *Tracker) RunAll(ctx context.Context) error {
	if !t.running.TryLock() {
		return ErrBusy
	}
	defer t.running.Unlock()

	err := t.runAll(ctx)
	t.lastRun.Lock()
	t.lastRun.at, t.lastRun.err = t.now().UTC(), err
	t.lastRun.Unlock()
	return err
}

func (t *Tracker) runAll(ctx context.Context) error {
	if t.searcher == nil {
		return ErrNotConfigured
	}
	ds, err := t.store.ListDestinations(ctx)
	if err != nil {
		return err
	}
	t.log.Info("starting search run", "destinations", len(ds))
	var errs []error
	for _, d := range ds {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := t.SearchDestination(ctx, d); err != nil {
			if errors.Is(err, ErrNoMonthsInRange) {
				t.log.Info("skipping destination, no months in range", "iata", d.IATA)
				continue
			}
			t.log.Error("search failed", "iata", d.IATA, "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", d.IATA, err))
		}
	}
	return errors.Join(errs...)
}

// Running reports whether a full search run is in progress.
func (t *Tracker) Running() bool {
	if t.running.TryLock() {
		t.running.Unlock()
		return false
	}
	return true
}

// LastRun returns when the last full run finished and its error, if any.
func (t *Tracker) LastRun() (time.Time, error) {
	t.lastRun.Lock()
	defer t.lastRun.Unlock()
	return t.lastRun.at, t.lastRun.err
}

// Configured reports whether a search backend is available.
func (t *Tracker) Configured() bool { return t.searcher != nil }

// Schedule runs RunAll every cfg.Interval until ctx is cancelled.
func (t *Tracker) Schedule(ctx context.Context) {
	run := func() {
		if err := t.RunAll(ctx); err != nil && !errors.Is(err, ErrBusy) {
			t.log.Error("scheduled search run failed", "err", err)
		}
	}
	if t.cfg.RunOnStartup {
		run()
	}
	ticker := time.NewTicker(t.cfg.Interval)
	defer ticker.Stop()
	for {
		t.setNextRun(t.now().Add(t.cfg.Interval))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (t *Tracker) setNextRun(at time.Time) {
	t.lastRun.Lock()
	t.lastRun.next = at.UTC()
	t.lastRun.Unlock()
}

// NextRun returns when the scheduler runs next, or the zero time if it isn't running.
func (t *Tracker) NextRun() time.Time {
	t.lastRun.Lock()
	defer t.lastRun.Unlock()
	return t.lastRun.next
}
