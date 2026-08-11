package dockerops

import (
	"fmt"
	"path"
)

// buildTrampolineShell creates the guarded helper-container script.
func buildTrampolineShell(composeHostDir, service string, delaySec int, opts TrampolineOpts) string {
	logPrefix := ""
	if opts.LogStdoutPath != "" && opts.LogStderrPath != "" {
		logPrefix = fmt.Sprintf("exec >%q 2>%q; ", opts.LogStdoutPath, opts.LogStderrPath)
	}
	header := fmt.Sprintf(
		"echo \"[trampoline] start id=%s target=%s service=%s sleep=%ds wall=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)\"; ",
		opts.TrampolineID, opts.TargetVersion, service, delaySec,
	)
	if opts.SentinelPath == "" {
		return logPrefix + header + fmt.Sprintf(
			"sleep %d && cd %q && docker rm -f %s >/dev/null 2>&1 || true; docker compose up -d --force-recreate %s 2>&1",
			delaySec, composeHostDir, service, service,
		)
	}

	sentinelDir := path.Dir(opts.SentinelPath)
	tmpPath := opts.SentinelPath + ".tmp"
	return logPrefix + header + fmt.Sprintf(
		"sleep %d; "+
			"if ! cd %q; then "+
			"  echo \"[trampoline] FATAL: cd to compose host dir failed — old container preserved\"; "+
			"  mkdir -p %q; "+
			"  now_iso_99=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ); "+
			"  printf '{\"trampoline_id\":\"%s\",\"target_version\":\"%s\",\"service\":\"%s\",\"op_id\":\"%s\",\"exit_code\":99,\"completed_at\":\"%%s\",\"error\":\"cd_to_compose_host_dir_failed\"}\\n' "+
			"  \"$now_iso_99\" > %q && mv %q %q; "+
			"  echo \"[trampoline] sentinel-99 written: %s\"; "+
			"  exit 99; "+
			"fi; "+
			"rc=0; "+
			"docker rm -f %s >/dev/null 2>&1 || true; "+
			"docker compose up -d --force-recreate --remove-orphans %s || rc=$?; "+
			"echo \"[trampoline] compose up exit_code=$rc\"; "+
			"mkdir -p %q; "+
			"now_iso=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ); "+
			"printf '{\"trampoline_id\":\"%s\",\"target_version\":\"%s\",\"service\":\"%s\",\"op_id\":\"%s\",\"exit_code\":%%d,\"completed_at\":\"%%s\"}\\n' "+
			"\"$rc\" \"$now_iso\" > %q && mv %q %q; "+
			"echo \"[trampoline] sentinel written: %s\"; "+
			"exit $rc",
		delaySec, composeHostDir,
		sentinelDir,
		opts.TrampolineID, opts.TargetVersion, service, opts.OpID,
		tmpPath, tmpPath, opts.SentinelPath, opts.SentinelPath,
		service, service, sentinelDir,
		opts.TrampolineID, opts.TargetVersion, service, opts.OpID,
		tmpPath, tmpPath, opts.SentinelPath, opts.SentinelPath,
	)
}
