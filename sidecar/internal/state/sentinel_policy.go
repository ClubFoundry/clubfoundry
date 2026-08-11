package state

import (
	"fmt"
	"log"
)

func applySentinelToSelfState(s *State, sent TrampolineSentinel, currentSidecarVersion string) {
	if sent.ExitCode == 0 {
		if sent.TargetVersion != "" && currentSidecarVersion != "" && sent.TargetVersion != currentSidecarVersion {
			msg := fmt.Sprintf("self-update sentinel target=%q but running version=%q (image swap did not take effect)", sent.TargetVersion, currentSidecarVersion)
			log.Printf("sentinel: %s", msg)
			s.MarkError("SELF_UPDATE_VERSION_MISMATCH", msg)
			return
		}
		log.Printf("sentinel: trampoline %s success target=%s", sent.TrampolineID, sent.TargetVersion)
		s.Reset()
		return
	}
	msg := fmt.Sprintf("self-update trampoline failed (id=%s exit_code=%d service=%s target=%s)", sent.TrampolineID, sent.ExitCode, sent.Service, sent.TargetVersion)
	log.Printf("sentinel: %s", msg)
	s.MarkError("SELF_UPDATE_TRAMPOLINE_FAILED", msg)
}
