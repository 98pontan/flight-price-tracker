package tracker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"flight-price-tracker/internal/config"
	"flight-price-tracker/internal/model"
	"flight-price-tracker/internal/serpapi"
	"flight-price-tracker/internal/store"
)

type fakeSearcher struct {
	byMonth map[int][]float64
	calls   []int
}

func (f *fakeSearcher) Explore(_ context.Context, p serpapi.ExploreParams) (*serpapi.ExploreResult, error) {
	f.calls = append(f.calls, p.Month)
	prices, ok := f.byMonth[p.Month]
	if !ok {
		return nil, errors.New("boom")
	}
	res := &serpapi.ExploreResult{}
	for _, pr := range prices {
		res.Flights = append(res.Flights, serpapi.Flight{Price: pr, StartDate: "s", EndDate: "e"})
	}
	return res, nil
}

type recNotifier struct{ n int }

func (r *recNotifier) NotifyDeal(context.Context, model.Destination, model.SearchResult) error {
	r.n++
	return nil
}

func setup(t *testing.T, fs Searcher, n *recNotifier, limit int) (*Tracker, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Default().Search
	tr := New(st, fs, n, cfg, limit, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tr.now = func() time.Time { return time.Date(2026, time.November, 13, 0, 0, 0, 0, time.UTC) }
	return tr, st
}

func TestSearchPicksCheapestAndSkipsOutOfRangeMonths(t *testing.T) {
	fs := &fakeSearcher{byMonth: map[int][]float64{1: {900, 450.4}, 3: {700}}}
	n := &recNotifier{}
	tr, st := setup(t, fs, n, 10)
	d := model.Destination{IATA: "RIX", City: "Riga", MaxPrice: 500, Months: []string{"jan", "mar", "jul"}}
	if err := st.CreateDestination(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	r, err := tr.SearchDestination(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	// From November with a 6 month window, jul (8 months ahead) is skipped.
	if len(fs.calls) != 2 {
		t.Fatalf("searched months %v, want jan and mar only", fs.calls)
	}
	if r.LowestPrice == nil || *r.LowestPrice != 450 || !r.BelowMax || r.FlightsFound != 3 {
		t.Fatalf("unexpected result %+v", r)
	}
	if n.n != 1 {
		t.Fatalf("notifications = %d, want 1", n.n)
	}
}

func TestNoMonthsInRange(t *testing.T) {
	tr, _ := setup(t, &fakeSearcher{}, &recNotifier{}, 10)
	_, err := tr.SearchDestination(context.Background(), model.Destination{IATA: "XXX", Months: []string{"jul"}})
	if !errors.Is(err, ErrNoMonthsInRange) {
		t.Fatalf("err = %v", err)
	}
}

func TestHistoryIsPrunedToLimit(t *testing.T) {
	fs := &fakeSearcher{byMonth: map[int][]float64{1: {1000}}}
	tr, st := setup(t, fs, &recNotifier{}, 3)
	d := model.Destination{IATA: "RIX", City: "Riga", MaxPrice: 500, Months: []string{"jan"}}
	ctx := context.Background()
	if err := st.CreateDestination(ctx, &d); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for range 5 {
		r, err := tr.SearchDestination(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	rs, err := st.ListResults(ctx, "RIX", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 || rs[0].ID != ids[2] || rs[2].ID != ids[4] {
		t.Fatalf("got %d results, want the last 3 oldest-first", len(rs))
	}
	if err := st.DeleteDestination(ctx, "RIX"); err != nil {
		t.Fatal(err)
	}
	if rs, _ := st.ListResults(ctx, "RIX", 100); len(rs) != 0 {
		t.Fatal("history not deleted with destination")
	}
}
