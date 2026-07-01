# Bundle Stack Metadata Generation — Design Spec

**Date:** 2026-06-30
**Status:** Approved

## Problem

When a bundle is instantiated via `generate`, the created stack file (`stack.tm.hcl`) only contains `stack_id`. All other metadata defined in the bundle's stack definition (`name`, `description`, `tags`, `before`, `after`, `wants`, `wanted_by`, `watch`) are set in memory but never persisted to disk.

Additionally, when the stack file already exists (e.g., after manual edits), the bundle metadata is ignored entirely on subsequent `generate` runs.

## Goal

1. On new stack creation from a bundle: write all metadata fields to `stack.tm.hcl`.
2. On `generate` with an existing stack: overwrite bundle-managed fields in the stack file. **The bundle always wins.**

## Architecture

### Change 1 — `generate/generate_bundle.go` : `generateBundleStack()`

**Fix creation:** Populate `stackCfg` (type `config.Stack`) with all fields from `stackMeta` before calling `stack.Create()`. Remove the dead in-memory assignments that follow (they never wrote to disk).

```go
stackCfg := config.Stack{
    Dir:         stackMeta.Dir,
    ID:          uuid.NewString(),
    Name:        stackMeta.Name,
    Description: stackMeta.Description,
    Tags:        stackMeta.Tags,
    After:       stackMeta.After,
    Before:      stackMeta.Before,
    Wants:       stackMeta.Wants,
    WantedBy:    stackMeta.WantedBy,
    Watch:       watchPaths, // []string → project.Paths conversion
}
```

**Fix existing stack:** Instead of returning early when the stack exists, call `stack.UpdateMetadata(root, stackMeta)` and report the changed file.

```go
if ok && stackTree.IsStack() {
    if err := stack.UpdateMetadata(g.root, stackMeta); err != nil {
        report.AddFailure(stackMeta.Dir, err)
        return
    }
    dirReport := genreport.Dir{}
    dirReport.AddChangedFile(stack.DefaultFilename)
    report.AddDirReport(stackMeta.Dir, dirReport)
    stackTree.Node.Components = mergeComponentList(...)
    return
}
```

### Change 2 — `stack/update.go` (new file): `UpdateMetadata()`

New function that surgically updates a `stack.tm.hcl` file using `hclwrite`:

- **Reads** the existing `stack.tm.hcl` file
- **Finds** the `stack {}` block
- **Updates** bundle-managed attributes:
  - `name`, `description` — set if non-empty, remove if empty
  - `tags`, `before`, `after`, `wants`, `wanted_by`, `watch` — set if non-empty list, remove if empty list
- **Never touches** `id` — preserved as-is
- **Preserves** all other user-defined attributes and blocks
- **Writes back** atomically (temp file + rename to avoid partial writes)

### Type conversion note

`StackMetadata.Watch` is `[]string`; `config.Stack.Watch` is `project.Paths`. Conversion: iterate and call `project.NewPath(s)` for each element.

## Data Flow

```
generate
  └── generateBundleStack()
        ├── stack does NOT exist
        │     └── stack.Create(root, stackCfg{all fields}) → report "created"
        └── stack EXISTS
              └── stack.UpdateMetadata(root, stackMeta)   → report "changed"
                    └── (also merges components)
```

## Error Handling

- Creation failure: `report.AddFailure(stackMeta.Dir, err)` — unchanged.
- Update failure: `report.AddFailure(stackMeta.Dir, err)` — same pattern.

## What Is NOT Changed

- Stack ID generation (always preserved on update)
- Components merge logic (`mergeComponentList`) — unchanged
- Report structure — `AddChangedFile` already exists in `genreport.Dir`
- Any user content in `stack.tm.hcl` outside the `stack {}` block (imports, other blocks)
