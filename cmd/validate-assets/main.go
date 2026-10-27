package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fivenorth-io/tokenlist/validator"
)

func main() {
	root := flag.String("root", ".", "tokenlist checkout")
	base := flag.String("base", "", "optional base checkout for immutable identity checks")
	flag.Parse()
	if err := run(*root, *base); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, base string) error {
	v, err := validator.New()
	if err != nil {
		return err
	}
	entries, err := v.ValidateRepository(os.DirFS(root))
	if err != nil {
		return err
	}
	if base != "" {
		previous, err := v.ValidateRepository(os.DirFS(base))
		if err != nil {
			return fmt.Errorf("base registry: %w", err)
		}
		if err := validator.ValidateTransition(previous, entries); err != nil {
			return err
		}
	}
	fmt.Printf("Validated %d asset metadata files across all networks\n", len(entries))
	return nil
}
