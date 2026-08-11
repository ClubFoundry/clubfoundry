package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/clubfoundry/updater/internal/history"
)

func (r *Reporter) runReport(entry history.Entry) {
	// Completely fresh context: see FireAfterSettle for rationale.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	settle := r.settleWindow()
	var settleResult string
	select {
	case <-time.After(settle):
		settleResult = r.checkSettleHealth(ctx, entry.ToVersion)
	case <-ctx.Done():
		// Only fires on process shutdown or the 30-minute delivery ceiling.
		settleResult = "interrupted"
	}

	log := r.readLog(entry.ID)

	payload := UpdateReport{
		ReportVersion:  1,
		InstanceID:     r.InstanceID,
		UpdateID:       entry.ID,
		FromVersion:    entry.FromVersion,
		ToVersion:      entry.ToVersion,
		Outcome:        string(entry.Outcome),
		StartedAt:      entry.StartedAt.Format(time.RFC3339),
		FinishedAt:     entry.FinishedAt.Format(time.RFC3339),
		DurationMS:     entry.DurationMS,
		ErrorText:      entry.Error,
		SettleAfterSec: int64(settle.Seconds()),
		SettleResult:   settleResult,
		Log:            log,
	}
	if len(entry.Steps) > 0 {
		payload.Steps = make([]ReportStep, 0, len(entry.Steps))
		for _, s := range entry.Steps {
			payload.Steps = append(payload.Steps, ReportStep{
				From:     s.From,
				To:       s.To,
				Outcome:  string(s.Outcome),
				Duration: s.Duration,
				Error:    s.Error,
			})
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: marshal report: %v\n", err)
		return
	}
	r.deliverWithBackoff(ctx, body)
}
