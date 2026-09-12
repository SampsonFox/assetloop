package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"github.com/SampsonFox/assetloop/internal/domain"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

type imageImportStore struct{ ModelImageStore }

func (imageImportStore) GetProductModel(_ context.Context, tenant, id string) (domain.ProductModel, error) {
	return domain.ProductModel{ID: id, TenantID: tenant}, nil
}

type failingImageDownload struct{ calls int }

func (d *failingImageDownload) DownloadImage(context.Context, string) ([]byte, error) {
	d.calls++
	return nil, errors.New("network unavailable")
}
func TestImageImportAuthorizationAndDownloadFailure(t *testing.T) {
	s := NewModelImageService(imageImportStore{}, nil, nil, "local")
	d := &failingImageDownload{}
	p := Principal{TenantID: "11111111-1111-4111-8111-111111111111", UserID: "33333333-3333-4333-8333-333333333333", Role: RoleViewer}
	model := "22222222-2222-4222-8222-222222222222"
	if _, err := s.Import(context.Background(), p, model, "https://example.com/image", "", d); !errors.Is(err, ErrForbidden) || d.calls != 0 {
		t.Fatal("unauthorized download attempted")
	}
	p.Role = RoleOwner
	_, err := s.Import(context.Background(), p, model, "https://example.com/image", "", d)
	var input InputError
	if !errors.As(err, &input) || input.Code != "image.download_failed" || d.calls != 1 {
		t.Fatalf("download failure: %v", err)
	}
}

func TestModelImageContentValidation(t *testing.T) {
	var p, j bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(&p, im); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&j, im, nil); err != nil {
		t.Fatal(err)
	}
	w, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	for mime, data := range map[string][]byte{"image/png": p.Bytes(), "image/jpeg": j.Bytes(), "image/webp": w} {
		if got, err := validateModelImage(data); err != nil || got != mime {
			t.Errorf("%s: got %s / %v", mime, got, err)
		}
	}
	for _, data := range [][]byte{nil, []byte("<svg/>"), p.Bytes()[:24], make([]byte, MaxModelImageBytes+1)} {
		if _, err := validateModelImage(data); err == nil {
			t.Fatal("unsafe image accepted")
		}
	}
}
