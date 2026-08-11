package backup

import (
	"fmt"
	"io"
	"os"
)

// copyFile writes through a sibling temporary file and atomically renames it.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := preserveOwnership(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("preserve ownership of %s: %w", dst, err)
	}
	return os.Rename(tmp, dst)
}
