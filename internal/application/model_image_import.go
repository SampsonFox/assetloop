package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// ImportModelImage is a confirmed replacement of the active image of one shared
// product model. URL is the download source; SourceURL is attribution only.
type ImportModelImage struct {
	ModelID   string
	URL       string
	SourceURL string
}

// UploadModelImage is the same confirmed replacement fed with already-decoded
// image bytes. It never performs network I/O.
type UploadModelImage struct {
	ModelID   string
	SourceURL string
	Data      []byte
}

// uploadModelImageFingerprint is the bounded durable replay payload for an
// upload: the image digest and size stand in for the raw bytes so a same-key
// retry proves identical content without re-hashing a large command.
type uploadModelImageFingerprint struct {
	ModelID   string
	SourceURL string
	SHA256    string
	SizeBytes int64
}

// ModelImageImportService adds durable command replay to the existing image
// upload path. It follows ModelImportService: authorization and the receipt
// fingerprint are checked before any network or blob I/O, the download never
// holds a database transaction, and the verified blob is committed with the
// receipt in one management transaction. No new schema or storage abstraction is
// involved.
type ModelImageImportService struct {
	management *ManagementService
	images     *ModelImageService
	downloader ImageDownloader
	slots      chan struct{}
}

func NewModelImageImportService(m *ManagementService, images *ModelImageService, d ImageDownloader) *ModelImageImportService {
	return &ModelImageImportService{m, images, d, make(chan struct{}, 2)}
}

var errImageImportRequired = errors.New("image import required")
var errImageImportReplayed = errors.New("image import replayed")

// receiptRun binds tenant authorization, request-key validation and the durable
// receipt to one management transaction. A brand-new key surfaces
// errImageImportRequired so the caller can finish blob I/O before persistence;
// an existing key replays the original receipt or reports a content conflict.
func (s *ModelImageImportService) receiptRun(ctx context.Context, actor Principal, key, operation string, command any) func(func(ManagementStore) (ModelImage, error)) (ModelImage, error) {
	return func(fn func(ManagementStore) (ModelImage, error)) (ModelImage, error) {
		return managementWrite(ctx, s.management, actor, key, operation, CapabilityManageCatalog, command, fn)
	}
}

func (s *ModelImageImportService) Import(ctx context.Context, actor Principal, key string, cmd ImportModelImage) (ModelImage, error) {
	run := s.receiptRun(ctx, actor, key, "import_model_image", cmd)
	// Reuse receipt authorization, fingerprinting and conflict policy before I/O.
	result, err := run(func(ManagementStore) (ModelImage, error) {
		return ModelImage{}, errImageImportRequired
	})
	if !errors.Is(err, errImageImportRequired) {
		return result, err
	}
	// A new command validates the shared target and its attribution source before
	// any download, so an invalid command never reaches the network.
	if err := s.images.validateImportTarget(ctx, actor, cmd.ModelID, cmd.SourceURL); err != nil {
		return result, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return result, NewInputError("image.unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	data, err := s.downloader.DownloadImage(ctx, cmd.URL)
	if err != nil {
		return result, err
	}
	if int64(len(data)) > MaxModelImageBytes {
		return result, NewInputError("image.invalid")
	}
	return s.commit(ctx, actor, cmd.ModelID, data, cmd.SourceURL, run)
}

// Upload applies the same receipt, authorization and atomic binding contract to
// image bytes the caller already holds. No downloader is consulted.
func (s *ModelImageImportService) Upload(ctx context.Context, actor Principal, key string, cmd UploadModelImage) (ModelImage, error) {
	digest := sha256.Sum256(cmd.Data)
	run := s.receiptRun(ctx, actor, key, "upload_model_image", uploadModelImageFingerprint{
		ModelID: cmd.ModelID, SourceURL: cmd.SourceURL, SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(cmd.Data)),
	})
	result, err := run(func(ManagementStore) (ModelImage, error) {
		return ModelImage{}, errImageImportRequired
	})
	if !errors.Is(err, errImageImportRequired) {
		return result, err
	}
	return s.commit(ctx, actor, cmd.ModelID, cmd.Data, cmd.SourceURL, run)
}

// commit stores and verifies the immutable blob, then persists the active image
// binding and the receipt in the same management transaction. A losing identical
// concurrent command rolls back only its own uncommitted blob and returns the
// winner's revision.
func (s *ModelImageImportService) commit(ctx context.Context, actor Principal, modelID string, data []byte, source string, run func(func(ManagementStore) (ModelImage, error)) (ModelImage, error)) (ModelImage, error) {
	var result ModelImage
	_, err := s.images.uploadWithCommit(ctx, actor, modelID, data, source, func(m ModelImage) (ModelImage, error) {
		var commitErr error
		result, commitErr = run(func(store ManagementStore) (ModelImage, error) {
			if _, e := store.GetProductModel(ctx, actor.TenantID, modelID); e != nil {
				return ModelImage{}, e
			}
			if e := store.ClearModelImage(ctx, actor.TenantID, modelID); e != nil {
				return ModelImage{}, e
			}
			if e := store.InsertModelImage(ctx, m); e != nil {
				return ModelImage{}, e
			}
			return m, nil
		})
		if commitErr != nil {
			return ModelImage{}, commitErr
		}
		if result.ID != m.ID {
			return ModelImage{}, errImageImportReplayed
		}
		return result, nil
	})
	if errors.Is(err, errImageImportReplayed) {
		err = nil
	}
	return result, err
}
