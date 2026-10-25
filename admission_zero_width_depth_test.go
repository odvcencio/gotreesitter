//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type admissionDepthFixture struct {
	name         string
	lang         func() *gts.Language
	source       string
	wantDigest   string
	mustRoute    bool
	mustFallback bool
}

func TestAdmissionCandidateYAMLZeroWidthDepth(t *testing.T) {
	fixtures := []admissionDepthFixture{
		{name: "github-actions", lang: grammars.YamlLanguage, mustRoute: true, wantDigest: "67d6dbaf8985d9b3e87cf8b09097eabfe63bc902db7863c91e7e061d85ca8cd7", source: `name: Go
on:
  push:
    branches: [main]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Test
        run: go test -v ./...
`},
		{name: "docker-compose", lang: grammars.YamlLanguage, wantDigest: "2ee78d1baf7420e08673cd262567263f9b2373670125305d0e985aed3df0b54e", source: `services:
  frontend:
    build:
      context: frontend
      target: development
    ports:
      - 3000:3000
    volumes:
      - ./frontend:/usr/src/app
    depends_on:
      - backend
  backend:
    image: example/backend:latest
    environment:
      NODE_ENV: production
`},
		{name: "kubernetes", lang: grammars.YamlLanguage, wantDigest: "8d18e5d28f828e2a8b4de34c6c1bdf394ecd57da3290b65450892ab975a2d624", source: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:1.14.2
          ports:
            - containerPort: 80
          resources:
            limits:
              memory: "128Mi"
`},
	}
	requireAdmissionDepthRatchet(t, fixtures, 1)
}

func TestAdmissionCandidateRepresentativeDepthRatchet(t *testing.T) {
	fixtures := []admissionDepthFixture{
		{name: "go", lang: grammars.GoLanguage, mustRoute: true, wantDigest: "95760c1ae5d0e46b32df59ef54c88931731093c391edd8786886c49aed739e94", source: "package demo\n\nfunc add(a, b int) int { return a + b }\n"},
		{name: "javascript", lang: grammars.JavascriptLanguage, mustRoute: true, wantDigest: "3a1225ed47eacdbe9fa6372cbfe2ccbf6550a9d5fbcc2f31a58cc8b56c00b6a0", source: "export function add(a, b) {\n  return a + b;\n}\n"},
		{name: "python", lang: grammars.PythonLanguage, mustRoute: true, wantDigest: "17d7802770da0f2972f52980a8af01cee4bb274b8cc497184804968ccb1773aa", source: "def greet(name):\n    return f\"hello {name}\"\n\nprint(greet(\"world\"))\n"},
		{name: "rust", lang: grammars.RustLanguage, wantDigest: "34b82eb28fe0bc3f105ad63604104e7fcdbaf420af12e1ff9696a2640a93a3b2", source: "fn main() {\n    let values = [1, 2, 3];\n    println!(\"{}\", values.len());\n}\n"},
		{name: "typescript", lang: grammars.TypescriptLanguage, mustRoute: true, wantDigest: "be604fe1d9a6e5430ce2a6ff58b7a7777f9881100d20805a2b2d09952c369729", source: "interface User { id: number; name: string }\nexport const user: User = { id: 1, name: \"Ada\" };\n"},
		{name: "html", lang: grammars.HtmlLanguage, wantDigest: "e870fad244cda7aa86e5a2ca0e310eb0dbe4be6a2920f791ef788c23786f82b4", source: "<!doctype html>\n<html><body><main><h1>Hello</h1><p>World</p></main></body></html>\n"},
		{name: "powershell", lang: grammars.PowershellLanguage, mustRoute: true, wantDigest: "874998165753303f1c085af48c1256fdc637c857b8a9b1aed17f2c7010512213", source: "$items = @(1, 2, 3)\nforeach ($item in $items) { Write-Output $item }\n"},
	}
	requireAdmissionDepthRatchet(t, fixtures, 5)
}

func requireAdmissionDepthRatchet(t *testing.T, fixtures []admissionDepthFixture, minRouted int) {
	t.Helper()
	routedFixtures := 0
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			lang := fixture.lang()
			source := []byte(fixture.source)

			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			want, err := production.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer want.Release()
			wantInspection, err := benchfixtures.InspectGoTree(want.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			if wantInspection.SHA256 != fixture.wantDigest {
				t.Fatalf("production digest drifted: got=%s want=%s", wantInspection.SHA256, fixture.wantDigest)
			}

			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			gts.ResetAdmissionCandidateCountersForTest()
			got, err := candidate.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Release()
			routed, fallback := gts.AdmissionCandidateCounters()
			if routed+fallback != 1 || routed > 1 || fallback > 1 {
				t.Fatalf("candidate counters are not one exact decision: routed=%d fallback=%d", routed, fallback)
			}
			if fixture.mustRoute && routed != 1 {
				t.Fatalf("candidate route regressed: fallback=%d reason=%q", fallback, gts.AdmissionCandidateLastFallbackReason())
			}
			if fixture.mustFallback && fallback != 1 {
				t.Fatalf("candidate route bypassed the required fallback: routed=%d", routed)
			}
			if routed == 1 {
				routedFixtures++
			}
			gotInspection, err := benchfixtures.InspectGoTree(got.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			if gotInspection.SHA256 != fixture.wantDigest {
				t.Fatalf("candidate digest drifted: got=%s want=%s", gotInspection.SHA256, fixture.wantDigest)
			}
		})
	}
	if routedFixtures < minRouted {
		t.Fatalf("depth coverage regressed: routed=%d want>=%d", routedFixtures, minRouted)
	}
}
