package web

import (
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"net/http"
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
	if r.FormValue("query_again") == "1" {
		s.renderMarket(w, r, a, 200, "", nil)
		return
	}
	q, e := s.options.Market.Preview(r.Context(), a, application.MarketQuery{Keyword: r.FormValue("keyword"), FilterCriteria: r.FormValue("filter_criteria")})
	if e != nil {
		s.renderMarket(w, r, a, 422, textFor(a.Locale, application.MarketErrorCode(e)), nil)
		return
	}
	s.renderMarket(w, r, a, 200, "", &q)
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
	m, e := s.options.Market.Create(r.Context(), a, application.CreateMarketItem{Name: r.FormValue("name"), Query: application.MarketQuery{Keyword: r.FormValue("keyword"), FilterCriteria: r.FormValue("filter_criteria")}, ConfirmedModel: r.FormValue("confirmed_model")})
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
