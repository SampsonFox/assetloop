package web

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

func (s *Server) uploadAppearance(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok {
		return
	}
	if s.options.Specifications == nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, application.MaxProductModel3DBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		s.renderAppearance(w, r, p, http.StatusRequestEntityTooLarge, textFor(p.Locale, "resource.upload_invalid"))
		return
	}
	defer r.MultipartForm.RemoveAll()
	if !s.verifyCSRF(w, r) {
		return
	}
	file, _, err := r.FormFile("model_3d")
	if err != nil {
		s.renderAppearance(w, r, p, 422, textFor(p.Locale, "resource.file_required"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, application.MaxProductModel3DBytes+1))
	if err != nil || int64(len(data)) > application.MaxProductModel3DBytes {
		s.renderAppearance(w, r, p, 413, textFor(p.Locale, "resource.upload_invalid"))
		return
	}
	draft := resourceDraft(r)
	_, err = s.modelMedia.UploadAppearance(r.Context(), p,
		application.UploadModel3DResource{Name: draft.Name, File: data, SourceURL: draft.SourceURL, Author: draft.Author, License: draft.License},
		application.SaveAppearanceDefault{ID: r.PostForm.Get("rule_id"), ModelID: r.PathValue("id"), TagIDs: nonemptyTagIDs(r.PostForm["tag_ids"])})
	if err != nil {
		s.renderAppearance(w, r, p, 422, s.resourceError(p.Locale, err))
		return
	}
	http.Redirect(w, r, "/admin/catalog/models/"+r.PathValue("id")+"/appearance", http.StatusSeeOther)
}

type appearanceRuleRow struct {
	ID, ResourceID string
	Tags           []string
}
type appearancePageData struct {
	Model              domain.ProductModel
	RuleID, ResourceID string
	TagIDs             []string
	Dimensions         []tagDimension
	Rules              []appearanceRuleRow
	Candidates         []application.AppearanceCandidate
	Manual             bool
}

func (s *Server) appearancePage(w http.ResponseWriter, r *http.Request) {
	p, ok := s.resourcePrincipal(w, r)
	if !ok {
		return
	}
	s.renderAppearance(w, r, p, http.StatusOK, "")
}

func (s *Server) renderAppearance(w http.ResponseWriter, r *http.Request, p application.Principal, status int, message string) {
	if s.options.Specifications == nil {
		http.NotFound(w, r)
		return
	}
	model, err := s.options.Specifications.Model(r.Context(), p, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, p, "resource.target_not_found")
		return
	}
	state, err := s.options.Specifications.Snapshot(r.Context(), p)
	if err != nil {
		s.renderError(w, r, 500, err)
		return
	}
	values := r.URL.Query()
	if r.Method == http.MethodPost {
		values = r.PostForm
	}
	view := appearancePageData{Model: model, RuleID: values.Get("rule_id"), ResourceID: values.Get("resource_id"), TagIDs: nonemptyTagIDs(values["tag_ids"]), Manual: values.Get("manual") == "1"}
	found := view.RuleID == ""
	for _, rule := range state.Defaults {
		if rule.ModelID != model.ID {
			continue
		}
		row := appearanceRuleRow{ID: rule.ID, ResourceID: rule.ResourceID}
		for _, id := range rule.TagIDs {
			if tag, ok := state.Tag(id); ok {
				row.Tags = append(row.Tags, tag.Name)
			}
		}
		view.Rules = append(view.Rules, row)
		if rule.ID == view.RuleID {
			found = true
			if r.Method == http.MethodGet && values.Get("search") == "" {
				view.TagIDs, view.ResourceID = rule.TagIDs, rule.ResourceID
			}
		}
	}
	if !found {
		s.renderNotFound(w, p, "validation.specification_missing")
		return
	}
	definition := state.Model(p.TenantID, model.ID)
	editor := modelTagEditorFor(state, p.TenantID, model.ID, view.TagIDs, definition.AppearanceOverrides)
	allowed := map[string]bool{}
	for _, id := range definition.AllowedTagIDs {
		allowed[id] = true
	}
	for _, dim := range editor.Dimensions {
		affects := dim.Appearance
		if override, ok := definition.AppearanceOverrides[dim.ID]; ok {
			affects = override
		}
		if !affects {
			continue
		}
		choices := []tagChoice{}
		for _, choice := range dim.Choices {
			if allowed[choice.ID] {
				choices = append(choices, choice)
			}
		}
		dim.Choices = choices
		if len(choices) > 0 {
			view.Dimensions = append(view.Dimensions, dim)
		}
	}
	query, page := values.Get("q"), queryPage(r)
	var resources []domain.Model3DResource
	total := 0
	if view.Manual {
		result, e := s.modelMedia.ListResources(r.Context(), p, application.Model3DResourceListOptions{Query: query, Page: page, PageSize: 20})
		err, resources, total = e, result.Resources, result.Total
	} else {
		result, e := s.options.Specifications.Candidates(r.Context(), p, model.ID, view.TagIDs, application.SpecificationListOptions{Query: query, Page: page, PageSize: 20})
		err, view.Candidates, total = e, result.Candidates, result.Total
	}
	if err != nil {
		if message == "" {
			message, status = s.userError(p.Locale, err), http.StatusUnprocessableEntity
		}
	}
	pages, previous, next := tablePagination(total, page, 20, func(n int) string {
		q := url.Values{"search": {"1"}, "q": {query}, "rule_id": {view.RuleID}, "tag_ids": view.TagIDs, "page": {strconv.Itoa(n)}}
		if view.Manual {
			q.Set("manual", "1")
		}
		return "/admin/catalog/models/" + model.ID + "/appearance?" + q.Encode()
	})
	draft := resourceDraft(r)
	s.render(w, status, "appearance", pageData{Title: textFor(p.Locale, "appearance.title"), ReturnTo: r.URL.RequestURI(), Principal: &p, CSRFToken: s.ensureCSRF(w, r), CanManageCatalog: p.Can(application.CapabilityManageCatalog), Error: message, Appearance: view, Resources: resources, Resource: &draft, TableQuery: query, TableTotal: total, TablePage: page, TableTotalPages: pages, TablePreviousURL: previous, TableNextURL: next})
}
