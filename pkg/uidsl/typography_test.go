package uidsl

import (
	"strings"
	"testing"
)

const validTypography = `
apiVersion: ciwi.ui/v1
kind: Typography
typography:
  families:
    body: {web: '"Ciwi Sans", sans-serif', native: Ciwi Sans}
    mono: {web: '"Ciwi Mono", monospace', native: Ciwi Mono}
  weights:
    regular: {web: 400, native: 400}
    medium: {web: 600, native: 600}
    strong: {web: 700, native: 700}
    title: {web: 800, native: 800}
  roles:
    body: {family: body, size: 16, weight: regular, lineHeight: 1.2}
    title: {family: body, size: 28, weight: title, lineHeight: 1.1}
    job-title: {family: body, size: 20, weight: strong, lineHeight: 1.2}
    heading: {family: body, size: 18, weight: strong, lineHeight: 1.2}
    subtitle: {family: body, size: 16, weight: regular, lineHeight: 1.2}
    control: {family: body, size: 14, weight: regular, lineHeight: 1.2}
    badge: {family: body, size: 12, weight: strong, lineHeight: 1.2}
    detail: {family: body, size: 14, weight: regular, lineHeight: 1.2}
    detail-small: {family: body, size: 12, weight: regular, lineHeight: 1.2}
    empty-state: {family: body, size: 13, weight: regular, lineHeight: 1.2}
    table-header: {family: body, size: 12, weight: strong, lineHeight: 1.2}
    code: {family: mono, size: 13, weight: regular, lineHeight: 1.45}
    code-inline: {family: mono, size: 13, weight: regular, lineHeight: 1.45}
    output-summary: {family: mono, size: 12, weight: strong, lineHeight: 1.35}
    output-meta: {family: mono, size: 11, weight: regular, lineHeight: 1.35}
    output-label: {family: mono, size: 12, weight: strong, lineHeight: 1.35}
    output-code: {family: mono, size: 12, weight: regular, lineHeight: 1.35}
`

func TestParseTypography(t *testing.T) {
	document, err := ParseTypography([]byte(validTypography))
	if err != nil {
		t.Fatal(err)
	}
	if got := document.Typography.Weights["regular"].Native; got != 400 {
		t.Fatalf("native regular weight = %d, want 400", got)
	}
	if got := document.Typography.Families["body"].Native; got != "Ciwi Sans" {
		t.Fatalf("native body family = %q, want Ciwi Sans", got)
	}
	if got := document.Typography.Roles["output-label"].Family; got != "mono" {
		t.Fatalf("output label family = %q, want mono", got)
	}
}

func TestTypographyRejectsUnknownRoleReferences(t *testing.T) {
	payload := strings.Replace(validTypography, "output-code: {family: mono", "output-code: {family: missing", 1)
	_, err := ParseTypography([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), `unknown family "missing"`) {
		t.Fatalf("ParseTypography error = %v, want unknown family", err)
	}
}
