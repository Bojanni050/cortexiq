// Modifications copyright (c) 2026 Bojan van der Heide
package modelprice

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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
