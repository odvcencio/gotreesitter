package gotreesitter

import (
	"errors"
	"reflect"
	"testing"
)

func TestRecoveryRuntimeDetailedProductionAPIIsInert(t *testing.T) {
	if _, ok := reflect.TypeOf(parserColdState{}).FieldByName("recoveryRuntimeDetailed"); ok {
		t.Fatal("production parser sidecar contains detailed telemetry storage")
	}

	EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { EnableRecoveryRuntimeTelemetry(false) })
	parser := &Parser{}
	if _, err := parser.Parse(nil); !errors.Is(err, ErrNoLanguage) {
		t.Fatalf("parse error = %v, want ErrNoLanguage", err)
	}
	if parser.forestDeclineMemo != nil {
		t.Fatal("production diagnostic API allocated a parser sidecar")
	}
	if attempts := parser.DebugRecoveryRuntimeAttempts(); len(attempts) != 0 {
		t.Fatalf("production attempt receipt = %+v, want empty", attempts)
	}
}
