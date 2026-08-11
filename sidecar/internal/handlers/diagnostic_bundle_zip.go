package handlers

import (
	"archive/zip"
	"io"
	"strings"
	"time"
)

func addBundleFile(zw *zip.Writer, zipPath string, body []byte) {
	hdr := &zip.FileHeader{
		Name:     zipPath,
		Method:   zip.Deflate,
		Modified: time.Now().UTC(),
	}
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return
	}
	_, _ = io.Copy(w, strings.NewReader(string(body)))
}
