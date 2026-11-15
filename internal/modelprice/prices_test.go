// Modifications copyright (c) 2026 Bojan van der Heide
package modelprice

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/llmreg"
	"github.com/Bojanni050/cortexiq/internal/observability"
)

// catalogue serves the Hopscotch shape: rates in thousandths of a dollar per
// million tokens, inside the uniblock block.
const hopscotchCatalogue = `{"data":[
	{"id":"deepinfra/gemma-4-31B-it-turbo","uniblock":{"input_rate_thousandths":90,"output_rate_thousandths":340,"currency_code":"USD"}},
	{"id":"deepinfra/Meta-Llama-3.1-8B-Instruct-Turbo","uniblock":{"input_rate_thousandths":20,"output_rate_thousandths":40,"currency_code":"USD"}},
	{"id":"local/embedding","uniblock":{"input_rate_thousandths":0,"output_rate_thousandths":0,"currency_code":"USD"}}
]}`

func catalogueServer(t *testing.T, body, wantKey string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantKey != "" {
			if got := r.Header.Get("Authorization"); got != "Bearer "+wantKey {
				t.Errorf("authorization = %q", got)
			}
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
}

func TestRefreshReadsPricesFromTheCatalogue(t *testing.T) {
	srv := catalogueServer(t, hopscotchCatalogue, "ub_live_test")
	defer srv.Close()

	r := NewRefresher(slog.Default(), []Source{{Name: "hopscotch", BaseURL: srv.URL + "/v1/", APIKey: "ub_live_test"}})
	if got := r.Refresh(context.Background()); got != 2 {
		t.Fatalf("priced models = %d, want 2 (the unpriced one must stay out)", got)
	}

	price, ok := r.bySource["hopscotch"]["deepinfra/gemma-4-31B-it-turbo"]
	if !ok {
		t.Fatal("the reasoner model is missing from the table")
	}
	if price.InputPerM != 0.09 || price.OutputPerM != 0.34 {
		t.Fatalf("price = %+v, want 0.09/0.34", price)
	}
}

func TestRefreshKeepsTheLastPricesOfAFailingProvider(t *testing.T) {
	good := catalogueServer(t, `{"data":[{"id":"a/one","uniblock":{"input_rate_thousandths":1000,"output_rate_thousandths":2000,"currency_code":"USD"}}]}`, "")
	defer good.Close()
	flaky := catalogueServer(t, `{"data":[{"id":"b/two","uniblock":{"input_rate_thousandths":3000,"output_rate_thousandths":4000,"currency_code":"USD"}}]}`, "")

	r := NewRefresher(slog.Default(), []Source{
		{Name: "good", BaseURL: good.URL + "/v1"},
		{Name: "flaky", BaseURL: flaky.URL + "/v1"},
	})
	if got := r.Refresh(context.Background()); got != 2 {
		t.Fatalf("first refresh priced %d models, want 2", got)
	}

	flaky.Close()
	// The healthy provider still answers, so the refresh succeeds and the
	// failing one's last known prices must survive instead of vanishing.
	if got := r.Refresh(context.Background()); got != 2 {
		t.Fatalf("after one provider failed the table holds %d models, want 2", got)
	}
}

func TestNewRefresherDropsDuplicateEndpoints(t *testing.T) {
	r := NewRefresher(nil, []Source{
		{Name: "a", BaseURL: "https://api.example.test/v1/"},
		{Name: "b", BaseURL: "https://api.example.test/v1"},
		{Name: "empty"},
	})
	if len(r.sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(r.sources))
	}
}

func TestRefreshWithoutSourcesKeepsTheTable(t *testing.T) {
	r := NewRefresher(nil, nil)
	if got := r.Refresh(context.Background()); got != 0 {
		t.Fatalf("priced %d models without any source", got)
	}
}

// fakePrices is the dashboard's table. Mutable, because a republish must see a
// rate saved after the first read.
type fakePrices struct{ rows []llmreg.ManualPrice }

func (f *fakePrices) ManualPrices(context.Context) ([]llmreg.ManualPrice, error) { return f.rows, nil }

// resetPrices clears the package-wide table so one test cannot price another.
func resetPrices(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { observability.SetModelPrices(nil) })
}

func TestManualPriceFillsAModelNoCataloguePrices(t *testing.T) {
	resetPrices(t)
	srv := catalogueServer(t, hopscotchCatalogue, "")
	defer srv.Close()

	manual := &fakePrices{rows: []llmreg.ManualPrice{{Model: "local/chat", InputPerMTok: 0.5, OutputPerMTok: 1.5}}}
	r := NewRefresher(slog.Default(), []Source{{Name: "hopscotch", BaseURL: srv.URL + "/v1"}})
	r.SetManualPrices(manual)
	if got := r.Refresh(context.Background()); got != 2 {
		t.Fatalf("catalogued models = %d, want 2", got)
	}

	if p, ok := observability.LookupPrice("local/chat"); !ok || p.InputPerM != 0.5 || p.OutputPerM != 1.5 {
		t.Fatalf("manual price = %+v, ok=%v", p, ok)
	}
	// Merging must not replace the catalogue's own rows.
	if p, ok := observability.LookupPrice("deepinfra/gemma-4-31B-it-turbo"); !ok || p.InputPerM != 0.09 {
		t.Fatalf("catalogue price = %+v, ok=%v", p, ok)
	}
}

// The whole point of the precedence: a provider that starts publishing a price
// takes over from the rate typed into the dashboard, including when the two
// spell the model's name with different capitalisation.
func TestCataloguePriceBeatsAManualOneRegardlessOfCase(t *testing.T) {
	resetPrices(t)
	srv := catalogueServer(t, hopscotchCatalogue, "")
	defer srv.Close()

	manual := &fakePrices{rows: []llmreg.ManualPrice{{
		Model: "deepinfra/Gemma-4-31B-it-turbo", InputPerMTok: 9, OutputPerMTok: 9,
	}}}
	r := NewRefresher(slog.Default(), []Source{{Name: "hopscotch", BaseURL: srv.URL + "/v1"}})
	r.SetManualPrices(manual)
	r.Refresh(context.Background())

	if p, ok := observability.LookupPrice("deepinfra/Gemma-4-31B-it-turbo"); !ok || p.InputPerM != 0.09 || p.OutputPerM != 0.34 {
		t.Fatalf("price = %+v, ok=%v; the catalogue must win", p, ok)
	}
	if !r.CataloguePriced("deepinfra/Gemma-4-31B-it-turbo") {
		t.Error("CataloguePriced says the model is not catalogued")
	}
}

func TestRepublishPicksUpASavedRateWithoutFetching(t *testing.T) {
	resetPrices(t)
	srv := catalogueServer(t, hopscotchCatalogue, "")
	manual := &fakePrices{}
	r := NewRefresher(slog.Default(), []Source{{Name: "hopscotch", BaseURL: srv.URL + "/v1"}})
	r.SetManualPrices(manual)
	r.Refresh(context.Background())

	// The catalogue is gone; a rate saved afterwards must still reach the table
	// the cost counters read.
	srv.Close()
	manual.rows = []llmreg.ManualPrice{{Model: "local/chat", InputPerMTok: 2, OutputPerMTok: 3}}
	r.Republish(context.Background())

	if p, ok := observability.LookupPrice("local/chat"); !ok || p.InputPerM != 2 || p.OutputPerM != 3 {
		t.Fatalf("after republish = %+v, ok=%v", p, ok)
	}
	if p, ok := observability.LookupPrice("deepinfra/gemma-4-31B-it-turbo"); !ok || p.InputPerM != 0.09 {
		t.Fatalf("republish dropped the last catalogue prices: %+v, ok=%v", p, ok)
	}
}

func TestManualPricesAloneStillPriceCalls(t *testing.T) {
	resetPrices(t)
	manual := &fakePrices{rows: []llmreg.ManualPrice{{Model: "local/chat", InputPerMTok: 1, OutputPerMTok: 2}}}
	r := NewRefresher(slog.Default(), nil)
	r.SetManualPrices(manual)

	// Zero means "no catalogue answered", which keeps the retry short, but the
	// manual rate must still be published.
	if got := r.Refresh(context.Background()); got != 0 {
		t.Fatalf("catalogued models = %d, want 0", got)
	}
	if _, ok := observability.LookupPrice("local/chat"); !ok {
		t.Fatal("a manual rate must be published even when no catalogue answers")
	}
	if r.CataloguePriced("local/chat") {
		t.Error("CataloguePriced must not claim a manual rate came from a catalogue")
	}
}
