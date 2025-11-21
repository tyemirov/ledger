package ledger

import (
	"errors"
	"testing"
)

func TestNewUserID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		wantErr error
		wantVal string
	}{
		{name: "valid", input: " user-123 ", wantVal: "user-123"},
		{name: "empty", input: "   ", wantErr: ErrInvalidUserID},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := NewUserID(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.String() != tc.wantVal {
				t.Fatalf("expected %q, got %q", tc.wantVal, result.String())
			}
		})
	}
}

func TestNewReservationID(t *testing.T) {
	t.Parallel()
	_, err := NewReservationID("")
	if !errors.Is(err, ErrInvalidReservationID) {
		t.Fatalf("expected ErrInvalidReservationID, got %v", err)
	}
}

func TestNewIdempotencyKey(t *testing.T) {
	t.Parallel()
	_, err := NewIdempotencyKey("   ")
	if !errors.Is(err, ErrInvalidIdempotencyKey) {
		t.Fatalf("expected ErrInvalidIdempotencyKey, got %v", err)
	}
}

func TestNewAmountCents(t *testing.T) {
	t.Parallel()
	_, err := NewAmountCents(0)
	if !errors.Is(err, ErrInvalidAmountCents) {
		t.Fatalf("expected ErrInvalidAmountCents, got %v", err)
	}
	value, err := NewAmountCents(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 100 {
		t.Fatalf("expected 100, got %d", value)
	}
}

func TestNewMetadataJSON(t *testing.T) {
	t.Parallel()
	meta, err := NewMetadataJSON("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.String() != "{}" {
		t.Fatalf("expected default metadata to be '{}', got %q", meta.String())
	}
	_, err = NewMetadataJSON("not-json")
	if !errors.Is(err, ErrInvalidMetadataJSON) {
		t.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}
