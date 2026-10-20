// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	relativeDaysRE = regexp.MustCompile(`(?i)\b(\d{1,3})\s+(?:days? ago|dagen geleden)\b`)
	monthYearRE    = regexp.MustCompile(`(?i)\b(?:in\s+)?([[:alpha:]]+)\s+(\d{4})\b`)
	monthOnlyRE    = regexp.MustCompile(`(?i)\b(?:in|during|on|tijdens|op|in de maand)\s+(january|jan|januari|february|feb|februari|march|mar|maart|april|apr|may|mei|june|jun|juni|july|jul|juli|august|aug|augustus|september|sep|sept|october|oct|oktober|okt|november|nov|december|dec)\b`)
)

var monthNames = map[string]time.Month{
	"january": time.January, "jan": time.January, "januari": time.January,
	"february": time.February, "feb": time.February, "februari": time.February,
	"march": time.March, "mar": time.March, "maart": time.March,
	"april": time.April, "apr": time.April,
	"may": time.May, "mei": time.May,
	"june": time.June, "jun": time.June, "juni": time.June,
	"july": time.July, "jul": time.July, "juli": time.July,
	"august": time.August, "aug": time.August, "augustus": time.August,
	"september": time.September, "sep": time.September, "sept": time.September,
	"october": time.October, "oct": time.October, "oktober": time.October, "okt": time.October,
	"november": time.November, "nov": time.November,
	"december": time.December, "dec": time.December,
}

// temporalWindow extracts a deliberately small, deterministic set of Dutch and
// English calendar expressions. Bounds are UTC and half-open [from, to).
// Returning nil bounds means the query contains no supported time expression.
func temporalWindow(query string, now time.Time) (from, to *time.Time) {
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	lower := strings.ToLower(query)
	set := func(a, b time.Time) (*time.Time, *time.Time) { return &a, &b }
	contains := func(words ...string) bool {
		for _, word := range words {
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`).MatchString(lower) {
				return true
			}
		}
		return false
	}

	if contains("yesterday", "gisteren") {
		return set(today.AddDate(0, 0, -1), today)
	}
	if contains("today", "vandaag") {
		return set(today, today.AddDate(0, 0, 1))
	}
	if contains("last week", "previous week", "vorige week", "afgelopen week") {
		monday := weekStart(today)
		return set(monday.AddDate(0, 0, -7), monday)
	}
	if contains("this week", "deze week") {
		return set(weekStart(today), weekStart(today).AddDate(0, 0, 7))
	}
	if contains("last month", "previous month", "vorige maand", "afgelopen maand") {
		start := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		return set(start.AddDate(0, -1, 0), start)
	}
	if contains("this month", "deze maand") {
		start := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		return set(start, start.AddDate(0, 1, 0))
	}
	if contains("last year", "previous year", "vorig jaar", "afgelopen jaar") {
		start := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return set(start.AddDate(-1, 0, 0), start)
	}

	if match := relativeDaysRE.FindStringSubmatch(query); len(match) == 2 {
		days, err := strconv.Atoi(match[1])
		if err == nil && days > 0 {
			day := today.AddDate(0, 0, -days)
			return set(day, day.AddDate(0, 0, 1))
		}
	}

	if match := monthYearRE.FindStringSubmatch(lower); len(match) == 3 {
		if month, ok := monthNames[match[1]]; ok {
			year, _ := strconv.Atoi(match[2])
			start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
			return set(start, start.AddDate(0, 1, 0))
		}
	}
	monthToken := ""
	if match := monthOnlyRE.FindStringSubmatch(lower); len(match) == 2 {
		monthToken = match[1]
	} else if monthNames[strings.TrimSpace(lower)] != 0 {
		monthToken = strings.TrimSpace(lower)
	}
	if monthToken != "" {
		if month, ok := monthNames[monthToken]; ok {
			year := today.Year()
			if month > today.Month() {
				year--
			}
			start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
			return set(start, start.AddDate(0, 1, 0))
		}
	}
	return nil, nil
}

func weekStart(day time.Time) time.Time {
	weekday := (int(day.Weekday()) + 6) % 7 // Monday = 0
	return day.AddDate(0, 0, -weekday)
}
