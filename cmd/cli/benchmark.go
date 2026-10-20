// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Bojanni050/cortexiq/internal/benchmark"
	"github.com/urfave/cli/v3"
)

// benchmarkRunCmd runs a standardized memory benchmark against CortexIQ.
// The JSON report goes to stdout (or --output), a summary to stderr.
func benchmarkRunCmd(ctx context.Context, cmd *cli.Command) error {
	datasetPath := cmd.String("dataset")
	if datasetPath == "" {
		return fmt.Errorf("dataset path is required (use --dataset <path>)")
	}

	ds, err := benchmark.LoadDataset(datasetPath)
	if err != nil {
		return fmt.Errorf("load dataset: %w", err)
	}

	bc := getBootstrap(cmd)

	// Create the judge: LLM judge if requested, exact match otherwise.
	var judge benchmark.Judge
	if cmd.Bool("llm-judge") {
		judge = benchmark.NewLLMJudge(bc.Brain.Reasoner())
	} else {
		judge = benchmark.ExactMatchJudge{}
	}

	runner := benchmark.NewRunner(bc.Brain, judge)

	cfg := benchmark.Config{
		Namespace:  cmd.String("namespace"),
		Limit:      cmd.Int("limit"),
		MaxItems:   cmd.Int("max-items"),
		Keep:       cmd.Bool("keep"),
		JudgeModel: cmd.String("judge-model"),
	}

	report, err := runner.Run(ctx, ds, cfg)
	if err != nil {
		return fmt.Errorf("benchmark run: %w", err)
	}

	// Write the JSON report.
	output := cmd.String("output")
	data, err := benchmark.MarshalJSON(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if output != "" && output != "-" {
		if err := os.WriteFile(output, data, 0644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Report written to %s\n", output)
	} else {
		fmt.Println(string(data))
	}

	printBenchmarkSummary(report)
	return nil
}

// benchmarkListCmd shows available benchmark datasets in the eval/benchmark directory.
func benchmarkListCmd(ctx context.Context, cmd *cli.Command) error {
	dir := cmd.String("dir")
	if dir == "" {
		dir = "eval/benchmark"
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read benchmark directory %s: %w", dir, err)
	}

	fmt.Printf("Benchmark datasets in %s:\n\n", dir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > 5 && name[len(name)-5:] == ".json" {
			path := dir + "/" + name
			ds, err := benchmark.LoadDataset(path)
			if err != nil {
				fmt.Printf("  %-30s (invalid: %v)\n", name, err)
				continue
			}
			fmt.Printf("  %-30s %d items, %d questions, %d sessions\n",
				name, len(ds.Items), ds.CountQuestions(), ds.CountSessions())
		}
	}
	return nil
}

// printBenchmarkSummary writes a human-readable summary to stderr.
func printBenchmarkSummary(report *benchmark.BenchmarkReport) {
	w := os.Stderr
	s := report.Summary

	fmt.Fprintf(w, "\n=== Benchmark: %s ===\n\n", report.Name)
	fmt.Fprintf(w, "Items:       %d\n", report.Items)
	fmt.Fprintf(w, "Questions:   %d\n", s.Questions)
	fmt.Fprintf(w, "Episodes:    %d\n", s.TotalEpisodes)
	fmt.Fprintf(w, "Duration:    %d ms\n", report.DurationMs)
	fmt.Fprintf(w, "Errors:      %d\n", s.Errors)
	fmt.Fprintf(w, "\n--- Scores ---\n\n")
	fmt.Fprintf(w, "Accuracy:           %.1f%%\n", s.Accuracy*100)
	fmt.Fprintf(w, "Avg judge score:    %.3f\n", s.AverageJudgeScore)
	fmt.Fprintf(w, "Exact match rate:   %.1f%%\n", s.ExactMatchRate*100)
	fmt.Fprintf(w, "Avg latency:        %.0f ms/question\n", s.AverageLatencyMs)

	if len(s.ByCategory) > 0 {
		fmt.Fprintf(w, "\n--- By Category ---\n\n")
		fmt.Fprintf(w, "%-25s %5s %10s %10s\n", "category", "n", "accuracy", "avg score")
		for cat, stats := range s.ByCategory {
			fmt.Fprintf(w, "%-25s %5d %9.1f%% %10.3f\n", cat, stats.Questions, stats.Accuracy*100, stats.AvgScore)
		}
	}

	// Show failed questions.
	var failed []benchmark.BenchmarkResult
	for _, item := range report.Results {
		for _, q := range item.Questions {
			if q.Error != "" || (q.JudgeScore >= 0 && q.JudgeScore < 0.5) {
				failed = append(failed, q)
			}
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(w, "\n--- Low Scores (%d) ---\n\n", len(failed))
		maxShow := 10
		if len(failed) < maxShow {
			maxShow = len(failed)
		}
		for i := 0; i < maxShow; i++ {
			q := failed[i]
			fmt.Fprintf(w, "[%s] %s\n", q.QuestionID, q.Question)
			fmt.Fprintf(w, "  expected: %s\n", truncate(q.ExpectedAnswer, 100))
			fmt.Fprintf(w, "  got:      %s\n", truncate(q.GeneratedAnswer, 100))
			fmt.Fprintf(w, "  score:    %.2f", q.JudgeScore)
			if q.JudgeReasoning != "" {
				fmt.Fprintf(w, " (%s)", q.JudgeReasoning)
			}
			fmt.Fprintf(w, "\n")
			if q.Error != "" {
				fmt.Fprintf(w, "  error:    %s\n", q.Error)
			}
			fmt.Fprintf(w, "\n")
		}
		if len(failed) > maxShow {
			fmt.Fprintf(w, "  ... and %d more\n", len(failed)-maxShow)
		}
	}
}

// truncate shortens a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
