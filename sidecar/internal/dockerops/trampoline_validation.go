package dockerops

import (
	"fmt"
	"strings"
)

func validateTrampolineRequest(service string, opts TrampolineOpts) error {
	if err := validateTrampolineToken("service", service, false); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "trampoline ID", value: opts.TrampolineID},
		{name: "target version", value: opts.TargetVersion},
		{name: "operation ID", value: opts.OpID},
	} {
		if err := validateTrampolineToken(field.name, field.value, true); err != nil {
			return err
		}
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "sentinel path", value: opts.SentinelPath},
		{name: "stdout log path", value: opts.LogStdoutPath},
		{name: "stderr log path", value: opts.LogStderrPath},
	} {
		if field.value != "" {
			if err := validateTrampolinePath(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTrampolineToken(name, value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("%s is empty", name)
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' ||
			strings.ContainsRune("._:+-", char) {
			continue
		}
		return fmt.Errorf("%s contains unsupported character %q", name, char)
	}
	return nil
}

func validateTrampolinePath(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", name)
	}
	if index := strings.IndexAny(value, "$`\r\n\x00"); index >= 0 {
		return fmt.Errorf("%s contains unsafe shell character %q", name, value[index])
	}
	return nil
}
