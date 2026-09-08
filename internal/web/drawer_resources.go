package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
)

// Binding details reuse the media use cases without sending a parent editor
// through the resource library. Each POST commits independently.
func (s *Server) resourceBindingPage(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok {
		return
	}
	s.renderResourceBinding(w, r, p, http.StatusOK, "")
}

func (s *Server) saveResourceBinding(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok || !s.verifyCSRF(w, r) {
		return
	}
	kind, id := r.PathValue("kind"), r.PathValue("id")
	current, err := s.modelMedia.Binding(r.Context(), p, kind, id)
	if err != nil {
		s.renderNotFound(w, p, "resource.target_not_found")
		return
	}
	if err := s.modelMedia.Bind(r.Context(), p, application.BindModel3DResource{Kind: kind, TargetID: id, ResourceID: r.PostForm.Get("resource_id")}); err != nil {
		s.renderResourceBinding(w, r, p, http.StatusUnprocessableEntity, s.resourceError(p.Locale, err))
		return
	}
	if drawerSaved(w, "binding", id, current.Name, true) {
		return
	}
	http.Redirect(w, r, resourceBindingURL(kind, id, "", 1), http.StatusSeeOther)
}

func resourceBindingURL(kind, id, query string, page int) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	path := "/admin/3d/binding/" + url.PathEscape(kind) + "/" + url.PathEscape(id)
	if len(values) > 0 {
		path += "?" + values.Encode()
	}
	return path
}

func (s *Server) renderResourceBinding(w http.ResponseWriter, r *http.Request, p application.Principal, status int, message string) {
	kind, id := r.PathValue("kind"), r.PathValue("id")
	if !validBindingKind(kind) {
		http.NotFound(w, r)
		return
	}
	binding, err := s.modelMedia.Binding(r.Context(), p, kind, id)
	if err != nil {
		s.renderNotFound(w, p, "resource.target_not_found")
		return
	}
	query, page := strings.TrimSpace(r.URL.Query().Get("q")), queryPage(r)
	result, err := s.modelMedia.ListResources(r.Context(), p, application.Model3DResourceListOptions{Query: query, Page: page, PageSize: 20})
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	data := pageData{Title: textFor(p.Locale, "resource.binding"), Principal: &p, CSRFToken: s.ensureCSRF(w, r), Error: message,
		BindingKind: kind, BindingID: id, BindingName: binding.Name, Binding: &binding,
		Resources: result.Resources, CanManageCatalog: p.Can(application.CapabilityManageCatalog),
		TableQuery: query, TablePage: page, TableTotal: result.Total, ReturnTo: resourceBindingURL(kind, id, "", 1)}
	resourceID := binding.ResourceID
	if resourceID == "" {
		resourceID = binding.EffectiveResourceID
	}
	if resourceID != "" {
		resource, err := s.modelMedia.GetResource(r.Context(), p, resourceID)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err)
			return
		}
		data.BoundResource = &resource
	}
	data.TableTotalPages, data.TablePreviousURL, data.TableNextURL = tablePagination(result.Total, page, 20, func(n int) string {
		return resourceBindingURL(kind, id, query, n)
	})
	s.render(w, status, "resource_binding", data)
}
