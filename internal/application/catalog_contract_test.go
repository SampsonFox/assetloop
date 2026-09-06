package application

import (
	"reflect"
	"testing"
)

func TestAssetWriteCommandsDoNotAcceptDerivedColor(t *testing.T) {
	for _, command := range []any{CreateCatalogAsset{}, UpdateCatalogAsset{}} {
		kind := reflect.TypeOf(command)
		if _, exists := kind.FieldByName("Color"); exists {
			t.Errorf("%s accepts an ignored color: color is derived from the selected variant", kind.Name())
		}
	}
}
