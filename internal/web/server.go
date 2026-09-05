package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

const (
	sessionCookie   = "assetloop_session"
	csrfCookie      = "assetloop_csrf"
	assetViewCookie = "assetloop_asset_view"
)

//go:embed templates/*.html static/*
var assets embed.FS

type Pinger interface {
	PingContext(context.Context) error
}

type Options struct {
	AuthMode      string
	SecureCookies bool
}

type Server struct {
	auth      *application.AuthService
	catalog   *application.CatalogService
	lifecycle *application.LifecycleService
	db        Pinger
	options   Options
	templates map[string]*template.Template
	limiter   *loginLimiter
}

type pageData struct {
	Title              string
	Locale             application.Locale
	Theme              application.Theme
	Strings            map[string]string
	ReturnTo           string
	CSRFToken          string
	Error              string
	Principal          *application.Principal
	Members            []application.Member
	Categories         []domain.ItemCategory
	Models             []domain.ProductModel
	Variants           []domain.ProductVariant
	Assets             []domain.Asset
	AssetSummaries     map[string]domain.AssetSummary
	Asset              *domain.Asset
	CanManageCatalog   bool
	Events             []domain.AssetEvent
	Summary            domain.AssetSummary
	BaseCurrency       string
	BaseCurrencyLocked bool
	NowValue           string
	CanManageLifecycle bool
	EventTypes         []domain.AssetEventTypeDefinition
	EventTypeError     string
	EventTypeForm      eventTypeFormData
	AssetCount         int
	TotalExpenseMinor  int64
	TotalIncomeMinor   int64
	TotalNetMinor      int64
	AssetView          string
	AssetQuery         string
	AssetStatus        string
	AssetSort          string
	AssetDirection     string
	AssetSortURLs      map[string]string
	AssetTotal         int
	AssetPage          int
	AssetTotalPages    int
	AssetPreviousURL   string
	AssetNextURL       string
	AssetListURL       string
	AssetGridURL       string
	AssetClearURL      string
	AssetHasFilters    bool
	AssetAdvanced      bool
	TableQuery         string
	TableFilter        string
	TableSort          string
	TableDirection     string
	TableShowVoided    bool
	TableAdvanced      bool
	TableTotal         int
	TablePage          int
	TableTotalPages    int
	TablePreviousURL   string
	TableNextURL       string
	TableClearURL      string
	TableHasFilters    bool
	TableSortURLs      map[string]string
	CategoryIcons      []application.CategoryIconOption
	CatalogFlow        string
	AssetFormAction    string
	AssetFormEditing   bool
	NavAssets          bool
	NavCatalog         bool
	NavMembers         bool
	EventForm          eventFormData
}

type eventFormData struct {
	RequestKey        string
	Type              string
	OccurredAt        string
	Amount            string
	Currency          string
	Source            string
	ExternalReference string
	FXRate            string
	FXRateDate        string
	FXRateSource      string
	Notes             string
}

type eventTypeFormData struct {
	Name     string
	Cashflow string
}

func New(auth *application.AuthService, catalog *application.CatalogService, lifecycle *application.LifecycleService, db Pinger, options Options) (*Server, error) {
	templates := map[string]*template.Template{}
	funcs := template.FuncMap{
		"money": domain.FormatMinor, "eventClass": eventClass,
		"t": func(values map[string]string, key string) string {
			if value := values[key]; value != "" {
				return value
			}
			return messages[application.LocaleZhCN][key]
		},
		"roleLabel": func(values map[string]string, role application.Role) string { return values["role."+string(role)] },
		"userInitial": func(value string) string {
			runes := []rune(strings.TrimSpace(value))
			if len(runes) == 0 {
				return "?"
			}
			return strings.ToUpper(string(runes[0]))
		},
		"eventLabel": func(values map[string]string, value domain.AssetEventType) string {
			if label := values["event."+string(value)]; label != "" {
				return label
			}
			return string(value)
		},
		"eventTypeLabel": func(values map[string]string, value domain.AssetEventTypeDefinition) string {
			if value.BuiltIn {
				return values["event."+value.Name]
			}
			return value.Name
		},
		"statusLabel":   func(values map[string]string, value string) string { return values["status."+value] },
		"productImage":  productImage,
		"iconLabel":     func(values map[string]string, key string) string { return values["icon."+key] },
		"dateTime":      func(value time.Time) string { return value.Local().Format("2006-01-02 15:04") },
		"dateTimeInput": func(value time.Time) string { return value.Local().Format("2006-01-02T15:04") },
		"date":          func(value time.Time) string { return value.Format("2006-01-02") },
		"ariaSort": func(current, direction, column string) string {
			if current != column {
				return "none"
			}
			if direction == "asc" {
				return "ascending"
			}
			return "descending"
		},
		"sortMark": func(current, direction, column string) string {
			if current != column {
				return "↕"
			}
			if direction == "asc" {
				return "↑"
			}
			return "↓"
		},
		"rate": formatRate, "canCorrect": func(event domain.AssetEvent) bool { return event.Type != domain.AssetEventVoid && !event.IsVoided },
	}
	for _, page := range []string{"setup", "login", "dashboard", "members", "assets", "catalog", "asset", "asset_form", "event_correct", "error"} {
		parsed, err := template.New("base.html").Funcs(funcs).ParseFS(assets, "templates/base.html", "templates/catalog_drawers.html", "templates/"+page+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", page, err)
		}
		templates[page] = parsed
	}
	return &Server{auth: auth, catalog: catalog, lifecycle: lifecycle, db: db, options: options, templates: templates, limiter: newLoginLimiter(5, 5*time.Minute)}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /{$}", s.assetsPage)
	mux.HandleFunc("GET /overview", s.dashboard)
	mux.HandleFunc("GET /setup", s.setupForm)
	mux.HandleFunc("POST /setup", s.setup)
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("POST /preferences", s.updatePreferences)
	mux.HandleFunc("POST /locale", s.updateLocale)
	mux.HandleFunc("GET /admin/members", s.members)
	mux.HandleFunc("POST /admin/members", s.addMember)
	mux.HandleFunc("GET /catalog", s.legacyCatalog)
	mux.HandleFunc("GET /admin/catalog", s.catalogPage)
	mux.HandleFunc("POST /admin/catalog/categories", s.createCategory)
	mux.HandleFunc("POST /admin/catalog/categories/{id}", s.updateCategory)
	mux.HandleFunc("POST /admin/catalog/models", s.createModel)
	mux.HandleFunc("POST /admin/catalog/models/{id}", s.updateModel)
	mux.HandleFunc("POST /admin/catalog/variants", s.createVariant)
	mux.HandleFunc("POST /admin/catalog/variants/{id}", s.updateVariant)
	mux.HandleFunc("POST /admin/catalog/variants/{id}/delete", s.deleteVariant)
	mux.HandleFunc("GET /assets/new", s.newAssetForm)
	mux.HandleFunc("POST /assets", s.createAsset)
	mux.HandleFunc("GET /assets/{id}/edit", s.editAssetForm)
	mux.HandleFunc("POST /assets/{id}", s.updateAsset)
	mux.HandleFunc("GET /assets/{id}", s.assetDetail)
	mux.HandleFunc("POST /assets/{id}/events", s.createAssetEvent)
	mux.HandleFunc("POST /admin/event-types", s.createAssetEventType)
	mux.HandleFunc("GET /events/{id}/correct", s.correctEventForm)
	mux.HandleFunc("POST /events/{id}/correct", s.correctEvent)
	staticFS, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return securityHeaders(mux)
}

func (s *Server) catalogPage(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageCatalog) {
		s.renderForbidden(w, principal, "error.forbidden_catalog")
		return
	}
	s.renderCatalog(w, r, http.StatusOK, principal, "")
}

func (s *Server) legacyCatalog(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusPermanentRedirect)
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	category, err := s.catalog.CreateCategory(r.Context(), principal, application.CreateCategory{Name: r.FormValue("name"), IconKey: r.FormValue("icon_key")})
	if err != nil {
		s.renderCatalogMutationError(w, r, principal, err)
		return
	}
	s.redirectAfterCatalogCreate(w, r, "model-drawer", "category_id", category.ID)
}

func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.catalog.UpdateCategory(r.Context(), principal, application.UpdateCategory{ID: r.PathValue("id"), Name: r.FormValue("name"), IconKey: r.FormValue("icon_key")})
	if err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
}

func (s *Server) createModel(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	model, err := s.catalog.CreateModel(r.Context(), principal, application.CreateModel{
		CategoryID: r.FormValue("category_id"), Name: r.FormValue("name"),
	})
	if err != nil {
		s.renderCatalogMutationError(w, r, principal, err)
		return
	}
	if r.FormValue("flow") == "asset" {
		s.redirectAfterCatalogCreate(w, r, "variant-drawer", "model_id", model.ID)
		return
	}
	s.redirectToModelEditor(w, r, model.ID)
}

func (s *Server) updateModel(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.catalog.UpdateModel(r.Context(), principal, application.UpdateModel{ID: r.PathValue("id"), CategoryID: r.FormValue("category_id"), Name: r.FormValue("name")})
	if err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	s.redirectToModelEditor(w, r, r.PathValue("id"))
}

func (s *Server) createVariant(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	variant, err := s.catalog.CreateVariant(r.Context(), principal, application.CreateVariant{
		ModelID: r.FormValue("model_id"), Name: r.FormValue("name"),
	})
	if err != nil {
		s.renderCatalogMutationError(w, r, principal, err)
		return
	}
	if r.FormValue("flow") == "asset" {
		query := url.Values{"variant_id": {variant.ID}}
		http.Redirect(w, r, "/assets/new?"+query.Encode(), http.StatusSeeOther)
		return
	}
	s.redirectToModelEditor(w, r, variant.ModelID)
}

func (s *Server) updateVariant(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	variant, err := s.catalog.UpdateVariant(r.Context(), principal, application.UpdateVariant{ID: r.PathValue("id"), ModelID: r.FormValue("model_id"), Name: r.FormValue("name")})
	if err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	modelID := r.FormValue("return_model_id")
	if modelID == "" {
		modelID = variant.ModelID
	}
	s.redirectToModelEditor(w, r, modelID)
}

func (s *Server) deleteVariant(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if err := s.catalog.DeleteVariant(r.Context(), principal, r.PathValue("id")); err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	s.redirectToModelEditor(w, r, r.FormValue("return_model_id"))
}

func (s *Server) createAsset(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	created, err := s.catalog.CreateAsset(r.Context(), principal, application.CreateCatalogAsset{
		VariantID: r.FormValue("variant_id"), DisplayName: r.FormValue("display_name"),
		SerialNumber: r.FormValue("serial_number"), Color: r.FormValue("color"),
		PurchaseChannel: r.FormValue("purchase_channel"), Notes: r.FormValue("notes"),
	})
	if err != nil {
		s.renderAssetMutationError(w, r, principal, assetFromForm(r, ""), err)
		return
	}
	http.Redirect(w, r, "/assets/"+created.ID, http.StatusSeeOther)
}

func (s *Server) newAssetForm(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageCatalog) {
		s.renderForbidden(w, principal, "error.forbidden_asset")
		return
	}
	s.renderAssetForm(w, r, http.StatusOK, principal, domain.Asset{VariantID: strings.TrimSpace(r.URL.Query().Get("variant_id"))}, "")
}

func (s *Server) editAssetForm(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageCatalog) {
		s.renderForbidden(w, principal, "error.forbidden_asset")
		return
	}
	asset, err := s.catalog.GetAsset(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	s.renderAssetForm(w, r, http.StatusOK, principal, asset, "")
}

func (s *Server) updateAsset(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.catalog.UpdateAsset(r.Context(), principal, application.UpdateCatalogAsset{
		ID: r.PathValue("id"), VariantID: r.FormValue("variant_id"), DisplayName: r.FormValue("display_name"),
		SerialNumber: r.FormValue("serial_number"), Color: r.FormValue("color"),
		PurchaseChannel: r.FormValue("purchase_channel"), Notes: r.FormValue("notes"),
	})
	if err != nil {
		s.renderAssetMutationError(w, r, principal, assetFromForm(r, r.PathValue("id")), err)
		return
	}
	http.Redirect(w, r, "/assets/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *Server) assetDetail(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderAsset(w, r, http.StatusOK, principal, r.PathValue("id"), "", "")
}

func (s *Server) createAssetEvent(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	cmd, err := s.recordEventFromForm(r, principal, r.PathValue("id"), domain.AssetEventType(r.FormValue("event_type")))
	if err == nil {
		_, err = s.lifecycle.Record(r.Context(), principal, cmd)
	}
	if err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.renderForbidden(w, principal, "error.forbidden_lifecycle")
			return
		}
		s.renderAsset(w, r, http.StatusUnprocessableEntity, principal, r.PathValue("id"), s.userError(principal.Locale, err), "")
		return
	}
	http.Redirect(w, r, "/assets/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *Server) createAssetEventType(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	assetID := r.FormValue("asset_id")
	if _, err := s.catalog.GetAsset(r.Context(), principal, assetID); err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	eventType, err := s.lifecycle.CreateEventType(r.Context(), principal, application.CreateAssetEventType{
		Name: r.FormValue("name"), Cashflow: domain.AssetEventCashflow(r.FormValue("cashflow")),
	})
	if err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.renderForbidden(w, principal, "error.forbidden_lifecycle")
			return
		}
		s.renderAsset(w, r, http.StatusUnprocessableEntity, principal, assetID, "", s.userError(principal.Locale, err))
		return
	}
	location := "/assets/" + assetID + "?dialog=event-drawer&event_type=" + url.QueryEscape(eventType.Name) + "#add-event"
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (s *Server) correctEventForm(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.renderForbidden(w, principal, "error.forbidden_correct")
		return
	}
	event, err := s.lifecycle.GetEvent(r.Context(), principal, r.PathValue("id"))
	if err != nil || event.Type == domain.AssetEventVoid || event.IsVoided {
		s.renderNotFound(w, principal, "error.not_found_event")
		return
	}
	s.renderCorrectionForm(w, r, http.StatusOK, principal, event, "", eventFormForCorrection(event))
}

func (s *Server) correctEvent(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	original, err := s.lifecycle.GetEvent(r.Context(), principal, r.PathValue("id"))
	if err == nil {
		var cmd application.RecordEvent
		cmd, err = s.recordEventFromForm(r, principal, original.AssetID, original.Type)
		if err == nil {
			_, err = s.lifecycle.Correct(r.Context(), principal, original.ID, cmd)
		}
	}
	if err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.renderForbidden(w, principal, "error.forbidden_correct")
			return
		}
		if original.ID == "" {
			s.renderNotFound(w, principal, "error.not_found_event")
			return
		}
		baseCurrency, _, baseErr := s.lifecycle.BaseCurrency(r.Context(), principal)
		if baseErr != nil {
			s.renderError(w, r, http.StatusInternalServerError, baseErr)
			return
		}
		form := eventFormFromRequest(r, baseCurrency, time.Now().Local().Format("2006-01-02T15:04"))
		s.renderCorrectionForm(w, r, http.StatusUnprocessableEntity, principal, original, s.userError(principal.Locale, err), form)
		return
	}
	http.Redirect(w, r, "/assets/"+original.AssetID, http.StatusSeeOther)
}

func (s *Server) renderCorrectionForm(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, event domain.AssetEvent, message string, form eventFormData) {
	baseCurrency, locked, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.render(w, status, "event_correct", pageData{
		Title: textFor(principal.Locale, "correct.heading"), CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message, ReturnTo: r.URL.RequestURI(),
		Events: []domain.AssetEvent{event}, BaseCurrency: baseCurrency, BaseCurrencyLocked: locked, EventForm: form,
		CanManageLifecycle: principal.Can(application.CapabilityManageLifecycle),
	})
}

func eventFormForCorrection(event domain.AssetEvent) eventFormData {
	currency, amountMinor := event.BaseCurrency, event.BaseAmountMinor
	form := eventFormData{RequestKey: randomToken(), OccurredAt: event.OccurredAt.Local().Format("2006-01-02T15:04"), Source: "manual-correction"}
	if event.FX != nil {
		currency, amountMinor = event.FX.OriginalCurrency, event.FX.OriginalAmountMinor
		form.FXRate = formatRate(event.FX.RateScaled)
		form.FXRateDate = event.FX.RateDate.Format("2006-01-02")
		form.FXRateSource = event.FX.RateSource
	}
	form.Currency = currency
	form.Amount = strings.TrimSuffix(domain.FormatMinor(amountMinor, currency), " "+currency)
	return form
}

func (s *Server) renderAsset(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, assetID, message, eventTypeMessage string) {
	asset, err := s.catalog.GetAsset(r.Context(), principal, assetID)
	if err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	eventType := strings.TrimSpace(r.URL.Query().Get("event_type"))
	sortKey := valueOrDefault(strings.TrimSpace(r.URL.Query().Get("sort")), "occurred")
	direction := valueOrDefault(strings.TrimSpace(r.URL.Query().Get("direction")), "asc")
	showVoided := r.URL.Query().Get("show_voided") == "1"
	page := queryPage(r)
	const pageSize = 20
	result, err := s.lifecycle.TimelinePage(r.Context(), principal, assetID, application.EventListOptions{
		Query: query, Type: eventType, Sort: sortKey, Direction: direction, ShowVoided: showVoided, Page: page, PageSize: pageSize,
	})
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	eventTypes, err := s.lifecycle.EventTypes(r.Context(), principal)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if result.Total == 0 && page > 1 {
		http.Redirect(w, r, eventListURL(assetID, query, eventType, sortKey, direction, showVoided, 1), http.StatusFound)
		return
	}
	totalPages, previousURL, nextURL := tablePagination(result.Total, page, pageSize, func(target int) string {
		return eventListURL(assetID, query, eventType, sortKey, direction, showVoided, target)
	})
	_, locked, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	nowValue := time.Now().Local().Format("2006-01-02T15:04")
	s.render(w, status, "asset", pageData{
		Title: asset.DisplayName, CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message, ReturnTo: r.URL.RequestURI(),
		Asset: &asset, CanManageCatalog: principal.Can(application.CapabilityManageCatalog), Events: result.Events,
		Summary: result.Summary, BaseCurrency: result.Summary.BaseCurrency, BaseCurrencyLocked: locked,
		NowValue:           nowValue,
		EventForm:          eventFormFromRequest(r, result.Summary.BaseCurrency, nowValue),
		EventTypes:         eventTypes,
		EventTypeError:     eventTypeMessage,
		EventTypeForm:      eventTypeFormFromRequest(r),
		CanManageLifecycle: principal.Can(application.CapabilityManageLifecycle),
		TableQuery:         query, TableFilter: eventType, TableSort: sortKey, TableDirection: direction, TableShowVoided: showVoided,
		TableTotal: result.Total, TablePage: page, TableTotalPages: totalPages,
		TablePreviousURL: previousURL, TableNextURL: nextURL,
		TableClearURL:   eventListURL(assetID, "", "", "occurred", "asc", false, 1),
		TableHasFilters: query != "" || eventType != "" || sortKey != "occurred" || direction != "asc" || showVoided,
		TableAdvanced:   eventType != "" || sortKey != "occurred" || direction != "asc" || showVoided,
	})
}

func eventListURL(assetID, query, eventType, sortKey, direction string, showVoided bool, page int) string {
	path := tableURL("/assets/"+assetID, query, "event_type", eventType, sortKey, direction, "occurred", "asc", page)
	if showVoided {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		path += separator + "show_voided=1"
	}
	return path + "#lifecycle-timeline"
}

func eventFormFromRequest(r *http.Request, baseCurrency, nowValue string) eventFormData {
	form := eventFormData{
		RequestKey: randomToken(),
		Type:       "purchase", OccurredAt: nowValue, Currency: baseCurrency, Source: "manual",
		FXRateDate: strings.SplitN(nowValue, "T", 2)[0],
	}
	if r.Method != http.MethodPost {
		return form
	}
	form.Type = r.FormValue("event_type")
	if key := r.FormValue("request_key"); key != "" {
		form.RequestKey = key
	}
	form.OccurredAt = r.FormValue("occurred_at")
	form.Amount = r.FormValue("amount")
	form.Currency = r.FormValue("currency")
	form.Source = r.FormValue("source")
	form.ExternalReference = r.FormValue("external_reference")
	form.FXRate = r.FormValue("fx_rate")
	form.FXRateDate = r.FormValue("fx_rate_date")
	form.FXRateSource = r.FormValue("fx_rate_source")
	form.Notes = r.FormValue("notes")
	return form
}

func eventTypeFormFromRequest(r *http.Request) eventTypeFormData {
	form := eventTypeFormData{Cashflow: string(domain.AssetEventNeutral)}
	if r.Method == http.MethodPost && r.URL.Path == "/admin/event-types" {
		form.Name = r.FormValue("name")
		form.Cashflow = r.FormValue("cashflow")
	}
	return form
}

func productImage(asset *domain.Asset) string {
	if asset != nil && asset.Model == "iPhone 17 Pro" {
		return "/static/product-demo-iphone-17-pro-deep-blue.jpg"
	}
	return ""
}

func (s *Server) recordEventFromForm(r *http.Request, principal application.Principal, assetID string, eventType domain.AssetEventType) (application.RecordEvent, error) {
	currency, err := domain.NormalizeCurrency(r.FormValue("currency"))
	if err != nil {
		return application.RecordEvent{}, err
	}
	amount, err := domain.ParseMajorAmount(r.FormValue("amount"), currency)
	if err != nil {
		return application.RecordEvent{}, err
	}
	occurredAt, err := parseFormTime(r.FormValue("occurred_at"))
	if err != nil {
		return application.RecordEvent{}, err
	}
	cmd := application.RecordEvent{
		RequestKey: r.FormValue("request_key"),
		AssetID:    assetID, Type: eventType, AmountMinor: amount, Currency: currency,
		OccurredAt: occurredAt, Source: r.FormValue("source"),
		ExternalReference: r.FormValue("external_reference"), Notes: r.FormValue("notes"),
	}
	baseCurrency, _, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		return application.RecordEvent{}, err
	}
	if currency != baseCurrency {
		cmd.FXRateScaled, err = domain.ParseFXRate(r.FormValue("fx_rate"))
		if err != nil {
			return application.RecordEvent{}, err
		}
		cmd.FXRateDate, err = parseFormDate(r.FormValue("fx_rate_date"))
		if err != nil {
			return application.RecordEvent{}, err
		}
		cmd.FXRateSource = r.FormValue("fx_rate_source")
		cmd.FXConfirmed = true
	}
	return cmd, nil
}

func (s *Server) renderCatalogError(w http.ResponseWriter, r *http.Request, principal application.Principal, err error) {
	if errors.Is(err, application.ErrForbidden) {
		s.renderForbidden(w, principal, "error.forbidden_catalog")
		return
	}
	s.renderCatalog(w, r, http.StatusUnprocessableEntity, principal, s.userError(principal.Locale, err))
}

func (s *Server) renderCatalogMutationError(w http.ResponseWriter, r *http.Request, principal application.Principal, err error) {
	if r.FormValue("flow") == "asset" {
		s.renderAssetMutationError(w, r, principal, domain.Asset{}, err)
		return
	}
	s.renderCatalogError(w, r, principal, err)
}

func (s *Server) redirectAfterCatalogCreate(w http.ResponseWriter, r *http.Request, dialog, idKey, id string) {
	if r.FormValue("flow") != "asset" {
		http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
		return
	}
	query := url.Values{"dialog": {dialog}, idKey: {id}}
	http.Redirect(w, r, "/assets/new?"+query.Encode(), http.StatusSeeOther)
}

func (s *Server) redirectToModelEditor(w http.ResponseWriter, r *http.Request, modelID string) {
	query := url.Values{"dialog": {"model-drawer"}, "edit_model_id": {modelID}}
	http.Redirect(w, r, "/admin/catalog?"+query.Encode(), http.StatusSeeOther)
}

func (s *Server) renderAssetMutationError(w http.ResponseWriter, r *http.Request, principal application.Principal, asset domain.Asset, err error) {
	if errors.Is(err, application.ErrForbidden) {
		s.renderForbidden(w, principal, "error.forbidden_asset")
		return
	}
	s.renderAssetForm(w, r, http.StatusUnprocessableEntity, principal, asset, s.userError(principal.Locale, err))
}

func assetFromForm(r *http.Request, id string) domain.Asset {
	return domain.Asset{
		ID: id, VariantID: r.FormValue("variant_id"), DisplayName: r.FormValue("display_name"),
		SerialNumber: r.FormValue("serial_number"), Color: r.FormValue("color"),
		PurchaseChannel: r.FormValue("purchase_channel"), Notes: r.FormValue("notes"),
	}
}

func (s *Server) renderAssetForm(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, asset domain.Asset, message string) {
	snapshot, err := s.catalog.Snapshot(r.Context(), principal)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	for _, variant := range snapshot.Variants {
		if variant.ID == asset.VariantID {
			asset.Category = variant.CategoryName
			asset.CategoryIcon = variant.CategoryIcon
			asset.Model = variant.ModelName
			asset.Variant = variant.Name
			break
		}
	}
	editing := asset.ID != ""
	titleKey := "assets.create"
	action := "/assets"
	if editing {
		titleKey = "assets.edit"
		action = "/assets/" + asset.ID
	}
	returnTo := "/assets/new"
	if editing {
		returnTo = "/assets/" + asset.ID + "/edit"
	}
	if r.Method == http.MethodGet {
		returnTo = r.URL.RequestURI()
	}
	s.render(w, status, "asset_form", pageData{
		Title: textFor(principal.Locale, titleKey), CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message, ReturnTo: returnTo,
		Categories: snapshot.Categories, Models: snapshot.Models, Variants: snapshot.Variants, Asset: &asset,
		CanManageCatalog: true, CategoryIcons: application.CategoryIconOptions, CatalogFlow: "asset",
		AssetFormAction: action, AssetFormEditing: editing,
	})
}

func (s *Server) assetsPage(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderAssets(w, r, http.StatusOK, principal, "")
}

func (s *Server) renderAssets(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, message string) {
	view := assetView(r)
	if requested := strings.TrimSpace(r.URL.Query().Get("view")); requested == "list" || requested == "grid" {
		view = requested
		http.SetCookie(w, &http.Cookie{Name: assetViewCookie, Value: view, Path: "/", MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	sortKey := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortKey == "" {
		sortKey = "created"
	}
	direction := strings.TrimSpace(r.URL.Query().Get("direction"))
	if direction == "" {
		direction = "desc"
	}
	page := queryPage(r)
	const pageSize = 25
	result, err := s.catalog.ListAssetsWithSummary(r.Context(), principal, application.AssetListOptions{
		Query: query, Status: statusFilter, Sort: sortKey, Direction: direction, Page: page, PageSize: pageSize,
	})
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	totalPages := 0
	if result.Total > 0 {
		totalPages = (result.Total + pageSize - 1) / pageSize
	}
	if totalPages > 0 && page > totalPages {
		http.Redirect(w, r, assetsURL(view, query, statusFilter, sortKey, direction, totalPages), http.StatusFound)
		return
	}
	assetRows := make([]domain.Asset, 0, len(result.Assets))
	summaries := make(map[string]domain.AssetSummary, len(result.Assets))
	for _, row := range result.Assets {
		assetRows = append(assetRows, row.Asset)
		summaries[row.Asset.ID] = row.Summary
	}
	previousURL, nextURL := "", ""
	if page > 1 {
		previousURL = assetsURL(view, query, statusFilter, sortKey, direction, page-1)
	}
	if page < totalPages {
		nextURL = assetsURL(view, query, statusFilter, sortKey, direction, page+1)
	}
	sortURLs := map[string]string{}
	for _, column := range []string{"name", "model", "status", "net", "cost", "created"} {
		sortURLs[column] = assetsURL(view, query, statusFilter, column, nextSortDirection(sortKey, direction, column), 1)
	}
	s.render(w, status, "assets", pageData{
		Title: textFor(principal.Locale, "title.assets"), CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message, ReturnTo: r.URL.RequestURI(),
		Assets: assetRows, AssetSummaries: summaries, CanManageCatalog: principal.Can(application.CapabilityManageCatalog),
		CanManageLifecycle: principal.Can(application.CapabilityManageLifecycle), AssetView: view,
		AssetQuery: query, AssetStatus: statusFilter, AssetSort: sortKey, AssetDirection: direction, AssetSortURLs: sortURLs,
		AssetTotal: result.Total, AssetPage: page, AssetTotalPages: totalPages,
		AssetPreviousURL: previousURL, AssetNextURL: nextURL,
		AssetListURL: assetsURL("list", query, statusFilter, sortKey, direction, page), AssetGridURL: assetsURL("grid", query, statusFilter, sortKey, direction, page),
		AssetClearURL: assetsURL(view, "", "", "created", "desc", 1), AssetHasFilters: query != "" || (statusFilter != "" && statusFilter != "all") || sortKey != "created" || direction != "desc",
		AssetAdvanced: (statusFilter != "" && statusFilter != "all") || sortKey != "created" || direction != "desc",
	})
}

func assetsURL(view, query, status, sortKey, direction string, page int) string {
	values := url.Values{}
	if view == "grid" || view == "list" {
		values.Set("view", view)
	}
	if query != "" {
		values.Set("q", query)
	}
	if status != "" && status != "all" {
		values.Set("status", status)
	}
	if sortKey != "" && sortKey != "created" {
		values.Set("sort", sortKey)
	}
	if direction != "" && direction != "desc" {
		values.Set("direction", direction)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if len(values) == 0 {
		return "/"
	}
	return "/?" + values.Encode()
}

func catalogURL(query, categoryID, sortKey, direction string, page int) string {
	return tableURL("/admin/catalog", query, "category", categoryID, sortKey, direction, "category", "asc", page)
}

func membersURL(query, role, sortKey, direction string, page int) string {
	return tableURL("/admin/members", query, "role", role, sortKey, direction, "username", "asc", page)
}

func tableURL(path, query, filterName, filter, sortKey, direction, defaultSort, defaultDirection string, page int) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if filter != "" {
		values.Set(filterName, filter)
	}
	if sortKey != "" && sortKey != defaultSort {
		values.Set("sort", sortKey)
	}
	if direction != "" && direction != defaultDirection {
		values.Set("direction", direction)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

func queryPage(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func nextSortDirection(currentSort, currentDirection, column string) string {
	if currentSort == column && currentDirection == "asc" {
		return "desc"
	}
	return "asc"
}

func tablePagination(total, page, pageSize int, pageURL func(int) string) (int, string, string) {
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	previousURL, nextURL := "", ""
	if page > 1 {
		previousURL = pageURL(page - 1)
	}
	if page < totalPages {
		nextURL = pageURL(page + 1)
	}
	return totalPages, previousURL, nextURL
}

func assetView(r *http.Request) string {
	if cookie, err := r.Cookie(assetViewCookie); err == nil && cookie.Value == "grid" {
		return "grid"
	}
	return "list"
}

func (s *Server) renderCatalog(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, message string) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	categoryID := strings.TrimSpace(r.URL.Query().Get("category"))
	sortKey := valueOrDefault(strings.TrimSpace(r.URL.Query().Get("sort")), "category")
	direction := valueOrDefault(strings.TrimSpace(r.URL.Query().Get("direction")), "asc")
	page := queryPage(r)
	const pageSize = 25
	categories, err := s.catalog.Categories(r.Context(), principal)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	result, err := s.catalog.ListModelsWithVariants(r.Context(), principal, application.ModelListOptions{
		Query: query, CategoryID: categoryID, Sort: sortKey, Direction: direction, Page: page, PageSize: pageSize,
	})
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if result.Total == 0 && page > 1 {
		http.Redirect(w, r, catalogURL(query, categoryID, sortKey, direction, 1), http.StatusFound)
		return
	}
	totalPages, previousURL, nextURL := tablePagination(result.Total, page, pageSize, func(target int) string {
		return catalogURL(query, categoryID, sortKey, direction, target)
	})
	sortURLs := map[string]string{}
	for _, column := range []string{"category", "name", "created"} {
		sortURLs[column] = catalogURL(query, categoryID, column, nextSortDirection(sortKey, direction, column), 1)
	}
	s.render(w, status, "catalog", pageData{
		Title: textFor(principal.Locale, "title.catalog"), CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message, ReturnTo: r.URL.RequestURI(),
		Categories: categories, Models: result.Models, Variants: result.Variants, CanManageCatalog: principal.Can(application.CapabilityManageCatalog),
		CategoryIcons: application.CategoryIconOptions,
		TableQuery:    query, TableFilter: categoryID, TableSort: sortKey, TableDirection: direction,
		TableTotal: result.Total, TablePage: page, TableTotalPages: totalPages,
		TablePreviousURL: previousURL, TableNextURL: nextURL,
		TableClearURL: catalogURL("", "", "category", "asc", 1), TableHasFilters: query != "" || categoryID != "" || sortKey != "category" || direction != "asc",
		TableSortURLs: sortURLs,
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	summary, err := s.lifecycle.PortfolioSummary(r.Context(), principal)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	data := pageData{
		Title: textFor(principal.Locale, "title.overview"), CSRFToken: s.ensureCSRF(w, r), Principal: &principal, ReturnTo: r.URL.RequestURI(),
		AssetCount: summary.AssetCount, BaseCurrency: summary.BaseCurrency,
		TotalExpenseMinor: summary.ExpenseMinor, TotalIncomeMinor: summary.IncomeMinor, TotalNetMinor: summary.NetMinor,
	}
	s.render(w, http.StatusOK, "dashboard", data)
}

func (s *Server) setupForm(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := s.auth.NeedsSetup(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if !needsSetup {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	locale := s.localeForRequest(r, nil)
	s.render(w, http.StatusOK, "setup", pageData{Title: textFor(locale, "title.setup"), Locale: locale, CSRFToken: s.ensureCSRF(w, r), ReturnTo: r.URL.RequestURI()})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	locale := s.localeForRequest(r, nil)
	credential, err := s.auth.Setup(r.Context(), application.SetupAuth{
		TenantName: r.FormValue("tenant_name"), BaseCurrency: r.FormValue("base_currency"),
		Username: r.FormValue("username"), Password: r.FormValue("password"), Locale: locale,
	})
	if err != nil {
		s.render(w, http.StatusUnprocessableEntity, "setup", pageData{Title: textFor(locale, "title.setup"), Locale: locale, CSRFToken: s.ensureCSRF(w, r), ReturnTo: "/setup", Error: s.userError(locale, err)})
		return
	}
	s.setSessionCookie(w, credential)
	s.setLocaleCookie(w, credential.Principal.Locale)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := s.auth.NeedsSetup(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if needsSetup {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if _, err := s.principal(r); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	locale := s.localeForRequest(r, nil)
	s.render(w, http.StatusOK, "login", pageData{Title: textFor(locale, "title.login"), Locale: locale, CSRFToken: s.ensureCSRF(w, r), ReturnTo: r.URL.RequestURI()})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	key := clientIP(r)
	if !s.limiter.Allow(key, time.Now()) {
		http.Error(w, textFor(s.localeForRequest(r, nil), "error.login_rate"), http.StatusTooManyRequests)
		return
	}
	credential, err := s.auth.Login(r.Context(), application.Login{Username: r.FormValue("username"), Password: r.FormValue("password")})
	if err != nil {
		locale := s.localeForRequest(r, nil)
		s.render(w, http.StatusUnauthorized, "login", pageData{Title: textFor(locale, "title.login"), Locale: locale, CSRFToken: s.ensureCSRF(w, r), ReturnTo: "/login", Error: textFor(locale, "error.login")})
		return
	}
	s.limiter.Reset(key)
	s.setSessionCookie(w, credential)
	s.setLocaleCookie(w, credential.Principal.Locale)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = s.auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) updatePreferences(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	locale, localeOK := supportedLocale(r.FormValue("locale"))
	theme := application.Theme(r.FormValue("theme"))
	if !localeOK || (theme != application.ThemeSystem && theme != application.ThemeLight && theme != application.ThemeDark) {
		key := "validation.locale_invalid"
		if localeOK {
			key = "validation.theme_invalid"
		}
		s.render(w, http.StatusUnprocessableEntity, "error", pageData{Title: textFor(principal.Locale, "title.error"), Principal: &principal, CSRFToken: s.ensureCSRF(w, r), Error: textFor(principal.Locale, key), ReturnTo: "/"})
		return
	}
	returnTo, ok := safeReturnTo(r.FormValue("return_to"), "/")
	if !ok {
		s.render(w, http.StatusUnprocessableEntity, "error", pageData{Title: textFor(principal.Locale, "title.error"), Principal: &principal, CSRFToken: s.ensureCSRF(w, r), Error: textFor(principal.Locale, "error.return_to"), ReturnTo: "/"})
		return
	}
	if _, err := s.auth.UpdatePreferences(r.Context(), principal, application.UpdatePreferences{Locale: locale, Theme: theme}); err != nil {
		s.render(w, http.StatusUnprocessableEntity, "error", pageData{Title: textFor(principal.Locale, "title.error"), Principal: &principal, CSRFToken: s.ensureCSRF(w, r), Error: s.userError(principal.Locale, err), ReturnTo: "/"})
		return
	}
	s.setLocaleCookie(w, locale)
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

func (s *Server) updateLocale(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	current := s.localeForRequest(r, nil)
	locale, ok := supportedLocale(r.FormValue("locale"))
	if !ok {
		s.render(w, http.StatusUnprocessableEntity, "error", pageData{Title: textFor(current, "title.error"), Locale: current, Error: textFor(current, "validation.locale_invalid"), CSRFToken: s.ensureCSRF(w, r), ReturnTo: "/login"})
		return
	}
	returnTo, ok := safeReturnTo(r.FormValue("return_to"), "/login")
	if !ok {
		s.render(w, http.StatusUnprocessableEntity, "error", pageData{Title: textFor(locale, "title.error"), Locale: locale, Error: textFor(locale, "error.return_to"), CSRFToken: s.ensureCSRF(w, r), ReturnTo: "/login"})
		return
	}
	s.setLocaleCookie(w, locale)
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderMembers(w, r, http.StatusOK, principal, "")
}

func (s *Server) renderMembers(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, message string) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	sortKey := valueOrDefault(strings.TrimSpace(r.URL.Query().Get("sort")), "username")
	direction := valueOrDefault(strings.TrimSpace(r.URL.Query().Get("direction")), "asc")
	page := queryPage(r)
	const pageSize = 25
	result, err := s.auth.ListMembers(r.Context(), principal, application.MemberListOptions{
		Query: query, Role: role, Sort: sortKey, Direction: direction, Page: page, PageSize: pageSize,
	})
	if errors.Is(err, application.ErrForbidden) {
		s.renderForbidden(w, principal, "error.forbidden_members")
		return
	}
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if result.Total == 0 && page > 1 {
		http.Redirect(w, r, membersURL(query, role, sortKey, direction, 1), http.StatusFound)
		return
	}
	totalPages, previousURL, nextURL := tablePagination(result.Total, page, pageSize, func(target int) string {
		return membersURL(query, role, sortKey, direction, target)
	})
	sortURLs := map[string]string{}
	for _, column := range []string{"username", "role", "created"} {
		sortURLs[column] = membersURL(query, role, column, nextSortDirection(sortKey, direction, column), 1)
	}
	s.render(w, status, "members", pageData{
		Title: textFor(principal.Locale, "title.members"), CSRFToken: s.ensureCSRF(w, r), Principal: &principal,
		Members: result.Members, Error: message, ReturnTo: r.URL.RequestURI(),
		TableQuery: query, TableFilter: role, TableSort: sortKey, TableDirection: direction,
		TableTotal: result.Total, TablePage: page, TableTotalPages: totalPages,
		TablePreviousURL: previousURL, TableNextURL: nextURL,
		TableClearURL: membersURL("", "", "username", "asc", 1), TableHasFilters: query != "" || role != "" || sortKey != "username" || direction != "asc",
		TableSortURLs: sortURLs,
	})
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.auth.AddMember(r.Context(), principal, application.AddMember{Username: r.FormValue("username"), Password: r.FormValue("password"), Role: application.Role(r.FormValue("role"))})
	if errors.Is(err, application.ErrForbidden) {
		s.renderForbidden(w, principal, "error.forbidden_members")
		return
	}
	if err != nil {
		s.renderMembers(w, r, http.StatusUnprocessableEntity, principal, s.userError(principal.Locale, err))
		return
	}
	http.Redirect(w, r, "/admin/members", http.StatusSeeOther)
}

func (s *Server) principal(r *http.Request) (application.Principal, error) {
	if s.options.AuthMode == "disabled" {
		return s.auth.LocalPrincipal(r.Context())
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return application.Principal{}, application.ErrUnauthorized
	}
	return s.auth.Authenticate(r.Context(), cookie.Value)
}

func (s *Server) requirePrincipal(w http.ResponseWriter, r *http.Request) (application.Principal, bool) {
	principal, err := s.principal(r)
	if err == nil {
		return principal, true
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
	return application.Principal{}, false
}

func (s *Server) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	value := randomToken()
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: value, Path: "/", HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
	return value
}

func (s *Server) verifyCSRF(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookie)
	if err != nil || cookie.Value == "" {
		http.Error(w, textFor(s.localeForRequest(r, nil), "error.csrf"), http.StatusForbidden)
		return false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, textFor(s.localeForRequest(r, nil), "error.csrf"), http.StatusBadRequest)
		return false
	}
	want, got := []byte(cookie.Value), []byte(r.FormValue("csrf_token"))
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1 {
		http.Error(w, textFor(s.localeForRequest(r, nil), "error.csrf"), http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, credential application.SessionCredential) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: credential.Token, Path: "/", Expires: credential.ExpiresAt, HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
}

func (s *Server) setLocaleCookie(w http.ResponseWriter, locale application.Locale) {
	http.SetCookie(w, &http.Cookie{Name: localeCookie, Value: string(locale), Path: "/", MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
}

func (s *Server) localeForRequest(r *http.Request, principal *application.Principal) application.Locale {
	if principal != nil {
		if locale, ok := supportedLocale(string(principal.Locale)); ok {
			return locale
		}
	}
	if cookie, err := r.Cookie(localeCookie); err == nil {
		if locale, ok := supportedLocale(cookie.Value); ok {
			return locale
		}
	}
	return matchLocale(r.Header.Get("Accept-Language"))
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data pageData) {
	if data.Principal != nil {
		data.Locale = data.Principal.Locale
		data.Theme = data.Principal.Theme
	}
	if _, ok := supportedLocale(string(data.Locale)); !ok {
		data.Locale = application.LocaleZhCN
	}
	if data.Theme != application.ThemeLight && data.Theme != application.ThemeDark {
		data.Theme = application.ThemeSystem
	}
	data.Strings = stringsFor(data.Locale)
	if data.ReturnTo == "" {
		data.ReturnTo = "/"
	}
	if returnURL, err := url.Parse(data.ReturnTo); err == nil {
		data.NavAssets = returnURL.Path == "/" || strings.HasPrefix(returnURL.Path, "/assets/") || strings.HasPrefix(returnURL.Path, "/events/")
		data.NavCatalog = strings.HasPrefix(returnURL.Path, "/admin/catalog")
		data.NavMembers = strings.HasPrefix(returnURL.Path, "/admin/members")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.templates[name].ExecuteTemplate(w, "base", data); err != nil {
		panic(err)
	}
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, err error) {
	slog.Error("web request failed", "status", status, "error", err)
	locale := s.localeForRequest(r, nil)
	data := pageData{Title: textFor(locale, "title.error"), Locale: locale, Error: textFor(locale, "error.internal"), ReturnTo: "/"}
	if principal, principalErr := s.principal(r); principalErr == nil {
		data.Principal = &principal
		data.Title = textFor(principal.Locale, "title.error")
		data.Error = textFor(principal.Locale, "error.internal")
	}
	s.render(w, status, "error", data)
}

func (s *Server) renderForbidden(w http.ResponseWriter, principal application.Principal, key string) {
	s.render(w, http.StatusForbidden, "error", pageData{Title: textFor(principal.Locale, "title.forbidden"), Principal: &principal, Error: textFor(principal.Locale, key)})
}

func (s *Server) renderNotFound(w http.ResponseWriter, principal application.Principal, key string) {
	s.render(w, http.StatusNotFound, "error", pageData{Title: textFor(principal.Locale, "title.error"), Principal: &principal, Error: textFor(principal.Locale, key)})
}

func (s *Server) userError(locale application.Locale, err error) string {
	if value, ok := inputErrorText(locale, err); ok {
		return value
	}
	message := err.Error()
	key := map[string]string{
		"amount is required": "validation.amount_required", "amount must be positive": "validation.amount_positive",
		"amount must not be negative": "validation.amount_positive", "invalid decimal amount": "validation.amount_invalid",
		"currency must be a three-letter ISO code": "validation.currency_invalid", "FX rate must be positive": "validation.fx_positive",
		"FX conversion must be confirmed": "validation.fx_confirm", "FX rate date and source are required": "validation.fx_evidence",
		"asset already has an active purchase event": "validation.event_purchase_exists", "asset must be purchased before repair or sale": "validation.event_purchase_first",
		"sold asset cannot receive another repair or sale event": "validation.event_after_sale", "occurred time is required": "validation.occurred_required",
		"occurred time cannot be in the future": "validation.occurred_future", "event type must be purchase, repair, or sale": "validation.event_type",
	}[message]
	if key != "" {
		return textFor(locale, key)
	}
	if strings.HasPrefix(message, "invalid FX rate") {
		return textFor(locale, "validation.fx_invalid")
	}
	if strings.Contains(message, " is required") || strings.Contains(message, " is too long") || strings.Contains(message, " must be a UUID") || strings.HasPrefix(message, "unsupported currency") || strings.HasPrefix(message, "amount supports at most") {
		return textFor(locale, "validation.input_invalid")
	}
	slog.Error("unexpected user-facing error", "error", err)
	return textFor(locale, "error.internal")
}

func safeReturnTo(value, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(value, "//") {
		return fallback, false
	}
	return u.RequestURI(), true
}

func randomToken() string {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

type loginAttempt struct {
	count int
	until time.Time
}

type loginLimiter struct {
	mu     sync.Mutex
	items  map[string]loginAttempt
	limit  int
	window time.Duration
}

func newLoginLimiter(limit int, window time.Duration) *loginLimiter {
	return &loginLimiter{items: map[string]loginAttempt{}, limit: limit, window: window}
}

func (l *loginLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt := l.items[key]
	if !attempt.until.After(now) {
		attempt = loginAttempt{until: now.Add(l.window)}
	}
	if attempt.count >= l.limit {
		l.items[key] = attempt
		return false
	}
	attempt.count++
	l.items[key] = attempt
	return true
}

func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.items, key)
}

func parseFormTime(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, strings.TrimSpace(value), time.Local)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, application.NewInputError("validation.datetime")
}

func parseFormDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, application.NewInputError("validation.fx_date")
	}
	return parsed, nil
}

func eventClass(event domain.AssetEvent) string {
	if event.Type == domain.AssetEventVoid {
		return "void"
	}
	if event.BaseAmountMinor > 0 {
		return "income"
	}
	if event.BaseAmountMinor < 0 {
		return "expense"
	}
	return "neutral"
}

func formatRate(value int64) string {
	whole := value / domain.FXRateScale
	fraction := fmt.Sprintf("%08d", value%domain.FXRateScale)
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "" {
		return fmt.Sprintf("%d", whole)
	}
	return fmt.Sprintf("%d.%s", whole, fraction)
}
