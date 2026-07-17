# Refactorisation du code upstream — Design (chantier 2)

- **Date** : 2026-07-17
- **Statut** : Approved
- **Périmètre** : code hérité d'upstream (`ui/tui/`, `hcl/`, `engine/`, `generate/`) + code fork listé au backlog (`hcl/block_define_parser.go`)
- **Prérequis** : chantier 1 terminé — voir [2026-07-17-refactoring-maintenabilite-design.md](2026-07-17-refactoring-maintenabilite-design.md)

## Contexte

Le chantier 1 couvre le code spécifique au fork (TUI, bundles, docs). Ce chantier 2 traite la dette du code hérité d'upstream, consignée dans `docs/BACKLOG.md` (créé en Phase 0 du chantier 1).

Le risque de conflit de merge amont — raison initiale pour ne pas toucher ce code — est jugé acceptable : depuis le retrait de Terramate Cloud, les merges depuis `terramate-io/terramate` seront quasi inexistants. Le fork assume la propriété de ce code.

Atout majeur : le cœur upstream est massivement testé (~85k lignes de tests, dont 13k sur `hcl/` et 20k sur `generate/`), ce qui sécurise les refactorings iso-comportement.

## Objectifs

- Éliminer les points de friction structurels du code upstream : dispatch CLI géant, god file parser, signatures incohérentes, couplages.
- Refactoring **iso-comportement** strict : aucune fonctionnalité ajoutée ni modifiée.

## Non-objectifs

- Réécrire des sous-systèmes entiers (le scheduler, le moteur d'évaluation HCL forké…).
- Suivre la structure upstream pour faciliter des merges : le fork assume sa divergence.

## Principes directeurs

Identiques au chantier 1 : phases livrables indépendamment (build + tests + lint verts), risque croissant, garde-fous (`go test -race` sur les packages touchés à chaque étape, `make test` + `make lint/all` en fin de phase).

## Phase U1 — Points d'entrée déjà remaniés par le fork (risque faible)

1. **Registre de commandes CLI** : remplacer `SelectCommand` (`ui/tui/cli_handler.go:75`, 438 lignes, 36 `case`) par une table de dispatch — chaque commande déclare son nom et un constructeur `func(...) commands.Executor` ; le handler se réduit à une résolution dans la table. Ajout d'une commande = un enregistrement, plus de switch à étendre (OCP). Ce fichier est déjà massivement modifié par le fork (226 lignes changées au retrait du Cloud), le refactor consolide cette divergence.
2. **Décomposition du parser `define`** (`hcl/block_define_parser.go`, 1 488 lignes ; `Parse` 266 l., `parseDefineBundleBlock` 163 l., `parseDefineBundleStackBlock` 100 l.) : un sous-parser par type de bloc imbriqué, chacun avec sa fonction courte et nommée. Code fork (2026), zéro risque de conflit — premier candidat naturel.

## Phase U2 — Unification des parsers de blocs (risque moyen)

1. **Interface `BlockParser` unique** dans `hcl/` : aujourd'hui 16 fichiers `block_*_parser.go` exposent `Parse` sous 3 signatures différentes (`(p, *ast.Block)`, `(p, *ast.MergedBlock)`, `(p, label, *ast.MergedBlock)`) avec 4 types de handlers parallèles (`unmergedBlockHandlers`, `mergedBlockHandlers`, `mergedLabelsBlockHandlers`, `uniqueBlockHandlers`). Cible : une interface commune (avec adaptateurs pour les variantes merged/labels), un seul mécanisme d'enregistrement.
   - Ordre : d'abord les blocs fork (`define`, `bundle`, `component`), puis les blocs upstream (`script`, `generate_hcl`, `generate_file`, `globals`…).
2. **Factoriser le pattern `lets`** : `NewCustomRawConfig(map[string]dupeHandler{...})` dupliqué dans `block_script_parser.go`, `block_generate_hcl_parser.go`, `block_generate_file_parser.go`.

## Phase U3 — Décomposition de `hcl/hcl.go` (2 210 lignes)

Extractions incrémentales par domaine, une par commit, chacune validée par les 13k lignes de tests du package :

1. Gestion des imports (`handleImport`, 103 l.) → `imports.go`.
2. Parsing de la config git (`parseGitConfig`, 93 l.) → `gitconfig.go`.
3. Utilitaires glob → fichier dédié.
4. Logique de merge de blocs → fichier dédié.
5. `hcl.go` ne conserve que l'orchestration du parsing et l'enregistrement des parsers (s'appuie sur l'interface unifiée de U2).

## Phase U4 — Architecture engine/generate (risque moyen-élevé)

1. **Inverser le couplage bidirectionnel `engine` ↔ `generate`** : `generate/generate.go` importe `engine` tandis que `engine/bundles.go` importe `generate/resolve`. Cible : `generate` définit une interface pour ce dont il a besoin de l'orchestrateur ; `engine` l'implémente. Le domaine génération redevient réutilisable hors engine.
2. **Cantonner `di/` au bootstrap** : le conteneur (service locator par clés string, résolution au runtime) reste dans `cmd/`/`ui/tui` pour le câblage initial ; à l'intérieur d'`engine`, `generate` et `commands`, les dépendances passent par les constructeurs et redeviennent visibles dans les signatures (testabilité, lisibilité du graphe).
3. **Découpage des fonctions longues** : `ApplyDependencyFilters` (`engine/engine.go:353`, 192 l.), `applyOutputSharingFilters` (97 l.), `generateRootFiles` (`generate/generate.go:1120`, 136 l.), `stackGenerate` (127 l.), `loadStackCodeCfgs` (110 l.) — extractions locales de sous-étapes nommées, sans changer les API publiques.

## Phase U5 — Hygiène

1. Trier les `t.Skip` des e2e (`e2etests/core/{exp_trigger,general,list_git}_test.go`) : distinguer skips d'environnement légitimes des tests désactivés à réactiver ou supprimer.
2. Retirer l'émission des await keys « plates » (`config/hcl_func.go:60-72`) si l'inventaire des consommateurs confirme que seules les clés scopées par classe sont utilisées.

## Stratégie de vérification

- Une extraction/un refactor = un commit, validé par `go test -race` sur le package touché.
- Fin de phase : `make test` complet + `make lint/all`.
- U1 (registre CLI) : vérifier l'iso-comportement du parsing des flags et de l'aide (`terramate --help`, sous-commandes) par comparaison de sortie avant/après.
- U4 : `go vet` + contrôle de l'absence de cycle d'imports (`go list -deps`).

## Risques et mitigations

| Risque | Mitigation |
|---|---|
| Régression dans le cœur (hcl, generate) | Couverture de tests massive existante ; extractions petites et incrémentales, un commit par extraction |
| Conflits si merge upstream malgré tout | Assumé : le fork diverge ; le backlog documente ce qui a été restructuré |
| U2 dérive en refonte du parsing | Interface unifiée = adaptateurs autour de l'existant, pas de réécriture des parsers |
| Retrait des await keys plates casse un consommateur externe | Inventaire exhaustif des usages avant retrait ; sinon on les conserve |

## Critères de succès

- Ajout d'une commande CLI sans modifier de switch central.
- `hcl/hcl.go` < 800 lignes, découpé par domaine.
- Une seule signature/mécanisme d'enregistrement pour les parsers de blocs.
- Plus d'import `engine` dans `generate/` (couplage unidirectionnel).
- `di.Resolve` absent d'`engine/`, `generate/` et `commands/` (hors bootstrap).
- `make test` et `make lint/all` verts à chaque fin de phase.
