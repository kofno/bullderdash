package explorer

import (
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestIsBenignCountError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: true},
		{name: "redis nil", err: redis.Nil, want: true},
		{name: "wrong type", err: errors.New("WRONGTYPE Operation against a key holding the wrong kind of value"), want: true},
		{name: "other redis error", err: errors.New("ERR something else"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBenignCountError(tc.err); got != tc.want {
				t.Fatalf("isBenignCountError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
