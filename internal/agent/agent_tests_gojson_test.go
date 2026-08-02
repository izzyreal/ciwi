package agent

import "testing"

func TestParseGoTestJSONSuiteCapturesSourceLocation(t *testing.T) {
	lines := []string{
		`{"Action":"run","Package":"github.com/acme/repo/pkg/math","Test":"TestAdd"}`,
		`{"Action":"output","Package":"github.com/acme/repo/pkg/math","Test":"TestAdd","Output":"    add_test.go:42: expected 2\n"}`,
		`{"Action":"fail","Package":"github.com/acme/repo/pkg/math","Test":"TestAdd","Elapsed":0.01}`,
	}
	suite := parseGoTestJSONSuite("go", lines)
	if len(suite.Cases) != 1 {
		t.Fatalf("expected one case, got %d", len(suite.Cases))
	}
	tc := suite.Cases[0]
	if tc.File != "add_test.go" || tc.Line != 42 {
		t.Fatalf("expected source location add_test.go:42, got %+v", tc)
	}
}

func TestParseGoTestJSONSuiteCapturesPackageBuildFailure(t *testing.T) {
	lines := []string{
		`{"Action":"output","Package":"github.com/acme/repo/cmd/app","Output":"# github.com/acme/repo/cmd/app\ncmd/app/main.go:12:3: undefined: launch\n"}`,
		`{"Action":"fail","Package":"github.com/acme/repo/cmd/app","Elapsed":0.02}`,
	}
	suite := parseGoTestJSONSuite("go", lines)
	if suite.Total != 1 || suite.Passed != 0 || suite.Failed != 1 {
		t.Fatalf("unexpected package failure summary: %+v", suite)
	}
	if len(suite.Cases) != 1 {
		t.Fatalf("expected one synthetic package case, got %d", len(suite.Cases))
	}
	tc := suite.Cases[0]
	if tc.Package != "github.com/acme/repo/cmd/app" || tc.Name != "Package build or setup" || tc.Status != "fail" {
		t.Fatalf("unexpected synthetic package case: %+v", tc)
	}
	if tc.File != "cmd/app/main.go" || tc.Line != 12 {
		t.Fatalf("expected source location cmd/app/main.go:12, got %+v", tc)
	}
}

func TestParseGoTestJSONSuiteDoesNotDoubleCountPackageFailure(t *testing.T) {
	lines := []string{
		`{"Action":"run","Package":"github.com/acme/repo/pkg/math","Test":"TestAdd"}`,
		`{"Action":"fail","Package":"github.com/acme/repo/pkg/math","Test":"TestAdd","Elapsed":0.01}`,
		`{"Action":"fail","Package":"github.com/acme/repo/pkg/math","Elapsed":0.02}`,
	}
	suite := parseGoTestJSONSuite("go", lines)
	if suite.Total != 1 || suite.Failed != 1 || len(suite.Cases) != 1 {
		t.Fatalf("expected named failure to represent package failure once, got %+v", suite)
	}
}

func TestParseGoTestOutputSourceLocation(t *testing.T) {
	file, line, ok := parseGoTestOutputSourceLocation("\tpkg/deep/case_test.go:108: boom")
	if !ok {
		t.Fatal("expected source location parse to succeed")
	}
	if file != "pkg/deep/case_test.go" || line != 108 {
		t.Fatalf("unexpected parsed source location: file=%q line=%d", file, line)
	}
}
