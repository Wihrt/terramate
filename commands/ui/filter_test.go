// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"testing"

	"github.com/terramate-io/terramate/config"
)

func TestBundleMatchesFilter(t *testing.T) {
	t.Parallel()
	b := &config.Bundle{
		DefinitionMetadata: config.Metadata{Name: "VPC-Network"},
		Alias:              "prod-vpc-1",
		Name:               "vpc",
	}

	testcases := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "empty query matches everything", query: "", want: true},
		{name: "matches definition name", query: "network", want: true},
		{name: "matches instance alias case-insensitively", query: "PROD-VPC", want: true},
		{name: "no match", query: "ecs", want: false},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := bundleMatchesFilter(b, tc.query); got != tc.want {
				t.Fatalf("bundleMatchesFilter(query=%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}
