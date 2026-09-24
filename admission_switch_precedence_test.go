package gotreesitter

import (
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
	for _, tc := range []struct {
		name   string
		global uint32
		parser admissionRouteMode
		listed bool
		want   bool
	}{
		{"implicit/unlisted", 0, admissionRouteFollowDefault, false, false},
		{"implicit/listed", 0, admissionRouteFollowDefault, true, true},
		{"explicit off/listed", 1, admissionRouteFollowDefault, true, false},
		{"explicit off/unlisted", 1, admissionRouteFollowDefault, false, false},
		{"explicit on/listed", 2, admissionRouteFollowDefault, true, true},
		{"explicit on/unlisted", 2, admissionRouteFollowDefault, false, true},
		{"parser off/listed", 2, admissionRouteProductionForced, true, false},
		{"parser on/unlisted", 1, admissionRouteCandidateForced, false, true},
		{"parser on/listed", 1, admissionRouteCandidateForced, true, true},
		{"parser off/implicit", 0, admissionRouteProductionForced, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			admissionCandidateRouteDefault.Store(tc.global)
			admissionCandidateLanguageAllowlist["test"] = tc.listed
			p := &Parser{language: lang, admissionCandidateRoute: tc.parser}
			if got := p.admissionCandidateRouteEnabled(); got != tc.want {
				t.Fatalf("enabled=%v, want %v", got, tc.want)
			}
		})
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
