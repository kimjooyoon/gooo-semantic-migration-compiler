package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kimjooyoon/gooo-semantic-migration-compiler/internal/compiler"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "migrate":
		runMigrate(os.Args[2:])
	case "conformance":
		runConformance(os.Args[2:])
	case "annotate-metrics":
		runAnnotateMetrics(os.Args[2:])
	case "validate-metrics":
		runValidateMetrics(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runMigrate(args []string) {
	flags := flag.NewFlagSet("migrate", flag.ExitOnError)
	root := flags.String("root", ".", "input repository root")
	metaPath := flags.String("meta", ".gooo/migration-compiler.gooo", "authoritative .gooo semantic meta source")
	casePath := flags.String("case", "", "fixed migration case JSON")
	out := flags.String("output-dir", "", "caller-owned output directory")
	flags.Parse(args)
	if *casePath == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "migrate requires -case and -output-dir")
		os.Exit(2)
	}
	meta, _, err := compiler.LoadMeta(resolve(*root, *metaPath))
	if err != nil {
		fatal(err)
	}
	report, err := compiler.EvaluateCase(meta, resolve(*root, *casePath), *root, *out)
	if err != nil {
		fatal(err)
	}
	printJSON(report)
}

func runConformance(args []string) {
	flags := flag.NewFlagSet("conformance", flag.ExitOnError)
	root := flags.String("root", ".", "input repository root")
	metaPath := flags.String("meta", ".gooo/migration-compiler.gooo", "authoritative .gooo semantic meta source")
	out := flags.String("output-dir", "", "caller-owned output directory")
	flags.Parse(args)
	if *out == "" {
		fmt.Fprintln(os.Stderr, "conformance requires -output-dir")
		os.Exit(2)
	}
	meta, raw, err := compiler.LoadMeta(resolve(*root, *metaPath))
	if err != nil {
		fatal(err)
	}
	index, err := compiler.RunConformance(meta, raw, *root, *out)
	if err != nil {
		fatal(err)
	}
	printJSON(index)
	for _, item := range index.Cases {
		if !item.Pass {
			os.Exit(1)
		}
	}
}

func runAnnotateMetrics(args []string) {
	flags := flag.NewFlagSet("annotate-metrics", flag.ExitOnError)
	indexPath := flags.String("index", "", "conformance-index.json")
	observationsPath := flags.String("observations", "", "strict runner observations JSON")
	flags.Parse(args)
	if *indexPath == "" || *observationsPath == "" {
		fmt.Fprintln(os.Stderr, "annotate-metrics requires -index and -observations")
		os.Exit(2)
	}
	index, err := compiler.AnnotateMetrics(*indexPath, *observationsPath)
	if err != nil {
		fatal(err)
	}
	printJSON(index)
}

func runValidateMetrics(args []string) {
	flags := flag.NewFlagSet("validate-metrics", flag.ExitOnError)
	metricsPath := flags.String("metrics", "", "metrics.json")
	flags.Parse(args)
	if *metricsPath == "" {
		fmt.Fprintln(os.Stderr, "validate-metrics requires -metrics")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*metricsPath)
	if err != nil {
		fatal(err)
	}
	if _, err := compiler.ParseMetrics(raw); err != nil {
		fatal(err)
	}
	fmt.Println("metrics_validation=CLOSED")
}

func resolve(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func printJSON(value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(raw))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gooo-semantic-migration-compiler migrate|conformance|annotate-metrics|validate-metrics [flags]")
}
