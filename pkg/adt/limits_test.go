package adt

import "testing"

func TestResolveRowLimit(t *testing.T) {
	tests := []struct {
		name           string
		wantsUnlimited bool
		requested      int
		want           int
	}{
		{
			name:           "wantsUnlimited means unlimited regardless of requested",
			wantsUnlimited: true,
			requested:      -1,
			want:           UnlimitedRows,
		},
		{
			name:           "unset defaults to 100",
			wantsUnlimited: false,
			requested:      0,
			want:           100,
		},
		{
			name:           "explicit zero without wantsUnlimited also defaults to 100 (MCP: 0 is indistinguishable from omitted)",
			wantsUnlimited: false,
			requested:      0,
			want:           100,
		},
		{
			name:           "positive request is honored",
			wantsUnlimited: false,
			requested:      5000,
			want:           5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRowLimit(tt.wantsUnlimited, tt.requested)
			if got != tt.want {
				t.Errorf("ResolveRowLimit(%v, %d) = %d, want %d", tt.wantsUnlimited, tt.requested, got, tt.want)
			}
		})
	}
}
