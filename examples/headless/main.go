// Headless demonstrates reading stored recommendations programmatically, no UI needed.
// Point it at an existing SQLite database written by another service that embeds LogLens.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Tragidra/loglens"
)

func main() {
	dbPath := "./loglens.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	// Open the same store the running service writes to.
	ll, err := loglens.New(loglens.Config{
		Inline:  loglens.InlineConfig{Enabled: true}, // satisfies the "at least one input" check
		Storage: loglens.StorageConfig{Kind: "sqlite", SQLitePath: dbPath},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()

	ctx := context.Background()
	if err := ll.Start(ctx); err != nil {
		log.Fatal(err)
	}

	recs, err := ll.RecentRecommendations(ctx, 5)
	if err != nil {
		log.Fatal(err)
	}

	if len(recs) == 0 {
		fmt.Println("no recommendations stored yet")
		return
	}

	for i, r := range recs {
		fmt.Printf("--- Recommendation %d ---\n", i+1)
		fmt.Printf("Severity:  %s\n", r.Severity)
		fmt.Printf("Cluster:   %s\n", r.ClusterID)
		fmt.Printf("Summary:   %s\n", r.Summary)
		fmt.Printf("Hypothesis: %s\n", r.RootCauseHypothesis)
		fmt.Printf("Confidence: %.0f%%\n", r.Confidence*100)
		fmt.Println("Actions:")
		for _, a := range r.SuggestedActions {
			fmt.Printf("  • %s\n", a)
		}
		fmt.Printf("(model=%s tokens=%d+%d latency=%dms)\n\n",
			r.ModelUsed, r.TokensInput, r.TokensOutput, r.LatencyMs)
	}
}
