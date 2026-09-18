//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import "testing"

func TestG18CertificateActivationRestoresCachedRunner(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)

	firstRestore := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	if !runner.certificateAdmissionEnabled {
		t.Fatal("certificate activation did not arm the cached runner")
	}

	secondRestore := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	firstRestore()
	if !runner.certificateAdmissionEnabled {
		t.Fatal("an older restore disabled a newer activation")
	}
	secondRestore()
	secondRestore()
	if runner.certificateAdmissionEnabled || runner.options.recordDropCohortCertificates {
		t.Fatal("idempotent restore did not clear cached activation")
	}
}

func TestG18CertificateActivationIgnoresProductionRoute(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(false)
	restore := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	restore()
	if p.admissionCandidateRunner != nil {
		t.Fatal("production route unexpectedly acquired a candidate runner")
	}
}

func TestG18CertificateActivationDefaultOffAndNilSafe(t *testing.T) {
	var nilParser *Parser
	nilParser.DiagnosticEnableDropCohortCertificateAdmissionForTest()()

	p := &Parser{}
	p.SetAdmissionCandidateRoute(true)
	p.DiagnosticEnableDropCohortCertificateAdmissionForTest()()
	if p.admissionCandidateRunner != nil {
		t.Fatal("failed runner acquisition published cached state")
	}

	runner := newAdmissionCandidateGoParser(t)
	runner.SetAdmissionCandidateRoute(true)
	cached, err := runner.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	if cached.certificateAdmissionEnabled || cached.options.recordDropCohortCertificates {
		t.Fatal("candidate runner defaulted to certificate admission")
	}
}

func TestG18CertificateActivationEligibilityIsolation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Parser) func()
	}{
		{name: "logger", setup: func(p *Parser) func() {
			p.SetLogger(func(ParserLogType, string) {})
			return p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
		}},
		{name: "suppressed", setup: func(p *Parser) func() {
			unsuppress := p.suppressAdmissionCandidateRoute()
			restore := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
			unsuppress()
			return restore
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := newAdmissionCandidateGoParser(t)
			p.SetAdmissionCandidateRoute(true)
			restore := test.setup(p)
			restore()
			if p.admissionCandidateRunner != nil {
				t.Fatal("ineligible activation acquired a runner")
			}
		})
	}
}

func TestG18CertificateActivationSupportsIncludedRanges(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	p.SetIncludedRanges([]Range{{StartByte: 0, EndByte: 1, EndPoint: Point{Column: 1}}})
	restore := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	runner, ok := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
	if !ok || runner == nil || !runner.certificateAdmissionEnabled {
		t.Fatal("included ranges did not acquire an active certificate runner")
	}
	restore()
	if runner.certificateAdmissionEnabled || runner.certificateAdmissionToken != nil || runner.options.recordDropCohortCertificates {
		t.Fatal("restore retained certificate activation for included ranges")
	}
}

func TestG18CertificateActivationRestoreIsolation(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	oldRestore := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	oldRunner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	p.pinToProductionRoute()
	oldRestore()
	if p.admissionCandidateRunner != nil {
		t.Fatal("restore republished a pinned runner")
	}

	p.SetAdmissionCandidateRoute(true)
	newRestore := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	newRunner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	if oldRunner == newRunner {
		t.Fatal("runner replacement did not allocate a new runner")
	}
	oldRestore()
	if !newRunner.certificateAdmissionEnabled {
		t.Fatal("out-of-date restore disabled the replacement runner")
	}
	if err := newRunner.compact.Reset(); err != nil {
		t.Fatal(err)
	}
	newRestore()
	if newRunner.certificateAdmissionEnabled || newRunner.certificateAdmissionToken != nil {
		t.Fatal("restore after reset did not clear the active runner")
	}
}

func TestG18CertificateActivationNestedOutOfOrderRestore(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	outer := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	inner := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	outer()
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	if !runner.certificateAdmissionEnabled {
		t.Fatal("outer restore disabled the nested activation")
	}
	inner()
	if runner.certificateAdmissionEnabled || runner.certificateAdmissionToken != nil {
		t.Fatal("nested restore did not clear activation")
	}
}

func TestG18CertificateActivationCachedParseBoundary(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	resetAdmissionCandidateCounters()
	source := []byte("package p\n")
	restore := p.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	runner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.certificateAdmissionEnabled || !runner.options.recordDropCohortCertificates {
		t.Fatal("activation did not remain armed until restore")
	}
	routed, fallback := AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf("first parse counters=%d/%d, want 1/0", routed, fallback)
	}
	first.Release()
	restore()
	if runner.certificateAdmissionEnabled || runner.options.recordDropCohortCertificates {
		t.Fatal("restore left the cached runner armed")
	}
	second, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	deferredRunner, err := p.acquireAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	if deferredRunner != runner {
		t.Fatal("second parse replaced the cached runner")
	}
	routed, fallback = AdmissionCandidateCounters()
	if routed != 2 || fallback != 0 {
		t.Fatalf("second parse counters=%d/%d, want 2/0", routed, fallback)
	}
	if runner.certificateAdmissionEnabled || runner.options.recordDropCohortCertificates {
		t.Fatal("restored runner re-armed certificate admission")
	}
	second.Release()
}
