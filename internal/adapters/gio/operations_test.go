//go:build darwin || ios || linux || windows

package gio

import (
	"errors"
	"testing"

	"github.com/izzyreal/ciwi/internal/presentation/operations"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
)

func TestNativeMutationFailureDistinguishesKnownAndAmbiguousOutcomes(t *testing.T) {
	operation := operations.Operation{Class: operations.ClassMutation}
	known := nativeOperationFailure(operation, &cnpclient.Error{
		Code: cnpv1.StatusCode_STATUS_CODE_INVALID_ARGUMENT, Message: "invalid request",
	})
	if known.State != operations.StateFailed {
		t.Fatalf("known server failure state = %s", known.State)
	}
	unknown := nativeOperationFailure(operation, errors.New("connection reset after request write"))
	if unknown.State != operations.StateOutcomeUnknown {
		t.Fatalf("ambiguous transport failure state = %s", unknown.State)
	}
}

func TestValidateNativeOperationRejectsInvalidIntentBeforeTransport(t *testing.T) {
	if err := validateNativeOperation(operations.Operation{
		Command: "run-pipeline", Arguments: map[string]string{"pipelineDbId": "0"},
	}); err == nil {
		t.Fatal("expected invalid pipeline operation to fail validation")
	}
	if err := validateNativeOperation(operations.Operation{
		Command: "run-chain", Arguments: map[string]string{"projectId": "2", "chainId": "release"},
	}); err != nil {
		t.Fatalf("valid chain operation failed validation: %v", err)
	}
}
