package application

import (
	"context"
	"errors"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
)

// ModelDownloader is an outbound infrastructure port, not arbitrary HTTP access.
type ModelDownloader interface {
	Download(context.Context, string) ([]byte, error)
}
type ImportModel3D struct{ URL, Name, SourceURL, Author, License string }
type ModelImportService struct {
	management *ManagementService
	media      *ModelMediaService
	downloader ModelDownloader
	slots      chan struct{}
}

func NewModelImportService(m *ManagementService, media *ModelMediaService, d ModelDownloader) *ModelImportService {
	return &ModelImportService{m, media, d, make(chan struct{}, 2)}
}

var errImportRequired = errors.New("import required")
var errImportReplayed = errors.New("import replayed")

func (s *ModelImportService) Import(ctx context.Context, actor Principal, key string, cmd ImportModel3D) (domain.Model3DResource, error) {
	// Reuse receipt authorization, fingerprinting and conflict policy before any I/O.
	run := func(fn func(ManagementStore) (domain.Model3DResource, error)) (domain.Model3DResource, error) {
		return managementWrite(ctx, s.management, actor, key, "import_3d_resource", CapabilityManageCatalog, cmd, fn)
	}
	result, err := run(func(ManagementStore) (domain.Model3DResource, error) {
		return domain.Model3DResource{}, errImportRequired
	})
	if !errors.Is(err, errImportRequired) {
		return result, err
	}
	if _, err = validateResourceMetadata(UpdateModel3DResource{Name: cmd.Name, SourceURL: cmd.SourceURL, Author: cmd.Author, License: cmd.License}); err != nil {
		return result, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return result, ErrModel3DUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	file, err := s.downloader.Download(ctx, cmd.URL)
	if err != nil {
		return result, err
	}
	if int64(len(file)) > MaxProductModel3DBytes {
		return result, NewInputError("validation.model_import_size")
	}
	if err = validateGLB(file); err != nil {
		return result, NewInputError("validation.model_import_glb")
	}
	_, err = s.media.uploadWithCommit(ctx, actor, UploadModel3DResource{Name: cmd.Name, File: file, SourceURL: cmd.SourceURL, Author: cmd.Author, License: cmd.License}, func(media domain.Model3DResource) error {
		var commitErr error
		result, commitErr = run(func(store ManagementStore) (domain.Model3DResource, error) {
			return media, store.CreateModel3DResource(ctx, media)
		})
		if commitErr != nil {
			return commitErr
		}
		// A concurrent identical command won. Reuse existing upload rollback cleanup
		// for only this uncommitted blob, then return the winner's durable result.
		if result.ID != media.ID {
			return errImportReplayed
		}
		return nil
	})
	if errors.Is(err, errImportReplayed) {
		err = nil
	}
	return result, err
}
