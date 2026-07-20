// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"testing"

	"github.com/terramate-io/terramate/config"
)

// TestBuildAllPromoteBundlesCombinesEnvAndTextFilter proves that the target-env
// filter (the "e" key cycle, exposed via m.promoteEnvFilter)
// and the free-text filter genuinely combine via AND in buildAllPromoteBundles.
//
// Two target envs (prod and qa) both promote from staging, and neither has any
// existing bundles yet, so without the env filter both would be equally
// eligible destinations for every staging bundle — making the env-filter
// dimension meaningfully discriminating. Similarly, both vpc-1 and ecs-1 are
// eligible for promotion into the filtered target env, so without the text
// filter both would show up. Only the intersection of "eligible for the
// filtered target env (prod)" AND "matches the text query (vpc)" should
// remain: (vpc-1, prod).
func TestBuildAllPromoteBundlesCombinesEnvAndTextFilter(t *testing.T) {
	t.Parallel()
	staging := &config.Environment{ID: "staging", Name: "Staging"}
	prod := &config.Environment{ID: "prod", Name: "Production", PromoteFrom: "staging"}
	qa := &config.Environment{ID: "qa", Name: "QA", PromoteFrom: "staging"}
	bundles := []*config.Bundle{
		{DefinitionMetadata: config.Metadata{Name: "vpc"}, Alias: "vpc-1", Environment: staging},
		{DefinitionMetadata: config.Metadata{Name: "ecs"}, Alias: "ecs-1", Environment: staging},
	}

	m := Model{
		EngineState: &EngineState{Registry: &config.Registry{
			Bundles:      bundles,
			Environments: []*config.Environment{staging, prod, qa},
		}},
		promoteEnvFilter: envFilterCycle{filters: []envFilterState{{env: prod, label: "Production", shortID: "prod"}}, pos: 0},
		promoteFilter:    newTextFilter(),
	}
	m.promoteFilter.input.SetValue("vpc")

	got, targetEnvs := m.buildAllPromoteBundles()
	if len(got) != 1 || got[0].Alias != "vpc-1" {
		t.Fatalf("expected the env+text filter to narrow to [vpc-1], got %v", got)
	}
	if len(targetEnvs) != 1 || targetEnvs[0].ID != "prod" {
		t.Fatalf("expected the target environment to be prod, got %v", targetEnvs)
	}
}
