package blob

import "github.com/SampsonFox/assetloop/internal/application"

type Registry map[string]application.BlobStore

func (r Registry) Get(id string) (application.BlobStore, bool) { store, ok := r[id]; return store, ok }
