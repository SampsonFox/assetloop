package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

func (s *Server) imageURLs(ctx context.Context, p application.Principal, assets []domain.Asset) map[string]string {
	urls := map[string]string{}
	if s.options.ModelImages == nil {
		return urls
	}
	for _, a := range assets {
		if _, seen := urls[a.ModelID]; seen {
			continue
		}
		urls[a.ModelID] = ""
		if m, err := s.options.ModelImages.Get(ctx, p, a.ModelID); err == nil {
			urls[a.ModelID] = "/models/" + a.ModelID + "/image?v=" + m.SHA256
		}
	}
	return urls
}
func (s *Server) imagePrincipal(w http.ResponseWriter, r *http.Request, manage bool) (application.Principal, bool) {
	p, ok := s.requirePrincipal(w, r)
	if !ok {
		return p, false
	}
	if manage && !p.Can(application.CapabilityManageCatalog) {
		s.renderForbidden(w, p, "error.forbidden_catalog")
		return p, false
	}
	if s.options.ModelImages == nil {
		http.NotFound(w, r)
		return p, false
	}
	return p, true
}
func (s *Server) modelImagePage(w http.ResponseWriter, r *http.Request) {
	p, ok := s.imagePrincipal(w, r, true)
	if !ok {
		return
	}
	s.renderModelImage(w, r, p, http.StatusOK, "")
}
func (s *Server) renderModelImage(w http.ResponseWriter, r *http.Request, p application.Principal, status int, message string) {
	model, err := s.options.ModelImages.Model(r.Context(), p, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var current *application.ModelImage
	m, err := s.options.ModelImages.Get(r.Context(), p, model.ID)
	if err == nil {
		current = &m
	} else if !errors.Is(err, application.ErrImageNotFound) {
		s.renderError(w, r, http.StatusInternalServerError, errors.New(textFor(p.Locale, "image.unavailable")))
		return
	}
	s.render(w, status, "model_image", pageData{Title: textFor(p.Locale, "image.title"), Principal: &p, CanManageCatalog: true, CSRFToken: s.ensureCSRF(w, r), ReturnTo: "/admin/catalog/models/" + model.ID + "/image", CatalogEditingModel: &model, ModelImage: current, Error: message})
}
func (s *Server) saveModelImage(w http.ResponseWriter, r *http.Request) {
	p, ok := s.imagePrincipal(w, r, true)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, application.MaxModelImageBytes+(1<<20))
	err := r.ParseMultipartForm(application.MaxModelImageBytes)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		s.renderModelImage(w, r, p, http.StatusRequestEntityTooLarge, textFor(p.Locale, "image.invalid"))
		return
	}
	if !s.verifyCSRF(w, r) {
		return
	}
	if r.FormValue("action") == "clear" {
		err = s.options.ModelImages.Clear(r.Context(), p, r.PathValue("id"))
	} else if r.FormValue("action") == "import" && s.options.ImageDownloader != nil {
		_, err = s.options.ModelImages.Import(r.Context(), p, r.PathValue("id"), r.FormValue("image_url"), r.FormValue("source_url"), s.options.ImageDownloader)
	} else {
		part, _, e := r.FormFile("image")
		err = e
		if err == nil {
			defer part.Close()
			var data []byte
			data, err = io.ReadAll(io.LimitReader(part, application.MaxModelImageBytes+1))
			if err == nil {
				_, err = s.options.ModelImages.Upload(r.Context(), p, r.PathValue("id"), data, r.FormValue("source_url"))
			}
		}
	}
	if err != nil {
		message := textFor(p.Locale, "image.invalid")
		if localized, ok := inputErrorText(p.Locale, err); ok {
			message = localized
		}
		s.renderModelImage(w, r, p, http.StatusUnprocessableEntity, message)
		return
	}
	http.Redirect(w, r, "/admin/catalog/models/"+r.PathValue("id")+"/image", http.StatusSeeOther)
}
func (s *Server) serveModelImage(w http.ResponseWriter, r *http.Request) {
	p, ok := s.imagePrincipal(w, r, false)
	if !ok {
		return
	}
	reader, m, err := s.options.ModelImages.Open(r.Context(), p, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer reader.Close()
	etag := `"` + m.SHA256 + `"`
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Length", fmt.Sprint(m.SizeBytes))
	_, _ = io.Copy(w, reader)
}
