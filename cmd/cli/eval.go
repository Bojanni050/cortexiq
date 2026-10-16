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
