package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

type specificationPageData struct {
	Values                                              bool
	TypeID, FilterTypeID, EditingID, Name, OriginalName string
	Enabled, Multiple, Appearance, ConfirmRename        bool
	Types                                               []application.SpecificationTypeSummary
	Tags                                                []application.SpecificationTagSummary
	AllTypes                                            []domain.SpecificationTagType
	References                                          []application.SpecificationLink
}

func (s *Server) specificationsPage(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderSpecifications(w, r, actor, http.StatusOK, "")
}

func (s *Server) renderSpecifications(w http.ResponseWriter, r *http.Request, actor application.Principal, status int, message string) {
	if s.options.Specifications == nil {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query()
	opts := application.SpecificationListOptions{Query: query.Get("q"), Status: query.Get("status"), TypeID: query.Get("type_id"), Page: queryPage(r), PageSize: 20}
	state, err := s.options.Specifications.Snapshot(r.Context(), actor)
	if err != nil {
		s.renderError(w, r, 500, err)
		return
	}
	data := specificationPageData{Values: query.Get("view") != "types", TypeID: opts.TypeID, FilterTypeID: opts.TypeID, AllTypes: state.Types, Enabled: true, EditingID: query.Get("edit")}
	if r.Method == http.MethodPost {
		data.Values = strings.HasPrefix(r.URL.Path, "/admin/tags/values")
		data.EditingID = r.PathValue("id")
	}
	if data.EditingID != "" {
		if data.Values {
			tag, found := state.Tag(data.EditingID)
			if !found {
				s.renderNotFound(w, actor, "validation.specification_missing")
				return
			}
			data.Name, data.TypeID, data.Enabled = tag.Name, tag.TypeID, tag.Enabled
			data.References = state.TagReferences(tag.ID)
		} else {
			kind, found := state.Type(data.EditingID)
			if !found {
				s.renderNotFound(w, actor, "validation.specification_missing")
				return
			}
			data.Name, data.Enabled, data.Multiple, data.Appearance = kind.Name, kind.Enabled, kind.Multiple, kind.AffectsAppearance
			data.References = state.TypeReferences(kind.ID)
		}
	}
	data.OriginalName = data.Name
	if r.Method == http.MethodPost && message != "" {
		data.Name, data.TypeID = r.FormValue("name"), r.FormValue("type_id")
		data.Enabled, data.Multiple, data.Appearance, data.ConfirmRename = r.FormValue("enabled") == "1", r.FormValue("multiple") == "1", r.FormValue("appearance") == "1", r.FormValue("confirm_rename") == "1"
	}
	total := 0
	if data.Values {
		result, e := s.options.Specifications.ListTags(r.Context(), actor, opts)
		err = e
		data.Tags, total = result.Tags, result.Total
	} else {
		result, e := s.options.Specifications.ListTypes(r.Context(), actor, opts)
		err = e
		data.Types, total = result.Types, result.Total
	}
	if err != nil {
		s.renderError(w, r, 400, err)
		return
	}
	pages, previous, next := tablePagination(total, opts.Page, opts.PageSize, func(page int) string {
		values := url.Values{"q": {opts.Query}, "status": {opts.Status}, "type_id": {opts.TypeID}, "page": {strconv.Itoa(page)}}
		if !data.Values {
			values.Set("view", "types")
		}
		return "/admin/tags?" + values.Encode()
	})
	s.render(w, status, "specifications", pageData{Title: textFor(actor.Locale, "tags.title"), ReturnTo: r.URL.RequestURI(), Principal: &actor, CSRFToken: s.ensureCSRF(w, r), CanManageCatalog: actor.Can(application.CapabilityManageCatalog), Error: message, Specifications: data, TableQuery: opts.Query, TableFilter: opts.Status, TablePage: opts.Page, TableTotalPages: pages, TableTotal: total, TablePreviousURL: previous, TableNextURL: next})
}

func (s *Server) saveSpecificationType(w http.ResponseWriter, r *http.Request) {
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
	saved, err := s.options.Specifications.SaveType(r.Context(), actor, application.SaveSpecificationType{ID: r.PathValue("id"), Name: r.FormValue("name"), Enabled: r.FormValue("enabled") == "1", Multiple: r.FormValue("multiple") == "1", AffectsAppearance: r.FormValue("appearance") == "1", ConfirmSharedRename: r.FormValue("confirm_rename") == "1"})
	if err != nil {
		s.specificationError(w, r, actor, err)
		return
	}
	if drawerSaved(w, "tag-type", saved.ID, saved.Name, saved.Enabled, map[string]any{"appearance": saved.AffectsAppearance, "multiple": saved.Multiple}) {
		return
	}
	http.Redirect(w, r, "/admin/tags?view=types", http.StatusSeeOther)
}

func (s *Server) saveSpecificationTag(w http.ResponseWriter, r *http.Request) {
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
	saved, err := s.options.Specifications.SaveTag(r.Context(), actor, application.SaveSpecificationTag{ID: r.PathValue("id"), TypeID: r.FormValue("type_id"), Name: r.FormValue("name"), Enabled: r.FormValue("enabled") == "1", ConfirmSharedRename: r.FormValue("confirm_rename") == "1"})
	if err != nil {
		s.specificationError(w, r, actor, err)
		return
	}
	if drawerSaved(w, "tag", saved.ID, saved.Name, saved.Enabled, map[string]any{"type_id": saved.TypeID}) {
		return
	}
	http.Redirect(w, r, "/admin/tags?type_id="+url.QueryEscape(r.FormValue("type_id")), http.StatusSeeOther)
}

func (s *Server) specificationError(w http.ResponseWriter, r *http.Request, actor application.Principal, err error) {
	if errors.Is(err, application.ErrForbidden) {
		s.renderForbidden(w, actor, "error.forbidden_asset")
		return
	}
	s.renderSpecifications(w, r, actor, http.StatusUnprocessableEntity, s.userError(actor.Locale, err))
}
