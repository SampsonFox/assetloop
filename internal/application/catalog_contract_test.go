package application

import (
	"reflect"
	"testing"
)

func TestAssetWriteCommandsDoNotAcceptDerivedColor(t *testing.T) {
	for _, command := range []any{SaveSpecificationAsset{}} {
		kind := reflect.TypeOf(command)
		if _, exists := kind.FieldByName("Color"); exists {
			t.Errorf("%s accepts an ignored color: color is a selected typed tag", kind.Name())
		}
	}
}
