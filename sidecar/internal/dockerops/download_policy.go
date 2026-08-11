package dockerops

import (
	"fmt"
	"io"
	"time"
)

// ErrSlowNetwork reports sustained throughput below 5 KB/s for three minutes.
var ErrSlowNetwork = fmt.Errorf("slow_network: sustained throughput below 5 KB/s for >180s")

const (
	slowNetworkThresholdBps = 5 * 1024
	slowNetworkDwell        = 180 * time.Second
	raceWindow              = 10 * time.Second
	stallTimeout            = 30 * time.Second
)

func writeLog(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}
