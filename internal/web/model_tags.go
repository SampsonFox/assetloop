package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

type tagChoice struct {
	ID, Name          string
	Selected, Enabled bool
}

// Old bookmarks and forms lead back to the catalog. They must never maintain a
// second specification tree once typed selections are enabled.
func (s *Server) retiredVariantWrite(w http.ResponseWriter, r *http.Request) bool {
	if s.options.Specifications == nil {
		return false
	}
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return true
	}
	if !actor.Can(application.CapabilityManageCatalog) {
		s.renderForbidden(w, actor, "error.forbidden_catalog")
		return true
	}
	if !s.verifyCSRF(w, r) {
		return true
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
	return true
}

func (s *Server) saveAppearance(w http.ResponseWriter, r *http.Request) {
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
	cmd := application.SaveAppearanceDefault{ID: r.PostForm.Get("rule_id"), ModelID: r.PathValue("id"), ResourceID: r.PostForm.Get("resource_id"), TagIDs: r.PostForm["tag_ids"]}
	rule, err := s.options.Specifications.SaveAppearance(r.Context(), actor, cmd)
	if err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.renderForbidden(w, actor, "error.forbidden_asset")
			return
		}
		s.renderAppearance(w, r, actor, 422, s.userError(actor.Locale, err))
		return
	}
	if r.PostForm.Get("return_appearance") == "1" {
		http.Redirect(w, r, "/admin/catalog/models/"+rule.ModelID+"/appearance?rule_id="+rule.ID, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/catalog?"+url.Values{"dialog": {"model-drawer"}, "edit_model_id": {rule.ModelID}, "appearance_rule_id": {rule.ID}}.Encode(), http.StatusSeeOther)
}
func (s *Server) deleteAppearance(w http.ResponseWriter, r *http.Request) {
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
	if err := s.options.Specifications.DeleteAppearance(r.Context(), actor, r.PathValue("id")); err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.renderForbidden(w, actor, "error.forbidden_asset")
			return
		}
		s.renderCatalog(w, r, 422, actor, s.userError(actor.Locale, err))
		return
	}
	if modelID := r.PostForm.Get("return_model_id"); modelID != "" {
		if _, err := s.options.Specifications.Model(r.Context(), actor, modelID); err == nil {
			http.Redirect(w, r, "/admin/catalog/models/"+modelID+"/appearance", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
}

type tagDimension struct {
	ID, Name, Override   string
	Multiple, Appearance bool
	Choices              []tagChoice
}
type modelTagEditor struct {
	ModelID     string
	Dimensions  []tagDimension
	Summary     []string
	LegacyCount int
}

func modelTagEditors(state application.SpecificationSnapshot, tenant string, models []domain.ProductModel) []modelTagEditor {
	var result []modelTagEditor
	for _, model := range models {
		definition := state.Model(tenant, model.ID)
		result = append(result, modelTagEditorFor(state, tenant, model.ID, definition.AllowedTagIDs, definition.AppearanceOverrides))
	}
	return result
}
func modelTagEditorFor(state application.SpecificationSnapshot, tenant, modelID string, ids []string, overrides map[string]bool) modelTagEditor {
	result := modelTagEditor{ModelID: modelID}
	for _, mapping := range state.LegacyMedia {
		if mapping.ModelID == modelID && !mapping.Resolved {
			result.LegacyCount++
		}
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	for _, kind := range state.Types {
		dim := tagDimension{ID: kind.ID, Name: kind.Name, Multiple: kind.Multiple, Appearance: kind.AffectsAppearance}
		if value, ok := overrides[kind.ID]; ok {
			if value {
				dim.Override = "yes"
			} else {
				dim.Override = "no"
			}
		}
		for _, tag := range state.Tags {
			if tag.TypeID != kind.ID {
				continue
			}
			enabled := tag.Enabled && kind.Enabled
			if !enabled && !selected[tag.ID] {
				continue
			}
			dim.Choices = append(dim.Choices, tagChoice{ID: tag.ID, Name: tag.Name, Selected: selected[tag.ID], Enabled: enabled})
			if selected[tag.ID] {
				result.Summary = append(result.Summary, tag.Name)
			}
		}
		if len(dim.Choices) > 0 {
			result.Dimensions = append(result.Dimensions, dim)
		}
	}
	return result
}

func modelTagsFromForm(r *http.Request) application.SaveModelSpecification {
	_ = r.ParseForm()
	cmd := application.SaveModelSpecification{ModelID: r.PathValue("id"), TagIDs: r.PostForm["tag_ids"], AppearanceOverrides: map[string]bool{}}
	for key, values := range r.PostForm {
		if strings.HasPrefix(key, "appearance_") && len(values) > 0 && values[0] != "" {
			cmd.AppearanceOverrides[strings.TrimPrefix(key, "appearance_")] = values[0] == "yes"
		}
	}
	return cmd
}
func (s *Server) saveModelTags(w http.ResponseWriter, r *http.Request) {
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
	for key, values := range r.PostForm {
		if strings.HasPrefix(key, "appearance_") && (len(values) != 1 || (values[0] != "" && values[0] != "yes" && values[0] != "no")) {
			s.renderCatalog(w, r, http.StatusUnprocessableEntity, actor, s.userError(actor.Locale, application.NewInputError("validation.filter_invalid")))
			return
		}
	}
	if err := s.options.Specifications.SaveModel(r.Context(), actor, cmd); err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.renderForbidden(w, actor, "error.forbidden_asset")
			return
		}
		var inUse application.SpecificationInUseError
		errors.As(err, &inUse)
		s.renderCatalog(w, r, http.StatusUnprocessableEntity, actor, s.userError(actor.Locale, err), inUse.References...)
		return
	}
	s.redirectToModelEditor(w, r, cmd.ModelID)
}
