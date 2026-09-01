package migrations

import (
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestDialectVersionsMatch(t *testing.T) {
	versions := func(dir string) []string {
		entries, err := fs.ReadDir(FS, dir)
		if err != nil {
			t.Fatal(err)
		}
		var result []string
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
				result = append(result, entry.Name())
			}
		}
		sort.Strings(result)
		return result
	}
	if sqlite, postgres := versions("sqlite"), versions("postgres"); !reflect.DeepEqual(sqlite, postgres) {
		t.Fatalf("migration versions differ: sqlite=%v postgres=%v", sqlite, postgres)
	}
}
