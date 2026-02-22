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

func TestParseGoTestOutputSourceLocation(t *testing.T) {
	file, line, ok := parseGoTestOutputSourceLocation("\tpkg/deep/case_test.go:108: boom")
	if !ok {
		t.Fatal("expected source location parse to succeed")
	}
	if file != "pkg/deep/case_test.go" || line != 108 {
		t.Fatalf("unexpected parsed source location: file=%q line=%d", file, line)
	}
}
