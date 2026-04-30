// Basic demonstrates the minimal file-mode integration: LogLens tails one or more log files and
// clusters them in the background. Run the Dashboard with `loglens ui --db ./loglens.db` to inspect results.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tragidra/loglens"
)

func main() {
	logPath := "./app.log"
	if len(os.Args) > 1 {
		logPath = os.Args[1]
	}

	ll, err := loglens.New(loglens.Config{
		Sources: []loglens.SourceConfig{
			{
				Kind:      "file",
				Path:      logPath,
				StartFrom: "beginning",
			},
		},
		// AI provider not required
		// Storage defaults to SQLite at ./loglens.db.
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

	log.Printf("LogLens tailing %s — Ctrl+C to stop", logPath)
	log.Println("Inspect clusters with: loglens ui --db ./loglens.db")

	<-ctx.Done()
	log.Printf("shutting down (dropped=%d)", ll.Stats().Dropped)
}
