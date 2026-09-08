// Package mcp adapts semantic tools to the same application services as Web.
package mcp

import (
	"context"
	"errors"
	"net/http"

	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ScopeRead      = "assets:read"
	ScopeCatalog   = "assets:catalog"
	ScopeLifecycle = "assets:lifecycle"
)

// Identity is supplied by the token verifier, never by tool arguments.
type Identity struct {
	Principal application.Principal
	Scopes    []string
}
type Authenticate func(context.Context, *http.Request) (Identity, error)

type Services struct {
	Catalog        *application.CatalogService
	Specifications *application.SpecificationService
	Lifecycle      *application.LifecycleService
	Media          *application.ModelMediaService
}

type identityKey struct{}
type Result struct {
	Data  any        `json:"data,omitempty"`
	Error *ToolError `json:"error,omitempty"`
}
type ToolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// NewHandler deliberately requires a verifier. Production wiring is added with
// OAuth; a nil verifier fails closed even for initialization and tool discovery.
func NewHandler(services Services, authenticate Authenticate) http.Handler {
	server := sdk.NewServer(&sdk.Implementation{Name: "assetloop", Version: "0.1.0"}, nil)
	registerQueries(server, services)
	transport := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if authenticate == nil {
			http.Error(w, "MCP authorization required", http.StatusUnauthorized)
			return
		}
		identity, err := authenticate(r.Context(), r)
		if err != nil || identity.Principal.Require(application.CapabilityView) != nil {
			http.Error(w, "MCP authorization required", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		transport.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
	})
}

func register[I any](server *sdk.Server, name, description, scope string, capability application.Capability, run func(context.Context, application.Principal, I) (any, error)) {
	sdk.AddTool(server, &sdk.Tool{Name: name, Description: description, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: scope == ScopeRead}},
		func(ctx context.Context, _ *sdk.CallToolRequest, input I) (*sdk.CallToolResult, Result, error) {
			identity, ok := ctx.Value(identityKey{}).(Identity)
			allowed := false
			for _, granted := range identity.Scopes {
				if granted == scope {
					allowed = true
				}
			}
			if !ok || !allowed || identity.Principal.Require(capability) != nil {
				return failure("forbidden", "This client or account is not allowed to perform this operation.", false)
			}
			value, err := run(ctx, identity.Principal, input)
			if err == nil {
				return nil, Result{Data: value}, nil
			}
			var inputError application.InputError
			switch {
			case errors.As(err, &inputError):
				return failure("invalid_input", inputError.Error(), false)
			case errors.Is(err, application.ErrForbidden), errors.Is(err, application.ErrUnauthorized):
				return failure("forbidden", "This client or account is not allowed to perform this operation.", false)
			case errors.Is(err, application.ErrModel3DNotFound):
				return failure("not_found", "The requested resource does not exist.", false)
			case errors.Is(err, application.ErrModel3DReferenced):
				return failure("referenced", "The resource is still referenced.", false)
			default:
				return failure("unavailable", "The operation could not be completed.", true)
			}
		})
}

func failure(code, message string, retryable bool) (*sdk.CallToolResult, Result, error) {
	result := Result{Error: &ToolError{Code: code, Message: message, Retryable: retryable}}
	return &sdk.CallToolResult{IsError: true, StructuredContent: result, Content: []sdk.Content{&sdk.TextContent{Text: message}}}, result, nil
}
