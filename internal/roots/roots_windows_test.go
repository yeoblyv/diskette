//go:build windows

package roots

import (
	"reflect"
	"testing"
)

func TestDrivesFromMask(t *testing.T) {
	tests := []struct {
		name string
		mask uint32
		want []string
	}{
		{"none", 0, nil},
		{"C only", 1 << 2, []string{`C:\`}},
		{"A, C, Z", 1<<0 | 1<<2 | 1<<25, []string{`A:\`, `C:\`, `Z:\`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := drivesFromMask(tc.mask)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("drivesFromMask(%b) = %v, want %v", tc.mask, got, tc.want)
			}
		})
	}
}
