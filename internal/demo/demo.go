// Package demo is a stand-in flight search that makes up plausible prices, so the
// server and UI can be used without a SerpApi key. Enable it with search.provider: demo.
package demo

import (
	"context"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"time"

	"flight-price-tracker/internal/serpapi"
)

var airlines = []string{"SAS", "Norwegian", "Finnair", "Lufthansa", "KLM", "airBaltic", "Ryanair", "Wizz Air"}

type Searcher struct {
	now func() time.Time
}

func New() *Searcher { return &Searcher{now: time.Now} }

// Explore returns 4-11 flights for the month. Each route has its own base price, summer and
// December cost more, and every call adds noise so repeated searches form a price trend.
func (s *Searcher) Explore(ctx context.Context, p serpapi.ExploreParams) (*serpapi.ExploreResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h := fnv.New32a()
	h.Write([]byte(p.Departure + "-" + p.Arrival))
	base := 500 + float64(h.Sum32()%2000)
	if !p.OneWay {
		base *= 1.7
	}
	season := 1 + 0.25*math.Cos(float64(p.Month-7)/12*2*math.Pi)
	if p.Month == 12 {
		season += 0.2
	}
	drift := 1 + rand.NormFloat64()*0.08

	now := s.now()
	year := now.Year()
	if p.Month < int(now.Month()) {
		year++
	}
	first := time.Date(year, time.Month(p.Month), 1, 0, 0, 0, 0, time.UTC)
	days := first.AddDate(0, 1, -1).Day()

	res := &serpapi.ExploreResult{
		StartDate: first.Format(time.DateOnly),
		EndDate:   first.AddDate(0, 0, days-1).Format(time.DateOnly),
	}
	for range 4 + rand.IntN(8) {
		start := first.AddDate(0, 0, rand.IntN(days))
		f := serpapi.Flight{
			Price:     math.Round(base * season * drift * (0.8 + rand.Float64()*0.6)),
			StartDate: start.Format(time.DateOnly),
			Airline:   airlines[rand.IntN(len(airlines))],
		}
		if !p.OneWay {
			f.EndDate = start.AddDate(0, 0, 3+rand.IntN(8)).Format(time.DateOnly)
		}
		res.Flights = append(res.Flights, f)
	}
	return res, nil
}
