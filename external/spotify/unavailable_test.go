package spotify

import (
	"errors"
	"fmt"
	"testing"

	"github.com/devgianlu/go-librespot/audio"
)

func TestKeyErrorClassification(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantAuth    bool
		wantUnavail bool
	}{
		{
			// Spotify's "this account can't play this track" answer. It is
			// not a session problem, so reconnecting/sign-in must not fire.
			name:        "unavailable track",
			err:         &audio.KeyProviderError{Code: aesKeyErrUnavailable},
			wantAuth:    false,
			wantUnavail: true,
		},
		{
			name:        "other key error stays an auth error",
			err:         &audio.KeyProviderError{Code: 1},
			wantAuth:    true,
			wantUnavail: false,
		},
		{
			name:        "wrapped unavailable is still recognised",
			err:         fmt.Errorf("new stream: %w", &audio.KeyProviderError{Code: aesKeyErrUnavailable}),
			wantAuth:    false,
			wantUnavail: true,
		},
		{
			name:        "unrelated error is neither",
			err:         errors.New("connection reset"),
			wantAuth:    false,
			wantUnavail: false,
		},
		{
			name:        "nil is neither",
			err:         nil,
			wantAuth:    false,
			wantUnavail: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAuthError(tc.err); got != tc.wantAuth {
				t.Errorf("isAuthError() = %v, want %v", got, tc.wantAuth)
			}
			if got := isTrackUnavailable(tc.err); got != tc.wantUnavail {
				t.Errorf("isTrackUnavailable() = %v, want %v", got, tc.wantUnavail)
			}
		})
	}
}
