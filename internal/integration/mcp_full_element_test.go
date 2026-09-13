package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	"github.com/SampsonFox/assetloop/internal/domain"
	transport "github.com/SampsonFox/assetloop/internal/mcp"
	webtransport "github.com/SampsonFox/assetloop/internal/web"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct{ token string }

// mcpStringPointer supplies the optional related-asset value of an ordinary
// record without changing the command's omission semantics.
func mcpStringPointer(value string) *string { return &value }

// Network behavior is tested by modeldownload; this port fixture keeps the
// cumulative dual-store MCP scenario deterministic and offline.
type modelDownloadFixture struct{}

func (modelDownloadFixture) Download(context.Context, string) ([]byte, error) {
	return fullElementGLB(), nil
}

// The full-element MCP scenario must stay offline; this deterministic image
// downloader also proves that a same-key retry performs no network I/O.
type modelImageFixture struct {
	calls atomic.Int32
}

func (f *modelImageFixture) DownloadImage(context.Context, string) ([]byte, error) {
	f.calls.Add(1)
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(copy)
}

func runMCPFullElement(t *testing.T, db *sql.DB, store scenarioStore, session application.SessionCredential, modelID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	host := httptest.NewUnstartedServer(nil)
	issuer := "http://" + host.Listener.Addr().String()
	defer host.Close()
	oauth, err := application.NewOAuthService(store.(application.OAuthStore), issuer+"/mcp", []application.OAuthClient{{ID: "full-element-client", Name: "Full element client", RedirectURIs: []string{"http://127.0.0.1/callback"}}})
	if err != nil {
		t.Fatal(err)
	}
	oauthHTTP, err := transport.NewOAuthHTTP(oauth, issuer)
	if err != nil {
		t.Fatal(err)
	}
	auth, catalog, lifecycle, specs := application.NewAuthService(store), application.NewCatalogService(store), application.NewLifecycleService(store), application.NewSpecificationService(store)
	local, err := localblob.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs := blob.Registry{"local": local}
	media := application.NewModelMediaService(store, blobs, blob.ObjectKeyMapper{}, "local")
	management := application.NewManagementService(store.(application.ManagementStore), blobs)
	importer := application.NewModelImportService(management, media, modelDownloadFixture{})
	images := application.NewModelImageService(store.(application.ModelImageStore), blobs, blob.ObjectKeyMapper{}, "local")
	imageFixture := &modelImageFixture{}
	imageImporter := application.NewModelImageImportService(management, images, imageFixture)
	marketFixture := newMCPMarketFixture()
	market := application.NewMarketService(store, marketFixture, nil, application.MarketOptions{})
	web, err := webtransport.New(auth, catalog, lifecycle, db, webtransport.Options{Market: market, AuthMode: "local", Specifications: specs, OAuth: oauth, OAuthIssuer: issuer, ModelImages: images})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", web.Handler())
	mux.Handle("/oauth/token", oauthHTTP.Guard(http.HandlerFunc(oauthHTTP.Token)))
	mux.Handle("/mcp", oauthHTTP.Protected(transport.NewHandler(transport.Services{Market: market, Catalog: catalog, Specifications: specs, Lifecycle: lifecycle, Media: media, Management: management, Import: importer, Images: images, ImageImport: imageImporter}, oauthHTTP.Authenticate)))
	host.Config.Handler = mux
	host.Start()
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cookies := []*http.Cookie{{Name: "assetloop_session", Value: session.Token}}
	request := func(method, path string, form url.Values) (*http.Response, []byte) {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, method, issuer+path, strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		if method == "POST" {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal("HTTP request failed")
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return response, body
	}
	verifier := strings.Repeat("f", 43)
	digest := sha256.Sum256([]byte(verifier))
	form := url.Values{"client_id": {"full-element-client"}, "response_type": {"code"}, "redirect_uri": {"http://127.0.0.1:49152/callback"}, "resource": {issuer + "/mcp"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}, "code_challenge_method": {"S256"}, "scope": {strings.Join([]string{transport.ScopeRead, transport.ScopeCatalog, transport.ScopeLifecycle}, " ")}, "state": {"full-element-state"}}
	consent, body := request("GET", "/oauth/authorize?"+form.Encode(), nil)
	if consent.StatusCode != 200 || !strings.Contains(string(body), "Full element client") {
		t.Fatal("consent page unavailable")
	}
	var csrf string
	for _, cookie := range consent.Cookies() {
		cookies = append(cookies, cookie)
		if cookie.Name == "assetloop_csrf" {
			csrf = cookie.Value
		}
	}
	if csrf == "" {
		t.Fatal("consent omitted CSRF cookie")
	}
	form.Set("csrf_token", csrf)
	form.Set("decision", "allow")
	allowed, _ := request("POST", "/oauth/authorize", form)
	callback, err := url.Parse(allowed.Header.Get("Location"))
	if err != nil || allowed.StatusCode != 303 || callback.Query().Get("state") != "full-element-state" || callback.Query().Get("iss") != issuer {
		t.Fatal("consent callback invalid")
	}
	exchanged, tokenJSON := request("POST", "/oauth/token", url.Values{"grant_type": {"authorization_code"}, "client_id": {"full-element-client"}, "resource": {issuer + "/mcp"}, "redirect_uri": {form.Get("redirect_uri")}, "code_verifier": {verifier}, "code": {callback.Query().Get("code")}})
	var tokens application.OAuthTokens
	if exchanged.StatusCode != 200 || json.Unmarshal(tokenJSON, &tokens) != nil || tokens.AccessToken == "" {
		t.Fatal("OAuth exchange failed")
	}
	mcpClient, err := sdk.NewClient(&sdk.Implementation{Name: "full-element", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: issuer + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{tokens.AccessToken}, Timeout: 10 * time.Second}}, nil)
	if err != nil {
		t.Fatal("MCP connection failed")
	}
	defer mcpClient.Close()
	list, err := mcpClient.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil || len(list.Tools) != 57 {
		t.Fatal("OAuth MCP discovery failed")
	}
	called := map[string]bool{}
	call := func(name string, input, output any) {
		t.Helper()
		result, err := mcpClient.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: input})
		if err != nil || result.IsError {
			t.Fatalf("MCP %s failed", name)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil || json.Unmarshal(data, output) != nil {
			t.Fatalf("MCP %s result invalid", name)
		}
		if strings.Contains(name, "market") || strings.Contains(name, "image") {
			for _, hidden := range []string{"private-provider", "lease_token", "lease_until", "NextPageToken", "Metric", "object_key", "store_id", "tenant_id", "ObjectKey", "StoreID"} {
				if strings.Contains(string(data), hidden) {
					t.Fatalf("%s leaked internal metadata", name)
				}
			}
		}
		called[name] = true
		t.Logf("HTTP tools/call PASS %s", name)
	}
	var imported struct{ Data struct{ ID string } }
	importInput := transport.ImportResourceInput{URL: "https://models.example/model.glb", Name: "MCP walkthrough GLB", License: "test fixture", RequestKey: "full-mcp-import"}
	call("import_3d_resource_from_url", importInput, &imported)
	resource := struct{ ID string }{imported.Data.ID}
	call("import_3d_resource_from_url", importInput, &imported)
	if resource.ID == "" || imported.Data.ID != resource.ID {
		t.Fatal("import replay changed identity")
	}
	var asset struct{ Data domain.Asset }
	assetInput := transport.SaveAssetInput{RequestKey: "full-mcp-asset", ModelID: modelID, DisplayName: "MCP confirmed full element", TagIDs: []string{}}
	call("save_asset", assetInput, &asset)
	assetID := asset.Data.ID
	call("save_asset", assetInput, &asset)
	if assetID == "" || asset.Data.ID != assetID {
		t.Fatal("asset replay changed identity")
	}
	before, _, err := lifecycle.Timeline(ctx, session.Principal, assetID)
	if err != nil || len(before) != 0 {
		t.Fatal("asset creation implicitly wrote purchase")
	}
	var types struct {
		Data application.EventTypeListResult
	}
	call("list_event_types", transport.TagQuery{PageSize: 200}, &types)
	var purchaseID string
	for _, kind := range types.Data.Types {
		if kind.SystemCode == "purchase" {
			purchaseID = kind.ID
		}
	}
	if purchaseID == "" {
		t.Fatal("purchase type absent")
	}
	input := transport.EventInput{AssetID: assetID, TypeID: purchaseID, EventFields: transport.EventFields{RequestKey: "full-mcp-purchase", AmountMinor: 100, Currency: "USD", OccurredAt: "2026-08-01T10:00:00Z", FXRateScaled: 712000000, FXRateDate: "2026-08-01", FXRateSource: "full-element-fixture", FXConfirmed: true}}
	var event struct{ Data domain.AssetEvent }
	call("record_event", input, &event)
	originalID := event.Data.ID
	call("record_event", input, &event)
	if event.Data.ID != originalID || event.Data.BaseAmountMinor != -712 || event.Data.FX == nil || event.Data.FX.OriginalAmountMinor != 100 {
		t.Fatal("purchase replay or exact FX mismatch")
	}
	replacement := input.EventFields
	replacement.RequestKey, replacement.AmountMinor = "full-mcp-correction", 200
	correction := transport.CorrectEventInput{EventID: originalID, Replacement: replacement}
	call("correct_event", correction, &event)
	replacementID := event.Data.ID
	call("correct_event", correction, &event)
	if event.Data.ID != replacementID {
		t.Fatal("correction replay changed identity")
	}
	history, summary, err := lifecycle.Timeline(ctx, session.Principal, assetID)
	if err != nil || len(history) != 3 || summary.ExpenseMinor != 1424 {
		t.Fatal("MCP append-only history mismatch")
	}
	original, err := lifecycle.GetEvent(ctx, session.Principal, originalID)
	if err != nil || !original.IsVoided || original.BaseAmountMinor != -712 {
		t.Fatal("original economic evidence overwritten")
	}
	page, body := request("GET", "/assets/"+assetID, nil)
	if page.StatusCode != 200 || !strings.Contains(string(body), assetInput.DisplayName) {
		t.Fatal("MCP asset not visible through authenticated Web")
	}
	// A confirmed image import replaces the shared model image without touching
	// 3D or lifecycle data; the same key replays without re-downloading.
	imageInput := transport.ImportModelImageInput{ModelID: modelID, URL: "https://images.example/model.png", SourceURL: "https://example.com/product", RequestKey: "full-mcp-model-image"}
	var image struct{ Data transport.ModelImageResult }
	call("import_model_image_from_url", imageInput, &image)
	imageID, downloads := image.Data.ID, imageFixture.calls.Load()
	if imageID == "" || image.Data.ModelID != modelID || image.Data.SHA256 == "" || downloads == 0 {
		t.Fatalf("MCP image import produced no active revision: %+v", image.Data)
	}
	call("import_model_image_from_url", imageInput, &image)
	if image.Data.ID != imageID || imageFixture.calls.Load() != downloads {
		t.Fatal("image import replay re-downloaded or replaced the revision")
	}
	call("get_model_image", transport.ModelImageInput{ModelID: modelID}, &image)
	if image.Data.ID != imageID || image.Data.SHA256 == "" || image.Data.SizeBytes == 0 {
		t.Fatalf("get_model_image lost the active revision: %+v", image.Data)
	}
	page, body = request("GET", "/assets/"+assetID, nil)
	if imageURL := "/models/" + modelID + "/image?v=" + image.Data.SHA256; page.StatusCode != 200 || !strings.Contains(string(body), imageURL) {
		t.Fatal("imported model image not visible through authenticated Web")
	}
	// A bounded base64 content upload replaces the same shared revision without any
	// download; the same key replays the original revision and stays Web-visible.
	uploadInput := transport.UploadModelImageInput{ModelID: modelID, ContentBase64: base64.StdEncoding.EncodeToString(testModelImagePNG(t)), SourceURL: "https://example.com/product", RequestKey: "full-mcp-model-image-upload"}
	var uploaded struct{ Data transport.ModelImageResult }
	call("upload_model_image", uploadInput, &uploaded)
	uploadID, uploadSHA := uploaded.Data.ID, uploaded.Data.SHA256
	if uploadID == "" || uploaded.Data.ModelID != modelID || uploadSHA == "" || uploaded.Data.SizeBytes == 0 {
		t.Fatalf("MCP image upload produced no active revision: %+v", uploaded.Data)
	}
	call("upload_model_image", uploadInput, &uploaded)
	if uploaded.Data.ID != uploadID || uploaded.Data.SHA256 != uploadSHA {
		t.Fatal("image content upload replay replaced the original revision")
	}
	call("get_model_image", transport.ModelImageInput{ModelID: modelID}, &image)
	if image.Data.ID != uploadID || image.Data.SHA256 != uploadSHA {
		t.Fatalf("get_model_image lost the uploaded revision: %+v", image.Data)
	}
	page, body = request("GET", "/assets/"+assetID, nil)
	if imageURL := "/models/" + modelID + "/image?v=" + uploadSHA; page.StatusCode != 200 || !strings.Contains(string(body), imageURL) {
		t.Fatal("uploaded model image not visible through authenticated Web")
	}
	runMCPToolWalkthrough(t, call, resource.ID, assetID, originalID)
	_, costsBefore, err := lifecycle.Timeline(ctx, session.Principal, assetID)
	if err != nil {
		t.Fatal(err)
	}
	runMCPMarketWalkthrough(t, ctx, mcpClient, call, assetID, marketFixture, market, management, session.Principal)
	_, costsAfter, err := lifecycle.Timeline(ctx, session.Principal, assetID)
	if err != nil || costsBefore != costsAfter {
		t.Fatal("market changed lifecycle costs", err)
	}
	page, body = request("GET", "/assets/"+assetID, nil)
	if page.StatusCode != 200 || !strings.Contains(string(body), "5100.00") {
		t.Fatal("MCP market price not visible in Web")
	}
	denied := func(name string, input any) {
		t.Helper()
		result, err := mcpClient.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: input})
		if err != nil || !result.IsError {
			t.Fatalf("MCP %s should have been refused", name)
		}
	}
	// The earlier modeling mistake recorded service and accessory purchases as
	// built-in purchases. Reusable custom cost categories are the supported model,
	// and an already recorded purchase is reclassified through the optional
	// top-level type_id of correct_event.
	var serviceType, accessoryType, giftType struct {
		Data domain.AssetEventTypeDefinition
	}
	call("create_event_type", transport.CreateEventTypeInput{RequestKey: "full-mcp-service-type", Name: "Device service", Cashflow: "expense"}, &serviceType)
	call("create_event_type", transport.CreateEventTypeInput{RequestKey: "full-mcp-accessory-type", Name: "Phone accessory", Cashflow: "expense"}, &accessoryType)
	call("create_event_type", transport.CreateEventTypeInput{RequestKey: "full-mcp-gift-type", Name: "Free gift", Cashflow: "neutral"}, &giftType)
	if serviceType.Data.ID == "" || accessoryType.Data.ID == "" || giftType.Data.ID == "" {
		t.Fatal("MCP custom cost types were not created")
	}
	customCosts := []transport.EventInput{
		{AssetID: assetID, TypeID: serviceType.Data.ID, EventFields: transport.EventFields{RequestKey: "full-mcp-service-warranty", AmountMinor: 19_900, Currency: "CNY", OccurredAt: "2026-08-02T10:00:00Z", Notes: "extended warranty service"}},
		{AssetID: assetID, TypeID: serviceType.Data.ID, EventFields: transport.EventFields{RequestKey: "full-mcp-service-setup", AmountMinor: 5_900, Currency: "CNY", OccurredAt: "2026-08-02T11:00:00Z", Notes: "setup service"}},
		{AssetID: assetID, TypeID: serviceType.Data.ID, EventFields: transport.EventFields{RequestKey: "full-mcp-service-shield", AmountMinor: 9_900, Currency: "CNY", OccurredAt: "2026-08-03T10:00:00Z", Notes: "screen protection service"}},
		{AssetID: assetID, TypeID: accessoryType.Data.ID, EventFields: transport.EventFields{RequestKey: "full-mcp-accessory-case", AmountMinor: 8_900, Currency: "CNY", OccurredAt: "2026-08-03T11:00:00Z", Notes: "protective case"}},
		{AssetID: assetID, TypeID: accessoryType.Data.ID, EventFields: transport.EventFields{RequestKey: "full-mcp-accessory-charger", AmountMinor: 14_900, Currency: "CNY", OccurredAt: "2026-08-04T10:00:00Z", Notes: "charger"}},
	}
	for _, cost := range customCosts {
		call("record_event", cost, &event)
		call("record_event", cost, &event)
	}
	call("record_event", transport.EventInput{AssetID: assetID, TypeID: giftType.Data.ID, EventFields: transport.EventFields{RequestKey: "full-mcp-gift", AmountMinor: 0, Currency: "CNY", OccurredAt: "2026-08-05T10:00:00Z", Notes: "free gift"}}, &event)
	if event.Data.TypeID != giftType.Data.ID || event.Data.BaseAmountMinor != 0 {
		t.Fatalf("MCP neutral gift mismatch: %+v", event.Data)
	}
	reclassify := replacement
	reclassify.RequestKey = "full-mcp-reclassify-service"
	reclassified := transport.CorrectEventInput{EventID: replacementID, TypeID: serviceType.Data.ID, Replacement: reclassify}
	call("correct_event", reclassified, &event)
	if event.Data.TypeID != serviceType.Data.ID || event.Data.BaseAmountMinor != -1424 || event.Data.FX == nil {
		t.Fatalf("MCP reclassification mismatch: %+v", event.Data)
	}
	reclassifiedID := event.Data.ID
	call("correct_event", reclassified, &event)
	if event.Data.ID != reclassifiedID {
		t.Fatal("MCP reclassification replay changed identity")
	}
	// A built-in target stays refused, and the original purchase keeps its evidence.
	denied("correct_event", transport.CorrectEventInput{EventID: reclassifiedID, TypeID: purchaseID, Replacement: transport.EventFields{RequestKey: "full-mcp-reclassify-builtin", AmountMinor: 200, Currency: "USD", OccurredAt: "2026-08-01T10:00:00Z", FXRateScaled: 712000000, FXRateDate: "2026-08-01", FXRateSource: "full-element-fixture", FXConfirmed: true}})
	voidedPurchase, err := lifecycle.GetEvent(ctx, session.Principal, replacementID)
	if err != nil || !voidedPurchase.IsVoided || voidedPurchase.BaseAmountMinor != -1424 || voidedPurchase.FX == nil {
		t.Fatalf("MCP reclassification overwrote the original purchase: %+v %v", voidedPurchase, err)
	}
	customCost, err := lifecycle.CostDashboard(ctx, session.Principal, assetID)
	if err != nil {
		t.Fatal(err)
	}
	grouped := map[string]int64{}
	for _, category := range customCost.Categories {
		grouped[category.TypeID] = category.AmountMinor
	}
	// The acquisition is now a custom service cost, so no purchase category remains.
	if len(customCost.Categories) != 2 || grouped[serviceType.Data.ID] != 37_124 || grouped[accessoryType.Data.ID] != 23_800 {
		t.Fatalf("MCP custom cost grouping mismatch: %+v", customCost.Categories)
	}
	_, customSummary, err := lifecycle.Timeline(ctx, session.Principal, assetID)
	if err != nil || customSummary.ExpenseMinor != 60_924 || customSummary.IncomeMinor != 0 {
		t.Fatalf("MCP custom cost totals mismatch: %+v %v", customSummary, err)
	}

	// Trade-in: two fresh items reuse one confirmed purchase and let the command
	// record exactly one sale, then the paired relation is previewed, replayed,
	// corrected, cancelled and refused when a stale version or a one-sided paired
	// event is submitted.
	var tradeInSourceTypeID string
	for _, kind := range types.Data.Types {
		if kind.SystemCode == domain.AssetEventTradeInSource {
			tradeInSourceTypeID = kind.ID
		}
	}
	if tradeInSourceTypeID == "" {
		t.Fatal("the trade_in_source system type is absent")
	}
	var tradeInNew, tradeInOld, tradeInOldTwo, tradeInNewTwo struct{ Data domain.Asset }
	call("save_asset", transport.SaveAssetInput{RequestKey: "full-mcp-trade-in-new", ModelID: modelID, DisplayName: "MCP trade-in new item", TagIDs: []string{}}, &tradeInNew)
	call("save_asset", transport.SaveAssetInput{RequestKey: "full-mcp-trade-in-old", ModelID: modelID, DisplayName: "MCP trade-in old item", TagIDs: []string{}}, &tradeInOld)
	call("save_asset", transport.SaveAssetInput{RequestKey: "full-mcp-trade-in-old-two", ModelID: modelID, DisplayName: "MCP trade-in second old item", TagIDs: []string{}}, &tradeInOldTwo)
	call("save_asset", transport.SaveAssetInput{RequestKey: "full-mcp-trade-in-new-two", ModelID: modelID, DisplayName: "MCP trade-in second new item", TagIDs: []string{}}, &tradeInNewTwo)
	if tradeInNew.Data.ID == "" || tradeInOld.Data.ID == "" || tradeInOldTwo.Data.ID == "" || tradeInNewTwo.Data.ID == "" {
		t.Fatal("MCP trade-in assets were not created")
	}
	newPurchase := transport.EventInput{AssetID: tradeInNew.Data.ID, TypeID: purchaseID, EventFields: transport.EventFields{RequestKey: "full-mcp-trade-in-new-purchase", AmountMinor: 600_000, Currency: "CNY", OccurredAt: "2026-08-06T10:00:00Z"}}
	call("record_event", newPurchase, &event)
	newPurchaseID := event.Data.ID
	oldPurchase := newPurchase
	oldPurchase.AssetID, oldPurchase.RequestKey, oldPurchase.AmountMinor = tradeInOld.Data.ID, "full-mcp-trade-in-old-purchase", 250_000
	call("record_event", oldPurchase, &event)
	secondOldPurchase := oldPurchase
	secondOldPurchase.AssetID, secondOldPurchase.RequestKey, secondOldPurchase.AmountMinor = tradeInOldTwo.Data.ID, "full-mcp-trade-in-old-two-purchase", 210_000
	call("record_event", secondOldPurchase, &event)
	secondNewPurchase := newPurchase
	secondNewPurchase.AssetID, secondNewPurchase.RequestKey, secondNewPurchase.AmountMinor = tradeInNewTwo.Data.ID, "full-mcp-trade-in-new-two-purchase", 640_000
	call("record_event", secondNewPurchase, &event)
	secondNewPurchaseID := event.Data.ID
	// The old item has no sale yet: the confirmed trade-in records exactly one.
	tradeInInput := transport.RecordTradeInInput{
		PreviewTradeInInput: transport.PreviewTradeInInput{
			CurrentAssetID: tradeInNew.Data.ID, Direction: "source",
			Current: transport.TradeInEconomicInput{AssetID: tradeInNew.Data.ID, ExistingEventID: newPurchaseID},
			Counterparts: []transport.TradeInEconomicInput{{
				AssetID:  tradeInOld.Data.ID,
				NewEvent: &transport.TradeInNewEventInput{AmountMinor: 180_000, Currency: "CNY", OccurredAt: "2026-08-06T10:00:00Z"},
			}},
			OccurredAt: "2026-08-06T10:00:00Z", ExternalReference: "MCP-TRADE-1", Notes: "confirmed trade-in",
		},
		RequestKey: "full-mcp-trade-in-record",
	}
	var tradeIn struct{ Data transport.TradeInResult }
	call("record_trade_in", tradeInInput, &tradeIn)
	pair := tradeIn.Data.Pairs[0]
	if len(tradeIn.Data.Pairs) != 1 || pair.LinkID == "" || pair.SourceEventID == "" || pair.DestinationEventID == "" ||
		pair.LinkStatus != "created" || pair.NewEconomicEventID != newPurchaseID || pair.NewEconomicStatus != "reused" || pair.OldEconomicStatus != "created" {
		t.Fatalf("MCP trade-in result mismatch: %+v", tradeIn.Data)
	}
	call("record_trade_in", tradeInInput, &tradeIn)
	if len(tradeIn.Data.Pairs) != 1 || tradeIn.Data.Pairs[0].LinkID != pair.LinkID || tradeIn.Data.Pairs[0].SourceEventID != pair.SourceEventID {
		t.Fatal("MCP trade-in replay changed the result")
	}
	if _, _, err := lifecycle.Timeline(ctx, session.Principal, tradeInOld.Data.ID); err != nil {
		t.Fatal(err)
	}
	_, tradeInSummary, err := lifecycle.Timeline(ctx, session.Principal, tradeInNew.Data.ID)
	if err != nil || tradeInSummary.IncomeMinor != 0 {
		t.Fatalf("neutral paired events changed the new item's money: %+v %v", tradeInSummary, err)
	}
	var preview struct{ Data transport.TradeInPreview }
	call("preview_trade_in", tradeInInput.PreviewTradeInInput, &preview)
	if !preview.Data.Ready || len(preview.Data.Pairs) != 1 || preview.Data.Pairs[0].NewSelection != "linked" ||
		preview.Data.Pairs[0].OldSelection != "linked" || preview.Data.Pairs[0].LinkID != pair.LinkID ||
		preview.Data.Pairs[0].NewEventID != newPurchaseID || preview.Data.Pairs[0].OldEventID != pair.OldEconomicEventID {
		t.Fatalf("MCP trade-in preview mismatch: %+v", preview.Data)
	}
	correct := transport.CorrectTradeInLinkInput{
		RequestKey: "full-mcp-trade-in-correct", LinkID: pair.LinkID,
		ExpectedSourceEventID: pair.SourceEventID, ExpectedDestinationEventID: pair.DestinationEventID,
		NewAssetID: tradeInNew.Data.ID, OldAssetID: tradeInOld.Data.ID,
		OccurredAt: "2026-08-07T10:00:00Z", ExternalReference: "MCP-TRADE-R", Notes: "association date corrected",
	}
	call("correct_trade_in_link", correct, &tradeIn)
	corrected := tradeIn.Data.Pairs[0]
	if corrected.LinkStatus != "corrected" || corrected.LinkID != pair.LinkID ||
		corrected.SourceEventID == pair.SourceEventID || corrected.DestinationEventID == pair.DestinationEventID {
		t.Fatalf("MCP trade-in correction mismatch: %+v", corrected)
	}
	call("correct_trade_in_link", correct, &tradeIn)
	if tradeIn.Data.Pairs[0].SourceEventID != corrected.SourceEventID || tradeIn.Data.Pairs[0].DestinationEventID != corrected.DestinationEventID {
		t.Fatal("MCP trade-in correction replay changed the result")
	}
	cancelInput := transport.CancelTradeInLinkInput{
		RequestKey: "full-mcp-trade-in-cancel", LinkID: corrected.LinkID,
		ExpectedSourceEventID: corrected.SourceEventID, ExpectedDestinationEventID: corrected.DestinationEventID,
		OccurredAt: "2026-08-08T10:00:00Z", Notes: "no longer traded",
	}
	call("cancel_trade_in_link", cancelInput, &tradeIn)
	if tradeIn.Data.Pairs[0].LinkStatus != "cancelled" || tradeIn.Data.Pairs[0].SourceEventID == corrected.SourceEventID {
		t.Fatalf("MCP trade-in cancellation mismatch: %+v", tradeIn.Data)
	}
	call("cancel_trade_in_link", cancelInput, &tradeIn)
	denied("cancel_trade_in_link", transport.CancelTradeInLinkInput{
		RequestKey: "full-mcp-trade-in-cancel-stale", LinkID: pair.LinkID,
		ExpectedSourceEventID: pair.SourceEventID, ExpectedDestinationEventID: pair.DestinationEventID,
	})
	// One old item against two new items: the sale is recorded once while both
	// purchases are reused, and no pair is inferred.
	multi := transport.RecordTradeInInput{
		PreviewTradeInInput: transport.PreviewTradeInInput{
			CurrentAssetID: tradeInOldTwo.Data.ID, Direction: "destination",
			Current: transport.TradeInEconomicInput{AssetID: tradeInOldTwo.Data.ID, NewEvent: &transport.TradeInNewEventInput{AmountMinor: 120_000, Currency: "CNY", OccurredAt: "2026-08-10T10:00:00Z"}},
			Counterparts: []transport.TradeInEconomicInput{
				{AssetID: tradeInNew.Data.ID, ExistingEventID: newPurchaseID},
				{AssetID: tradeInNewTwo.Data.ID, ExistingEventID: secondNewPurchaseID},
			},
			OccurredAt: "2026-08-10T10:00:00Z", Notes: "one old item, two new items",
		},
		RequestKey: "full-mcp-trade-in-multi",
	}
	call("record_trade_in", multi, &tradeIn)
	if len(tradeIn.Data.Pairs) != 2 {
		t.Fatalf("MCP multi-counterpart trade-in mismatch: %+v", tradeIn.Data)
	}
	links := map[string]bool{}
	for _, item := range tradeIn.Data.Pairs {
		if item.OldAssetID != tradeInOldTwo.Data.ID || item.OldEconomicStatus != "created" || item.NewEconomicStatus != "reused" {
			t.Fatalf("MCP multi-counterpart pair mismatch: %+v", item)
		}
		links[item.LinkID] = true
	}
	if len(links) != 2 {
		t.Fatalf("MCP multi-counterpart links are not distinct: %+v", links)
	}
	// A paired system type, a self counterpart and an unknown asset are refused
	// through the shared service instead of writing a one-sided relation.
	denied("record_event", transport.EventInput{AssetID: tradeInNew.Data.ID, TypeID: tradeInSourceTypeID, EventFields: transport.EventFields{RequestKey: "full-mcp-trade-in-direct", Currency: "CNY", OccurredAt: "2026-08-11T10:00:00Z"}})
	denied("record_trade_in", transport.RecordTradeInInput{
		PreviewTradeInInput: transport.PreviewTradeInInput{
			CurrentAssetID: tradeInNew.Data.ID, Direction: "source",
			Current:      transport.TradeInEconomicInput{AssetID: tradeInNew.Data.ID, ExistingEventID: newPurchaseID},
			Counterparts: []transport.TradeInEconomicInput{{AssetID: tradeInNew.Data.ID, ExistingEventID: newPurchaseID}},
			OccurredAt:   "2026-08-11T10:00:00Z",
		},
		RequestKey: "full-mcp-trade-in-self",
	})
	denied("record_trade_in", transport.RecordTradeInInput{
		PreviewTradeInInput: transport.PreviewTradeInInput{
			CurrentAssetID: tradeInNew.Data.ID, Direction: "source",
			Current:      transport.TradeInEconomicInput{AssetID: tradeInNew.Data.ID, ExistingEventID: newPurchaseID},
			Counterparts: []transport.TradeInEconomicInput{{AssetID: "99999999-9999-4999-8999-999999999999", NewEvent: &transport.TradeInNewEventInput{AmountMinor: 1_000, Currency: "CNY", OccurredAt: "2026-08-11T10:00:00Z"}}},
			OccurredAt:   "2026-08-11T10:00:00Z",
		},
		RequestKey: "full-mcp-trade-in-unknown",
	})
	// An ordinary record keeps its optional neutral relation on another item.
	call("record_event", transport.EventInput{AssetID: tradeInNew.Data.ID, TypeID: serviceType.Data.ID, EventFields: transport.EventFields{RequestKey: "full-mcp-related-asset", AmountMinor: 5_000, Currency: "CNY", OccurredAt: "2026-08-11T10:00:00Z", RelatedAssetID: mcpStringPointer(tradeInOld.Data.ID)}}, &event)
	if event.Data.RelatedAssetID != tradeInOld.Data.ID || event.Data.RelatedAssetName == "" {
		t.Fatalf("MCP related asset was not recorded: %+v", event.Data)
	}

	call("bind_asset_market", transport.BindMarketInput{AssetID: assetID, MarketItemID: "", RequestKey: "mcp-market-finish-unbind"}, new(any))
	for _, tool := range list.Tools {
		if !called[tool.Name] {
			t.Errorf("discovered tool was not successfully called: %s", tool.Name)
		}
	}
	t.Logf("HTTP walkthrough successfully called %d/%d discovered tools", len(called), len(list.Tools))
	grants, err := oauth.Grants(ctx, session.Principal)
	if err != nil || len(grants) != 1 {
		t.Fatal("client grant missing")
	}
	revoked, _ := request("POST", "/account/clients/"+grants[0].ID+"/revoke", url.Values{"csrf_token": {csrf}})
	if revoked.StatusCode != 303 {
		t.Fatal("Web client revocation failed")
	}
	if _, err := mcpClient.CallTool(ctx, &sdk.CallToolParams{Name: "get_asset", Arguments: transport.IDInput{ID: assetID}}); err == nil {
		t.Fatal("revoked client retained access")
	}
	page, _ = request("GET", "/assets/"+assetID, nil)
	if page.StatusCode != 200 {
		t.Fatal("client revocation incorrectly revoked Web session")
	}
}
