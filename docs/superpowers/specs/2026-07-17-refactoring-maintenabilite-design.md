# Refactorisation & maintenabilité du fork — Design

- **Date** : 2026-07-17
- **Statut** : Approved
- **Périmètre** : code spécifique au fork (`commands/ui/`, code bundle, résidus cloud) + documentation (README.md, CLAUDE.md, AGENTS.md)

## Contexte

Ce repo est un fork de `terramate-io/terramate` (~62k lignes de Go de production, ~85k de tests), 6 commits d'avance sur upstream. Le fork a : supprimé Terramate Cloud, ajouté une TUI Bubble Tea (`terramate ui`, ~10k lignes dans `commands/ui/`), et étendu la fonctionnalité bundles (génération de métadonnées de stacks, scoping des await keys).

Un état des lieux complet (4 audits parallèles : TUI, code bundle, structure globale, documentation) a identifié :

1. **TUI** : ~500-700 lignes de copié-collé entre les 3 vues de sélection, un god object `Model` (~48 champs, 6 responsabilités), de la logique métier (évaluation HCL, chargement config) dans les handlers clavier, et ~7 000 lignes sans aucun test (`inputs_form.go`, `change.go`, tous les widgets).
2. **Code bundle hors UI** : sain et bien testé ; factorisations mineures possibles ; couverture insuffisante sur `engine/buffer_group.go`.
3. **Résidus cloud** : package `http/` entièrement orphelin (0 importeur), `.tmtriggers/cloud/`, `.goreleaser.yaml.bak`, divers commentaires.
4. **Documentation** : README verbatim de l'upstream (instructions d'installation qui installent l'amont), AGENTS.md factuellement faux (asdf au lieu de mise, Go 1.24 au lieu de 1.25.8), CLAUDE.md ignorant tout le code du fork ; TUI et bundles documentés nulle part.

## Objectifs

- Factoriser le code du fork selon SOLID / KISS / Clean Architecture, en refactoring **iso-comportement** (aucune fonctionnalité ajoutée ni modifiée).
- Remettre la documentation (README.md, CLAUDE.md, AGENTS.md) en accord avec l'existant.
- Consigner la dette technique du code upstream dans `docs/BACKLOG.md` sans y toucher.

## Non-objectifs

- Refactoriser le code upstream (`hcl/hcl.go`, `engine/`, `generate/`, `stack/manager.go`, `ui/tui/cli_handler.go`…) : consigné au backlog, traité plus tard de façon incrémentale.
- Ajouter des fonctionnalités à la TUI ou aux bundles.
- Documenter le produit pour des utilisateurs externes au-delà du README (pas de site de docs).

## Principes directeurs

- **Phases livrables indépendamment** : chaque phase se termine build vert + tests verts + lint vert. On peut s'arrêter entre deux phases sans laisser le repo en chantier.
- **Risque croissant** : on commence par ce qui est sans risque (docs, code mort), on finit par ce qui est risqué (restructuration de code non testé), préparé par des tests de caractérisation.
- **Garde-fous** : `make build` + `go test -race` sur les packages touchés après chaque étape ; `make test` complet + `make lint/all` en fin de phase.

## Phase 0 — Documentation + hygiène (risque ~nul)

### Documentation

1. **README.md** — réécriture pour le fork :
   - Bandeau « fork de terramate-io/terramate » avec les différences : − Terramate Cloud, + TUI `terramate ui`, + bundles/components/packages/scaffold.
   - Installation : build local uniquement (`mise run build` ou `make build`, prérequis mise + Go 1.25.x). Suppression des instructions `brew install` / `go install github.com/terramate-io/...` qui installent l'amont.
   - Badges corrigés (ou retirés s'ils n'ont pas d'équivalent pour le fork).
   - Sections fonctionnalités : noyau conservé (orchestration, change detection, code generation — liens docs amont valables pour le noyau OSS uniquement) + nouveautés du fork avec exemples de commandes.
2. **CLAUDE.md** — mise à niveau :
   - Architecture complétée : `ui/tui/` (bootstrap CLI kong), `commands/ui/` (TUI Bubble Tea), `ls/` (implémentation LSP), `typeschema/`, `scaffold/`, écosystème bundles (`config/bundle.go`, `generate/generate_bundle.go`, `hcl/block_define_parser.go`, `commands/{package,component}/`).
   - Section « Spécificités du fork » : Cloud supprimé, TUI, bundles.
   - mise documenté comme task runner aux côtés de make.
3. **AGENTS.md** — corrections factuelles : mise (pas asdf), Go 1.25.8, sortie de build réelle (`bin/terramate`, `bin/terramate-ls` ; `helper` via `make test/helper`), structure du repo alignée sur CLAUDE.md.
4. **`docs/BACKLOG.md`** — création à partir du backlog upstream (annexe A de ce document).

### Hygiène (code mort)

5. Suppression du package `http/` (0 importeur dans tout le repo, résidu cloud avec `TMC_API_DEBUG`).
6. Suppression de `.tmtriggers/cloud/` et `.goreleaser.yaml.bak`.
7. Correction du commentaire « TMC user UUID » dans `ui/tui/telemetry/telemetry.go`.
8. Correction de `Spec.Name()` dans `commands/ui/ui.go` : retourne `"prompt"` alors que la commande CLI est `ui`.

## Phase 1 — Factorisations sûres (code testé ou changements mécaniques)

### TUI

1. **`filterController`** : extraire en un composant unique le triplet dupliqué des 3 vues — type `xFilterState`, `newXFilterState`, `applyXFilter`, `currentXFilter`, `nextXFilterName`, `buildXFilters`, `XBundleMatchesFilter` (fonctions quasi/byte-identiques entre `view_promote.go`, `view_reconfig.go`, `view_create_select.go`). Gain estimé : ~150-200 lignes. Les tests existants du filtre valident l'iso-comportement.
2. **Helpers de pagination et de liste** : fusionner les fonctions byte-identiques (`promotePageCursor`/`reconfigPageCursor`/`flatBundlePageCursor`, `XCursorForItem`) et regrouper les helpers déjà partagés mais mal rangés (`scrollWindowVar`, `renderScrollbar`, `renderDetailBox`, `renderErrorBox`, `firstSelectableIndex`/`lastSelectableIndex`, `truncateEnd/Start`, `groupBundles`) dans un fichier dédié `list_component.go` (au lieu d'être hébergés arbitrairement dans `view_create_select.go` et `view_reconfig.go`).
3. **`baseWidget`** : struct embarqué portant les implémentations par défaut (`WidgetContext()`, `ForwardMsg` no-op, `AcceptSubFormResult` → true) pour supprimer ~100 lignes de boilerplate répété sur les 13 widgets.

### Hors UI

4. `generate/generate_bundle.go` : découper `generateBundleStack` (~90 lignes) en `updateExistingBundleStack` / `createBundleStack` ; factoriser le triple `stackTree.Node.Components = mergeComponentList(...)`.
5. `config/hcl_func.go` : factoriser le branchement dupliqué `useAwait ? classKey : plainKey` (répété pour UUID et alias) en un helper.
6. `engine/buffer_group_test.go` : renforcer la couverture (lignes partielles réassemblées, EOF sans `\n` final, reader défaillant, erreurs d'écriture) — ce code est sur le chemin de tous les runs parallèles.

## Phase 2 — Composant `selectableListView` générique

Unifier les 3 vues de sélection (create-select, promote, reconfig) sur un composant liste unique, paramétré par :

- fournisseur d'items (flat bundles / bundles groupés par classe),
- champs de détail affichés et en-tête de liste,
- textes de breadcrumb et ligne d'aide,
- action déclenchée par Enter.

Le composant intègre paging, filtre (via `filterController` de la Phase 1), navigation clavier et rendu (scrollbar, groupes, détails). Élimine les ~500-700 lignes de copié-collé restantes ; toute évolution future (raccourci, filtre, rendu) se fait à un seul endroit.

Validation : tests existants des 3 vues (paging/filtre) + tests de rendu comparant la sortie avant/après sur des états représentatifs.

## Phase 3 — Clean Architecture de la TUI (risque le plus élevé, préparé par des tests)

Ordre impératif : les tests de caractérisation (étape 1) précèdent toute restructuration.

1. **Tests de caractérisation** sur `inputs_form.go` (1 990 l.) et `change.go` (666 l.), aujourd'hui 0 test : figer le comportement observable actuel (séquences de touches → état/rendu ; changements → YAML généré) avant de bouger quoi que ce soit.
2. **Extraction de la logique métier des vues** vers une couche application (`bundleLoader` / usecases) : sortir des handlers clavier les appels `config.Eval*`, `est.ResolveAPI`, chargements HCL (`loadBundleDef`, `loadReconfigBundle`, `loadPromoteBundle` — 30+ appels au total). Crée la frontière testable présentation ↔ domaine ; les vues ne reçoivent plus que des données prêtes à afficher.
3. **Décomposition du god object `Model`** : sous-états par vue (`overviewState`, `createState`, `reconfigState`, `promoteState`) + interface de vue commune (`Update`/`View`), supprimant les switch géants dupliqués de `model.go` (ajout d'une vue = un nouveau fichier, pas 3 switch à modifier — OCP).
4. **Découpage de `inputs_form.go`** en unités cohérentes : state-machine, rendu, gestion des sous-formulaires ; découpage des fonctions > 150 lignes (`View`, `renderCompletedPanelContent`, `openSubForm`, `updateCompleted`).

## Stratégie de vérification

- Après chaque étape : `make build` + `go test -race ./commands/ui/...` (+ packages touchés hors UI).
- Fin de chaque phase : `make test` complet (nécessite `make build` préalable) + `make lint/all`.
- Phase 2 et 3 : comparaison de comportement avant/après (tests de caractérisation, golden rendering sur états représentatifs).
- Vérification manuelle de la TUI (`terramate ui`) sur un sandbox de test en fin de Phases 2 et 3.

## Risques et mitigations

| Risque | Mitigation |
|---|---|
| Régression TUI sur code non testé (Phases 2-3) | Tests de caractérisation AVANT restructuration ; phases livrables séparément |
| Conflits de merge upstream futurs | Périmètre limité au code fork ; le code upstream n'est pas touché |
| Refactoring qui dérive en réécriture | Iso-comportement strict ; toute envie de « corriger » un comportement passe par un ticket séparé |
| `make test` long / dépendances d'environnement | Tests ciblés par package à chaque étape, suite complète seulement en fin de phase |

## Critères de succès

- Duplication des 3 vues éliminée (fonctions byte-identiques : 0).
- Aucune évaluation HCL/config dans les handlers d'input des vues.
- `Model` sans champs spécifiques à une vue ; ajout d'une vue sans modifier les switch centraux.
- `inputs_form.go` et `change.go` couverts par des tests.
- README/CLAUDE.md/AGENTS.md exacts et cohérents entre eux (vérifiables : commandes documentées exécutables telles quelles).
- `make test` et `make lint/all` verts à chaque fin de phase.

---

## Annexe A — Backlog dette technique upstream (à matérialiser dans `docs/BACKLOG.md`)

Dette du code hérité d'upstream, **hors périmètre de ce chantier**, classée par priorité. « Upstream » = un refactor créerait des conflits si un merge amont avait lieu (jugé peu probable depuis le retrait du Cloud).

| Prio | Dette | Localisation | Effort | Upstream ? |
|---|---|---|---|---|
| P1 | Dispatch CLI géant : `SelectCommand`, 438 lignes, 36 `case` — remplacer par une table/registre de commandes | `ui/tui/cli_handler.go:75` | M | Partiel (déjà très modifié par le fork) |
| P1 | God file parser : 2 210 lignes mêlant parsing, merge, git config, imports, glob | `hcl/hcl.go` | L | Oui — extractions incrémentales uniquement |
| P2 | 3 signatures `Parse` différentes sur 16 parsers `block_*_parser.go` ; 4 types de handlers parallèles — unifier d'abord les blocs fork (`define`/`bundle`/`component`) | `hcl/block_*.go` | M | Partiel |
| P2 | `di/` utilisé en service locator transversal : dépendances masquées, erreurs repoussées au runtime — cantonner au bootstrap, injection par constructeur ailleurs | `engine`, `generate`, `ui/tui`, `commands` | M/L | Non |
| P2 | Couplage bidirectionnel `engine` ↔ `generate` (`engine/bundles.go` importe `generate/resolve`, `generate` importe `engine`) — inverser via interface côté `generate` | `engine/bundles.go`, `generate/generate.go` | M | Oui |
| P2 | Fonctions longues : `ApplyDependencyFilters` (192 l.), `generateRootFiles` (136 l.), `stackGenerate` (127 l.), `loadStackCodeCfgs` (110 l.) | `engine/engine.go:353`, `generate/generate.go` | M | Oui |
| P2 | Parser `define` : `Parse` 266 l. + helpers 100-163 l. (code fork, zéro risque de conflit — bon premier candidat après ce chantier) | `hcl/block_define_parser.go:224`, `config/bundle.go` | M | Non |
| P3 | Tests e2e avec `t.Skip` à trier (légitimes vs à réactiver/supprimer) | `e2etests/core/{exp_trigger,general,list_git}_test.go` | S | Partiel |
| P3 | Await keys « plates » émises en plus des clés scopées par classe : retirer si plus aucun consommateur | `config/hcl_func.go:60-72` | S | Non |
