package application

import (
	"errors"
	"testing"
)

func TestErrorKindOf(t *testing.T) {
	cause := errors.New("duplicate")
	err := NewError(ErrorConflict, "already exists", cause)
	if got := ErrorKindOf(err); got != ErrorConflict {
		t.Fatalf("ErrorKindOf=%q want=%q", got, ErrorConflict)
	}
	if !errors.Is(err, cause) {
		t.Fatal("application error did not retain cause")
	}
}
