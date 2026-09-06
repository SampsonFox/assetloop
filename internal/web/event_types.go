package web

import (
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Server) eventTypesPage(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderEventTypes(w, r, http.StatusOK, actor, "")
}
func (s *Server) renderEventTypes(w http.ResponseWriter, r *http.Request, status int, actor application.Principal, message string) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	state := r.URL.Query().Get("status")
	page := queryPage(r)
	result, err := s.lifecycle.EventTypePage(r.Context(), actor, application.EventTypeListOptions{Query: query, Status: state, Page: page, PageSize: 20})
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, err)
		return
	}
	if result.Total == 0 && page > 1 && r.Method == http.MethodGet {
		http.Redirect(w, r, eventTypesURL(query, state, 1), http.StatusSeeOther)
		return
	}
	pages, previous, next := tablePagination(result.Total, page, 20, func(p int) string { return eventTypesURL(query, state, p) })
	var editing *domain.AssetEventTypeDefinition
	id := r.URL.Query().Get("edit_type_id")
	if r.Method == http.MethodPost {
		id = r.PathValue("id")
	}
	if id != "" {
		items, err := s.lifecycle.EventTypes(r.Context(), actor)
		if err != nil {
			s.renderError(w, r, 500, err)
			return
		}
		for _, item := range items {
			if item.ID == id {
				copy := item
				editing = &copy
				break
			}
		}
		if editing == nil {
			s.renderNotFound(w, actor, "validation.event_type")
			return
		}
	}
	form := eventTypeFormData{Cashflow: "neutral"}
	if editing != nil {
		form.Name = editing.Name
		form.Cashflow = string(editing.Cashflow)
	}
	if r.Method == http.MethodPost && message != "" {
		form = eventTypeFormFromRequest(r)
	}
	s.render(w, status, "event_types", pageData{Title: textFor(actor.Locale, "types.title"), ReturnTo: eventTypesURL(query, state, page), Principal: &actor, CSRFToken: s.ensureCSRF(w, r), CanManageLifecycle: actor.Can(application.CapabilityManageLifecycle), EventTypes: result.Types, EditingEventType: editing, EventTypeForm: form, Error: message, TableQuery: query, TableFilter: state, TablePage: page, TableTotalPages: pages, TableTotal: result.Total, TablePreviousURL: previous, TableNextURL: next})
}
func eventTypesURL(query, state string, page int) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if state != "" {
		values.Set("status", state)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	return "/admin/event-types?" + values.Encode()
}
func (s *Server) createManagedEventType(w http.ResponseWriter, r *http.Request, actor application.Principal) {
	_, err := s.lifecycle.CreateEventType(r.Context(), actor, application.CreateAssetEventType{Name: r.FormValue("name"), Cashflow: domain.AssetEventCashflow(r.FormValue("cashflow"))})
	if err != nil {
		s.eventTypeError(w, r, actor, err)
		return
	}
	http.Redirect(w, r, "/admin/event-types", http.StatusSeeOther)
}
func (s *Server) updateAssetEventType(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.lifecycle.UpdateEventType(r.Context(), actor, r.PathValue("id"), application.UpdateEventType{Name: r.FormValue("name"), Cashflow: domain.AssetEventCashflow(r.FormValue("cashflow"))})
	if err != nil {
		s.eventTypeError(w, r, actor, err)
		return
	}
	http.Redirect(w, r, "/admin/event-types", http.StatusSeeOther)
}
func (s *Server) setAssetEventTypeStatus(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	enabled := r.FormValue("enabled")
	if enabled != "0" && enabled != "1" {
		s.eventTypeError(w, r, actor, application.NewInputError("validation.filter_invalid"))
		return
	}
	_, err := s.lifecycle.SetEventTypeEnabled(r.Context(), actor, r.PathValue("id"), enabled == "1")
	if err != nil {
		s.eventTypeError(w, r, actor, err)
		return
	}
	http.Redirect(w, r, "/admin/event-types", http.StatusSeeOther)
}
func (s *Server) eventTypeError(w http.ResponseWriter, r *http.Request, actor application.Principal, err error) {
	if errors.Is(err, application.ErrForbidden) {
		s.renderForbidden(w, actor, "error.forbidden_lifecycle")
		return
	}
	s.renderEventTypes(w, r, http.StatusUnprocessableEntity, actor, s.userError(actor.Locale, err))
}
