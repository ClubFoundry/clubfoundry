package updater

import (
	"io"
	"os"
)

func readLogTail(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := st.Size()
	if size <= maxBytes {
		body, err := io.ReadAll(f)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	if _, err := f.Seek(size-maxBytes, io.SeekStart); err != nil {
		return "", err
	}
	buf := make([]byte, maxBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	return string(buf[:n]), nil
}
