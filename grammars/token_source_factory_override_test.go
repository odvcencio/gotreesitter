package grammars_test

import (
	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"reflect"
	"testing"
)

func TestLazyFactoryCustomCatalogOverride(t *testing.T) {
	entry := grammars.DetectLanguageByName("c")
	if entry == nil {
		t.Fatal("missing c")
	}
	copied := *entry
	original := copied
	defer grammars.Register(original)
	custom := func([]byte, *ts.Language) ts.TokenSource { return nil }
	copied.TokenSourceFactory = custom
	grammars.Register(copied)
	got := grammars.DetectLanguageByName("c")
	if got == nil || got.TokenSourceFactory == nil || reflect.ValueOf(got.TokenSourceFactory).Pointer() != reflect.ValueOf(custom).Pointer() {
		t.Fatal("custom factory was not preserved")
	}
}
