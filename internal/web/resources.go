package web

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

func (s *Server) resourcePrincipal(w http.ResponseWriter, r *http.Request) (application.Principal, bool) {
	p, ok := s.requirePrincipal(w, r)
	if !ok {
		return p, false
	}
	if r.Method != http.MethodGet && !p.Can(application.CapabilityManageCatalog) {
		s.renderForbidden(w, p, "error.forbidden_catalog")
		return p, false
	}
	if s.modelMedia == nil {
		s.render(w, http.StatusServiceUnavailable, "error", pageData{Principal: &p, Error: textFor(p.Locale, "resource.unavailable")})
		return p, false
	}
	return p, true
}

func resourceLibraryURL(query, kind, id, name string, page int) string {
	v := url.Values{}
	if query != "" {
		v.Set("q", query)
	}
	if kind != "" {
		v.Set("kind", kind)
		v.Set("target", id)
		v.Set("name", name)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	if len(v) == 0 {
		return "/admin/3d"
	}
	return "/admin/3d?" + v.Encode()
}

func validBindingKind(kind string) bool {
	return kind == "model" || kind == "variant" || kind == "asset"
}

func (s *Server) resourcesPage(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok {
		return
	}
	s.renderResources(w, r, p, http.StatusOK, "", nil)
}

func (s *Server) renderResources(w http.ResponseWriter, r *http.Request, p application.Principal, status int, message string, draft *domain.Model3DResource) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	kind, id, name := r.FormValue("kind"), r.FormValue("target"), r.FormValue("target_name")
	if name == "" {
		name = r.URL.Query().Get("name")
	}
	if !validBindingKind(kind) {
		kind, id, name = "", "", ""
	}
	var binding *application.Model3DBinding
	var boundResource *domain.Model3DResource
	if kind != "" {
		current, err := s.modelMedia.Binding(r.Context(), p, kind, id)
		if err != nil {
			s.renderNotFound(w, p, "resource.target_not_found")
			return
		}
		binding, name = &current, current.Name
		resourceID := current.ResourceID
		if resourceID == "" {
			resourceID = current.EffectiveResourceID
		}
		if resourceID != "" {
			resource, err := s.modelMedia.GetResource(r.Context(), p, resourceID)
			if err == nil {
				boundResource = &resource
			}
		}
	}
	page := queryPage(r)
	result, err := s.modelMedia.ListResources(r.Context(), p, application.Model3DResourceListOptions{Query: query, Page: page, PageSize: 25})
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	pages, previous, next := tablePagination(result.Total, page, 25, func(n int) string { return resourceLibraryURL(query, kind, id, name, n) })
	if page > pages && page > 1 && status == http.StatusOK {
		http.Redirect(w, r, resourceLibraryURL(query, kind, id, name, 1), http.StatusFound)
		return
	}
	s.render(w, status, "resources", pageData{Title: textFor(p.Locale, "resource.title"), Principal: &p, CSRFToken: s.ensureCSRF(w, r), Error: message,
		Resources: result.Resources, Resource: draft, BindingKind: kind, BindingID: id, BindingName: name, Binding: binding, BoundResource: boundResource, CanManageCatalog: p.Can(application.CapabilityManageCatalog),
		TableQuery: query, TableTotal: result.Total, TablePage: page, TableTotalPages: pages, TablePreviousURL: previous, TableNextURL: next,
		TableClearURL: resourceLibraryURL("", kind, id, name, 1), ReturnTo: r.URL.RequestURI()})
}

func (s *Server) resourcePage(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok {
		return
	}
	s.renderResource(w, r, p, http.StatusOK, "", nil)
}

func (s *Server) renderResource(w http.ResponseWriter, r *http.Request, p application.Principal, status int, message string, draft *domain.Model3DResource) {
	resource, err := s.modelMedia.GetResource(r.Context(), p, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, p, "resource.not_found")
		return
	}
	references, err := s.modelMedia.References(r.Context(), p, resource.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if draft != nil {
		resource.Name, resource.SourceURL, resource.Author, resource.License = draft.Name, draft.SourceURL, draft.Author, draft.License
	}
	s.render(w, status, "resource", pageData{Title: resource.Name, Principal: &p, CSRFToken: s.ensureCSRF(w, r), Error: message, Resource: &resource, References: references, CanManageCatalog: p.Can(application.CapabilityManageCatalog), ReturnTo: "/admin/3d/" + resource.ID})
}

func resourceDraft(r *http.Request) domain.Model3DResource {
	return domain.Model3DResource{Name: r.FormValue("name"), ProductModel3D: domain.ProductModel3D{SourceURL: r.FormValue("model_3d_source_url"), Author: r.FormValue("model_3d_author"), License: r.FormValue("model_3d_license")}}
}

func (s *Server) uploadResource(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, application.MaxProductModel3DBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		s.renderResources(w, r, p, http.StatusRequestEntityTooLarge, textFor(p.Locale, "resource.upload_invalid"), nil)
		return
	}
	defer r.MultipartForm.RemoveAll()
	if !s.verifyCSRF(w, r) {
		return
	}
	draft := resourceDraft(r)
	file, _, err := r.FormFile("model_3d")
	if err != nil {
		s.renderResources(w, r, p, http.StatusUnprocessableEntity, textFor(p.Locale, "resource.file_required"), &draft)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, application.MaxProductModel3DBytes+1))
	if err != nil || int64(len(data)) > application.MaxProductModel3DBytes {
		s.renderResources(w, r, p, http.StatusRequestEntityTooLarge, textFor(p.Locale, "resource.upload_invalid"), &draft)
		return
	}
	cmd := application.UploadModel3DResource{Name: draft.Name, File: data, SourceURL: draft.SourceURL, Author: draft.Author, License: draft.License}
	var resource domain.Model3DResource
	if kind := r.FormValue("kind"); kind != "" {
		resource, err = s.modelMedia.UploadAndBind(r.Context(), p, cmd, application.BindModel3DResource{Kind: kind, TargetID: r.FormValue("target")})
	} else {
		resource, err = s.modelMedia.Upload(r.Context(), p, cmd)
	}
	if err != nil {
		s.renderResources(w, r, p, http.StatusUnprocessableEntity, s.resourceError(p.Locale, err), &draft)
		return
	}
	http.Redirect(w, r, "/admin/3d/"+resource.ID, http.StatusSeeOther)
}

func (s *Server) updateResource(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok || !s.verifyCSRF(w, r) {
		return
	}
	draft := resourceDraft(r)
	_, err := s.modelMedia.UpdateResource(r.Context(), p, application.UpdateModel3DResource{ID: r.PathValue("id"), Name: draft.Name, SourceURL: draft.SourceURL, Author: draft.Author, License: draft.License})
	if err != nil {
		s.renderResource(w, r, p, http.StatusUnprocessableEntity, s.resourceError(p.Locale, err), &draft)
		return
	}
	http.Redirect(w, r, "/admin/3d/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *Server) deleteResource(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok || !s.verifyCSRF(w, r) {
		return
	}
	if err := s.modelMedia.DeleteResource(r.Context(), p, r.PathValue("id")); err != nil {
		s.renderResource(w, r, p, http.StatusConflict, textFor(p.Locale, "resource.delete_failed"), nil)
		return
	}
	http.Redirect(w, r, "/admin/3d", http.StatusSeeOther)
}

func (s *Server) bindResource(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok || !s.verifyCSRF(w, r) {
		return
	}
	kind, id := r.PathValue("kind"), r.PathValue("id")
	err := s.modelMedia.Bind(r.Context(), p, application.BindModel3DResource{Kind: kind, TargetID: id, ResourceID: r.FormValue("resource_id")})
	if err != nil {
		r.Form.Set("kind", kind)
		r.Form.Set("target", id)
		s.renderResources(w, r, p, http.StatusUnprocessableEntity, s.resourceError(p.Locale, err), nil)
		return
	}
	if kind == "asset" {
		http.Redirect(w, r, "/assets/"+id, http.StatusSeeOther)
		return
	}
	if kind == "model" {
		s.redirectToModelEditor(w, r, id)
		return
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
}

func (s *Server) resourceGLB(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok {
		return
	}
	opened, err := s.modelMedia.OpenResource(r.Context(), p, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer opened.Reader.Close()
	etag := `"sha256-` + opened.Model.SHA256 + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "model/gltf-binary")
	w.Header().Set("Content-Length", strconv.FormatInt(opened.Info.Size, 10))
	_, _ = io.Copy(w, opened.Reader)
}

func (s *Server) resourceError(locale application.Locale, err error) string {
	if errors.Is(err, application.ErrForbidden) {
		return textFor(locale, "error.forbidden_catalog")
	}
	if message, ok := inputErrorText(locale, err); ok {
		return message
	}
	return textFor(locale, "resource.invalid")
}
