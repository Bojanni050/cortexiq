package brain

import "testing"

func TestIntegrationRememberStoresImportance(t *testing.T) {
	e := newEnv(t, nil)

	id, err := e.b.RememberWithImportance(e.ctx, e.ns, "decision: use postgres", nil, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	var got float32
	if err := e.b.pool.QueryRow(e.ctx, "SELECT importance FROM episodes WHERE id = $1", id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 0.9 {
		t.Errorf("importance = %v, want 0.9", got)
	}

	plain, err := e.b.Remember(e.ctx, e.ns, "some passing remark", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.b.pool.QueryRow(e.ctx, "SELECT importance FROM episodes WHERE id = $1", plain).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != DefaultImportance {
		t.Errorf("default importance = %v, want %v", got, DefaultImportance)
	}

	for _, bad := range []float32{-0.1, 1.1} {
		if _, err := e.b.RememberWithImportance(e.ctx, e.ns, "x marks the spot", nil, bad); err == nil {
			t.Errorf("importance %v should be rejected", bad)
		}
	}
}

func TestIntegrationRecallImportanceWeightPromotes(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.ImportanceWeight = 0.9 })
	minor := e.insertFact(t, "kubernetes cluster upgrade planned", nil)
	major := e.insertFact(t, "kubernetes cluster upgrade planned soon", nil)
	for id, imp := range map[int64]float32{minor: 0.0, major: 1.0} {
		if _, err := e.b.pool.Exec(e.ctx, "UPDATE facts SET importance = $2 WHERE id = $1", id, imp); err != nil {
			t.Fatal(err)
		}
	}

	res, err := e.b.Recall(e.ctx, []string{e.ns}, "kubernetes cluster upgrade planned", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) < 2 || res[0].ID != major {
		t.Fatalf("important fact should rank first, got %+v", res)
	}
	if res[0].Importance != 1 || res[1].Importance != 0 {
		t.Errorf("importance not returned in results: %+v", res)
	}
}
