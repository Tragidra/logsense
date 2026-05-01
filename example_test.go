package logstruct_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Tragidra/logstruct"
	"github.com/Tragidra/logstruct/model"
)

// ExampleNew shows the minimal file-mode setup: logstruct tails one log file and groups events into clusters
// Run the dashboard to inspect results:
//
//	logstruct ui --db ./logstruct.db
func ExampleNew() {
	ll, err := logstruct.New(logstruct.Config{
		Sources: []logstruct.SourceConfig{
			{
				Kind:    "file",
				Path:    "/var/log/myapp.log",
				Service: "myapp",
			},
		},
		AI: logstruct.AIConfig{
			Provider: "logstruct-ai", // local LM Studio / Ollama on :1234
		},
		Storage: logstruct.StorageConfig{
			Kind:       "sqlite",
			SQLitePath: "./logstruct.db",
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
	// logstruct.yaml:
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
	//     sqlite_path: ./logstruct.db

	ll, err := logstruct.NewFromYAML("logstruct.yaml")
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()

	if err := ll.Start(context.Background()); err != nil {
		log.Fatal(err)
	}

	select {}
}

// Examplelogstruct_Report shows inline mode: errors from your code are reported into the pipeline
// and clustered alongside file-sourced logs.
// Report is non-blocking — if the pipeline is full, the event is dropped.
func Examplelogstruct_Report() {
	ll, err := logstruct.New(logstruct.Config{
		AI: logstruct.AIConfig{
			Provider: "openrouter",
			APIKey:   os.Getenv("OPENROUTER_API_KEY"),
			Model:    "anthropic/claude-3.5-haiku",
		},
		Inline: logstruct.InlineConfig{
			Enabled:     true,
			MinPriority: 40,
		},
		Storage: logstruct.StorageConfig{Kind: "sqlite"},
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
		ll.Report(ctx, dbErr, logstruct.Fields{
			"query":   query,
			"user_id": userID,
		})
	}
}

// Examplelogstruct_AnalyzeNow performs a synchronous one-shot LLM analysis of a single error, bypassing clustering
// The returned [model.Analysis] type is in the github.com/Tragidra/logstruct/model package.
func Examplelogstruct_AnalyzeNow() {
	ll, err := logstruct.New(logstruct.Config{
		AI: logstruct.AIConfig{
			Provider: "openrouter",
			APIKey:   os.Getenv("OPENROUTER_API_KEY"),
			Model:    "anthropic/claude-3.5-haiku",
		},
		Inline:  logstruct.InlineConfig{Enabled: true},
		Storage: logstruct.StorageConfig{Kind: "memory"},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()
	ll.Start(context.Background())

	orderID := "ord_9912"
	paymentErr := fmt.Errorf("stripe: card declined: insufficient_funds")

	var rec *model.Analysis
	rec, err = ll.AnalyzeNow(context.Background(), paymentErr.Error(), logstruct.Fields{
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

// Examplelogstruct_RecentRecommendations reads the most recently stored analyses across all clusters.
// Each element is a [model.Analysis].
func Examplelogstruct_RecentRecommendations() {
	ll, err := logstruct.New(logstruct.Config{
		Sources: []logstruct.SourceConfig{
			{Kind: "file", Path: "/var/log/myapp.log"},
		},
		AI: logstruct.AIConfig{
			Provider: "logstruct-ai",
		},
		Storage: logstruct.StorageConfig{
			Kind:       "sqlite",
			SQLitePath: "./logstruct.db",
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
