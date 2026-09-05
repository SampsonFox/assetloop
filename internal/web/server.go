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
	"io"
	"io/fs"
	"net"
	"net/http"
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
	AuthMode          string
	SecureCookies     bool
	DisabledPrincipal application.Principal
	ModelMedia        *application.ModelMediaService
}

type Server struct {
	auth       *application.AuthService
	catalog    *application.CatalogService
	lifecycle  *application.LifecycleService
	modelMedia *application.ModelMediaService
	db         Pinger
	options    Options
	templates  map[string]*template.Template
	limiter    *loginLimiter
}

type pageData struct {
	Title              string
	CSRFToken          string
	Error              string
	Principal          *application.Principal
	Members            []application.Member
	Categories         []domain.ItemCategory
	Models             []domain.ProductModel
	Variants           []domain.ProductVariant
	Assets             []domain.Asset
	Asset              *domain.Asset
	CanManageCatalog   bool
	Events             []domain.AssetEvent
	Summary            domain.AssetSummary
	Drafts             []domain.ImportDraft
	Draft              *domain.ImportDraft
	BaseCurrency       string
	BaseCurrencyLocked bool
	NowValue           string
	CanManageLifecycle bool
	AssetCount         int
	TotalExpenseMinor  int64
	TotalIncomeMinor   int64
	TotalNetMinor      int64
	AssetView          string
	CategoryIcons      []application.CategoryIconOption
	Model3D            *domain.ProductModel3D
}

func New(auth *application.AuthService, catalog *application.CatalogService, lifecycle *application.LifecycleService, db Pinger, options Options) (*Server, error) {
	templates := map[string]*template.Template{}
	funcs := template.FuncMap{
		"money": domain.FormatMinor, "eventLabel": eventLabel, "eventClass": eventClass,
		"dateTime":      func(value time.Time) string { return value.Local().Format("2006-01-02 15:04") },
		"dateTimeInput": func(value time.Time) string { return value.Local().Format("2006-01-02T15:04") },
		"date":          func(value time.Time) string { return value.Format("2006-01-02") },
		"rate":          formatRate, "canCorrect": func(event domain.AssetEvent) bool { return event.Type != domain.AssetEventVoid && !event.IsVoided },
	}
	for _, page := range []string{"setup", "login", "dashboard", "members", "assets", "catalog", "asset", "event_correct", "imports", "import_confirm", "error"} {
		parsed, err := template.New("base.html").Funcs(funcs).ParseFS(assets, "templates/base.html", "templates/"+page+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", page, err)
		}
		templates[page] = parsed
	}
	return &Server{auth: auth, catalog: catalog, lifecycle: lifecycle, modelMedia: options.ModelMedia, db: db, options: options, templates: templates, limiter: newLoginLimiter(5, 5*time.Minute)}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /", s.assetsPage)
	mux.HandleFunc("GET /overview", s.dashboard)
	mux.HandleFunc("GET /setup", s.setupForm)
	mux.HandleFunc("POST /setup", s.setup)
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /admin/members", s.members)
	mux.HandleFunc("POST /admin/members", s.addMember)
	mux.HandleFunc("GET /catalog", s.legacyCatalog)
	mux.HandleFunc("GET /admin/catalog", s.catalogPage)
	mux.HandleFunc("POST /admin/catalog/categories", s.createCategory)
	mux.HandleFunc("POST /admin/catalog/categories/{id}", s.updateCategory)
	mux.HandleFunc("POST /admin/catalog/models", s.createModel)
	mux.HandleFunc("POST /admin/catalog/models/{id}", s.updateModel)
	mux.HandleFunc("POST /admin/catalog/models/{id}/3d", s.updateModel3D)
	mux.HandleFunc("POST /admin/catalog/variants", s.createVariant)
	mux.HandleFunc("POST /admin/catalog/variants/{id}", s.updateVariant)
	mux.HandleFunc("POST /assets", s.createAsset)
	mux.HandleFunc("POST /assets/{id}", s.updateAsset)
	mux.HandleFunc("GET /assets/{id}", s.assetDetail)
	mux.HandleFunc("GET /assets/{id}/model.glb", s.assetModel3D)
	mux.HandleFunc("POST /assets/{id}/events", s.createAssetEvent)
	mux.HandleFunc("GET /events/{id}/correct", s.correctEventForm)
	mux.HandleFunc("POST /events/{id}/correct", s.correctEvent)
	mux.HandleFunc("GET /imports", s.importsPage)
	mux.HandleFunc("POST /imports", s.createImportDraft)
	mux.HandleFunc("GET /imports/{id}", s.confirmImportForm)
	mux.HandleFunc("POST /imports/{id}/confirm", s.confirmImport)
	staticFS, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return securityHeaders(mux)
}

func (s *Server) updateModel3D(w http.ResponseWriter, r *http.Request) {
	if s.modelMedia == nil {
		s.renderError(w, http.StatusServiceUnavailable, errors.New("3D 模型存储未配置"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, application.MaxProductModel3DBytes+(1<<20))
	if err := r.ParseMultipartForm(application.MaxProductModel3DBytes); err != nil {
		s.renderError(w, http.StatusRequestEntityTooLarge, errors.New("上传内容超过 25 MiB"))
		return
	}
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	var file []byte
	part, _, err := r.FormFile("model_3d")
	if err == nil {
		defer part.Close()
		file, err = io.ReadAll(io.LimitReader(part, application.MaxProductModel3DBytes+1))
		if err == nil && int64(len(file)) > application.MaxProductModel3DBytes {
			err = errors.New("GLB 文件超过 25 MiB")
		}
	} else if !errors.Is(err, http.ErrMissingFile) {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	if err == nil {
		_, err = s.modelMedia.Update(r.Context(), principal, application.UpdateProductModel3D{ModelID: r.PathValue("id"), File: file, SourceURL: r.FormValue("model_3d_source_url"), Author: r.FormValue("model_3d_author"), License: r.FormValue("model_3d_license")})
	}
	if err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
}

func (s *Server) assetModel3D(w http.ResponseWriter, r *http.Request) {
	if s.modelMedia == nil {
		http.NotFound(w, r)
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	opened, err := s.modelMedia.OpenForAsset(r.Context(), principal, r.PathValue("id"))
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
	w.Header().Set("Content-Length", fmt.Sprintf("%d", opened.Info.Size))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, opened.Reader)
}

func (s *Server) catalogPage(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageCatalog) {
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能维护物品配置"})
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
	if _, err := s.catalog.CreateCategory(r.Context(), principal, application.CreateCategory{Name: r.FormValue("name"), IconKey: r.FormValue("icon_key")}); err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
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
	_, err := s.catalog.CreateModel(r.Context(), principal, application.CreateModel{
		CategoryID: r.FormValue("category_id"), Name: r.FormValue("name"),
	})
	if err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
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
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
}

func (s *Server) createVariant(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.catalog.CreateVariant(r.Context(), principal, application.CreateVariant{
		ModelID: r.FormValue("model_id"), Name: r.FormValue("name"),
	})
	if err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
}

func (s *Server) updateVariant(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.catalog.UpdateVariant(r.Context(), principal, application.UpdateVariant{ID: r.PathValue("id"), ModelID: r.FormValue("model_id"), Name: r.FormValue("name")})
	if err != nil {
		s.renderCatalogError(w, r, principal, err)
		return
	}
	http.Redirect(w, r, "/admin/catalog", http.StatusSeeOther)
}

func (s *Server) createAsset(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	_, err := s.catalog.CreateAsset(r.Context(), principal, application.CreateCatalogAsset{
		VariantID: r.FormValue("variant_id"), DisplayName: r.FormValue("display_name"),
		SerialNumber: r.FormValue("serial_number"), Color: r.FormValue("color"),
		PurchaseChannel: r.FormValue("purchase_channel"), Notes: r.FormValue("notes"),
	})
	if err != nil {
		s.renderAssetsError(w, r, principal, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
		s.renderAssetsError(w, r, principal, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) assetDetail(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderAsset(w, r, http.StatusOK, principal, r.PathValue("id"), "")
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
			s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能修改生命周期记录"})
			return
		}
		s.renderAsset(w, r, http.StatusUnprocessableEntity, principal, r.PathValue("id"), err.Error())
		return
	}
	http.Redirect(w, r, "/assets/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *Server) correctEventForm(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能更正生命周期记录"})
		return
	}
	event, err := s.lifecycle.GetEvent(r.Context(), principal, r.PathValue("id"))
	if err != nil || event.Type == domain.AssetEventVoid || event.IsVoided {
		s.renderError(w, http.StatusNotFound, errors.New("可更正记录不存在"))
		return
	}
	baseCurrency, locked, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	s.render(w, http.StatusOK, "event_correct", pageData{
		Title: "更正生命周期记录", CSRFToken: s.ensureCSRF(w, r), Principal: &principal,
		Events: []domain.AssetEvent{event}, BaseCurrency: baseCurrency, BaseCurrencyLocked: locked,
		CanManageLifecycle: principal.Can(application.CapabilityManageLifecycle),
	})
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
			s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能更正生命周期记录"})
			return
		}
		s.renderError(w, http.StatusUnprocessableEntity, err)
		return
	}
	http.Redirect(w, r, "/assets/"+original.AssetID, http.StatusSeeOther)
}

func (s *Server) importsPage(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderImports(w, r, http.StatusOK, principal, "")
}

func (s *Server) createImportDraft(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	currency := r.FormValue("currency")
	amount, err := domain.ParseMajorAmount(r.FormValue("amount"), currency)
	if err == nil {
		var occurredAt time.Time
		occurredAt, err = parseFormTime(r.FormValue("occurred_at"))
		if err == nil {
			_, err = s.lifecycle.CreateDraft(r.Context(), principal, application.CreateImportDraft{
				AssetID: r.FormValue("asset_id"), Type: domain.AssetEventType(r.FormValue("event_type")),
				AmountMinor: amount, Currency: currency, OccurredAt: occurredAt, Source: r.FormValue("source"),
				ExternalReference: r.FormValue("external_reference"), Notes: r.FormValue("notes"), RawText: r.FormValue("raw_text"),
			})
		}
	}
	if err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能创建导入记录"})
			return
		}
		s.renderImports(w, r, http.StatusUnprocessableEntity, principal, err.Error())
		return
	}
	http.Redirect(w, r, "/imports", http.StatusSeeOther)
}

func (s *Server) confirmImportForm(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能确认导入记录"})
		return
	}
	draft, err := s.lifecycle.GetDraft(r.Context(), principal, r.PathValue("id"))
	if err != nil || draft.Status != "pending" {
		s.renderError(w, http.StatusNotFound, errors.New("待确认记录不存在"))
		return
	}
	baseCurrency, locked, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	s.render(w, http.StatusOK, "import_confirm", pageData{
		Title: "确认导入", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Draft: &draft,
		BaseCurrency: baseCurrency, BaseCurrencyLocked: locked,
		NowValue:           time.Now().Local().Format("2006-01-02T15:04"),
		CanManageLifecycle: principal.Can(application.CapabilityManageLifecycle),
	})
}

func (s *Server) confirmImport(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	draft, err := s.lifecycle.GetDraft(r.Context(), principal, r.PathValue("id"))
	confirmation := application.ConfirmImport{}
	if err == nil {
		baseCurrency, _, baseErr := s.lifecycle.BaseCurrency(r.Context(), principal)
		if baseErr != nil {
			err = baseErr
		} else if draft.Currency != baseCurrency {
			confirmation.FXRateScaled, err = domain.ParseFXRate(r.FormValue("fx_rate"))
			if err == nil {
				confirmation.FXRateDate, err = parseFormDate(r.FormValue("fx_rate_date"))
			}
			confirmation.FXRateSource = r.FormValue("fx_rate_source")
			confirmation.FXConfirmed = r.FormValue("fx_confirmed") == "on"
		}
	}
	if err == nil {
		_, err = s.lifecycle.ConfirmDraft(r.Context(), principal, draft.ID, confirmation)
	}
	if err != nil {
		if errors.Is(err, application.ErrForbidden) {
			s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能确认导入记录"})
			return
		}
		s.renderError(w, http.StatusUnprocessableEntity, err)
		return
	}
	http.Redirect(w, r, "/assets/"+draft.AssetID, http.StatusSeeOther)
}

func (s *Server) renderAsset(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, assetID, message string) {
	asset, err := s.catalog.GetAsset(r.Context(), principal, assetID)
	if err != nil {
		s.renderError(w, http.StatusNotFound, errors.New("物品不存在"))
		return
	}
	events, summary, err := s.lifecycle.Timeline(r.Context(), principal, assetID)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	_, locked, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	var model3D *domain.ProductModel3D
	if s.modelMedia != nil {
		if media, mediaErr := s.modelMedia.GetForModel(r.Context(), principal, asset.ModelID); mediaErr == nil {
			model3D = &media
		}
	}
	s.render(w, status, "asset", pageData{
		Title: asset.DisplayName, CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message,
		Asset: &asset, CanManageCatalog: principal.Can(application.CapabilityManageCatalog), Events: events,
		Summary: summary, BaseCurrency: summary.BaseCurrency, BaseCurrencyLocked: locked,
		NowValue:           time.Now().Local().Format("2006-01-02T15:04"),
		CanManageLifecycle: principal.Can(application.CapabilityManageLifecycle),
		Model3D:            model3D,
	})
}

func (s *Server) renderImports(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, message string) {
	snapshot, err := s.catalog.Snapshot(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	drafts, err := s.lifecycle.PendingDrafts(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	baseCurrency, locked, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	s.render(w, status, "imports", pageData{
		Title: "待确认导入", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message,
		Assets: snapshot.Assets, Drafts: drafts, BaseCurrency: baseCurrency, BaseCurrencyLocked: locked,
		NowValue:           time.Now().Local().Format("2006-01-02T15:04"),
		CanManageLifecycle: principal.Can(application.CapabilityManageLifecycle),
	})
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
		AssetID: assetID, Type: eventType, AmountMinor: amount, Currency: currency,
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
		cmd.FXConfirmed = r.FormValue("fx_confirmed") == "on"
	}
	return cmd, nil
}

func (s *Server) renderCatalogError(w http.ResponseWriter, r *http.Request, principal application.Principal, err error) {
	if errors.Is(err, application.ErrForbidden) {
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能修改物品目录"})
		return
	}
	s.renderCatalog(w, r, http.StatusUnprocessableEntity, principal, err.Error())
}

func (s *Server) renderAssetsError(w http.ResponseWriter, r *http.Request, principal application.Principal, err error) {
	if errors.Is(err, application.ErrForbidden) {
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", Principal: &principal, Error: "当前角色不能修改具体物品"})
		return
	}
	s.renderAssets(w, r, http.StatusUnprocessableEntity, principal, err.Error())
}

func (s *Server) assetsPage(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	s.renderAssets(w, r, http.StatusOK, principal, "")
}

func (s *Server) renderAssets(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, message string) {
	snapshot, err := s.catalog.Snapshot(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	view := assetView(r)
	if requested := strings.TrimSpace(r.URL.Query().Get("view")); requested == "list" || requested == "grid" {
		view = requested
		http.SetCookie(w, &http.Cookie{Name: assetViewCookie, Value: view, Path: "/", MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
	}
	s.render(w, status, "assets", pageData{
		Title: "我的物品", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message,
		Categories: snapshot.Categories, Models: snapshot.Models, Variants: snapshot.Variants,
		Assets: snapshot.Assets, CanManageCatalog: principal.Can(application.CapabilityManageCatalog), AssetView: view,
	})
}

func assetView(r *http.Request) string {
	if cookie, err := r.Cookie(assetViewCookie); err == nil && cookie.Value == "grid" {
		return "grid"
	}
	return "list"
}

func (s *Server) renderCatalog(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, message string) {
	snapshot, err := s.catalog.Snapshot(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	s.render(w, status, "catalog", pageData{
		Title: "物品配置", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: message,
		Categories: snapshot.Categories, Models: snapshot.Models, Variants: snapshot.Variants,
		Assets: snapshot.Assets, CanManageCatalog: principal.Can(application.CapabilityManageCatalog),
		CategoryIcons: application.CategoryIconOptions,
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
	snapshot, err := s.catalog.Snapshot(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	baseCurrency, _, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	data := pageData{
		Title: "概览", CSRFToken: s.ensureCSRF(w, r), Principal: &principal,
		AssetCount: len(snapshot.Assets), BaseCurrency: baseCurrency,
	}
	for _, asset := range snapshot.Assets {
		_, summary, err := s.lifecycle.Timeline(r.Context(), principal, asset.ID)
		if err != nil {
			s.renderError(w, http.StatusInternalServerError, err)
			return
		}
		data.TotalExpenseMinor += summary.ExpenseMinor
		data.TotalIncomeMinor += summary.IncomeMinor
		data.TotalNetMinor += summary.NetCashflowMinor
	}
	s.render(w, http.StatusOK, "dashboard", data)
}

func (s *Server) setupForm(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := s.auth.NeedsSetup(r.Context())
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	if !needsSetup {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "setup", pageData{Title: "初始化", CSRFToken: s.ensureCSRF(w, r)})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	credential, err := s.auth.Setup(r.Context(), application.SetupAuth{
		TenantName: r.FormValue("tenant_name"), BaseCurrency: r.FormValue("base_currency"),
		Username: r.FormValue("username"), Password: r.FormValue("password"),
	})
	if err != nil {
		s.render(w, http.StatusUnprocessableEntity, "setup", pageData{Title: "初始化", CSRFToken: s.ensureCSRF(w, r), Error: err.Error()})
		return
	}
	s.setSessionCookie(w, credential)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := s.auth.NeedsSetup(r.Context())
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
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
	s.render(w, http.StatusOK, "login", pageData{Title: "登录", CSRFToken: s.ensureCSRF(w, r)})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	key := clientIP(r)
	if !s.limiter.Allow(key, time.Now()) {
		http.Error(w, "登录尝试过多，请稍后再试", http.StatusTooManyRequests)
		return
	}
	credential, err := s.auth.Login(r.Context(), application.Login{Username: r.FormValue("username"), Password: r.FormValue("password")})
	if err != nil {
		s.render(w, http.StatusUnauthorized, "login", pageData{Title: "登录", CSRFToken: s.ensureCSRF(w, r), Error: "用户名或密码错误"})
		return
	}
	s.limiter.Reset(key)
	s.setSessionCookie(w, credential)
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

func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	members, err := s.auth.ListMembers(r.Context(), principal)
	if errors.Is(err, application.ErrForbidden) {
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: "只有 Owner 可以管理成员"})
		return
	}
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
	s.render(w, http.StatusOK, "members", pageData{Title: "成员", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Members: members})
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
		s.render(w, http.StatusForbidden, "error", pageData{Title: "无权限", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Error: "只有 Owner 可以管理成员"})
		return
	}
	if err != nil {
		members, _ := s.auth.ListMembers(r.Context(), principal)
		s.render(w, http.StatusUnprocessableEntity, "members", pageData{Title: "成员", CSRFToken: s.ensureCSRF(w, r), Principal: &principal, Members: members, Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/admin/members", http.StatusSeeOther)
}

func (s *Server) principal(r *http.Request) (application.Principal, error) {
	if s.options.AuthMode == "disabled" {
		return s.options.DisabledPrincipal, nil
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
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return false
	}
	want, got := []byte(cookie.Value), []byte(r.FormValue("csrf_token"))
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1 {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, credential application.SessionCredential) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: credential.Token, Path: "/", Expires: credential.ExpiresAt, HttpOnly: true, Secure: s.options.SecureCookies, SameSite: http.SameSiteLaxMode})
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.templates[name].ExecuteTemplate(w, "base", data); err != nil {
		panic(err)
	}
}

func (s *Server) renderError(w http.ResponseWriter, status int, err error) {
	s.render(w, status, "error", pageData{Title: "错误", Error: err.Error()})
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
	return time.Time{}, errors.New("日期时间格式无效")
}

func parseFormDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, errors.New("汇率日期格式无效")
	}
	return parsed, nil
}

func eventLabel(value domain.AssetEventType) string {
	switch value {
	case domain.AssetEventPurchase:
		return "买入"
	case domain.AssetEventRepair:
		return "维修"
	case domain.AssetEventSale:
		return "卖出"
	case domain.AssetEventVoid:
		return "作废"
	default:
		return string(value)
	}
}

func eventClass(value domain.AssetEventType) string {
	if value == domain.AssetEventSale {
		return "income"
	}
	if value == domain.AssetEventVoid {
		return "void"
	}
	return "expense"
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
