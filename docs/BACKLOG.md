# Backlog — dette technique

État des lieux réalisé le 2026-07-17 (4 audits : TUI, code bundle, structure globale,
documentation). Deux chantiers de refactorisation sont spécifiés :

- **Chantier 1 — code fork** : [spec](superpowers/specs/2026-07-17-refactoring-maintenabilite-design.md)
  (docs, hygiène, factorisation TUI, Clean Architecture TUI).
- **Chantier 2 — code upstream** : [spec](superpowers/specs/2026-07-17-refactoring-upstream-design.md)
  (registre CLI, unification des parsers, décomposition `hcl/hcl.go`, découplage
  engine/generate). Exécuté après le chantier 1.

## Items (par priorité)

| Prio | Dette | Localisation | Effort | Couvert par |
|---|---|---|---|---|
| P1 | Duplication des 3 vues TUI (~500-700 lignes), god object `Model`, logique métier dans les vues, ~7k lignes sans test | `commands/ui/` | L | Chantier 1, phases 1-3 |
| P1 | Dispatch CLI géant : `SelectCommand`, 438 lignes, 36 `case` | `ui/tui/cli_handler.go:75` | M | Chantier 2, U1 |
| P1 | God file parser : 2 210 lignes (parsing, merge, git config, imports, glob) | `hcl/hcl.go` | L | Chantier 2, U3 |
| P2 | 3 signatures `Parse` différentes sur 16 parsers `block_*_parser.go`, 4 types de handlers | `hcl/block_*.go` | M | Chantier 2, U2 |
| P2 | `di/` utilisé en service locator transversal (dépendances masquées) | `engine`, `generate`, `ui/tui`, `commands` | M/L | Chantier 2, U4 |
| P2 | Couplage bidirectionnel `engine` ↔ `generate` | `engine/bundles.go`, `generate/generate.go` | M | Chantier 2, U4 |
| P2 | Fonctions longues : `ApplyDependencyFilters` (192 l.), `generateRootFiles` (136 l.), `stackGenerate` (127 l.), `loadStackCodeCfgs` (110 l.) | `engine/engine.go:353`, `generate/generate.go` | M | Chantier 2, U4 |
| P2 | Parser `define` : `Parse` 266 l. + helpers 100-163 l. | `hcl/block_define_parser.go:224`, `config/bundle.go` | M | Chantier 2, U1 |
| P2 | Couverture insuffisante de `BufferGroup` (1 seul test, code sur le chemin de tous les runs parallèles) | `engine/buffer_group_test.go` | S | Chantier 1, phase 1 |
| P3 | Tests e2e avec `t.Skip` à trier | `e2etests/core/{exp_trigger,general,list_git}_test.go` | S | Chantier 2, U5 |
| P3 | Await keys « plates » émises en plus des clés scopées par classe | `config/hcl_func.go:60-72` | S | Chantier 2, U5 |

## Hors backlog (déjà traité)

- Résidus du retrait Cloud (package `http/` orphelin, `.tmtriggers/cloud/`,
  `.goreleaser.yaml.bak`, commentaire TMC, `Spec.Name()`) — traités en Phase 0 du
  chantier 1.
