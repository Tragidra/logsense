// Inline demonstrates embedding LogLens in an HTTP service. Every call to ll.Report() sends the error
// through the normalize → cluster → score pipeline in the background.
// High-priority clusters are automatically sent to the LLM for analysis (requires OPENROUTER_API_KEY env var).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tragidra/loglens"
)

func main() {
	ll, err := loglens.New(loglens.Config{
		AI: loglens.AIConfig{
			Provider: "openrouter",
			APIKey:   os.Getenv("OPENROUTER_API_KEY"),
			Model:    "anthropic/claude-3.5-haiku",
		},
		Inline: loglens.InlineConfig{
			Enabled:       true,
			MinPriority:   40,
			MaxConcurrent: 2,
		},
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

	mux := http.NewServeMux()
	mux.HandleFunc("/pay", func(w http.ResponseWriter, r *http.Request) {
		orderID := fmt.Sprintf("ord-%d", rand.Intn(10000))

		if rand.Intn(3) == 0 {
			err := errors.New("connection to payment-gateway timed out")
			ll.Report(r.Context(), err, loglens.Fields{
				"order_id": orderID,
				"amount":   rand.Intn(1000),
				"currency": "USD",
			})
			http.Error(w, "payment failed", http.StatusBadGateway)
			return
		}
		fmt.Fprintf(w, "OK %s\n", orderID)
	})

	srv := &http.Server{Addr: ":8081", Handler: mux, ReadTimeout: 5 * time.Second}
	go func() {
		log.Println("listening on :8081")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Println("server error:", err)
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	log.Printf("done (dropped=%d)", ll.Stats().Dropped)
}
