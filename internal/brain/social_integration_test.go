package brain

// Integration tests for the social-closer gate. Skipped unless CORTEXIQ_TEST_DSN
// is set; see integration_test.go.

import (
	"errors"
	"testing"
)

// The refused write must leave nothing behind: no episode, no embedding, nothing
// for the consolidator to pick up.
func TestIntegrationRememberRefusesSocialCloserAndStoresNothing(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.Remember(e.ctx, e.ns, "dank je wel, tot morgen!", nil); !errors.Is(err, ErrSocialCloser) {
		t.Fatalf("err = %v, want ErrSocialCloser", err)
	}
	n, err := e.b.CountEpisodes(e.ctx, []string{e.ns})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("episodes = %d, want 0: a refused write must not be stored", n)
	}
}

// Turning the filter off restores the old behaviour: every write becomes an
// episode again, so nobody is locked out of storing a short message on purpose.
func TestIntegrationSocialFilterCanBeDisabled(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.SocialFilter = false })
	id, err := e.b.Remember(e.ctx, e.ns, "dank je wel, tot morgen!", nil)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if id == 0 {
		t.Fatal("expected an episode id")
	}
}
