package web

import (
	"database/sql"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"net/http"
)

func (s *Server) assetDeletePage(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !actor.Can(application.CapabilityDeleteAssets) {
		s.renderForbidden(w, actor, "error.forbidden_asset")
		return
	}
	asset, err := s.getAsset(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, actor, "error.not_found_asset")
		return
	}
	s.render(w, http.StatusOK, "asset_delete", pageData{Title: textFor(actor.Locale, "asset.delete"), Principal: &actor, Asset: &asset, CSRFToken: s.ensureCSRF(w, r), ReturnTo: r.URL.RequestURI()})
}
func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !actor.Can(application.CapabilityDeleteAssets) {
		s.renderForbidden(w, actor, "error.forbidden_asset")
		return
	}
	if !s.verifyCSRF(w, r) {
		return
	}
	if r.PostForm.Get("confirm_delete") != "yes" {
		http.Error(w, textFor(actor.Locale, "asset.delete_confirm"), http.StatusBadRequest)
		return
	}
	if err := s.options.Specifications.DeleteAsset(r.Context(), actor, r.PathValue("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.renderNotFound(w, actor, "error.not_found_asset")
			return
		}
		if errors.Is(err, application.ErrForbidden) {
			s.renderForbidden(w, actor, "error.forbidden_asset")
			return
		}
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
