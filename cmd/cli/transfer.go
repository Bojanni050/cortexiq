package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/alash3al/stash/internal/brain"
	"github.com/urfave/cli/v3"
)

// exportCmd writes episodes and facts as JSON Lines to a file or stdout.
// The summary goes to stderr so stdout stays a clean data stream.
func exportCmd(ctx context.Context, cmd *cli.Command) error {
	namespaces := cmd.StringSlice("namespaces")
	if len(namespaces) == 0 {
		namespaces = []string{"/"}
	}

	var out io.Writer = os.Stdout
	if path := cmd.String("output"); path != "" && path != "-" {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	bw := bufio.NewWriter(out)

	bc := getBootstrap(cmd)
	res, err := bc.Brain.Export(ctx, bw, namespaces, cmd.StringSlice("types"))
	if err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "exported %d episodes and %d facts\n", res.Episodes, res.Facts)
	return nil
}

// importCmd reads a file produced by exportCmd ("-" reads stdin).
func importCmd(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() == 0 {
		return fmt.Errorf("file argument is required (use - for stdin)")
	}

	var in io.Reader = os.Stdin
	if path := cmd.Args().First(); path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}

	bc := getBootstrap(cmd)
	res, err := bc.Brain.Import(ctx, in, brain.ImportOptions{
		Types:  cmd.StringSlice("types"),
		DryRun: cmd.Bool("dry-run"),
	})
	if err != nil {
		return err
	}
	if err := printJSON(res); err != nil {
		return err
	}
	if res.Failed > 0 {
		return fmt.Errorf("%d records could not be imported", res.Failed)
	}
	return nil
}
