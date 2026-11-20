package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/fivenorth-io/tokenlist/submission"
)

func main() {
	root := flag.String("root", ".", "tokenlist checkout to check")
	base := flag.String("base", "", "base checkout; enables the rules that need a diff")
	summary := flag.String("summary", "", "file to append the Markdown summary to, such as $GITHUB_STEP_SUMMARY")
	flag.Parse()
	if err := run(*root, *base, *summary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, base, summary string) error {
	var baseFS fs.FS
	if base != "" {
		baseFS = os.DirFS(base)
	}
	report, err := submission.Check(os.DirFS(root), baseFS)
	if err != nil {
		return err
	}
	if summary != "" {
		f, err := os.OpenFile(summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, werr := f.WriteString(submission.Markdown(report))
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
	}
	for _, w := range report.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %q: %s\n", w.Path, oneLine(w.Message))
	}
	for _, e := range report.Errors {
		fmt.Fprintf(os.Stderr, "error: %q: %s\n", e.Path, oneLine(e.Message))
	}
	if n := len(report.Errors); n > 0 {
		plural := "s"
		if n == 1 {
			plural = ""
		}
		return fmt.Errorf("%d error%s in %d entries", n, plural, report.Entries)
	}
	fmt.Printf("Checked %d entries: no errors, %d warnings\n", report.Entries, len(report.Warnings))
	return nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
