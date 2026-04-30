package loglens_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Tragidra/loglens"
	"github.com/Tragidra/loglens/model"
)

// ExampleNew shows the minimal file-mode setup: LogLens tails one log file and groups events into clusters
// Run the dashboard to inspect results:
//
//	loglens ui --db ./loglens.db
func ExampleNew() {
	ll, err := loglens.New(loglens.Config{
		Sources: []loglens.SourceConfig{
			{
				Kind:    "file",
				Path:    "/var/log/myapp.log",
				Service: "myapp",
			},
		},
		AI: loglens.AIConfig{
			Provider: "logsense-ai", // local LM Studio / Ollama on :1234
		},
		Storage: loglens.StorageConfig{
			Kind:       "sqlite",
			SQLitePath: "./loglens.db",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()

	if err := ll.Start(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Block until the process is interrupted
	select {}
}

// ExampleNew_yamlConfig shows the same setup loaded from a YAML file, the file supports ${VAR}
// and ${VAR:-default} env expansion.
func ExampleNew_yamlConfig() {
	// loglens.yaml:
	//
	//   sources:
	//     - kind: file
	//       path: /var/log/myapp.log
	//   ai:
	//     provider: openrouter
	//     api_key: ${OPENROUTER_API_KEY}
	//     model: anthropic/claude-3.5-haiku
	//   storage:
	//     kind: sqlite
	//     sqlite_path: ./loglens.db

	ll, err := loglens.NewFromYAML("loglens.yaml")
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()

	if err := ll.Start(context.Background()); err != nil {
		log.Fatal(err)
	}

	select {}
}

// ExampleLogLens_Report shows inline mode: errors from your code are reported into the pipeline
// and clustered alongside file-sourced logs.
// Report is non-blocking — if the pipeline is full, the event is dropped.
func ExampleLogLens_Report() {
	ll, err := loglens.New(loglens.Config{
		AI: loglens.AIConfig{
			Provider: "openrouter",
			APIKey:   os.Getenv("OPENROUTER_API_KEY"),
			Model:    "anthropic/claude-3.5-haiku",
		},
		Inline: loglens.InlineConfig{
			Enabled:     true,
			MinPriority: 40,
		},
		Storage: loglens.StorageConfig{Kind: "sqlite"},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()
	ll.Start(context.Background())

	ctx := context.Background()

	userID := "u_42"
	query := "SELECT * FROM orders WHERE id = $1"
	dbErr := fmt.Errorf("connection timeout after 30s")

	if dbErr != nil {
		ll.Report(ctx, dbErr, loglens.Fields{
			"query":   query,
			"user_id": userID,
		})
	}
}

// ExampleLogLens_AnalyzeNow performs a synchronous one-shot LLM analysis of a single error, bypassing clustering
// The returned [model.Analysis] type is in the github.com/Tragidra/loglens/model package.
func ExampleLogLens_AnalyzeNow() {
	ll, err := loglens.New(loglens.Config{
		AI: loglens.AIConfig{
			Provider: "openrouter",
			APIKey:   os.Getenv("OPENROUTER_API_KEY"),
			Model:    "anthropic/claude-3.5-haiku",
		},
		Inline:  loglens.InlineConfig{Enabled: true},
		Storage: loglens.StorageConfig{Kind: "memory"},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()
	ll.Start(context.Background())

	orderID := "ord_9912"
	paymentErr := fmt.Errorf("stripe: card declined: insufficient_funds")

	var rec *model.Analysis
	rec, err = ll.AnalyzeNow(context.Background(), paymentErr.Error(), loglens.Fields{
		"order_id": orderID,
	})
	if err != nil {
		log.Printf("analysis failed: %v", err)
		return
	}

	fmt.Printf("severity:   %s\n", rec.Severity)
	fmt.Printf("summary:    %s\n", rec.Summary)
	fmt.Printf("hypothesis: %s\n", rec.RootCauseHypothesis)
	fmt.Println("actions:")
	for _, action := range rec.SuggestedActions {
		fmt.Printf("  - %s\n", action)
	}
}

// ExampleLogLens_RecentRecommendations reads the most recently stored analyses across all clusters.
// Each element is a [model.Analysis].
func ExampleLogLens_RecentRecommendations() {
	ll, err := loglens.New(loglens.Config{
		Sources: []loglens.SourceConfig{
			{Kind: "file", Path: "/var/log/myapp.log"},
		},
		AI: loglens.AIConfig{
			Provider: "logsense-ai",
		},
		Storage: loglens.StorageConfig{
			Kind:       "sqlite",
			SQLitePath: "./loglens.db",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()
	ll.Start(context.Background())

	recs, err := ll.RecentRecommendations(context.Background(), 5)
	if err != nil {
		log.Fatal(err)
	}

	for _, r := range recs {
		fmt.Printf("[%s] %s\n", r.Severity, r.Summary)
		for _, action := range r.SuggestedActions {
			fmt.Printf("  • %s\n", action)
		}
	}
}
