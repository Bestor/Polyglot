package vault

import (
	"context"
	"errors"
	"testing"
	"time"

	vaultapi "github.com/openbao/openbao/api/v2"
)

func TestPathFor(t *testing.T) {
	got := PathFor("valorant", "auth_token")
	want := "datasources/valorant/auth_token"
	if got != want {
		t.Errorf("PathFor() = %q, want %q", got, want)
	}
}

type fakeSys struct {
	statusCalls int
	statusErrs  int // fail this many calls before succeeding
	sealed      bool
	unsealed    bool
	unsealErr   error
}

func (f *fakeSys) SealStatusWithContext(ctx context.Context) (*vaultapi.SealStatusResponse, error) {
	f.statusCalls++
	if f.statusCalls <= f.statusErrs {
		return nil, errors.New("connection refused")
	}
	return &vaultapi.SealStatusResponse{Sealed: f.sealed}, nil
}

func (f *fakeSys) UnsealWithContext(ctx context.Context, key string) (*vaultapi.SealStatusResponse, error) {
	f.unsealed = true
	if f.unsealErr != nil {
		return nil, f.unsealErr
	}
	return &vaultapi.SealStatusResponse{Sealed: false}, nil
}

func TestUnsealSysIfNeeded_AlreadyUnsealed(t *testing.T) {
	sys := &fakeSys{sealed: false}
	if err := unsealSysIfNeeded(context.Background(), sys, "key"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sys.unsealed {
		t.Error("expected Unseal not to be called when already unsealed")
	}
}

func TestUnsealSysIfNeeded_Sealed(t *testing.T) {
	sys := &fakeSys{sealed: true}
	if err := unsealSysIfNeeded(context.Background(), sys, "key"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sys.unsealed {
		t.Error("expected Unseal to be called when sealed")
	}
}

func TestUnsealSysIfNeeded_RetriesTransientStatusFailures(t *testing.T) {
	restoreDelay := unsealRetryDelay
	unsealRetryDelay = time.Millisecond
	defer func() { unsealRetryDelay = restoreDelay }()

	sys := &fakeSys{sealed: true, statusErrs: 3}
	if err := unsealSysIfNeeded(context.Background(), sys, "key"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sys.unsealed {
		t.Error("expected Unseal to be called after transient failures cleared")
	}
}

func TestUnsealSysIfNeeded_GivesUpAfterMaxAttempts(t *testing.T) {
	restoreDelay, restoreAttempts := unsealRetryDelay, unsealMaxAttempts
	unsealRetryDelay = time.Millisecond
	unsealMaxAttempts = 3
	defer func() { unsealRetryDelay, unsealMaxAttempts = restoreDelay, restoreAttempts }()

	sys := &fakeSys{sealed: true, statusErrs: 100}
	err := unsealSysIfNeeded(context.Background(), sys, "key")
	if err == nil {
		t.Fatal("expected an error once max attempts are exhausted")
	}
	if sys.unsealed {
		t.Error("expected Unseal not to be called when seal status could never be determined")
	}
}
