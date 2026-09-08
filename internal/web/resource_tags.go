package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
)

// Resource descriptions aid discovery; saving them never changes a confirmed
// appearance rule or any item override.
func (s *Server) saveResourceTags(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok || !s.verifyCSRF(w, r) {
		return
	}
	if s.options.Specifications == nil {
		http.NotFound(w, r)
		return
	}
	err := s.options.Specifications.SaveResource(r.Context(), p, application.SaveResourceSpecification{
		ResourceID: r.PathValue("id"), TagIDs: nonemptyTagIDs(r.PostForm["tag_ids"]), CategoryIDs: r.PostForm["category_ids"],
	})
	if err != nil {
		s.renderResource(w, r, p, http.StatusUnprocessableEntity, s.userError(p.Locale, err), nil)
		return
	}
	resource, err := s.modelMedia.GetResource(r.Context(), p, r.PathValue("id"))
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if drawerSaved(w, "resource", resource.ID, resource.Name, true) {
		return
	}
	http.Redirect(w, r, "/admin/3d/"+r.PathValue("id")+"#resource-tags", http.StatusSeeOther)
}

func nonemptyTagIDs(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func (s *Server) resourceTagData(r *http.Request, p application.Principal, data *pageData) error {
	data.ReferenceURLs = map[string]string{}
	for _, ref := range data.References {
		if ref.Kind == "asset" {
			data.ReferenceURLs[ref.ID] = "/assets/" + ref.ID
		} else if ref.Kind == "model" {
			data.ReferenceURLs[ref.ID] = "/admin/catalog?" + url.Values{"dialog": {"model-drawer"}, "edit_model_id": {ref.ID}}.Encode()
		}
	}
	if s.options.Specifications == nil {
		return nil
	}
	state, err := s.options.Specifications.Snapshot(r.Context(), p)
	if err != nil {
		return err
	}
	ids := state.Selected("resource", data.Resource.ID)
	categoryIDs := state.Selected("resource-category", data.Resource.ID)
	if r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/tags") || r.FormValue("resource_configuration") == "1") && data.Error != "" {
		ids, categoryIDs = nonemptyTagIDs(r.PostForm["tag_ids"]), r.PostForm["category_ids"]
	}
	data.ResourceTags = modelTagEditorFor(state, p.TenantID, "", ids, nil)
	categories, err := s.catalog.Categories(r.Context(), p)
	if err != nil {
		return err
	}
	for _, category := range categories {
		choice := tagChoice{ID: category.ID, Name: category.Name, Enabled: true}
		for _, id := range categoryIDs {
			choice.Selected = choice.Selected || id == category.ID
		}
		data.ResourceCategories = append(data.ResourceCategories, choice)
	}
	for _, ref := range data.References {
		modelID := ""
		if ref.Kind == "model" {
			modelID = ref.ID
		} else if ref.Kind == "appearance" {
			for _, rule := range state.Defaults {
				if rule.ID == ref.ID {
					data.ReferenceURLs[ref.ID] = "/admin/catalog/models/" + rule.ModelID + "/appearance?" + url.Values{"rule_id": {rule.ID}}.Encode()
				}
			}
		}
		if modelID != "" {
			data.ReferenceURLs[ref.ID] = "/admin/catalog?" + url.Values{"dialog": {"model-drawer"}, "edit_model_id": {modelID}}.Encode()
		}
	}
	return nil
}
