package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rlrghb/olkcli/internal/msauth"
)

func TestExplainLoginTimeout(t *testing.T) {
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	live, cancelLive := context.WithTimeout(context.Background(), time.Hour)
	defer cancelLive()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name     string
		loginCtx context.Context
		err      error
		want     error
	}{
		{"budget ran out between polls", expired, context.DeadlineExceeded, errLoginTimedOut},
		{"budget ran out during a poll request", expired, fmt.Errorf("polling for token: %w", context.DeadlineExceeded), errLoginTimedOut},
		{"code expired is kept", expired, msauth.ErrDeviceCodeExpired, msauth.ErrDeviceCodeExpired},
		{"deadline from elsewhere while budget remains is kept", live, context.DeadlineExceeded, context.DeadlineExceeded},
		{"cancellation is kept", cancelled, context.Canceled, context.Canceled},
		{"other errors are kept", expired, errors.New("token request failed"), nil},
		{"no error", expired, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := explainLoginTimeout(tt.loginCtx, tt.err, false)
			switch {
			case tt.err == nil:
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
			case tt.want == nil:
				if !errors.Is(got, tt.err) {
					t.Fatalf("got %v, want the original error %v", got, tt.err)
				}
			default:
				if !errors.Is(got, tt.want) {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
