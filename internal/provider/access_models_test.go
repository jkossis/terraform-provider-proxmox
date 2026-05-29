// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"testing"
)

func TestProxmoxBackupServerBoolUnmarshal(t *testing.T) {
	tests := map[string]bool{
		`true`:  true,
		`false`: false,
		`1`:     true,
		`0`:     false,
		`"1"`:   true,
		`"0"`:   false,
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			var got proxmoxBackupServerBool
			if err := json.Unmarshal([]byte(input), &got); err != nil {
				t.Fatalf("Unmarshal returned error: %s", err)
			}
			if bool(got) != want {
				t.Fatalf("unexpected value: got %t, want %t", got, want)
			}
		})
	}
}
