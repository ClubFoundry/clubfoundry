package updater

// classifyError maps operational failures to stable UI error codes.
func classifyError(updateErr, rollbackErr error) string {
	if updateErr == nil {
		return ""
	}
	msg := updateErr.Error()
	switch {
	case rollbackErr != nil:
		return "UPDATE_AND_ROLLBACK_FAILED"
	case contains(msg, "INSUFFICIENT_DISK"):
		return "INSUFFICIENT_DISK"
	case contains(msg, "NETWORK_UNREACHABLE"):
		return "NETWORK_UNREACHABLE"
	case contains(msg, "version mismatch"):
		return "VERSION_MISMATCH"
	case contains(msg, "sha256 mismatch"):
		return "SHA256_MISMATCH"
	case contains(msg, "context deadline exceeded"), contains(msg, "Timeout"):
		return "DOWNLOAD_TIMEOUT"
	case contains(msg, "post-update health"):
		return "HEALTH_TIMEOUT"
	case contains(msg, "already in use by container"):
		return "CONTAINER_NAME_CONFLICT"
	case contains(msg, "pull image"):
		return "IMAGE_PULL_FAILED"
	case contains(msg, "compose pre-flight"):
		// Validation errors need different remediation from compose-up failures.
		return "COMPOSE_VALIDATION_FAILED"
	case contains(msg, "docker compose up"):
		return "COMPOSE_UP_FAILED"
	case contains(msg, "backup db"):
		return "BACKUP_FAILED"
	}
	return "UNKNOWN_ERROR"
}

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return false
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
