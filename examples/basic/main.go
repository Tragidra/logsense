// Basic demonstrates the minimal file-mode integration: logsense tails one or more log files and
// clusters them in the background. Run the Dashboard with `logsense ui --db ./logsense.db` to inspect results.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tragidra/logsense"
)

func main() {
	logPath := "./app.log"
	if len(os.Args) > 1 {
		logPath = os.Args[1]
	}

	ll, err := logsense.New(logsense.Config{
		Sources: []logsense.SourceConfig{
			{
				Kind:      "file",
				Path:      logPath,
				StartFrom: "beginning",
			},
		},
		// AI provider not required
		// Storage defaults to SQLite at ./logsense.db.
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ll.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := ll.Start(ctx); err != nil {
		log.Fatal(err)
	}

	log.Printf("logsense tailing %s — Ctrl+C to stop", logPath)
	log.Println("Inspect clusters with: logsense ui --db ./logsense.db")

	<-ctx.Done()
	log.Printf("shutting down (dropped=%d)", ll.Stats().Dropped)
}
