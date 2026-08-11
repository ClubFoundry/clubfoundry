//go:build !linux

package backup

// preserveOwnership is a no-op outside Linux. The updater is deployed as a
// Linux container, while this fallback keeps local tooling and tests portable.
func preserveOwnership(_, _ string) error {
	return nil
}
