package justsyntax

import (
	"reflect"
	"testing"
)

func TestImportsFollowTopLevelJustSyntax(t *testing.T) {
	value := "import 'just/single.just'\r\n" +
		`import "just/escaped\x2ejust" # decoded double quote` + "\r\n" +
		"import?\t\"just/optional.just\"\r\n" +
		"ci:\r\n" +
		"    import \"just/recipe-body-command.just\"\r\n"
	want := []Import{
		{Path: "just/single.just"},
		{Path: "just/escaped.just"},
		{Optional: true, Path: "just/optional.just"},
	}
	if got := Imports(value); !reflect.DeepEqual(got, want) {
		t.Fatalf("imports = %#v, want %#v", got, want)
	}
}

func TestRecipesFollowTopLevelJustSyntax(t *testing.T) {
	value := "@ci:\r\n" +
		"check target=(arch() + ':' + os()):\r\n" +
		"quoted value='not:a:delimiter':\r\n" +
		"ci_variable := \"not-a-recipe\"\r\n" +
		"    nested:\r\n"
	want := map[string]bool{"ci": true, "check": true, "quoted": true}
	if got := Recipes(value); !reflect.DeepEqual(got, want) {
		t.Fatalf("recipes = %#v, want %#v", got, want)
	}
}
