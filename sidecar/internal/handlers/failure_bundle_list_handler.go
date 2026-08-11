package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

func registerFailureBundleListHandler(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("/failure-bundles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET only")
			return
		}
		dir := failureBundleDirFromLogDir(d.LogDir)
		if dir == "" {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, []any{})
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]failureBundleSummary, 0, len(entries))
		for _, ent := range entries {
			if ent.IsDir() || filepath.Ext(ent.Name()) != ".json" {
				continue
			}
			info, err := ent.Info()
			if err != nil {
				continue
			}
			summary := failureBundleSummary{
				Filename:   ent.Name(),
				SizeBytes:  info.Size(),
				ModifiedAt: info.ModTime().Unix(),
			}
			// Headline fields are best-effort; malformed bundles still appear.
			if body, err := os.ReadFile(filepath.Join(dir, ent.Name())); err == nil {
				var headline failureBundleHeadline
				if err := json.Unmarshal(body, &headline); err == nil {
					summary.UpdateID = headline.UpdateID
					summary.FromVersion = headline.FromVersion
					summary.ToVersion = headline.ToVersion
					summary.Outcome = headline.Outcome
					summary.Source = headline.Source
				}
			}
			out = append(out, summary)
		}
		writeJSON(w, http.StatusOK, out)
	})
}
