// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"testing"
)

func TestBundleAwaitKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		classScoped bool
		isUUID      bool
		want        string
	}{
		{name: "plain alias", classScoped: false, isUUID: false, want: "env1:web"},
		{name: "class-scoped alias", classScoped: true, isUUID: false, want: "env1:vpc:web"},
		{name: "plain uuid", classScoped: false, isUUID: true, want: "env1:web"},
		{name: "class-scoped uuid", classScoped: true, isUUID: true, want: "env1:vpc:web"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := bundleAwaitKey(tc.classScoped, tc.isUUID, "vpc", "web", "env1")
			if got != tc.want {
				t.Fatalf("bundleAwaitKey(%v, %v) = %q, want %q", tc.classScoped, tc.isUUID, got, tc.want)
			}
		})
	}
}
