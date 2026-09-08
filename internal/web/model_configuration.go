package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
)

// One request and one application transaction for the model drawer.
func (s *Server) saveModelConfiguration(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if s.options.Specifications == nil {
		http.NotFound(w, r)
		return
	}
	cmd := modelTagsFromForm(r)
	cmd.Details = &application.ModelConfigurationDetails{CategoryID: r.PostForm.Get("category_id"), Name: r.PostForm.Get("name")}
	var err error
	for key, values := range r.PostForm {
		if strings.HasPrefix(key, "appearance_") && (len(values) != 1 || (values[0] != "" && values[0] != "yes" && values[0] != "no")) {
			err = application.NewInputError("validation.filter_invalid")
		}
	}
	if err == nil {
		err = s.options.Specifications.SaveModel(r.Context(), actor, cmd)
	}
	if err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.renderForbidden(w, actor, "error.forbidden_asset")
			return
		}
		var inUse application.SpecificationInUseError
		errors.As(err, &inUse)
		s.renderCatalog(w, r, http.StatusUnprocessableEntity, actor, s.userError(actor.Locale, err), inUse.References...)
		return
	}
	if drawerSaved(w, "model", cmd.ModelID, cmd.Details.Name, true) {
		return
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
}
