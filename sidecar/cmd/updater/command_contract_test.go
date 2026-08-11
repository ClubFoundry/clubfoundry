package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/clubfoundry/updater/internal/state"
)

func TestLogMiddlewareRecordsResponseStatus(t *testing.T) {
	oldOutput := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(oldOutput)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	})

	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	log.SetPrefix("")

	handler := logMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusAccepted)
	}
	if got := logs.String(); !strings.Contains(got, "GET /probe 202 ") {
		t.Fatalf("access log = %q, want method, path, and status", got)
	}
}

func TestChainedResumePreservesQueueWithoutCloud(t *testing.T) {
	mainState := state.New()
	mainState.SetPendingMainTarget("1.3.144")
	resumeStarted := make(chan struct{})

	runChainedMainResume(nil, mainState, nil, nil, "1.3.144", resumeStarted)

	select {
	case <-resumeStarted:
	default:
		t.Fatal("resume completion signal was not closed")
	}
	if got := mainState.PendingMainTarget(); got != "1.3.144" {
		t.Fatalf("pending target = %q, want queue preserved", got)
	}
}

func TestCloudVersionAdapterAllowsOfflineMode(t *testing.T) {
	meta, err := (cloudVersionAdapter{}).FetchVersionMetadata(context.Background(), "1.3.143")
	if err != nil {
		t.Fatalf("FetchVersionMetadata returned error: %v", err)
	}
	if meta != nil {
		t.Fatalf("metadata = %#v, want nil without a cloud client", meta)
	}
}
