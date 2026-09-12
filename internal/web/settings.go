package web

import (
	"net/http"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
)

func settingsSection(path string) string {
	for _, section := range []string{"catalog", "tags", "3d", "event-types", "market"} {
		root := "/admin/" + section
		if path == root || strings.HasPrefix(path, root+"/") {
			return section
		}
	}
	return ""
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	target := "/admin/tags"
	if actor.Can(application.CapabilityManageCatalog) {
		target = "/admin/catalog"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
