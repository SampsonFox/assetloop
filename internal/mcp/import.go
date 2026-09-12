package mcp

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ImportResourceInput struct {
	URL        string `json:"url" jsonschema:"Public HTTPS GLB direct URL; no credentials or private addresses. Maximum 25 MiB."`
	Name       string `json:"name"`
	SourceURL  string `json:"source_url" jsonschema:"Public attribution page, never a credential-bearing download URL."`
	Author     string `json:"author"`
	License    string `json:"license" jsonschema:"User-confirmed permission or license for this model."`
	RequestKey string `json:"request_key" jsonschema:"Stable command key. Reuse with the same payload on retries."`
}

func registerImport(server *sdk.Server, s Services) {
	register(server, "import_3d_resource_from_url", "Import a user-confirmed public HTTPS GLB using server-side bounded download. Does not bind. Confirm license and destination; reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q ImportResourceInput) (any, error) {
		if s.Import == nil {
			return nil, application.ErrModel3DUnavailable
		}
		r, err := s.Import.Import(ctx, p, q.RequestKey, application.ImportModel3D{URL: q.URL, Name: q.Name, SourceURL: q.SourceURL, Author: q.Author, License: q.License})
		return resourceResult(r), err
	})
}
