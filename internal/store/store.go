// Package store persists destinations and their search history in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no CGO required

	"flight-price-tracker/internal/model"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

const timeFormat = time.RFC3339Nano

const schema = `
CREATE TABLE IF NOT EXISTS destinations (
	iata       TEXT PRIMARY KEY,
	city       TEXT NOT NULL,
	max_price  INTEGER NOT NULL,
	months     TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS search_results (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	iata          TEXT NOT NULL REFERENCES destinations(iata) ON DELETE CASCADE,
	searched_at   TEXT NOT NULL,
	departure     TEXT NOT NULL,
	currency      TEXT NOT NULL,
	one_way       INTEGER NOT NULL,
	lowest_price  INTEGER,
	start_date    TEXT NOT NULL DEFAULT '',
	end_date      TEXT NOT NULL DEFAULT '',
	airline       TEXT NOT NULL DEFAULT '',
	flights_found INTEGER NOT NULL,
	below_max     INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_search_results_iata ON search_results(iata, id DESC);
`

type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens (creating if needed) the SQLite database at path and applies the schema.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows a single writer; one connection avoids "database is locked" errors.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// SeedDestinations inserts destinations that don't already exist and returns how many were added.
// Destinations that already exist are left alone so changes made through the API are kept.
func (s *Store) SeedDestinations(ctx context.Context, ds []model.Destination) (int, error) {
	added := 0
	for _, d := range ds {
		err := s.CreateDestination(ctx, &d)
		switch {
		case err == nil:
			added++
		case errors.Is(err, ErrExists):
		default:
			return added, err
		}
	}
	return added, nil
}

const destCols = `iata, city, max_price, months, created_at, updated_at`

func (s *Store) ListDestinations(ctx context.Context) ([]model.Destination, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+destCols+` FROM destinations ORDER BY iata`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Destination{}
	for rows.Next() {
		d, err := scanDestination(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDestination(ctx context.Context, iata string) (model.Destination, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+destCols+` FROM destinations WHERE iata = ?`, iata)
	d, err := scanDestination(row)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// CreateDestination inserts d and fills in its timestamps. It returns ErrExists if the IATA code is taken.
func (s *Store) CreateDestination(ctx context.Context, d *model.Destination) error {
	now := s.now()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO destinations (`+destCols+`) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(iata) DO NOTHING`,
		d.IATA, d.City, d.MaxPrice, strings.Join(d.Months, ","), now.Format(timeFormat), now.Format(timeFormat))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrExists
	}
	d.CreatedAt, d.UpdatedAt = now, now
	return nil
}

// UpdateDestination replaces city, max price and months of an existing destination.
func (s *Store) UpdateDestination(ctx context.Context, d *model.Destination) error {
	now := s.now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE destinations SET city = ?, max_price = ?, months = ?, updated_at = ? WHERE iata = ?`,
		d.City, d.MaxPrice, strings.Join(d.Months, ","), now.Format(timeFormat), d.IATA)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	updated, err := s.GetDestination(ctx, d.IATA)
	if err != nil {
		return err
	}
	*d = updated
	return nil
}

// DeleteDestination removes a destination and its search history.
func (s *Store) DeleteDestination(ctx context.Context, iata string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM destinations WHERE iata = ?`, iata)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddResult stores r and prunes the destination's history down to the keep most recent results.
func (s *Store) AddResult(ctx context.Context, r *model.SearchResult, keep int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `INSERT INTO search_results
		(iata, searched_at, departure, currency, one_way, lowest_price, start_date, end_date, airline, flights_found, below_max)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.IATA, r.SearchedAt.UTC().Format(timeFormat), r.Departure, r.Currency, r.OneWay,
		r.LowestPrice, r.StartDate, r.EndDate, r.Airline, r.FlightsFound, r.BelowMax)
	if err != nil {
		return err
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM search_results WHERE iata = ? AND id NOT IN (
		SELECT id FROM search_results WHERE iata = ? ORDER BY id DESC LIMIT ?)`, r.IATA, r.IATA, keep); err != nil {
		return err
	}
	return tx.Commit()
}

// ListResults returns up to limit of the most recent results for iata, oldest first.
func (s *Store) ListResults(ctx context.Context, iata string, limit int) ([]model.SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM (
		SELECT id, iata, searched_at, departure, currency, one_way, lowest_price, start_date, end_date, airline, flights_found, below_max
		FROM search_results WHERE iata = ? ORDER BY id DESC LIMIT ?) ORDER BY id ASC`, iata, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SearchResult{}
	for rows.Next() {
		var (
			r     model.SearchResult
			at    string
			price sql.NullInt64
		)
		if err := rows.Scan(&r.ID, &r.IATA, &at, &r.Departure, &r.Currency, &r.OneWay, &price,
			&r.StartDate, &r.EndDate, &r.Airline, &r.FlightsFound, &r.BelowMax); err != nil {
			return nil, err
		}
		if r.SearchedAt, err = time.Parse(timeFormat, at); err != nil {
			return nil, err
		}
		if price.Valid {
			p := int(price.Int64)
			r.LowestPrice = &p
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanDestination(sc scanner) (model.Destination, error) {
	var (
		d                model.Destination
		ms, created, upd string
	)
	if err := sc.Scan(&d.IATA, &d.City, &d.MaxPrice, &ms, &created, &upd); err != nil {
		return d, err
	}
	d.Months = strings.Split(ms, ",")
	var err error
	if d.CreatedAt, err = time.Parse(timeFormat, created); err != nil {
		return d, err
	}
	d.UpdatedAt, err = time.Parse(timeFormat, upd)
	return d, err
}
