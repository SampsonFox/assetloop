package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/domain"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

// tradeInWebFixture composes the real Web transport over a real SQLite store with
// the receipt-backed management service, so the trade-in screens, the shared
// application policy and the persisted events are exercised together.
type tradeInWebFixture struct {
	handler      http.Handler
	lifecycle    *application.LifecycleService
	auth         *application.AuthService
	csrf         *http.Cookie
	ownerSession *http.Cookie
	owner        application.Principal
	modelID      string
}

func newTradeInWebFixture(t *testing.T) *tradeInWebFixture {
	t.Helper()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "trade-in-web.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatal(err)
	}
	adapter := sqlite.New(db)
	auth := application.NewAuthService(adapter)
	catalog := application.NewCatalogService(adapter)
	lifecycle := application.NewLifecycleService(adapter)
	localStore, err := localblob.New(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	blobs := blob.Registry{"local": localStore}
	options := Options{
		AuthMode: "local", ModelMedia: application.NewModelMediaService(adapter, blobs, blob.ObjectKeyMapper{}, "local"),
		Specifications: application.NewSpecificationService(adapter), Management: application.NewManagementService(adapter, blobs),
	}
	options.ModelImages = application.NewModelImageService(adapter, blobs, blob.ObjectKeyMapper{}, "local")
	server, err := New(auth, catalog, lifecycle, db, options)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &tradeInWebFixture{handler: server.Handler(), lifecycle: lifecycle, auth: auth}
	setupPage := request(t, fixture.handler, http.MethodGet, "/setup", nil, nil)
	fixture.csrf = responseCookie(t, setupPage, csrfCookie)
	setup := fixture.post(t, "/setup", url.Values{
		"tenant_name": {"Trade-in Tenant"}, "base_currency": {"CNY"}, "username": {"owner"}, "password": {"owner secure password"},
	})
	if setup.Code != http.StatusSeeOther {
		t.Fatalf("setup: %d %s", setup.Code, setup.Body.String())
	}
	fixture.ownerSession = responseCookie(t, setup, sessionCookie)
	fixture.owner, err = auth.Authenticate(context.Background(), fixture.ownerSession.Value)
	if err != nil {
		t.Fatal(err)
	}
	category := fixture.post(t, "/admin/catalog/categories", url.Values{"name": {"手机"}, "icon_key": {"smartphone"}})
	if category.Code != http.StatusSeeOther {
		t.Fatalf("create category: %d %s", category.Code, category.Body.String())
	}
	page := request(t, fixture.handler, http.MethodGet, "/admin/catalog", nil, fixture.cookies())
	categoryID := optionID(t, page.Body.String(), "手机")
	model := fixture.post(t, "/admin/catalog/models", url.Values{"category_id": {categoryID}, "name": {"换新测试机"}})
	if model.Code != http.StatusSeeOther {
		t.Fatalf("create model: %d %s", model.Code, model.Body.String())
	}
	page = request(t, fixture.handler, http.MethodGet, "/assets/new", nil, fixture.cookies())
	fixture.modelID = optionID(t, page.Body.String(), "手机 / 换新测试机")
	return fixture
}

func (f *tradeInWebFixture) post(t *testing.T, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	if form == nil {
		form = url.Values{}
	}
	if form.Get("csrf_token") == "" {
		form.Set("csrf_token", f.csrf.Value)
	}
	return request(t, f.handler, http.MethodPost, path, form, f.cookies())
}

func (f *tradeInWebFixture) cookies() []*http.Cookie {
	cookies := []*http.Cookie{f.csrf}
	if f.ownerSession != nil {
		cookies = append(cookies, f.ownerSession)
	}
	return cookies
}

func (f *tradeInWebFixture) createAsset(t *testing.T, name string) string {
	t.Helper()
	response := f.post(t, "/assets", url.Values{"model_id": {f.modelID}, "display_name": {name}})
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create asset %s: %d %s", name, response.Code, response.Body.String())
	}
	location := response.Header().Get("Location")
	if !strings.HasPrefix(location, "/assets/") {
		t.Fatalf("created asset location: %q", location)
	}
	return strings.TrimPrefix(location, "/assets/")
}

func (f *tradeInWebFixture) record(t *testing.T, assetID, kind, amount, occurredAt string) string {
	t.Helper()
	response := f.post(t, "/assets/"+assetID+"/events", url.Values{
		"event_type": {kind}, "amount": {amount}, "currency": {"CNY"}, "occurred_at": {occurredAt}, "source": {"manual"},
	})
	if response.Code != http.StatusSeeOther {
		t.Fatalf("record %s on %s: %d %s", kind, assetID, response.Code, response.Body.String())
	}
	for _, event := range f.events(t, assetID) {
		if !event.IsVoided && event.Kind() == domain.AssetEventType(kind) {
			return event.ID
		}
	}
	t.Fatalf("effective %s was not recorded on %s", kind, assetID)
	return ""
}

func (f *tradeInWebFixture) events(t *testing.T, assetID string) []domain.AssetEvent {
	t.Helper()
	events, _, err := f.lifecycle.Timeline(context.Background(), f.owner, assetID)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func (f *tradeInWebFixture) count(t *testing.T, assetID, kind string) int {
	t.Helper()
	total := 0
	for _, event := range f.events(t, assetID) {
		if !event.IsVoided && event.Kind() == domain.AssetEventType(kind) {
			total++
		}
	}
	return total
}

func (f *tradeInWebFixture) requestKey(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="request_key" value="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("request key missing from the rendered form: %s", body)
	}
	return match[1]
}

// inputValues returns every value of one named input. The assertion is
// attribute-aware and order-independent on purpose: the money control places
// inputmode between name and value, and a repeated hidden field appears once per
// rendered row.
func inputValues(t *testing.T, body, name string) []string {
	t.Helper()
	tags := regexp.MustCompile(`<input[^>]*name="`+regexp.QuoteMeta(name)+`"[^>]*>`).FindAllString(body, -1)
	values := make([]string, 0, len(tags))
	for _, tag := range tags {
		match := regexp.MustCompile(`value="([^"]*)"`).FindStringSubmatch(tag)
		if len(match) != 2 {
			t.Fatalf("input %q has no value attribute: %s", name, tag)
		}
		values = append(values, match[1])
	}
	return values
}

// inputValue returns the single value of one named input.
func inputValue(t *testing.T, body, name string) string {
	t.Helper()
	values := inputValues(t, body, name)
	if len(values) != 1 {
		t.Fatalf("input %q must appear exactly once, got %d: %s", name, len(values), body)
	}
	return values[0]
}

// assertInputValues proves the exact submitted set of one repeated field.
func assertInputValues(t *testing.T, body, name string, want ...string) {
	t.Helper()
	got, sorted := inputValues(t, body, name), append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(sorted)
	if !slices.Equal(got, sorted) {
		t.Fatalf("input %q = %v, want %v: %s", name, got, sorted, body)
	}
}

// assertServerRenderedFX proves the FX evidence controls are reachable without
// JavaScript: the script hides the base-currency groups, while a server-rendered
// group must never be hidden, or an error form could lose a field it still needs.
func assertServerRenderedFX(t *testing.T, body string) {
	t.Helper()
	if regexp.MustCompile(`<label[^>]*data-trade-in-fx[^>]*hidden`).MatchString(body) {
		t.Fatalf("server-rendered FX controls must stay reachable: %s", body)
	}
}

// TestTradeInWebSelectionPreviewAndRecord walks the whole user-visible flow: the
// dedicated form, the reviewed money step, one reuse plus one newly recorded
// sale, the paired timeline row, the dedicated correction and cancellation
// actions, and the optional related asset of an ordinary record.
func TestTradeInWebSelectionPreviewAndRecord(t *testing.T) {
	f := newTradeInWebFixture(t)
	newAsset, oldAsset := f.createAsset(t, "新手机"), f.createAsset(t, "旧手机")
	newPurchase := f.record(t, newAsset, "purchase", "5000.00", "2026-08-01T10:00")
	f.record(t, oldAsset, "purchase", "2000.00", "2026-01-05T10:00")
	oldEventsBefore := len(f.events(t, oldAsset))

	// The ordinary record drawer routes a pairing type away from record_event and
	// offers the same destination as a native link for a no-JavaScript user.
	detail := request(t, f.handler, http.MethodGet, "/assets/"+newAsset, nil, f.cookies())
	for _, want := range []string{
		`href="/assets/` + newAsset + `/trade-in?direction=source"`,
		`href="/assets/` + newAsset + `/trade-in?direction=destination"`,
		`data-trade-in-base="/assets/` + newAsset + `/trade-in"`,
		`data-system-code="trade_in_source"`,
		`data-system-code="trade_in_destination"`,
	} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("asset page missing %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
	}

	selectPage := request(t, f.handler, http.MethodGet, "/assets/"+newAsset+"/trade-in?direction=source&q=旧", nil, f.cookies())
	selectBody := selectPage.Body.String()
	for _, want := range []string{
		`class="card trade-in-card"`,
		`action="/assets/` + newAsset + `/trade-in/preview" method="post"`,
		`name="direction" value="source"`,
		`换新来源（当前物品是新物品）`,
		`name="selected" value="` + oldAsset + `"`,
		// The rendered page submits its own displayed rows, which is what lets a
		// search or page change distinguish "not on this page" from "unchecked".
		`name="displayed" value="` + oldAsset + `"`,
		`name="step" value="review"`,
		`type="module" src="/static/trade-in.js"`,
	} {
		if selectPage.Code != http.StatusOK || !strings.Contains(selectBody, want) {
			t.Fatalf("trade-in selection step missing %q: status=%d body=%s", want, selectPage.Code, selectBody)
		}
	}
	// Nothing is selected yet, so no retained selection is rendered.
	assertInputValues(t, selectBody, "retain")
	if strings.Contains(selectBody, `action="/assets/`+newAsset+`/events"`) {
		t.Fatal("the trade-in form must never submit through the ordinary record route")
	}

	preview := f.post(t, "/assets/"+newAsset+"/trade-in/preview", url.Values{
		"direction": {"source"}, "page": {"1"}, "q": {"旧"}, "step": {"review"},
		"selected": {oldAsset}, "association_date": {"2026-08-20T10:00"},
	})
	previewBody := preview.Body.String()
	for _, want := range []string{
		`action="/assets/` + newAsset + `/trade-in" method="post"`,
		`name="counterpart_id" value="` + oldAsset + `"`,
		`name="existing_` + newAsset + `" value="` + newPurchase + `"`,
		`name="amount_` + oldAsset + `"`,
		`name="currency_` + oldAsset + `"`,
		`name="occurred_` + oldAsset + `"`,
		`name="association_date" value="2026-08-20T10:00"`,
		`name="association_reference"`,
		`换新日期`,
		`无资金变动`,
		`将建立的关系`,
	} {
		if preview.Code != http.StatusOK || !strings.Contains(previewBody, want) {
			t.Fatalf("trade-in review step missing %q: status=%d body=%s", want, preview.Code, previewBody)
		}
	}
	if strings.Contains(previewBody, `name="amount_`+newAsset+`"`) {
		t.Fatal("an existing record must be reviewed read-only instead of offering a second amount")
	}
	// The FX evidence controls are rendered reachable for every money group; the
	// script hides the base-currency groups, so the enhancement never removes the
	// no-JavaScript path.
	assertServerRenderedFX(t, previewBody)
	if !strings.Contains(previewBody, `5000.00 CNY`) {
		t.Fatalf("the reused record must show its original money: %s", previewBody)
	}

	commit := f.post(t, "/assets/"+newAsset+"/trade-in", url.Values{
		"direction": {"source"}, "counterpart_id": {oldAsset},
		"existing_" + newAsset: {newPurchase},
		"amount_" + oldAsset:   {"1800.00"}, "currency_" + oldAsset: {"CNY"}, "occurred_" + oldAsset: {"2026-08-20T10:00"},
		"association_date": {"2026-08-20T10:00"}, "association_reference": {"TRADE-WEB-1"}, "association_notes": {"以旧换新备注"},
		"request_key": {f.requestKey(t, previewBody)},
	})
	if commit.Code != http.StatusSeeOther || commit.Header().Get("Location") != "/assets/"+newAsset {
		t.Fatalf("trade-in commit: status=%d location=%q body=%s", commit.Code, commit.Header().Get("Location"), commit.Body.String())
	}
	if f.count(t, oldAsset, "sale") != 1 || f.count(t, oldAsset, "trade_in_destination") != 1 ||
		len(f.events(t, oldAsset)) != oldEventsBefore+2 {
		t.Fatalf("the trade-in must record exactly one sale and one destination event: %+v", f.events(t, oldAsset))
	}
	if f.count(t, newAsset, "purchase") != 1 || f.count(t, newAsset, "trade_in_source") != 1 {
		t.Fatal("the trade-in repeated the reused purchase or missed the source event")
	}

	detail = request(t, f.handler, http.MethodGet, "/assets/"+newAsset, nil, f.cookies())
	detailBody := detail.Body.String()
	for _, want := range []string{
		`换新来源 →`, `href="/assets/` + oldAsset + `"`, `旧手机`, `无资金变动`,
		`/assets/` + newAsset + `/trade-in/`, `/edit"`, `/cancel"`,
		`以旧换新备注`, `TRADE-WEB-1`,
	} {
		if detail.Code != http.StatusOK || !strings.Contains(detailBody, want) {
			t.Fatalf("trade-in timeline row missing %q: status=%d body=%s", want, detail.Code, detailBody)
		}
	}
	// Only the money record keeps an ordinary correction action; the paired event
	// exposes the dedicated relation actions instead.
	if links := regexp.MustCompile(`/events/([0-9a-f-]{36})/correct`).FindAllStringSubmatch(detailBody, -1); len(links) != 1 {
		t.Fatalf("a paired event must not offer the ordinary correction route: %v", links)
	}
	linkMatch := regexp.MustCompile(`/assets/` + newAsset + `/trade-in/([0-9a-f-]{36})/edit`).FindStringSubmatch(detailBody)
	if len(linkMatch) != 2 {
		t.Fatalf("the dedicated relation editor is missing: %s", detailBody)
	}
	linkID := linkMatch[1]
	source, destination := f.pairEvents(t, newAsset, oldAsset, linkID)
	if source.TradeInState != domain.TradeInStateActive || destination.TradeInState != domain.TradeInStateActive {
		t.Fatalf("the persisted pair is not active: %+v %+v", source, destination)
	}

	// The dedicated editor preloads the expected pair event IDs and lets the
	// association metadata change without touching money.
	editForm := request(t, f.handler, http.MethodGet, "/assets/"+newAsset+"/trade-in/"+linkID+"/edit", nil, f.cookies())
	for _, want := range []string{
		`name="source_event_id" value="` + source.ID + `"`,
		`name="destination_event_id" value="` + destination.ID + `"`,
		`name="new_asset_id"`,
		`name="old_asset_id"`,
		`id="trade-in-assets"`,
		`name="association_date"`,
	} {
		if editForm.Code != http.StatusOK || !strings.Contains(editForm.Body.String(), want) {
			t.Fatalf("relation editor missing %q: status=%d body=%s", want, editForm.Code, editForm.Body.String())
		}
	}
	corrected := f.post(t, "/assets/"+newAsset+"/trade-in/"+linkID+"/edit", url.Values{
		"source_event_id": {source.ID}, "destination_event_id": {destination.ID},
		"new_asset_id": {newAsset}, "old_asset_id": {oldAsset},
		"association_date": {"2026-08-21T10:00"}, "association_reference": {"TRADE-WEB-R"}, "association_notes": {"日期更正"},
		"request_key": {"trade-in-web-correct"},
	})
	if corrected.Code != http.StatusSeeOther {
		t.Fatalf("relation correction: %d %s", corrected.Code, corrected.Body.String())
	}
	replacement, replacementDestination := f.pairEvents(t, newAsset, oldAsset, linkID)
	if replacement.TradeInLinkID != linkID || replacement.ReplacesEventID != source.ID || replacement.TradeInState != domain.TradeInStateActive {
		t.Fatalf("the corrected pair lost its lineage: %+v", replacement)
	}
	if replacement.ExternalReference != "TRADE-WEB-R" || replacementDestination.ID == destination.ID {
		t.Fatalf("the corrected pair must project its own reference and a new destination: %+v %+v", replacement, replacementDestination)
	}
	if f.count(t, oldAsset, "sale") != 1 {
		t.Fatal("correcting the relation changed money")
	}

	// Cancelling keeps both money records, hides the relation from the effective
	// view and keeps it in history with its cancelled state.
	cancelForm := request(t, f.handler, http.MethodGet, "/assets/"+newAsset, nil, f.cookies())
	cancelBody := cancelForm.Body.String()
	if !strings.Contains(cancelBody, `取消换新关联不会改动买入或卖出金额`) {
		t.Fatalf("the cancellation confirmation must state that money is retained: %s", cancelBody)
	}
	cancelled := f.post(t, "/assets/"+newAsset+"/trade-in/"+linkID+"/cancel", url.Values{
		"source_event_id": {replacement.ID}, "destination_event_id": {replacementDestination.ID},
		"association_date": {"2026-08-22T10:00"}, "request_key": {"trade-in-web-cancel"},
	})
	if cancelled.Code != http.StatusSeeOther {
		t.Fatalf("relation cancellation: %d %s", cancelled.Code, cancelled.Body.String())
	}
	if f.count(t, oldAsset, "sale") != 1 || f.count(t, newAsset, "purchase") != 1 {
		t.Fatal("cancelling the relation changed money")
	}
	effective := request(t, f.handler, http.MethodGet, "/assets/"+newAsset, nil, f.cookies())
	if strings.Contains(effective.Body.String(), `换新来源 →`) {
		t.Fatal("a cancelled relation stayed in the effective view")
	}
	history := request(t, f.handler, http.MethodGet, "/assets/"+newAsset+"?show_voided=1", nil, f.cookies())
	if !strings.Contains(history.Body.String(), `已取消`) || !strings.Contains(history.Body.String(), `换新来源 →`) {
		t.Fatalf("cancellation history must keep the relation: %s", history.Body.String())
	}

	// An ordinary record keeps an optional related asset, preserves it when the
	// correction omits the field and clears it when the user clears it.
	repair := f.record(t, newAsset, "repair", "300.00", "2026-08-23T10:00")
	related := f.post(t, "/events/"+repair+"/correct", url.Values{
		"amount": {"300.00"}, "currency": {"CNY"}, "occurred_at": {"2026-08-23T10:00"},
		"source": {"manual-correction"}, "notes": {"保留关联"}, "related_asset_id": {oldAsset},
		"request_key": {"trade-in-web-related"},
	})
	if related.Code != http.StatusSeeOther {
		t.Fatalf("related relation correction: %d %s", related.Code, related.Body.String())
	}
	linked := f.relationOf(t, newAsset, "repair")
	if linked.RelatedAssetID != oldAsset {
		t.Fatalf("the related asset was not recorded: %+v", linked)
	}
	preserved := f.post(t, "/events/"+linked.ID+"/correct", url.Values{
		"amount": {"320.00"}, "currency": {"CNY"}, "occurred_at": {"2026-08-23T10:00"},
		"source": {"manual-correction"}, "notes": {"省略关联"}, "request_key": {"trade-in-web-related-preserve"},
	})
	if preserved.Code != http.StatusSeeOther {
		t.Fatalf("omitted relation correction: %d %s", preserved.Code, preserved.Body.String())
	}
	kept := f.relationOf(t, newAsset, "repair")
	if kept.RelatedAssetID != oldAsset || kept.ReplacesEventID != linked.ID {
		t.Fatalf("an omitted relation was not preserved: %+v", kept)
	}
	cleared := f.post(t, "/events/"+kept.ID+"/correct", url.Values{
		"amount": {"340.00"}, "currency": {"CNY"}, "occurred_at": {"2026-08-23T10:00"},
		"source": {"manual-correction"}, "notes": {"解除关联"}, "clear_related": {"1"},
		"request_key": {"trade-in-web-related-clear"},
	})
	if cleared.Code != http.StatusSeeOther {
		t.Fatalf("cleared relation correction: %d %s", cleared.Code, cleared.Body.String())
	}
	if f.relationOf(t, newAsset, "repair").RelatedAssetID != "" {
		t.Fatal("an explicitly cleared relation was kept")
	}
}

// latestPair returns the newest effective paired event of one asset.
func (f *tradeInWebFixture) latestPair(t *testing.T, assetID string) domain.AssetEvent {
	t.Helper()
	for _, event := range f.events(t, assetID) {
		if !event.IsVoided && domain.IsTradeInSystemType(event.Kind()) && event.TradeInState == domain.TradeInStateActive {
			return event
		}
	}
	t.Fatalf("no effective trade-in pair on %s", assetID)
	return domain.AssetEvent{}
}

// pairEvents returns the effective source and destination events of one link, so
// a dedicated action carries both expected IDs.
func (f *tradeInWebFixture) pairEvents(t *testing.T, newAssetID, oldAssetID, linkID string) (domain.AssetEvent, domain.AssetEvent) {
	t.Helper()
	var source, destination domain.AssetEvent
	for _, event := range f.events(t, newAssetID) {
		if event.TradeInLinkID == linkID && !event.IsVoided && event.Kind() == domain.AssetEventTradeInSource {
			source = event
		}
	}
	for _, event := range f.events(t, oldAssetID) {
		if event.TradeInLinkID == linkID && !event.IsVoided && event.Kind() == domain.AssetEventTradeInDestination {
			destination = event
		}
	}
	if source.ID == "" || destination.ID == "" {
		t.Fatalf("the pair %s is incomplete: %+v %+v", linkID, source, destination)
	}
	return source, destination
}

// relationOf returns the newest effective event of one kind.
func (f *tradeInWebFixture) relationOf(t *testing.T, assetID, kind string) domain.AssetEvent {
	t.Helper()
	var found domain.AssetEvent
	for _, event := range f.events(t, assetID) {
		if !event.IsVoided && event.Kind() == domain.AssetEventType(kind) {
			found = event
		}
	}
	if found.ID == "" {
		t.Fatalf("no effective %s on %s", kind, assetID)
	}
	return found
}

// TestTradeInWebGuardsAndStalePreview covers CSRF, role denial and the stale
// preview conflict that must never leave a partial write.
func TestTradeInWebGuardsAndStalePreview(t *testing.T) {
	f := newTradeInWebFixture(t)
	newAsset, oldAsset := f.createAsset(t, "守卫新机"), f.createAsset(t, "守卫旧机")
	newPurchase := f.record(t, newAsset, "purchase", "4000.00", "2026-08-03T10:00")
	f.record(t, oldAsset, "purchase", "1200.00", "2026-01-08T10:00")

	noCSRF := request(t, f.handler, http.MethodPost, "/assets/"+newAsset+"/trade-in/preview", url.Values{"direction": {"source"}}, []*http.Cookie{f.ownerSession})
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("preview without CSRF: got %d", noCSRF.Code)
	}
	if _, err := f.auth.AddMember(context.Background(), f.owner, application.AddMember{Username: "viewer", Password: "viewer secure password", Role: application.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	login := request(t, f.handler, http.MethodPost, "/login", url.Values{
		"csrf_token": {f.csrf.Value}, "username": {"viewer"}, "password": {"viewer secure password"},
	}, []*http.Cookie{f.csrf})
	viewer := responseCookie(t, login, sessionCookie)
	viewerForm := request(t, f.handler, http.MethodGet, "/assets/"+newAsset+"/trade-in?direction=source", nil, []*http.Cookie{viewer, f.csrf})
	if viewerForm.Code != http.StatusForbidden {
		t.Fatalf("a viewer must not open the trade-in form: got %d", viewerForm.Code)
	}
	viewerPost := request(t, f.handler, http.MethodPost, "/assets/"+newAsset+"/trade-in", url.Values{
		"csrf_token": {f.csrf.Value}, "direction": {"source"}, "counterpart_id": {oldAsset}, "request_key": {"viewer-write"},
	}, []*http.Cookie{viewer, f.csrf})
	if viewerPost.Code != http.StatusForbidden {
		t.Fatalf("a viewer must not record a trade-in: got %d", viewerPost.Code)
	}

	// A rejected commit keeps the selection, the edited economic values and the
	// request key, and writes nothing.
	before := len(f.events(t, oldAsset))
	invalid := f.post(t, "/assets/"+newAsset+"/trade-in", url.Values{
		"direction": {"source"}, "counterpart_id": {oldAsset},
		"existing_" + newAsset: {newPurchase},
		"amount_" + oldAsset:   {"not-an-amount"}, "currency_" + oldAsset: {"CNY"}, "occurred_" + oldAsset: {"2026-08-24T10:00"},
		"association_date": {"2026-08-24T10:00"}, "association_reference": {"KEEP-ME"}, "association_notes": {"保留备注"},
		"request_key": {"trade-in-web-invalid"},
	})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid commit: %d %s", invalid.Code, invalid.Body.String())
	}
	invalidBody := invalid.Body.String()
	for _, retained := range []struct{ name, value string }{
		{"amount_" + oldAsset, "not-an-amount"},
		{"currency_" + oldAsset, "CNY"},
		{"occurred_" + oldAsset, "2026-08-24T10:00"},
		{"association_reference", "KEEP-ME"},
		{"request_key", "trade-in-web-invalid"},
	} {
		if value := inputValue(t, invalidBody, retained.name); value != retained.value {
			t.Fatalf("a rejected commit must retain %q=%q, got %q: %s", retained.name, retained.value, value, invalidBody)
		}
	}
	assertInputValues(t, invalidBody, "counterpart_id", oldAsset)
	for _, want := range []string{`保留备注`, `data-error-summary`, `金额格式无效`} {
		if !strings.Contains(invalidBody, want) {
			t.Fatalf("a rejected commit must report %q: %s", want, invalidBody)
		}
	}
	if len(f.events(t, oldAsset)) != before {
		t.Fatal("a rejected commit wrote events")
	}

	// A stale explicit record is refused by the shared preview and the service.
	first := f.post(t, "/assets/"+newAsset+"/trade-in", url.Values{
		"direction": {"source"}, "counterpart_id": {oldAsset},
		"existing_" + newAsset: {newPurchase},
		"amount_" + oldAsset:   {"1100.00"}, "currency_" + oldAsset: {"CNY"}, "occurred_" + oldAsset: {"2026-08-24T10:00"},
		"association_date": {"2026-08-24T10:00"}, "request_key": {"trade-in-web-guard-record"},
	})
	if first.Code != http.StatusSeeOther {
		t.Fatalf("guard trade-in record: %d %s", first.Code, first.Body.String())
	}
	guardSource, guardDestination := f.pairEvents(t, newAsset, oldAsset, f.latestPair(t, newAsset).TradeInLinkID)
	if cancel := f.post(t, "/assets/"+newAsset+"/trade-in/"+guardSource.TradeInLinkID+"/cancel", url.Values{
		"source_event_id": {guardSource.ID}, "destination_event_id": {guardDestination.ID},
		"request_key": {"trade-in-web-guard-cancel"},
	}); cancel.Code != http.StatusSeeOther {
		t.Fatalf("guard cancellation: %d %s", cancel.Code, cancel.Body.String())
	}
	staleNew, staleOld := f.createAsset(t, "过期新机"), f.createAsset(t, "过期旧机")
	f.record(t, staleNew, "purchase", "3000.00", "2026-08-04T10:00")
	f.record(t, staleOld, "purchase", "1000.00", "2026-01-09T10:00")
	voided := f.voidedPairEvent(t, newAsset)
	staleBefore := len(f.events(t, staleOld))
	stale := f.post(t, "/assets/"+staleNew+"/trade-in", url.Values{
		"direction": {"source"}, "counterpart_id": {staleOld},
		"existing_" + staleNew: {voided},
		"amount_" + staleOld:   {"900.00"}, "currency_" + staleOld: {"CNY"}, "occurred_" + staleOld: {"2026-08-25T10:00"},
		"association_date": {"2026-08-25T10:00"}, "request_key": {"trade-in-web-stale"},
	})
	if stale.Code != http.StatusUnprocessableEntity || !strings.Contains(stale.Body.String(), "所选买卖记录已变更") {
		t.Fatalf("stale preview guard: %d %s", stale.Code, stale.Body.String())
	}
	// The stale submission is kept verbatim and never replaced by a different
	// effective record, and the group offers reselection instead of silently
	// recording new money for the asset whose record changed.
	staleBody := stale.Body.String()
	if value := inputValue(t, staleBody, "existing_"+staleNew); value != voided {
		t.Fatalf("the stale selection must stay bound to %q, got %q: %s", voided, value, staleBody)
	}
	if strings.Contains(staleBody, `name="amount_`+staleNew+`"`) {
		t.Fatal("a stale selection must not become new-money inputs")
	}
	if value := inputValue(t, staleBody, "amount_"+staleOld); value != "900.00" {
		t.Fatalf("the untouched side must keep its edited amount, got %q: %s", value, staleBody)
	}
	if !strings.Contains(staleBody, `返回选择`) {
		t.Fatalf("a stale selection needs the explicit reselection path: %s", staleBody)
	}
	if len(f.events(t, staleOld)) != staleBefore || f.count(t, staleOld, "sale") != 0 {
		t.Fatal("a stale preview left a partial write")
	}
}

// voidedPairEvent returns a voided paired event, which is never a valid reuse.
func (f *tradeInWebFixture) voidedPairEvent(t *testing.T, assetID string) string {
	t.Helper()
	for _, event := range f.events(t, assetID) {
		if event.IsVoided && domain.IsTradeInSystemType(event.Kind()) {
			return event.ID
		}
	}
	t.Fatalf("no voided paired event on %s", assetID)
	return ""
}

// TestTradeInWebSelectionMergeAcrossSearch proves the selection merger is decided
// by the rows the rendered page actually displayed. Changing the search or the
// page must never resurrect a row the reviewer unchecked, and a row that is no
// longer displayed must keep its retained selection.
func TestTradeInWebSelectionMergeAcrossSearch(t *testing.T) {
	f := newTradeInWebFixture(t)
	current := f.createAsset(t, "合并新机")
	first := f.createAsset(t, "旧机一号")
	second := f.createAsset(t, "旧机二号")

	// Both rows are checked on the rendered page.
	selected := f.post(t, "/assets/"+current+"/trade-in/preview", url.Values{
		"direction": {"source"}, "page": {"1"}, "step": {"search"},
		"selected": {first, second}, "displayed": {first, second},
		"association_date": {"2026-08-28T10:00"},
	})
	if selected.Code != http.StatusOK {
		t.Fatalf("selection step: %d %s", selected.Code, selected.Body.String())
	}
	assertInputValues(t, selected.Body.String(), "retain", first, second)

	// The reviewer narrows the search and unchecks the row that stays visible: the
	// deselection stands, and the row that is not displayed is retained.
	narrowed := f.post(t, "/assets/"+current+"/trade-in/preview", url.Values{
		"direction": {"source"}, "page": {"1"}, "q": {"旧机一号"}, "step": {"search"},
		"selected": {second}, "displayed": {first, second}, "retain": {first, second},
		"association_date": {"2026-08-28T10:00"},
	})
	if narrowed.Code != http.StatusOK {
		t.Fatalf("narrowed search: %d %s", narrowed.Code, narrowed.Body.String())
	}
	narrowedBody := narrowed.Body.String()
	assertInputValues(t, narrowedBody, "retain", second)
	if !strings.Contains(narrowedBody, "已选择 1 件") {
		t.Fatalf("the narrowed search must keep exactly one selection: %s", narrowedBody)
	}
	checkbox := `<input type="checkbox" name="selected" value="` + first + `"`
	if !strings.Contains(narrowedBody, checkbox) || strings.Contains(narrowedBody, checkbox+` checked`) {
		t.Fatalf("the unchecked row must render unchecked: %s", narrowedBody)
	}

	// Paging submits the same displayed rows, so changing the page cannot restore
	// a deselection either.
	paged := f.post(t, "/assets/"+current+"/trade-in/preview", url.Values{
		"direction": {"source"}, "page": {"2"}, "goto": {"1"}, "q": {"旧机一号"},
		"displayed": {first}, "retain": {first, second},
		"association_date": {"2026-08-28T10:00"},
	})
	if paged.Code != http.StatusOK {
		t.Fatalf("paging: %d %s", paged.Code, paged.Body.String())
	}
	assertInputValues(t, paged.Body.String(), "retain", second)
}

// TestTradeInWebMalformedEconomicInputsKeepTheForm proves a rejected economic
// value never becomes a generic error page: the review form re-renders with the
// submitted selection, every edited value and the retained request key, and the
// message is the actionable localized one for the rejected field.
func TestTradeInWebMalformedEconomicInputsKeepTheForm(t *testing.T) {
	f := newTradeInWebFixture(t)
	current, counterpart := f.createAsset(t, "校验新机"), f.createAsset(t, "校验旧机")
	purchase := f.record(t, current, "purchase", "4200.00", "2026-08-27T10:00")
	f.record(t, counterpart, "purchase", "1300.00", "2026-02-03T10:00")
	before := len(f.events(t, counterpart))
	overflow := strings.Repeat("9", 24)

	base := func() url.Values {
		return url.Values{
			"direction": {"source"}, "counterpart_id": {counterpart},
			"existing_" + current:   {purchase},
			"amount_" + counterpart: {"1100.00"}, "currency_" + counterpart: {"CNY"},
			"occurred_" + counterpart: {"2026-08-27T10:00"},
			"association_date":        {"2026-08-27T10:00"}, "association_reference": {"KEEP-INPUT"}, "association_notes": {"保留输入"},
			"request_key": {"trade-in-web-malformed"},
		}
	}
	for _, tc := range []struct {
		name, field, value, currency, message string
		mutate                                func(url.Values)
	}{
		{
			name: "amount", field: "amount_" + counterpart, value: "not-an-amount", currency: "CNY", message: "金额格式无效。",
			mutate: func(form url.Values) { form.Set("amount_"+counterpart, "not-an-amount") },
		},
		{
			// A magnitude the minor-unit range cannot hold is still an input
			// problem with an actionable message, never an internal failure.
			name: "amount range", field: "amount_" + counterpart, value: overflow, currency: "CNY", message: "金额格式无效。",
			mutate: func(form url.Values) { form.Set("amount_"+counterpart, overflow) },
		},
		{
			name: "currency", field: "currency_" + counterpart, value: "US", currency: "US", message: "货币必须是三位 ISO 代码。",
			mutate: func(form url.Values) { form.Set("currency_"+counterpart, "US") },
		},
		{
			name: "occurred", field: "occurred_" + counterpart, value: "not-a-date", currency: "CNY", message: "日期时间格式无效。",
			mutate: func(form url.Values) { form.Set("occurred_"+counterpart, "not-a-date") },
		},
		{
			name: "fx rate", field: "fx_rate_" + counterpart, value: "not-a-rate", currency: "USD", message: "汇率格式无效。",
			mutate: func(form url.Values) {
				form.Set("currency_"+counterpart, "USD")
				form.Set("fx_rate_"+counterpart, "not-a-rate")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := base()
			tc.mutate(form)
			response := f.post(t, "/assets/"+current+"/trade-in", form)
			body := response.Body.String()
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("malformed %s: %d %s", tc.name, response.Code, body)
			}
			for _, retained := range []struct{ name, value string }{
				{tc.field, tc.value},
				{"currency_" + counterpart, tc.currency},
				{"request_key", "trade-in-web-malformed"},
				{"association_reference", "KEEP-INPUT"},
				{"association_date", "2026-08-27T10:00"},
			} {
				if value := inputValue(t, body, retained.name); value != retained.value {
					t.Fatalf("a rejected %s must keep %q=%q, got %q: %s", tc.name, retained.name, retained.value, value, body)
				}
			}
			if !strings.Contains(body, tc.message) {
				t.Fatalf("a rejected %s must report %q: %s", tc.name, tc.message, body)
			}
			if !strings.Contains(body, "保留输入") {
				t.Fatalf("a rejected %s must retain the notes: %s", tc.name, body)
			}
			assertInputValues(t, body, "counterpart_id", counterpart)
			assertServerRenderedFX(t, body)
			if len(f.events(t, counterpart)) != before {
				t.Fatalf("a rejected %s wrote events", tc.name)
			}
		})
	}
}
