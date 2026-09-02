package main

import (
	"fmt"
	"os"

	"quantram/internal/semantics/tooling"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "validate":
		path := tooling.CanonicalJSONPath
		if len(rest) > 0 {
			path = rest[0]
		}
		if err := tooling.ValidateCatalog(); err != nil {
			return fmt.Errorf("catalog: %w", err)
		}
		if err := tooling.ValidatePath(path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		fmt.Printf("validate ok %s\n", path)
		return nil
	case "build":
		check := false
		path := tooling.CanonicalJSONPath
		for _, arg := range rest {
			if arg == "--check" {
				check = true
				continue
			}
			path = arg
		}
		if check {
			if err := tooling.CheckCanonical(path); err != nil {
				return err
			}
			fmt.Printf("build --check ok %s\n", path)
			return nil
		}
		if err := tooling.WriteCanonical(path); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", path)
		return nil
	case "audit":
		root := "."
		if len(rest) > 0 {
			root = rest[0]
		}
		report, err := tooling.Audit(root, nil)
		if err != nil {
			return err
		}
		for _, f := range report.Findings {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", f.Kind, f.ID, f.Token, f.Source, f.Detail)
		}
		fmt.Printf("audit findings=%d missing=%d orphaned=%d review=%d\n",
			len(report.Findings),
			len(report.ByKind(tooling.FindingMissing)),
			len(report.ByKind(tooling.FindingOrphaned)),
			len(report.ByKind(tooling.FindingReview)),
		)
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %s", cmd)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `quantram-semantics validate [json]
quantram-semantics audit [repo-root]
quantram-semantics build [--check] [json]
`)
}
