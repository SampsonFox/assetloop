package web

import (
	"net/http"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

// categoryDetail serves both a nested fragment and the native catalog backdrop.
// Register GET /admin/catalog/categories/{id} and /admin/catalog/categories/new.
func (s *Server) categoryDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !actor.Can(application.CapabilityManageCatalog) && (r.PathValue("id") == "new" || r.PathValue("id") == "") {
		s.renderForbidden(w, actor, "error.forbidden_catalog")
		return
	}
	s.renderCategoryDetail(w, r, actor, http.StatusOK, "")
}

// Category mutation error paths can reuse this renderer to retain submitted fields.
func (s *Server) renderCategoryDetail(w http.ResponseWriter, r *http.Request, actor application.Principal, status int, message string) {
	categories, err := s.catalog.Categories(r.Context(), actor)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	data := pageData{
		Title: textFor(actor.Locale, "catalog.category_add"), Principal: &actor,
		ReturnTo: r.URL.RequestURI(), CSRFToken: s.ensureCSRF(w, r), Error: message,
		Categories: categories, CategoryIcons: application.CategoryIconOptions,
		CanManageCatalog: actor.Can(application.CapabilityManageCatalog),
	}
	currentID := r.PathValue("id")
	if currentID != "" && currentID != "new" {
		for _, category := range categories {
			if category.ID == currentID {
				data.CatalogEditingCategory = &category
				break
			}
		}
		if data.CatalogEditingCategory == nil {
			s.renderNotFound(w, actor, "validation.specification_missing")
			return
		}
		data.Title = textFor(actor.Locale, "catalog.edit_category")
	}
	if data.CatalogEditingCategory == nil {
		data.CatalogEditingCategory = &domain.ItemCategory{}
	}
	if r.Method == http.MethodPost && message != "" {
		data.CatalogEditingCategory.Name = r.FormValue("name")
		data.CatalogEditingCategory.IconKey = r.FormValue("icon_key")
	}
	s.render(w, status, "catalog", data)
}
