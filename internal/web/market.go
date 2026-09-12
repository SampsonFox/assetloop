package web

import (
	"encoding/json"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"net/http"
	"strconv"
	"strings"
)

type marketPageData struct {
	Items                        []domain.MarketItem
	Editing                      *domain.MarketItem
	Preview                      *application.MarketQuote
	Name, Keyword, Filter, Model string
	Configured                   bool
	HasItems                     bool
	Latest                       *domain.MarketPrice
	Discovery                    application.MarketDiscoveryState
	Specs                        []marketSpecRow
	ExtraSpecs                   []marketSpecRow
	Selection                    *domain.MarketSelection
}

func (s *Server) marketPage(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderMarket(w, r, a, 200, "", nil)
}
func (s *Server) renderMarket(w http.ResponseWriter, r *http.Request, a application.Principal, status int, message string, preview *application.MarketQuote) {
	if s.options.Market == nil {
		http.NotFound(w, r)
		return
	}
	items, e := s.options.Market.List(r.Context(), a)
	if e != nil {
		s.renderError(w, r, 500, e)
		return
	}
	data := pageData{Title: textFor(a.Locale, "market.title"), Principal: &a, CSRFToken: s.ensureCSRF(w, r), ReturnTo: "/admin/market", CanManageCatalog: a.Can(application.CapabilityManageCatalog), Error: message}
	data.Market = marketPageData{Items: items, Preview: preview, Name: r.FormValue("name"), Keyword: r.FormValue("keyword"), Filter: r.FormValue("filter_criteria"), Configured: s.options.Market.Configured()}
	data.Market.HasItems = len(items) > 0
	if preview != nil {
		data.Market.Model = preview.ModelDesc
	}
	edit := r.URL.Query().Get("edit")
	if r.PathValue("id") != "" {
		edit = r.PathValue("id")
	}
	if edit != "" {
		for _, m := range items {
			if m.ID == edit {
				copy := m
				data.Market.Editing = &copy
				data.Market.Name = m.Name
				break
			}
		}
		if data.Market.Editing == nil {
			http.NotFound(w, r)
			return
		}
	}
	if data.CanManageCatalog && data.Market.Editing == nil {
		id := r.FormValue("draft_id")
		var state application.MarketDiscoveryState
		var err error
		if id != "" {
			state, err = s.options.Market.MarketDiscovery(r.Context(), a, id)
		}
		if id == "" || err != nil {
			if r.Header.Get("X-Assetloop-Drawer") == "market-editor" || r.URL.Query().Get("dialog") == "market-editor" || r.Method == "POST" {
				state, err = s.options.Market.NewMarketDiscovery(r.Context(), a, application.MarketQuery{Keyword: data.Market.Keyword, FilterCriteria: data.Market.Filter}, false)
			}
		}
		if err != nil {
			data.Error = textFor(a.Locale, application.MarketErrorCode(err))
		}
		data.Market.Discovery = state
		data.Market.Preview = state.Quote
		if data.Market.Name == "" && state.Quote != nil {
			data.Market.Name = state.Query.Keyword
		}
		if state.Step == "search" || state.Step == "detail" {
			data.Market.Keyword = state.SearchQuery.Keyword
			data.Market.Filter = state.SearchQuery.FilterCriteria
		} else if state.ID != "" {
			data.Market.Keyword = state.Query.Keyword
			data.Market.Filter = state.Query.FilterCriteria
		}
		if state.Detail != nil {
			data.Market.Selection = &state.Detail.Selection
		}
	}
	if data.Market.Editing != nil && data.Market.Editing.SelectionJSON != "" {
		var snapshot domain.MarketSelection
		if json.Unmarshal([]byte(data.Market.Editing.SelectionJSON), &snapshot) == nil {
			data.Market.Selection = &snapshot
		}
	}
	if data.Market.Selection != nil {
		data.Market.Specs, data.Market.ExtraSpecs = marketSpecRows(*data.Market.Selection)
	}
	data.TableQuery = strings.TrimSpace(r.URL.Query().Get("q"))
	filtered := make([]domain.MarketItem, 0, len(items))
	for _, m := range items {
		if strings.Contains(strings.ToLower(m.Name+" "+m.ModelDesc), strings.ToLower(data.TableQuery)) {
			filtered = append(filtered, m)
		}
	}
	page := queryPage(r)
	pages := (len(filtered) + 24) / 25
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * 25
	end := start + 25
	if end > len(filtered) {
		end = len(filtered)
	}
	data.Market.Items = filtered[start:end]
	data.TablePage = page
	data.TableTotalPages = pages
	data.TableTotal = len(filtered)
	data.TablePreviousURL = tableURL("/admin/market", data.TableQuery, "", "", "", "", "", "", page-1)
	data.TableNextURL = tableURL("/admin/market", data.TableQuery, "", "", "", "", "", "", page+1)
	s.render(w, status, "market", data)
}
func (s *Server) marketPreview(w http.ResponseWriter, r *http.Request) {
	s.marketDiscoveryAction(w, r, "preview")
}
func (s *Server) marketDiscover(w http.ResponseWriter, r *http.Request) {
	s.marketDiscoveryAction(w, r, r.FormValue("market_action"))
}
func (s *Server) marketDiscoveryAction(w http.ResponseWriter, r *http.Request, action string) {
	if !s.verifyCSRF(w, r) {
		return
	}
	a, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if s.options.Market == nil {
		http.NotFound(w, r)
		return
	}
	if !a.Can(application.CapabilityManageCatalog) {
		http.Error(w, "Forbidden", 403)
		return
	}
	q := application.MarketQuery{Keyword: r.FormValue("keyword"), FilterCriteria: r.FormValue("filter_criteria")}
	id := r.FormValue("draft_id")
	if id == "" {
		state, e := s.options.Market.NewMarketDiscovery(r.Context(), a, q, action == "preview")
		if e != nil {
			s.renderMarket(w, r, a, 422, textFor(a.Locale, application.MarketErrorCode(e)), nil)
			return
		}
		id = state.ID
		r.Form.Set("draft_id", id)
	}
	var err error
	if r.FormValue("query_again") == "1" {
		action = "edit"
	}
	switch action {
	case "find", "next":
		_, err = s.options.Market.SearchProducts(r.Context(), a, id, q, action == "next")
	case "detail":
		index, e := strconv.Atoi(r.FormValue("candidate"))
		if e != nil {
			err = application.ErrMarketInvalid
		} else {
			_, err = s.options.Market.GetProductDetail(r.Context(), a, id, index)
		}
	case "preview":
		_, err = s.options.Market.PreviewMarketDiscovery(r.Context(), a, id, q)
	default:
		_, err = s.options.Market.EditMarketDiscovery(r.Context(), a, id, action, q)
	}
	status, message := 200, ""
	if err != nil {
		status = 422
		message = textFor(a.Locale, application.MarketErrorCode(err))
	}
	s.renderMarket(w, r, a, status, message, nil)
}
func (s *Server) marketCreate(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	a, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if s.options.Market == nil {
		http.NotFound(w, r)
		return
	}
	if !a.Can(application.CapabilityManageCatalog) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	m, e := s.options.Market.SaveMarketDiscovery(r.Context(), a, r.FormValue("draft_id"), r.FormValue("name"), r.FormValue("accept_scope") == "1")
	if e != nil {
		s.renderMarket(w, r, a, 422, textFor(a.Locale, application.MarketErrorCode(e)), nil)
		return
	}
	if drawerSaved(w, "market-item", m.ID, m.Name, m.Enabled) {
		return
	}
	http.Redirect(w, r, "/admin/market", 303)
}
func (s *Server) marketUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	a, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if s.options.Market == nil {
		http.NotFound(w, r)
		return
	}
	if !a.Can(application.CapabilityManageCatalog) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	e := s.options.Market.Update(r.Context(), a, r.PathValue("id"), r.FormValue("name"), r.FormValue("enabled") == "1")
	if e != nil {
		s.renderMarket(w, r, a, 422, textFor(a.Locale, application.MarketErrorCode(e)), nil)
		return
	}
	if drawerSaved(w, "market-item", r.PathValue("id"), r.FormValue("name"), r.FormValue("enabled") == "1") {
		return
	}
	http.Redirect(w, r, "/admin/market", 303)
}
func (s *Server) marketRefresh(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	a, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if s.options.Market == nil {
		http.NotFound(w, r)
		return
	}
	if !a.Can(application.CapabilityManageCatalog) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	e := s.options.Market.Refresh(r.Context(), a, r.PathValue("id"))
	if e != nil {
		s.renderMarket(w, r, a, 422, textFor(a.Locale, application.MarketErrorCode(e)), nil)
		return
	}
	http.Redirect(w, r, "/admin/market", 303)
}

type marketSpecRow struct {
	Label, Name, Value string
	Explanations       []string
	Options            []domain.MarketSpecificationOption
}

func marketSpecRows(selection domain.MarketSelection) ([]marketSpecRow, []marketSpecRow) {
	keys := []string{"model", "storage", "ram", "color", "version", "condition"}
	rows := make([]marketSpecRow, len(keys))
	var extra []marketSpecRow
	for i, key := range keys {
		rows[i].Label = "market.spec_" + key
	}
	for _, spec := range selection.Specifications {
		row := marketSpecRow{Name: spec.Name, Value: application.MarketSpecValue(spec), Explanations: spec.Explanations, Options: spec.ValueOptions}
		kind := application.MarketSpecKind(spec.Name)
		found := false
		for i, key := range keys {
			if kind == key && rows[i].Name == "" {
				row.Label = rows[i].Label
				rows[i] = row
				found = true
				break
			}
		}
		if !found {
			extra = append(extra, row)
		}
	}
	if rows[5].Value == "" {
		rows[5].Value = selection.Condition
	}
	return rows, extra
}
