//go:build darwin && cgo

package cnpclient

import (
	"reflect"
	"testing"
)

func TestCompactTXTFields(t *testing.T) {
	got := compactTXTFields("version=v0.2.8\n\n api_version=1 \n")
	want := []string{"version=v0.2.8", "api_version=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
}
