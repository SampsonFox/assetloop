package web

import (
	"encoding/json"
	"net/http"
)

// Fragment negotiation changes presentation only; handlers retain authorization.
type drawerResponse struct {
	http.ResponseWriter
	target string
}

func drawerTransport(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.Header.Get("X-Assetloop-Drawer")
		w.Header().Add("Vary", "X-Assetloop-Drawer")
		if target != "" {
			switch target {
			case "tag-editor", "model-drawer", "category-drawer", "resource-editor", "resource-upload", "event-type-manage", "asset-editor", "asset-detail", "binding-editor", "appearance-editor":
			default:
				http.Error(w, "Unknown drawer", http.StatusBadRequest)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w = &drawerResponse{w, target}
		}
		next.ServeHTTP(w, r)
	})
}
func drawerSaved(w http.ResponseWriter, kind, id, name string, enabled bool, details ...map[string]any) bool {
	if _, ok := w.(*drawerResponse); !ok {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	result := map[string]any{"kind": kind, "id": id, "name": name, "enabled": enabled}
	for _, detail := range details {
		for key, value := range detail {
			result[key] = value
		}
	}
	_ = json.NewEncoder(w).Encode(result)
	return true
}
