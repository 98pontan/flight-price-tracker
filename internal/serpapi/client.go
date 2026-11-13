// Package serpapi is a small client for SerpApi's Google Travel Explore engine.
package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://serpapi.com/search.json"

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func New(apiKey string) *Client {
	return &Client{apiKey: apiKey, baseURL: defaultBaseURL, http: &http.Client{Timeout: 60 * time.Second}}
}

type ExploreParams struct {
	Departure string
	Arrival   string
	Currency  string
	OneWay    bool
	Month     int // 1-12
}

type Flight struct {
	Price     float64
	StartDate string
	EndDate   string
	Airline   string
}

type ExploreResult struct {
	StartDate string
	EndDate   string
	Flights   []Flight // only flights that have a price
}

type rawFlight struct {
	Price       *float64 `json:"price"`
	FlightPrice *float64 `json:"flight_price"`
	StartDate   string   `json:"start_date"`
	EndDate     string   `json:"end_date"`
	Airline     string   `json:"airline"`
}

type rawResponse struct {
	Error     string      `json:"error"`
	StartDate string      `json:"start_date"`
	EndDate   string      `json:"end_date"`
	Flights   []rawFlight `json:"flights"`
}

// Explore searches for flights from Departure to Arrival during Month.
func (c *Client) Explore(ctx context.Context, p ExploreParams) (*ExploreResult, error) {
	flightType := "1" // round trip
	if p.OneWay {
		flightType = "2"
	}
	q := url.Values{
		"engine":       {"google_travel_explore"},
		"departure_id": {p.Departure},
		"arrival_id":   {p.Arrival},
		"currency":     {p.Currency},
		"type":         {flightType},
		"month":        {strconv.Itoa(p.Month)},
		"api_key":      {c.apiKey},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// Don't leak the API key that is part of the URL.
		if ue, ok := err.(*url.Error); ok {
			err = ue.Err
		}
		return nil, fmt.Errorf("serpapi request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("serpapi read: %w", err)
	}
	var raw rawResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("serpapi: status %d, decode: %w", resp.StatusCode, err)
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("serpapi: %s", raw.Error)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("serpapi: unexpected status %d", resp.StatusCode)
	}

	out := &ExploreResult{StartDate: raw.StartDate, EndDate: raw.EndDate}
	for _, f := range raw.Flights {
		price := f.Price
		if price == nil {
			price = f.FlightPrice
		}
		if price == nil || *price <= 0 {
			continue
		}
		fl := Flight{Price: *price, StartDate: f.StartDate, EndDate: f.EndDate, Airline: f.Airline}
		if fl.StartDate == "" {
			fl.StartDate, fl.EndDate = raw.StartDate, raw.EndDate
		}
		out.Flights = append(out.Flights, fl)
	}
	return out, nil
}
