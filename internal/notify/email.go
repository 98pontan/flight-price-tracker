// Package notify sends deal notifications.
package notify

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"flight-price-tracker/internal/model"
)

// Notifier is told about flights cheaper than a destination's max price.
type Notifier interface {
	NotifyDeal(ctx context.Context, d model.Destination, r model.SearchResult) error
}

// Noop discards notifications.
type Noop struct{}

func (Noop) NotifyDeal(context.Context, model.Destination, model.SearchResult) error { return nil }

// Email sends notifications over SMTP (STARTTLS is used when the server offers it).
type Email struct {
	Host       string
	Port       int
	From       string
	Password   string
	Recipients []string
}

func (e *Email) NotifyDeal(_ context.Context, d model.Destination, r model.SearchResult) error {
	if r.LowestPrice == nil {
		return nil
	}
	subject := fmt.Sprintf("Cheap flight to %s: %d %s", d.City, *r.LowestPrice, r.Currency)
	trip := "round trip"
	if r.OneWay {
		trip = "one way"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found a %s flight from %s to %s (%s) for %d %s, below your limit of %d %s.\n\n",
		trip, r.Departure, d.City, d.IATA, *r.LowestPrice, r.Currency, d.MaxPrice, r.Currency)
	if r.StartDate != "" {
		fmt.Fprintf(&b, "Dates: %s - %s\n", r.StartDate, r.EndDate)
	}
	if r.Airline != "" {
		fmt.Fprintf(&b, "Airline: %s\n", r.Airline)
	}

	msg := "From: " + e.From + "\r\n" +
		"To: " + strings.Join(e.Recipients, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		strings.ReplaceAll(b.String(), "\n", "\r\n")

	auth := smtp.PlainAuth("", e.From, e.Password, e.Host)
	addr := net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
	if err := smtp.SendMail(addr, auth, e.From, e.Recipients, []byte(msg)); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}
