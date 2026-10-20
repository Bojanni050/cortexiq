// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// monthAlt matches every month name (and abbreviation) monthNames knows.
const monthAlt = `january|jan|januari|february|feb|februari|march|mar|maart|april|apr|may|mei|june|jun|juni|july|jul|juli|august|aug|augustus|september|sep|sept|october|oct|oktober|okt|november|nov|december|dec`

var (
	relativeDaysRE = regexp.MustCompile(`(?i)\b(\d{1,3})\s+(?:days? ago|dagen geleden)\b`)
	monthYearRE    = regexp.MustCompile(`(?i)\b(?:in\s+)?([[:alpha:]]+)\s+(\d{4})\b`)
	monthOnlyRE    = regexp.MustCompile(`(?i)\b(?:as of|in|during|on|tijdens|op|in de maand)\s+(` + monthAlt + `)\b`)
	lastMonthName  = regexp.MustCompile(`(?i)\b(?:last|previous|vorige|vorig|afgelopen)\s+(` + monthAlt + `)\b`)

	// knowledgeCueRE finds past-tense epistemic wording: the question is about what
	// the system held, not about what happened. Present tense ("what do we know")
	// is deliberately absent: that asks what is known now about a period.
	knowledgeCueRE = regexp.MustCompile(`\b(?:` +
		`did (?:we|i|you) (?:know|believe|think|remember)|` +
		`(?:we|i|you|they) (?:knew|believed|remembered)|` +
		`(?:was|were) (?:known|believed|recorded)|` +
		`had (?:on file|recorded|stored)|` +
		`as of|` +
		`wat (?:wisten|wist|dachten|geloofden) (?:we|ik|wij|jij|je)|` +
		`(?:wisten|wist|geloofden|herinnerden) (?:we|ik|wij)|` +
		`(?:was|waren) (?:er )?bekend|` +
		`hadden we (?:vastgelegd|opgeslagen|genoteerd)` +
		`)\b`)

	// aspectRE finds wording that attaches a time expression to the subject of the
	// question ("know about the January outage") instead of to the knowing.
	aspectRE = regexp.MustCompile(`\b(?:about|regarding|concerning|omtrent|betreffende|aangaande|van|over)\b`)
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

// phraseWindows are the fixed relative phrases, in priority order.
var phraseWindows = []struct {
	re     *regexp.Regexp
	window func(today time.Time) (time.Time, time.Time)
}{
	{phraseRE("yesterday", "gisteren"), func(t time.Time) (time.Time, time.Time) { return t.AddDate(0, 0, -1), t }},
	{phraseRE("today", "vandaag"), func(t time.Time) (time.Time, time.Time) { return t, t.AddDate(0, 0, 1) }},
	{phraseRE("last week", "previous week", "vorige week", "afgelopen week"), func(t time.Time) (time.Time, time.Time) {
		monday := weekStart(t)
		return monday.AddDate(0, 0, -7), monday
	}},
	{phraseRE("this week", "deze week"), func(t time.Time) (time.Time, time.Time) {
		return weekStart(t), weekStart(t).AddDate(0, 0, 7)
	}},
	{phraseRE("last month", "previous month", "vorige maand", "afgelopen maand"), func(t time.Time) (time.Time, time.Time) {
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start.AddDate(0, -1, 0), start
	}},
	{phraseRE("this month", "deze maand"), func(t time.Time) (time.Time, time.Time) {
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)
	}},
	{phraseRE("last year", "previous year", "vorig jaar", "afgelopen jaar"), func(t time.Time) (time.Time, time.Time) {
		start := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return start.AddDate(-1, 0, 0), start
	}},
}

func phraseRE(words ...string) *regexp.Regexp {
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(`\b(?:` + strings.Join(quoted, "|") + `)\b`)
}

// timeExpr is one calendar expression found in a query: a half-open UTC window,
// where it sits in the lowercased query, and its priority (lower wins).
type timeExpr struct {
	from, to   time.Time
	start, end int
	rank       int
}

// scanTime finds every supported calendar expression in the query. Expressions
// that overlap (such as "in march 2025", which both the month-year and the
// month-only pattern match) are reduced to the one with the better rank. The
// result is ordered by position.
func scanTime(query string, now time.Time) []timeExpr {
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	lower := strings.ToLower(query)
	var found []timeExpr
	add := func(from, to time.Time, start, end, rank int) {
		found = append(found, timeExpr{from: from, to: to, start: start, end: end, rank: rank})
	}

	for rank, pw := range phraseWindows {
		for _, loc := range pw.re.FindAllStringIndex(lower, -1) {
			from, to := pw.window(today)
			add(from, to, loc[0], loc[1], rank)
		}
	}
	rank := len(phraseWindows)

	for _, m := range relativeDaysRE.FindAllStringSubmatchIndex(lower, -1) {
		days, err := strconv.Atoi(lower[m[2]:m[3]])
		if err == nil && days > 0 {
			day := today.AddDate(0, 0, -days)
			add(day, day.AddDate(0, 0, 1), m[0], m[1], rank)
		}
	}
	rank++

	for _, m := range monthYearRE.FindAllStringSubmatchIndex(lower, -1) {
		if month, ok := monthNames[lower[m[2]:m[3]]]; ok {
			year, _ := strconv.Atoi(lower[m[4]:m[5]])
			start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
			add(start, start.AddDate(0, 1, 0), m[0], m[1], rank)
		}
	}
	rank++

	// "last March" is the most recent March that is already over, so naming the
	// current month means a year ago.
	for _, m := range lastMonthName.FindAllStringSubmatchIndex(lower, -1) {
		month := monthNames[lower[m[2]:m[3]]]
		year := today.Year()
		if month >= today.Month() {
			year--
		}
		start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		add(start, start.AddDate(0, 1, 0), m[0], m[1], rank)
	}
	rank++

	monthOnly := func(token string, start, end int) {
		month, ok := monthNames[token]
		if !ok {
			return
		}
		year := today.Year()
		if month > today.Month() {
			year--
		}
		from := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		add(from, from.AddDate(0, 1, 0), start, end, rank)
	}
	for _, m := range monthOnlyRE.FindAllStringSubmatchIndex(lower, -1) {
		monthOnly(lower[m[2]:m[3]], m[0], m[1])
	}
	if trimmed := strings.TrimSpace(lower); monthNames[trimmed] != 0 {
		at := strings.Index(lower, trimmed)
		monthOnly(trimmed, at, at+len(trimmed))
	}

	// Keep the best-ranked expression of any overlapping group.
	sort.SliceStable(found, func(i, j int) bool { return found[i].rank < found[j].rank })
	var kept []timeExpr
	for _, e := range found {
		overlaps := false
		for _, k := range kept {
			if e.start < k.end && k.start < e.end {
				overlaps = true
				break
			}
		}
		if !overlaps {
			kept = append(kept, e)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].start < kept[j].start })
	return kept
}

// temporalWindow extracts a deliberately small, deterministic set of Dutch and
// English calendar expressions. Bounds are UTC and half-open [from, to).
// Returning nil bounds means the query contains no supported time expression.
// With several expressions the highest-priority one wins; inferTimeAxes is the
// variant that tells knowledge time from valid time.
func temporalWindow(query string, now time.Time) (from, to *time.Time) {
	var best *timeExpr
	exprs := scanTime(query, now)
	for i := range exprs {
		if best == nil || exprs[i].rank < best.rank {
			best = &exprs[i]
		}
	}
	if best == nil {
		return nil, nil
	}
	a, b := best.from, best.to
	return &a, &b
}

// timeAxes is what a query's wording says about time, split by axis: Known* is
// the window of what the system held (knowledge time), From/To the window of
// what happened or was true (valid time). Nil bounds mean "not stated".
type timeAxes struct {
	KnownFrom, KnownTo *time.Time
	From, To           *time.Time
}

// inferTimeAxes reads the time expressions in a query and assigns each to an axis.
// Without past-tense epistemic wording ("what did we know", "as of") the best
// expression is a valid-time window, exactly as temporalWindow returns it. With
// such a cue, the first expression becomes the knowledge window ("what did we
// know in March"), unless wording like "about" or "over" sits between it and the
// cue, which ties it to the subject instead ("what did we know about the outage
// in March"): that one stays a valid-time window.
func inferTimeAxes(query string, now time.Time) timeAxes {
	lower := strings.ToLower(query)
	cue := knowledgeCueRE.FindStringIndex(lower)
	if cue == nil {
		from, to := temporalWindow(query, now)
		return timeAxes{From: from, To: to}
	}
	var axes timeAxes
	for _, e := range scanTime(query, now) {
		var gap string
		switch {
		case e.start >= cue[1]:
			gap = lower[cue[1]:e.start]
		case e.end <= cue[0]:
			gap = lower[e.end:cue[0]]
		}
		from, to := e.from, e.to
		if aspectRE.MatchString(gap) {
			if axes.From == nil {
				axes.From, axes.To = &from, &to
			}
			continue
		}
		if axes.KnownFrom == nil {
			axes.KnownFrom, axes.KnownTo = &from, &to
		}
	}
	return axes
}

func weekStart(day time.Time) time.Time {
	weekday := (int(day.Weekday()) + 6) % 7 // Monday = 0
	return day.AddDate(0, 0, -weekday)
}
