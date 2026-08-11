package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func awaitResumeStart(resumeStarted <-chan struct{}) {
	// Let a queued update claim the state before opening the listener.
	select {
	case <-resumeStarted:
	case <-time.After(15 * time.Second):
		log.Printf("chain resume signal not received within 15s — starting HTTP anyway")
	}
}

func serveUntilSignal(srv *http.Server, runningVersion string) {
	// Handle Docker stop and interactive termination gracefully.
	go func() {
		log.Printf("updater %s listening on %s", runningVersion, srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("updater: shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
