package gotreesitter

import (
	"fmt"
	"os"
	"testing"
)

func TestAdmissionRoutePrecedence(t *testing.T) {
	previous := admissionCandidateRouteDefault.Load()
	defer admissionCandidateRouteDefault.Store(previous)
	previousList := admissionCandidateLanguageAllowlist
	defer func() { admissionCandidateLanguageAllowlist = previousList }()
	admissionCandidateLanguageAllowlist = map[string]bool{"test": true}
	lang := &Language{Name: "test"}
	for _, global := range []uint32{0, 1, 2} {
		for _, parser := range []admissionRouteMode{admissionRouteFollowDefault, admissionRouteCandidateForced, admissionRouteProductionForced} {
			for _, listed := range []bool{false, true} {
				name := fmt.Sprintf("global=%d/parser=%d/listed=%v", global, parser, listed)
				t.Run(name, func(t *testing.T) {
					admissionCandidateRouteDefault.Store(global)
					admissionCandidateLanguageAllowlist["test"] = listed
					p := &Parser{language: lang, admissionCandidateRoute: parser}
					want := listed && global == 0 || global == 2
					switch parser {
					case admissionRouteCandidateForced:
						want = true
					case admissionRouteProductionForced:
						want = false
					}
					if got := p.admissionCandidateRouteEnabled(); got != want {
						t.Fatalf("enabled=%v, want %v", got, want)
					}
				})
			}
		}
	}
	SetAdmissionCandidateRouteDefault(false)
	if admissionCandidateRouteDefault.Load() != 1 {
		t.Fatal("setter off must be explicit")
	}
	SetAdmissionCandidateRouteDefault(true)
	if admissionCandidateRouteDefault.Load() != 2 {
		t.Fatal("setter on must be explicit")
	}
}

func TestAdmissionEnvironmentPrecedence(t *testing.T) {
	previous, present := os.LookupEnv("GTS_ADMISSION_CANDIDATE")
	defer func() {
		if present {
			os.Setenv("GTS_ADMISSION_CANDIDATE", previous)
		} else {
			os.Unsetenv("GTS_ADMISSION_CANDIDATE")
		}
	}()
	for _, tc := range []struct {
		value string
		want  uint32
	}{
		{"", 0}, {"unknown", 0}, {"0", 1}, {" FALSE ", 1}, {"off", 1}, {"no", 1}, {"1", 2}, {"TRUE", 2}, {"on", 2}, {"yes", 2},
	} {
		os.Setenv("GTS_ADMISSION_CANDIDATE", tc.value)
		if got := admissionCandidateEnvMode(); got != tc.want {
			t.Errorf("%q: mode=%d, want %d", tc.value, got, tc.want)
		}
	}
}
