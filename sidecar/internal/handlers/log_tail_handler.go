package handlers

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const logTailMaxBytes = 64 * 1024

func registerLogTailHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/log-tail", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET only")
			return
		}
		if d.LogDir == "" {
			writeError(w, http.StatusServiceUnavailable, "log dir not configured")
			return
		}
		updateID := r.URL.Query().Get("update_id")
		if updateID == "" {
			writeError(w, http.StatusBadRequest, "missing update_id")
			return
		}
		if strings.ContainsAny(updateID, "/\\") || strings.Contains(updateID, "..") {
			writeError(w, http.StatusBadRequest, "invalid update_id")
			return
		}
		var fromOffset int64
		if v := r.URL.Query().Get("from"); v != "" {
			parsed, err := strconv.ParseInt(v, 10, 64)
			if err != nil || parsed < 0 {
				writeError(w, http.StatusBadRequest, "invalid from offset")
				return
			}
			fromOffset = parsed
		}
		path := filepath.Join(d.LogDir, updateID+".log")
		fi, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, map[string]any{
					"content":    "",
					"offset":     fromOffset,
					"nextOffset": fromOffset,
					"truncated":  false,
				})
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		size := fi.Size()
		if fromOffset > size {
			fromOffset = 0
		}
		toRead := size - fromOffset
		truncated := false
		if toRead > logTailMaxBytes {
			fromOffset = size - logTailMaxBytes
			toRead = logTailMaxBytes
			truncated = true
		}
		f, err := os.Open(path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer f.Close()
		if _, err := f.Seek(fromOffset, io.SeekStart); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		buf := make([]byte, toRead)
		read, err := io.ReadFull(f, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"content":    string(buf[:read]),
			"offset":     fromOffset,
			"nextOffset": fromOffset + int64(read),
			"truncated":  truncated,
		})
	})
}
