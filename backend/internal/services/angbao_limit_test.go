package services

import (
	"errors"
	"testing"
)

// An unbounded amount lets two typos wrap the report total negative.
func TestValidateAngbaoAmt_RejectsOverMax(t *testing.T) {
	if err := validateAngbaoAmt(new(MaxAngbaoAmt)); err != nil {
		t.Fatalf("max amount: %v", err)
	}
	if err := validateAngbaoAmt(new(MaxAngbaoAmt + 1)); !errors.Is(err, ErrGuestPatchInvalid) {
		t.Fatalf("over max err = %v, want ErrGuestPatchInvalid", err)
	}
}
