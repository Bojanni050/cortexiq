package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/urfave/cli/v3"
)

// evalDedupCmd scores the dedup calibration set against the configured embedder.
// The report is JSON on stdout; a short summary goes to stderr. Nothing is
// written to the database.
func evalDedupCmd(ctx context.Context, cmd *cli.Command) error {
	f, err := os.Open(cmd.String("file"))
	if err != nil {
		return err
	}
	defer f.Close()
	pairs, err := brain.LoadDedupPairs(f)
	if err != nil {
		return fmt.Errorf("%s: %w", cmd.String("file"), err)
	}

	bc := getBootstrap(cmd)
	rep, err := bc.Brain.EvalDedup(ctx, pairs, brain.DedupEvalOptions{
		High:      float32(cmd.Float("threshold")),
		Review:    float32(cmd.Float("review-threshold")),
		LLMReview: cmd.Bool("llm"),
	})
	if err != nil {
		return err
	}
	if err := printJSON(rep); err != nil {
		return err
	}
	printDedupSummary(rep)
	return nil
}

func printDedupSummary(rep brain.DedupEvalReport) {
	w := os.Stderr
	fmt.Fprintf(w, "\nmodel %s, %d pairs\n\n", rep.Model, rep.Pairs)
	fmt.Fprintf(w, "%-10s %5s %6s %6s %6s\n", "label", "n", "min", "median", "max")
	for _, l := range []string{brain.PairSame, brain.PairNearMiss, brain.PairDifferent} {
		s := rep.Labels[l]
		fmt.Fprintf(w, "%-10s %5d %6.3f %6.3f %6.3f\n", l, s.Count, s.Min, s.Median, s.Max)
	}
	fmt.Fprintf(w, "\n%-9s %5s %9s %9s   (pairs at or above)\n", "threshold", "same", "near-miss", "different")
	for _, r := range rep.Sweep {
		fmt.Fprintf(w, "%9.2f %5d %9d %9d\n", r.Threshold, r.Same, r.NearMiss, r.Different)
	}
	c := rep.Current
	fmt.Fprintf(w, "\nat high=%.2f review=%.2f:\n", c.High, c.Review)
	fmt.Fprintf(w, "  auto-merged  same %d, other %d (other must be 0)\n", c.AutoMergedSame, c.AutoMergedOther)
	fmt.Fprintf(w, "  to LLM       same %d, other %d\n", c.ReviewedSame, c.ReviewedOther)
	fmt.Fprintf(w, "  missed twins %d\n", c.MissedSame)
	if l := c.LLM; l != nil {
		fmt.Fprintf(w, "  LLM review   %d calls: twins merged %d/%d, others wrongly merged %d/%d\n",
			l.Calls, l.SameAccepted, l.SameAccepted+l.SameRejected, l.OtherAccepted, l.OtherAccepted+l.OtherRejected)
	}
	for _, s := range rep.FalseMerges {
		fmt.Fprintf(w, "  FALSE MERGE  %s (%s) %.3f\n", s.ID, s.Label, s.Similarity)
	}
	for _, s := range rep.Missed {
		fmt.Fprintf(w, "  MISSED       %s %.3f\n", s.ID, s.Similarity)
	}
}

// evalRecallCmd stores the corpus in a temporary namespace under /eval/, asks
// the question set through the normal recall path and removes the namespace
// again. The JSON report goes to stdout, a summary to stderr.
func evalRecallCmd(ctx context.Context, cmd *cli.Command) error {
	docsFile, err := os.Open(cmd.String("corpus"))
	if err != nil {
		return err
	}
	defer docsFile.Close()
	docs, err := brain.LoadRecallDocs(docsFile)
	if err != nil {
		return fmt.Errorf("%s: %w", cmd.String("corpus"), err)
	}

	qFile, err := os.Open(cmd.String("queries"))
	if err != nil {
		return err
	}
	defer qFile.Close()
	qs, err := brain.LoadRecallQueries(qFile, docs)
	if err != nil {
		return fmt.Errorf("%s: %w", cmd.String("queries"), err)
	}

	bc := getBootstrap(cmd)
	rep, err := bc.Brain.EvalRecall(ctx, docs, qs, brain.RecallEvalOptions{
		Limit: cmd.Int("limit"),
		Keep:  cmd.Bool("keep"),
	})
	if err != nil {
		return err
	}
	if err := printJSON(rep); err != nil {
		return err
	}
	printRecallSummary(rep)
	return nil
}

func printRecallSummary(rep brain.RecallEvalReport) {
	w := os.Stderr
	fmt.Fprintf(w, "\nmodel %s, %d memories, k=%d\n\n", rep.Model, rep.Docs, rep.Limit)
	fmt.Fprintf(w, "%-15s %4s %6s %6s %6s %6s\n", "kind", "n", "hit@1", "hit@3", "hit@k", "MRR")
	row := func(name string, s brain.RecallStats) {
		fmt.Fprintf(w, "%-15s %4d %6.2f %6.2f %6.2f %6.3f\n", name, s.Questions, s.HitAt1, s.HitAt3, s.HitAtK, s.MRR)
	}
	for _, k := range []string{brain.QueryIdentifier, brain.QueryParaphrase, brain.QueryCrossLanguage, brain.QueryConfuser} {
		if s, ok := rep.Kinds[k]; ok {
			row(k, s)
		}
	}
	row("overall", rep.Overall)
	for _, m := range rep.Misses {
		rank := fmt.Sprintf("rank %d", m.Rank)
		if m.Rank == 0 {
			rank = "NOT FOUND"
		}
		fmt.Fprintf(w, "  MISS %s (%s) %s: %q -> top %v\n", m.ID, m.Kind, rank, m.Query, m.Top)
	}
}
