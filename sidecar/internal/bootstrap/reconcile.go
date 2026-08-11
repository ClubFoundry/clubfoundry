package bootstrap

import (
	"context"
	"fmt"
	"time"
)

// ReportComposeProjectDrift reports managed services outside the canonical
// Compose project. It is read-only because automatic container removal could
// disrupt an operator-managed installation.
//
// Empty arguments select the production Docker binary, project, and services.
// An empty report means the topology is clean.
func ReportComposeProjectDrift(ctx context.Context, dockerBin, canonicalProject string, services []string) DriftReport {
	if canonicalProject == "" {
		canonicalProject = "clubfoundry"
	}
	if dockerBin == "" {
		dockerBin = "docker"
	}
	if len(services) == 0 {
		services = []string{"clubfoundry", "clubfoundry-updater"}
	}

	r := DriftReport{
		CanonicalProject: canonicalProject,
		Services:         services,
		CheckedAt:        time.Now().UTC(),
	}

	for _, svc := range services {
		containers, err := listContainersByServiceLabel(ctx, dockerBin, svc)
		if err != nil {
			r.Errors = append(r.Errors,
				fmt.Sprintf("list service=%s: %v", svc, err))
			continue
		}
		if len(containers) == 0 {
			// Not a finding — service may be intentionally absent (fresh
			// install pre-bootstrap, operator-disabled, etc.).
			continue
		}
		if len(containers) > 1 {
			for _, c := range containers {
				r.Findings = append(r.Findings, DriftFinding{
					Service:        svc,
					ContainerID:    c.ID,
					ContainerName:  c.Names,
					ProjectLabel:   c.composeProject(),
					State:          c.State,
					ImageRef:       c.Image,
					Kind:           "duplicate",
					Recommendation: fmt.Sprintf("multiple containers for service=%s; keep the running one in project=%s, remove the others manually (`docker rm -f <id>`).", svc, canonicalProject),
				})
			}
			continue
		}
		c := containers[0]
		proj := c.composeProject()
		if proj == canonicalProject {
			continue // single container in canonical project — clean
		}
		r.Findings = append(r.Findings, DriftFinding{
			Service:        svc,
			ContainerID:    c.ID,
			ContainerName:  c.Names,
			ProjectLabel:   proj,
			State:          c.State,
			ImageRef:       c.Image,
			Kind:           "wrong_project",
			Recommendation: fmt.Sprintf("container for service=%s carries project label=%q but canonical project is %q; run `docker compose -p %s down` followed by `docker compose up -d --remove-orphans` from the compose dir to re-anchor.", svc, proj, canonicalProject, proj),
		})
	}
	return r
}
