// Evaluation runner for Code Review Agent.
//
//	go run ./cmd/eval/
//
// This runs the mock reviewer against all test cases and outputs a report.
// To use the real agent-go cognition, swap ReviewFunc at the call site.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LingMi1/code-review-agent/eval"
)

func main() {
	real := flag.Bool("real", false, "use real agent-go cognition (gRPC) instead of mock reviewer")
	cognitionAddr := flag.String("cognition", "localhost:50051", "agent-go cognition gRPC address")
	flag.Parse()

	// Resolve eval/ directories relative to project root
	root := findRoot()
	corpusDir := filepath.Join(root, "eval", "corpus")
	expectedDir := filepath.Join(root, "eval", "expected")
	reportPath := filepath.Join(root, "eval", "report.json")

	// Choose reviewer: mock (default) or real cognition
	var reviewFn eval.ReviewFunc
	if *real {
		fmt.Printf("Running evaluation with real cognition (%s)...\n", *cognitionAddr)
		var err error
		reviewFn, err = eval.CognitionReview(*cognitionAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect to cognition: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Running evaluation with mock reviewer...")
	}
	fmt.Println()

	report, err := eval.Run(corpusDir, expectedDir, reviewFn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evaluation failed: %v\n", err)
		os.Exit(1)
	}

	// Print to stdout
	eval.PrintReport(report)

	// Save JSON report
	if err := eval.SaveReport(report, reportPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Report saved to %s\n", reportPath)

	// Exit with non-zero if pass rate < 0.8
	if report.PassRate < 0.8 {
		fmt.Fprintf(os.Stderr, "\nWARNING: pass rate %.0f%% is below 80%% threshold\n", report.PassRate*100)
	}
}

// findRoot walks up from current dir to find go.mod.
func findRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
