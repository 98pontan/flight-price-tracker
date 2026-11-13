// Package model holds the types shared by the store, the tracker and the HTTP API.
package model

import "time"

// Destination is a place to watch for cheap flights.
type Destination struct {
	IATA      string    `json:"iata" doc:"IATA code of the destination airport or city" example:"BCN"`
	City      string    `json:"city" doc:"Human readable name" example:"Barcelona"`
	MaxPrice  int       `json:"max_price" doc:"Send a notification when a flight is cheaper than this" example:"1500"`
	Months    []string  `json:"months" enum:"jan,feb,mar,apr,may,jun,jul,aug,sep,oct,nov,dec" doc:"Months to search, as three-letter names" example:"[\"jun\",\"jul\"]"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SearchResult is the cheapest flight found for a destination during one search run.
type SearchResult struct {
	ID           int64     `json:"id"`
	IATA         string    `json:"iata" example:"BCN"`
	SearchedAt   time.Time `json:"searched_at" doc:"When the search was performed (UTC)"`
	Departure    string    `json:"departure" doc:"Departure airport" example:"ARN"`
	Currency     string    `json:"currency" example:"SEK"`
	OneWay       bool      `json:"one_way"`
	LowestPrice  *int      `json:"lowest_price" doc:"Cheapest price found, null if no flights were found"`
	StartDate    string    `json:"start_date,omitempty" doc:"Outbound date of the cheapest flight" example:"2027-06-12"`
	EndDate      string    `json:"end_date,omitempty" doc:"Return date of the cheapest flight" example:"2027-06-19"`
	Airline      string    `json:"airline,omitempty"`
	FlightsFound int       `json:"flights_found" doc:"Number of priced flights returned by the search"`
	BelowMax     bool      `json:"below_max_price" doc:"Whether lowest_price was below the destination's max_price"`
}

// Flight is one priced flight returned by an ad-hoc search.
type Flight struct {
	Month     string `json:"month" doc:"Month that was searched" example:"jun"`
	Price     int    `json:"price" example:"1290"`
	StartDate string `json:"start_date,omitempty" example:"2027-06-12"`
	EndDate   string `json:"end_date,omitempty" doc:"Return date, empty for one-way flights" example:"2027-06-19"`
	Airline   string `json:"airline,omitempty" example:"SAS"`
}
