package mcp

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ModelImageInput struct {
	ModelID string `json:"model_id" jsonschema:"Existing product model ID. A model image is shared by every item of that model."`
}

type ImportModelImageInput struct {
	ModelID    string `json:"model_id" jsonschema:"Existing product model ID. Confirm the shared model scope before replacing its active image."`
	URL        string `json:"url" jsonschema:"Public HTTPS image direct URL; no credentials or private addresses. Maximum 8 MiB."`
	SourceURL  string `json:"source_url" jsonschema:"Public attribution page for the image, never a credential-bearing download URL. Empty is allowed when the source page is unknown."`
	RequestKey string `json:"request_key" jsonschema:"Stable command key. Reuse with the same payload on retries."`
}

type UploadModelImageInput struct {
	ModelID       string `json:"model_id" jsonschema:"Existing product model ID. Visually verify the image shows this exact shared model and color before uploading, because every item of that model uses it."`
	ContentBase64 string `json:"content_base64" jsonschema:"Base64-encoded PNG, JPEG or WebP bytes of the user-confirmed image, at most 8 MiB decoded. No local path, URL or credential is accepted."`
	SourceURL     string `json:"source_url,omitempty" jsonschema:"Optional public attribution page for the image, never a credential-bearing URL."`
	RequestKey    string `json:"request_key" jsonschema:"Stable command key. Reuse with identical content on retries; different content under the same key is rejected."`
}

// decodeModelImageContent rejects an impossible encoded length before allocating,
// then decodes strictly. The existing validateModelImage still enforces the 8 MiB
// cap, decodable PNG/JPEG/WebP formats and pixel bounds.
func decodeModelImageContent(encoded string) ([]byte, error) {
	if encoded == "" || len(encoded) > base64.StdEncoding.EncodedLen(application.MaxModelImageBytes) {
		return nil, application.NewInputError("image.invalid")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, application.NewInputError("image.invalid")
	}
	return data, nil
}

// ModelImageResult is the only public projection: no tenant, store or object key.
type ModelImageResult struct {
	ID          string `json:"id"`
	ModelID     string `json:"model_id"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	SourceURL   string `json:"source_url"`
}

func modelImageResult(m application.ModelImage) ModelImageResult {
	return ModelImageResult{ID: m.ID, ModelID: m.ModelID, ContentType: m.ContentType, SizeBytes: m.SizeBytes, SHA256: m.SHA256, SourceURL: m.SourceURL}
}

func registerModelImages(server *sdk.Server, s Services) {
	register(server, "get_model_image", "Read the current product-model image metadata, or null when the model has no image. A model image is shared by every item of that model; storage locations are never returned.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q ModelImageInput) (any, error) {
		if s.Images == nil {
			return nil, application.ErrModel3DUnavailable
		}
		image, err := s.Images.Get(ctx, p, q.ModelID)
		if errors.Is(err, application.ErrImageNotFound) {
			return (*ModelImageResult)(nil), nil
		}
		if err != nil {
			return nil, err
		}
		result := modelImageResult(image)
		return result, nil
	})
	register(server, "import_model_image_from_url", "Download a user-confirmed public HTTPS product image and make it the active image of one shared model. Before importing, visually verify the image shows the correct model and color, and confirm the shared model scope because every item of that model uses it. Replaces any current image. Does not touch 3D resources, bindings or lifecycle events. Reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q ImportModelImageInput) (any, error) {
		if s.ImageImport == nil {
			return nil, application.ErrModel3DUnavailable
		}
		image, err := s.ImageImport.Import(ctx, p, q.RequestKey, application.ImportModelImage{ModelID: q.ModelID, URL: q.URL, SourceURL: q.SourceURL})
		if err != nil {
			return nil, err
		}
		result := modelImageResult(image)
		return result, nil
	})
	register(server, "upload_model_image", "Replace the active shared product-model image with bounded base64 content the user already confirmed, without downloading anything. Before uploading, visually verify the image shows the exact model and color, and confirm the shared model scope because every item of that model uses it. Replaces any current image; it does not touch 3D resources, bindings or lifecycle events. Reuse request_key on retries; the same key with different content is rejected.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q UploadModelImageInput) (any, error) {
		if s.ImageImport == nil {
			return nil, application.ErrModel3DUnavailable
		}
		data, err := decodeModelImageContent(q.ContentBase64)
		if err != nil {
			return nil, err
		}
		image, err := s.ImageImport.Upload(ctx, p, q.RequestKey, application.UploadModelImage{ModelID: q.ModelID, SourceURL: q.SourceURL, Data: data})
		if err != nil {
			return nil, err
		}
		result := modelImageResult(image)
		return result, nil
	})
}
