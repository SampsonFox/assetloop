package integration_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	transport "github.com/SampsonFox/assetloop/internal/mcp"
	webtransport "github.com/SampsonFox/assetloop/internal/web"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(copy)
}

func runMCPFullElement(t *testing.T, db *sql.DB, store scenarioStore, session application.SessionCredential, modelID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	web, err := webtransport.New(auth, catalog, lifecycle, db, webtransport.Options{AuthMode: "local", Specifications: specs, OAuth: oauth, OAuthIssuer: issuer})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", web.Handler())
	mux.Handle("/oauth/token", oauthHTTP.Guard(http.HandlerFunc(oauthHTTP.Token)))
	mux.Handle("/mcp", oauthHTTP.Protected(transport.NewHandler(transport.Services{Catalog: catalog, Specifications: specs, Lifecycle: lifecycle, Management: application.NewManagementService(store.(application.ManagementStore), nil)}, oauthHTTP.Authenticate)))
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
	if err != nil || len(list.Tools) != 39 {
		t.Fatal("OAuth MCP discovery failed")
	}
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
