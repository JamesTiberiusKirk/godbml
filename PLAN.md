# godbml — Plan

A local, native, open-source dbdiagram.io equivalent. Watches a `.dbml` file
and hot-reloads on change. Persists view state in a sidecar JSON next to the
`.dbml` so it can be git-versioned.

WASM is a future target — design choices stay pure Go (no cgo) so it stays open.

## Scope

- **Viewer only.** No DBML editor inside the app. The user edits `.dbml` in
  their own editor; the viewer reflects on save.
- **Native first**, WASM later.
- **Sidecar metadata** (`<name>.dbml.meta.json`) next to the source file.

## Locked decisions

1. **Stack:** Ebitengine. Pure Go, no cgo.
2. **Metadata storage:** sidecar JSON next to the `.dbml`.
3. **Metadata categories:** table positions, groupings, annotations, per-table
   styling, per-relationship styling, multiple views. Camera state is **not**
   persisted.
4. **Views:** full per-view layout. Each view owns its own positions, hidden
   set, annotations, styling. Same table can appear in different positions in
   different views.
5. **Groupings:** viewer-owned, per-view. DBML's native `TableGroup` seeds
   groupings on view creation only — no further sync after that.
6. **Parser:** `github.com/duythinht/dbml-go`. Verify coverage on a real `.dbml`
   before committing; fork or fall back to writing our own only if it has
   nontrivial gaps.
7. **Schema drift:** new tables auto-add visible to every view with edge
   placement. Removed tables are pruned. Renames are not detected — treated as
   remove + add. Column drops nuke any relationship using them.
8. **Layout:** force-directed (Fruchterman–Reingold) on first open of a file
   with no metadata. New tables edge-placed (optionally with a short FR pass
   to settle them near their FK neighbours). Re-layout button is roadmap.
9. **Edge routing:** orthogonal Z-routing, **no** obstacle avoidance. Lines
   may clip through unrelated tables on dense diagrams; force-directed layout
   makes this rare. Obstacle avoidance is roadmap.
10. **Annotations:** in-app, single-line text. Click-to-edit, Enter commits,
    Escape cancels.
11. **Colour selector:** fixed palette of 12 swatches (see Theme). Applied to
    annotations, tables, groups, and relationships.

## Theme

Minimal dark, neon accents. Default state is muted; neon appears only on user
intent (paint, select, hover).

- **Background:** `#0d0d10`
- **Surface (tables, annotations):** `#16161c` with `#222` border
- **Text:** `#e8e8ec` primary, `#7a7a85` muted
- **Default FK line:** `#3a3a44` (muted; neon on hover/selection)

**Neon palette** (also the colour-picker swatches):

| Name    | Hex       |
| ------- | --------- |
| cyan    | `#00f0ff` |
| magenta | `#ff2bd6` |
| green   | `#39ff14` |
| yellow  | `#f0ff00` |
| orange  | `#ff7a00` |
| red     | `#ff3860` |
| purple  | `#b14dff` |
| pink    | `#ff66c4` |
| sky     | `#4dc6ff` |
| lime    | `#b3ff66` |
| white   | `#ffffff` |
| grey    | `#808a99` |

Exact values are tunable in phase 1 once we see them on screen.

## Sidecar JSON shape

Filename: `<name>.dbml.meta.json` (e.g. `schema.dbml` → `schema.dbml.meta.json`).

```json
{
  "version": 1,
  "views": [
    {
      "id": "uuid",
      "name": "default",
      "tables": {
        "users": { "x": 100, "y": 200, "hidden": false, "color": "#00f0ff" }
      },
      "groups": [
        { "id": "uuid", "name": "auth", "tables": ["users", "sessions"], "color": "#b14dff" }
      ],
      "annotations": [
        { "id": "uuid", "x": 0, "y": 0, "w": 200, "h": 60, "text": "...", "color": "#f0ff00" }
      ],
      "relationships": {
        "users.id<-sessions.user_id": { "hidden": false, "color": "#ff2bd6" }
      }
    }
  ]
}
```

Top-level `version` lets us migrate the format later without breaking files.

## Package layout

```
cmd/godbml/main.go        entry, flag parsing, file open
internal/dbml/            parse + internal schema model
internal/meta/            sidecar JSON: schema, load/save, version, drift
internal/layout/          Fruchterman–Reingold
internal/routing/         orthogonal Z-routing
internal/watch/           fsnotify wrapper
internal/ui/              ebitengine app, camera, input
internal/ui/render/       table / relationship / annotation / group draw
internal/ui/widgets/      text input, colour palette, context menu
internal/ui/views.go      view switcher / state
```

## Phases

Each phase ends with something runnable.

- **Phase 0 — spike + verify.** Verify `dbml-go` against a representative
  `.dbml`. Hello-Ebitengine window with pan/zoom camera + dark background +
  faint grid.
- **Phase 1 + 2 — read-only viewer + persistence.** Likely one combined pass
  because rendering positions you can't save isn't useful. DBML parse,
  Fruchterman–Reingold bootstrap, render tables, render FKs as straight lines
  for now, drag-to-move, sidecar load/save, schema drift reconciliation, file
  watcher for both `.dbml` and `.meta.json`.
- **Phase 3 — orthogonal routing + groups.** Z-routing replaces straight lines.
  Render groupings as coloured regions. Create / edit / delete groups in UI.
- **Phase 4 — annotations + colour.** Single-line text input widget. Add /
  move / resize / delete annotations. Palette widget; apply colour to tables /
  groups / relationships / annotations.
- **Phase 5 — multiple views.** View switcher; create / rename / delete.
  Per-view state isolation.

## Roadmap (deferred from v1)

- WASM build pipeline
- Obstacle-avoiding orthogonal edge routing
- Hierarchical (Sugiyama / dagre) layout option
- Re-layout view button (cheap once force-directed is in)
- Multi-line annotations
- Markdown rendering inside annotations
- Annotation pinning to tables / relationships
- Full HSV / hex colour picker
- Rename detection across DBML edits
- Camera state persistence (only if it turns out to matter)
