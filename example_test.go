package logsense_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Tragidra/logsense"
	"github.com/Tragidra/logsense/model"
)

// ExampleNew shows the minimal file-mode setup: logsense tails one log file and groups events into clusters
// Run the dashboard to inspect results:
//
//	logsense ui --db ./logsense.db
func ExampleNew() {
	ll, err := logsense.New(logsense.Config{
		Sources: []logsense.SourceConfig{
			{
				Kind:    "file",
				Path:    "/var/log/myapp.log",
				Service: "myapp",
			},
		},
		AI: logsense.AIConfig{
			Provider: "logsense-ai", // local LM Studio / Ollama on :1234
		},
		Storage: logsense.StorageConfig{
			Kind:       "sqlite",
			SQLitePath: "./logsense.db",
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
	// logsense.yaml:
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
	//     sqlite_path: ./logsense.db

	ll, err := logsense.NewFromYAML("logsense.yaml")
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()

	if err := ll.Start(context.Background()); err != nil {
		log.Fatal(err)
	}

	select {}
}

// Examplelogsense_Report shows inline mode: errors from your code are reported into the pipeline
// and clustered alongside file-sourced logs.
// Report is non-blocking — if the pipeline is full, the event is dropped.
func Examplelogsense_Report() {
	ll, err := logsense.New(logsense.Config{
		AI: logsense.AIConfig{
			Provider: "openrouter",
			APIKey:   os.Getenv("OPENROUTER_API_KEY"),
			Model:    "anthropic/claude-3.5-haiku",
		},
		Inline: logsense.InlineConfig{
			Enabled:     true,
			MinPriority: 40,
		},
		Storage: logsense.StorageConfig{Kind: "sqlite"},
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
		ll.Report(ctx, dbErr, logsense.Fields{
			"query":   query,
			"user_id": userID,
		})
	}
}

// Examplelogsense_AnalyzeNow performs a synchronous one-shot LLM analysis of a single error, bypassing clustering
// The returned [model.Analysis] type is in the github.com/Tragidra/logsense/model package.
func Examplelogsense_AnalyzeNow() {
	ll, err := logsense.New(logsense.Config{
		AI: logsense.AIConfig{
			Provider: "openrouter",
			APIKey:   os.Getenv("OPENROUTER_API_KEY"),
			Model:    "anthropic/claude-3.5-haiku",
		},
		Inline:  logsense.InlineConfig{Enabled: true},
		Storage: logsense.StorageConfig{Kind: "memory"},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()
	ll.Start(context.Background())

	orderID := "ord_9912"
	paymentErr := fmt.Errorf("stripe: card declined: insufficient_funds")

	var rec *model.Analysis
	rec, err = ll.AnalyzeNow(context.Background(), paymentErr.Error(), logsense.Fields{
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

// Examplelogsense_RecentRecommendations reads the most recently stored analyses across all clusters.
// Each element is a [model.Analysis].
func Examplelogsense_RecentRecommendations() {
	ll, err := logsense.New(logsense.Config{
		Sources: []logsense.SourceConfig{
			{Kind: "file", Path: "/var/log/myapp.log"},
		},
		AI: logsense.AIConfig{
			Provider: "logsense-ai",
		},
		Storage: logsense.StorageConfig{
			Kind:       "sqlite",
			SQLitePath: "./logsense.db",
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
