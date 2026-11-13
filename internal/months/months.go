// Package months converts between three-letter month names and month numbers.
package months

import "strings"

// Names lists valid month names in calendar order.
var Names = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}

// Number returns the month number (1-12) for a name such as "Jan" or "jan".
func Number(name string) (int, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, n := range Names {
		if n == name {
			return i + 1, true
		}
	}
	return 0, false
}

// Normalize lower-cases and trims every name and reports the first invalid one.
func Normalize(names []string) ([]string, string, bool) {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if _, ok := Number(n); !ok {
			return nil, n, false
		}
		out = append(out, strings.ToLower(strings.TrimSpace(n)))
	}
	return out, "", true
}

// Ahead returns how many months month lies ahead of current (0-11).
func Ahead(month, current int) int {
	return ((month-current)%12 + 12) % 12
}
