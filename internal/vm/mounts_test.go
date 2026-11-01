package vm

import (
	"reflect"
	"testing"
)

func TestShareRootsKeepOnlyRequiredParents(t *testing.T) {
	roots, err := ShareRoots([]string{"/Users/ada/project", "/Users/ada/project/.git", "/Volumes/work/cards"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roots, []string{"/Users/ada/project", "/Volumes/work/cards"}) {
		t.Fatal(roots)
	}
}

func TestShareRootsRejectTartSyntaxAndBroadRoots(t *testing.T) {
	for _, path := range []string{"/", "/Users", "/Users/ada/project:ro", "/Users/ada/project,tag=x", "relative", "/Users/ada/project\nnext"} {
		t.Run(path, func(t *testing.T) {
			_, err := ShareRoots([]string{path})
			if err == nil {
				t.Fatal("must reject unsafe share root")
			}
		})
	}
}

func TestShareRootsPreserveSpacesAndUnicode(t *testing.T) {
	roots, err := ShareRoots([]string{"/Users/ada/雪 project"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roots, []string{"/Users/ada/雪 project"}) {
		t.Fatal(roots)
	}
}
