package dockerops

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// BuildxPrune applies optional size and idle-age retention to BuildKit cache.
func (c Config) BuildxPrune(ctx context.Context, keepBytes int64, untilHours int) (int64, error) {
	args := []string{"buildx", "prune", "--force"}
	if keepBytes > 0 {
		args = append(args, "--keep-storage", strconv.FormatInt(keepBytes, 10))
	}
	if untilHours > 0 {
		args = append(args, "--filter", fmt.Sprintf("until=%dh", untilHours))
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return 0, fmt.Errorf("docker buildx prune: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseReclaimedBytes(string(out)), nil
}
