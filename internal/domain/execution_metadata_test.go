package domain

import (
	"reflect"
	"testing"
)

func TestExecutionMetadataAccessors(t *testing.T) {
	metadata := ExecutionMetadata{
		ExecutionMetadataProjectID:   " 42 ",
		ExecutionMetadataDryRun:      "1",
		ExecutionMetadataNeedsJobIDs: " build, test ,, release ",
	}
	if got := metadata.Value(ExecutionMetadataProjectID); got != "42" {
		t.Fatalf("project ID = %q, want 42", got)
	}
	if value, ok := metadata.Int64(ExecutionMetadataProjectID); !ok || value != 42 {
		t.Fatalf("parsed project ID = %d, %v", value, ok)
	}
	if !metadata.Flag(ExecutionMetadataDryRun) {
		t.Fatal("dry-run flag was not recognized")
	}
	if got, want := metadata.CSV(ExecutionMetadataNeedsJobIDs), []string{"build", "test", "release"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("needs = %v, want %v", got, want)
	}
}

func TestExecutionMetadataCloneIsIndependent(t *testing.T) {
	original := ExecutionMetadata{ExecutionMetadataProject: "ciwi"}
	clone := original.Clone()
	clone.Set(ExecutionMetadataProject, "other")
	if original.Value(ExecutionMetadataProject) != "ciwi" {
		t.Fatal("changing clone mutated original")
	}
}
