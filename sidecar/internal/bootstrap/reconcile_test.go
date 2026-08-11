package bootstrap

import (
	"context"
	"testing"
)

func TestComposeProjectParsing(t *testing.T) {
	line := dockerPSLine{Labels: "other=value, com.docker.compose.project=clubfoundry, last=value"}
	if got := line.composeProject(); got != "clubfoundry" {
		t.Fatalf("composeProject = %q", got)
	}
	if got := (dockerPSLine{Labels: "other=value"}).composeProject(); got != "" {
		t.Fatalf("missing compose project = %q", got)
	}
}

func TestReportComposeProjectDriftContracts(t *testing.T) {
	const canonical = `{"ID":"one","Names":"cf-main","Image":"clubfoundry:1.3.138","State":"running","Labels":"com.docker.compose.project=clubfoundry"}`
	const legacy = `{"ID":"old","Names":"cf-old","Image":"clubfoundry:1.3.137","State":"exited","Labels":"com.docker.compose.project=legacy"}`

	t.Run("clean", func(t *testing.T) {
		t.Setenv("FAKE_DOCKER_STDOUT", canonical)
		report := ReportComposeProjectDrift(context.Background(), writeFakeDocker(t), "clubfoundry", []string{"clubfoundry"})
		if len(report.Findings) != 0 || len(report.Errors) != 0 || report.CanonicalProject != "clubfoundry" {
			t.Fatalf("clean report = %+v", report)
		}
	})

	t.Run("wrong project", func(t *testing.T) {
		t.Setenv("FAKE_DOCKER_STDOUT", legacy)
		report := ReportComposeProjectDrift(context.Background(), writeFakeDocker(t), "clubfoundry", []string{"clubfoundry"})
		if len(report.Findings) != 1 || report.Findings[0].Kind != "wrong_project" || report.Findings[0].ProjectLabel != "legacy" {
			t.Fatalf("wrong-project report = %+v", report)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Setenv("FAKE_DOCKER_STDOUT", canonical)
		t.Setenv("FAKE_DOCKER_STDOUT_2", legacy)
		report := ReportComposeProjectDrift(context.Background(), writeFakeDocker(t), "clubfoundry", []string{"clubfoundry"})
		if len(report.Findings) != 2 || report.Findings[0].Kind != "duplicate" || report.Findings[1].Kind != "duplicate" {
			t.Fatalf("duplicate report = %+v", report)
		}
	})

	t.Run("docker error", func(t *testing.T) {
		t.Setenv("FAKE_DOCKER_EXIT", "1")
		report := ReportComposeProjectDrift(context.Background(), writeFakeDocker(t), "", nil)
		if report.CanonicalProject != "clubfoundry" || len(report.Services) != 2 || len(report.Errors) != 2 {
			t.Fatalf("error/default report = %+v", report)
		}
	})
}
