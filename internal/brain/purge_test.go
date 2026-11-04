package brain

import "testing"

func TestPurgeOptionsNormalized(t *testing.T) {
	got := PurgeOptions{}.normalized()
	if got.Reason != "no reason given" {
		t.Fatalf("reason = %q; want the explicit placeholder", got.Reason)
	}
	if got.Actor != "unknown" {
		t.Fatalf("actor = %q; want unknown", got.Actor)
	}

	given := PurgeOptions{Reason: "  user asked  ", Actor: " cli "}.normalized()
	if given.Reason != "user asked" || given.Actor != "cli" {
		t.Fatalf("normalized = %+v; want trimmed reason and actor", given)
	}
	if given.DryRun {
		t.Fatal("normalization turned on DryRun")
	}
}

func TestValidateReason(t *testing.T) {
	if err := validateReason(""); err != ErrReasonRequired {
		t.Fatalf("empty reason err = %v; want ErrReasonRequired", err)
	}
	if err := validateReason("   "); err != ErrReasonRequired {
		t.Fatalf("blank reason err = %v; want ErrReasonRequired", err)
	}
	if err := validateReason("the queue is redis"); err != nil {
		t.Fatalf("valid reason err = %v; want nil", err)
	}
}
