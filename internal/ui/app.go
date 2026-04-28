package ui

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/JamesTiberiusKirk/godbml/internal/dbml"
	"github.com/JamesTiberiusKirk/godbml/internal/dbmledit"
	"github.com/JamesTiberiusKirk/godbml/internal/layout"
	"github.com/JamesTiberiusKirk/godbml/internal/meta"
	"github.com/JamesTiberiusKirk/godbml/internal/routing"
	"github.com/JamesTiberiusKirk/godbml/internal/theme"
	"github.com/JamesTiberiusKirk/godbml/internal/ui/render"
	"github.com/JamesTiberiusKirk/godbml/internal/ui/widgets"
	"github.com/JamesTiberiusKirk/godbml/internal/watch"
)

const gridSpacing = 100.0

type App struct {
	camera *Camera
	width  int
	height int

	dbmlPath string
	metaPath string

	schema *dbml.Schema
	doc    *meta.Document

	watcher *watch.Watcher

	middlePanning bool
	lastMidPanX   int
	lastMidPanY   int

	// Left-drag on empty canvas opens a rubber-band selection box. Until the
	// cursor moves past selectMoveThreshold, the press is treated as a plain
	// click that preserves the existing selection.
	maybeSelecting bool
	selecting      bool
	selectStartWX  float64
	selectStartWY  float64
	selectStartMX  int
	selectStartMY  int
	selectCurMX    int
	selectCurMY    int

	dragging                bool
	draggedTable            string
	dragStartCursor         [2]float64
	dragStartTablePositions map[string][2]float64
	dragStartAnnoPositions  map[string][2]float64

	hoveredTable   string
	hoveredGroup   string
	selectedTables map[string]bool

	dirty bool

	activeViewID string

	menu            *widgets.Menu
	palette         *widgets.Palette
	paletteCallback func(c color.NRGBA, clear bool)

	selectedAnnos    map[string]bool
	draggingAnno     string
	draggingAnnoMode int
	dragStartAnno    [4]float64

	editingAnno  string
	editBuffer   string
	caretBlinkOn bool
	caretFrame   int

	renamingView string
	renameBuffer string

	renamingGroup     string
	renameGroupBuffer string

	lastClickedGroupID    string
	lastClickedGroupFrame int

	cellEdit cellEdit

	lastLeftClickFrame int
	lastClickHit       tableHit

	frameCount int
}

type cellEditKind int

const (
	cellEditNone cellEditKind = iota
	cellEditTableName
	cellEditColumnName
	cellEditColumnType
)

type cellEdit struct {
	Kind   cellEditKind
	Table  string
	Column string
	Buffer string
}

func (e cellEdit) Active() bool { return e.Kind != cellEditNone }

type tableRegion int

const (
	regionNone tableRegion = iota
	regionHeader
	regionColumnName
	regionColumnType
)

type tableHit struct {
	Table  string
	Column string
	Region tableRegion
}

const (
	doubleClickFrames    = 24 // ~400ms at 60fps
	selectMoveThreshold  = 5  // pixels of movement before a press becomes a drag
)

func (a *App) selectOnlyTable(name string) {
	a.selectedTables = map[string]bool{name: true}
	a.selectedAnnos = nil
}

func (a *App) selectOnlyAnno(id string) {
	a.selectedAnnos = map[string]bool{id: true}
	a.selectedTables = nil
}

func (a *App) isTableSelected(name string) bool { return a.selectedTables[name] }
func (a *App) isAnnoSelected(id string) bool    { return a.selectedAnnos[id] }

// isTableHovered reports whether the cursor is "softly" over a given table —
// either directly hovering the table card, or hovering a group whose members
// include this table. Group-hover acts as a soft-selection so users can
// preview a group's connections at a glance.
func (a *App) isTableHovered(name string) bool {
	if a.hoveredTable == name {
		return true
	}
	if a.hoveredGroup == "" {
		return false
	}
	for _, g := range a.currentView().Groups {
		if g.ID != a.hoveredGroup {
			continue
		}
		for _, m := range g.Tables {
			if m == name {
				return true
			}
		}
		return false
	}
	return false
}

func (a *App) selectedTablesList() []string {
	out := make([]string, 0, len(a.selectedTables))
	for n := range a.selectedTables {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (a *App) selectedAnnosList() []string {
	out := make([]string, 0, len(a.selectedAnnos))
	for id := range a.selectedAnnos {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (a *App) selectionSize() int { return len(a.selectedTables) + len(a.selectedAnnos) }

// applySelectionColor paints every currently selected table and annotation
// with hex (or clears the colour when clear is true).
func (a *App) applySelectionColor(hex string, clear bool) {
	for _, name := range a.selectedTablesList() {
		if clear {
			a.setTableColor(name, "")
		} else {
			a.setTableColor(name, hex)
		}
	}
	for _, id := range a.selectedAnnosList() {
		if clear {
			a.setAnnotationColor(id, "")
		} else {
			a.setAnnotationColor(id, hex)
		}
	}
}

// removeSelectedTables tries to delete every selected table. Each removal is
// independent and may abort with a logged error if it has inline-ref
// dependents that aren't also being removed.
func (a *App) removeSelectedTables() {
	for _, name := range a.selectedTablesList() {
		a.removeTable(name)
	}
}

func (a *App) removeSelectedAnnotations() {
	for _, id := range a.selectedAnnosList() {
		a.deleteAnnotation(id)
	}
}

func (a *App) newGroupFromSelection() {
	view := a.currentView()
	members := a.selectedTablesList()
	if len(members) == 0 {
		return
	}
	view.Groups = append(view.Groups, &meta.Group{
		ID:     meta.NewID(),
		Name:   nextGroupName(view.Groups),
		Tables: members,
	})
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
}

func (a *App) addSelectionToGroup(groupID string) {
	view := a.currentView()
	for _, g := range view.Groups {
		if g.ID != groupID {
			continue
		}
		existing := map[string]bool{}
		for _, m := range g.Tables {
			existing[m] = true
		}
		for _, n := range a.selectedTablesList() {
			if !existing[n] {
				g.Tables = append(g.Tables, n)
				existing[n] = true
			}
		}
		a.dirty = true
		if err := a.persist(); err != nil {
			log.Printf("persist: %v", err)
		}
		return
	}
}

func (a *App) removeSelectionFromGroup(groupID string) {
	view := a.currentView()
	sel := a.selectedTables
	for _, g := range view.Groups {
		if g.ID != groupID {
			continue
		}
		out := g.Tables[:0]
		for _, m := range g.Tables {
			if !sel[m] {
				out = append(out, m)
			}
		}
		g.Tables = out
		a.dirty = true
		if err := a.persist(); err != nil {
			log.Printf("persist: %v", err)
		}
		return
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// applyBoxSelection finalises a rubber-band selection: every table whose AABB
// intersects the box becomes selected; every annotation whose AABB intersects
// becomes selected. The box is the axis-aligned rect spanned by the screen
// positions where the press started and ended.
func (a *App) applyBoxSelection() {
	x0, y0 := a.camera.ScreenToWorld(float64(a.selectStartMX), float64(a.selectStartMY))
	x1, y1 := a.camera.ScreenToWorld(float64(a.selectCurMX), float64(a.selectCurMY))
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}

	view := a.currentView()
	tables := map[string]bool{}
	for i := range a.schema.Tables {
		t := &a.schema.Tables[i]
		p, ok := view.Tables[t.Name]
		if !ok || p.Hidden || p.Orphaned {
			continue
		}
		box := render.MeasureTable(t)
		if rectsIntersect(p.X, p.Y, p.X+box.W, p.Y+box.H, x0, y0, x1, y1) {
			tables[t.Name] = true
		}
	}
	annos := map[string]bool{}
	for _, an := range view.Annotations {
		if rectsIntersect(an.X, an.Y, an.X+an.W, an.Y+an.H, x0, y0, x1, y1) {
			annos[an.ID] = true
		}
	}
	a.selectedTables = tables
	a.selectedAnnos = annos
}

func rectsIntersect(ax0, ay0, ax1, ay1, bx0, by0, bx1, by1 float64) bool {
	return ax1 >= bx0 && ax0 <= bx1 && ay1 >= by0 && ay0 <= by1
}

func (a *App) drawSelectionBox(screen *ebiten.Image) {
	if !a.selecting {
		return
	}
	x0 := float64(a.selectStartMX)
	y0 := float64(a.selectStartMY)
	x1 := float64(a.selectCurMX)
	y1 := float64(a.selectCurMY)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	fill := theme.ColorAccent
	fill.A = 0x18
	vector.FillRect(screen, float32(x0), float32(y0), float32(x1-x0), float32(y1-y0), fill, false)
	border := theme.ColorAccent
	border.A = 0xc0
	vector.StrokeRect(screen, float32(x0), float32(y0), float32(x1-x0), float32(y1-y0), 1, border, false)
}

const (
	annoModeMove   = 0
	annoModeResize = 1
)

func NewApp(dbmlPath string) (*App, error) {
	a := &App{
		camera:   NewCamera(),
		width:    1280,
		height:   800,
		dbmlPath: dbmlPath,
		metaPath: meta.SidecarPath(dbmlPath),
	}

	if err := a.reloadDBML(); err != nil {
		return nil, err
	}
	if err := a.loadOrCreateMeta(); err != nil {
		return nil, err
	}
	a.activeViewID = a.doc.DefaultView().ID

	w, err := watch.New(a.dbmlPath, a.metaPath)
	if err != nil {
		return nil, fmt.Errorf("watch: %w", err)
	}
	a.watcher = w
	return a, nil
}

func (a *App) currentView() *meta.View {
	for _, v := range a.doc.Views {
		if v.ID == a.activeViewID {
			return v
		}
	}
	return a.doc.DefaultView()
}

func (a *App) Close() {
	if a.watcher != nil {
		_ = a.watcher.Close()
	}
}

func (a *App) WindowSize() (int, int) { return a.width, a.height }

func (a *App) reloadDBML() error {
	s, err := dbml.ParseFile(a.dbmlPath)
	if err != nil {
		return fmt.Errorf("parse %s: %w", a.dbmlPath, err)
	}
	a.schema = s
	return nil
}

func (a *App) loadOrCreateMeta() error {
	doc, err := meta.Load(a.metaPath)
	if err != nil {
		return fmt.Errorf("load meta: %w", err)
	}
	if doc == nil {
		doc = meta.NewDocument()
		opts := layout.DefaultOptions()
		opts.SizeOf = a.tableSizeOf
		meta.BootstrapView(doc.DefaultView(), a.schema, opts)
		a.doc = doc
		return a.persist()
	}
	a.doc = doc
	report := meta.Reconcile(a.doc, a.schema)
	if !report.Empty() {
		log.Printf("schema drift: +%d orphaned=%d restored=%d",
			len(report.AddedTables), len(report.OrphanedTables), len(report.RestoredTables))
		return a.persist()
	}
	return nil
}

func (a *App) persist() error {
	if err := meta.Save(a.metaPath, a.doc); err != nil {
		return fmt.Errorf("save meta: %w", err)
	}
	a.dirty = false
	return nil
}

func (a *App) Update() error {
	a.frameCount++
	a.drainWatcher()

	mx, my := ebiten.CursorPosition()
	wx, wy := a.camera.ScreenToWorld(float64(mx), float64(my))

	if a.renamingView != "" {
		a.updateRenameEditing()
		a.tickCaret()
		return nil
	}

	if a.renamingGroup != "" {
		a.updateGroupRenameEditing()
		a.tickCaret()
		return nil
	}

	if a.editingAnno != "" {
		a.updateTextEditing()
		a.tickCaret()
		return nil
	}

	if a.cellEdit.Active() {
		a.updateCellEditing()
		a.tickCaret()
		return nil
	}

	if my < tabBarHeight {
		if a.handleTabBar(mx, my) {
			return nil
		}
	}

	if a.palette != nil {
		clicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
		c, result := a.palette.Update(mx, my, clicked)
		switch result {
		case widgets.PalettePicked:
			cb := a.paletteCallback
			a.palette = nil
			a.paletteCallback = nil
			if cb != nil {
				cb(c, false)
			}
		case widgets.PaletteCleared:
			cb := a.paletteCallback
			a.palette = nil
			a.paletteCallback = nil
			if cb != nil {
				cb(color.NRGBA{}, true)
			}
		case widgets.PaletteDismissed:
			a.palette = nil
			a.paletteCallback = nil
		}
		return nil
	}

	if a.menu != nil {
		leftDown := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
		rightDown := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
		activated, dismiss := a.menu.Update(mx, my, leftDown || rightDown)
		if activated >= 0 {
			action := a.menu.Items[activated].Action
			a.menu = nil
			if action != nil {
				action()
			}
			return nil
		}
		if dismiss {
			a.menu = nil
		}
		return nil
	}

	// Middle-button drag: pans the camera regardless of what the cursor is
	// over (including tables). Does not select, drag, or otherwise interact.
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		if !a.middlePanning {
			a.middlePanning = true
			a.lastMidPanX, a.lastMidPanY = mx, my
		} else {
			dx := float64(mx - a.lastMidPanX)
			dy := float64(my - a.lastMidPanY)
			a.camera.Pan(dx, dy)
			a.lastMidPanX, a.lastMidPanY = mx, my
		}
		return nil
	}
	a.middlePanning = false

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		if id, _ := a.annotationAt(wx, wy); id != "" {
			a.menu = a.buildAnnotationMenu(id, mx, my)
			return nil
		}
		if hit := a.hitTestTable(wx, wy); hit.Region != regionNone {
			a.menu = a.buildTableMenu(hit, mx, my)
			return nil
		}
		if gid := a.groupLabelAtWorld(wx, wy); gid != "" {
			a.menu = a.buildGroupMenu(gid, mx, my)
			return nil
		}
		if key := a.relationshipAtScreen(mx, my); key != "" {
			a.menu = a.buildRelationshipMenu(key, mx, my)
			return nil
		}
		a.menu = a.buildCanvasMenu(mx, my, wx, wy)
		return nil
	}

	if inpressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft); inpressed {
		switch {
		case a.draggingAnno != "" && a.draggingAnnoMode == annoModeResize:
			a.updateAnnotationDrag(wx, wy)
		case a.dragging || a.draggingAnno != "":
			a.updateGroupDrag(wx, wy)
		case a.maybeSelecting:
			dxm := mx - a.selectStartMX
			dym := my - a.selectStartMY
			if abs(dxm) >= selectMoveThreshold || abs(dym) >= selectMoveThreshold {
				a.maybeSelecting = false
				a.selecting = true
				a.selectCurMX = mx
				a.selectCurMY = my
			}
		case a.selecting:
			a.selectCurMX = mx
			a.selectCurMY = my
		default:
			// Double-click on table text → in-place edit.
			hit := a.hitTestTable(wx, wy)
			if a.isDoubleClick(hit) {
				switch hit.Region {
				case regionColumnType:
					a.startCellEdit(cellEditColumnType, hit.Table, hit.Column)
					a.lastLeftClickFrame = 0
					return nil
				case regionColumnName:
					a.startCellEdit(cellEditColumnName, hit.Table, hit.Column)
					a.lastLeftClickFrame = 0
					return nil
				case regionHeader:
					a.startCellEdit(cellEditTableName, hit.Table, "")
					a.lastLeftClickFrame = 0
					return nil
				}
			}
			a.recordClick(hit)

			// Click on a group label: double-click → rename, single click →
			// select every table that's a member of the group (so the user
			// can drag, colour, group-op them as one unit).
			if hit.Region == regionNone {
				if gid := a.groupLabelAtWorld(wx, wy); gid != "" {
					if a.lastClickedGroupID == gid && a.frameCount-a.lastClickedGroupFrame < doubleClickFrames {
						a.startRenamingGroup(gid)
						a.lastClickedGroupID = ""
						a.lastClickedGroupFrame = 0
						return nil
					}
					a.lastClickedGroupID = gid
					a.lastClickedGroupFrame = a.frameCount
					a.selectGroupMembers(gid)
					// Set up for group drag from this position so the user can
					// drag the whole group by holding the label.
					a.dragging = true
					a.draggedTable = ""
					a.dragStartCursor = [2]float64{wx, wy}
					a.captureGroupDragStart()
					return nil
				}
			}

			if id, mode := a.annotationAt(wx, wy); id != "" {
				if mode == annoModeResize {
					a.selectOnlyAnno(id)
					a.startAnnotationDrag(id, mode, wx, wy)
				} else {
					if !a.isAnnoSelected(id) {
						a.selectOnlyAnno(id)
					}
					a.draggingAnno = id
					a.draggingAnnoMode = annoModeMove
					a.dragStartCursor = [2]float64{wx, wy}
					a.captureGroupDragStart()
				}
			} else if name := a.tableAtWorld(wx, wy); name != "" {
				if !a.isTableSelected(name) {
					a.selectOnlyTable(name)
				}
				a.dragging = true
				a.draggedTable = name
				a.dragStartCursor = [2]float64{wx, wy}
				a.captureGroupDragStart()
			} else {
				// Left-down on empty canvas — defer the decision to either
				// "selection box" (if drag) or "no-op" (if just a click).
				a.maybeSelecting = true
				a.selectStartWX, a.selectStartWY = wx, wy
				a.selectStartMX, a.selectStartMY = mx, my
				a.selectCurMX, a.selectCurMY = mx, my
			}
		}
	} else {
		if (a.dragging || a.draggingAnno != "") && a.dirty {
			if err := a.persist(); err != nil {
				log.Printf("persist: %v", err)
			}
		}
		if a.selecting {
			a.applyBoxSelection()
			a.selecting = false
		} else if a.maybeSelecting {
			// Click on empty canvas without dragging → clear selection.
			a.selectedTables = nil
			a.selectedAnnos = nil
		}
		a.dragging = false
		a.draggedTable = ""
		a.draggingAnno = ""
		a.dragStartTablePositions = nil
		a.dragStartAnnoPositions = nil
		a.maybeSelecting = false
	}

	_, scrollY := ebiten.Wheel()
	if scrollY != 0 {
		factor := math.Pow(1.1, scrollY)
		a.camera.ZoomAt(float64(mx), float64(my), factor)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		a.fitToTables()
	}

	if a.menu == nil && a.palette == nil && my >= tabBarHeight {
		a.hoveredTable = a.tableAtWorld(wx, wy)
		if a.hoveredTable == "" {
			a.hoveredGroup = a.groupLabelAtWorld(wx, wy)
		} else {
			a.hoveredGroup = ""
		}
	} else {
		a.hoveredTable = ""
		a.hoveredGroup = ""
	}

	return nil
}

// fitToTables recentres and zooms the camera to fit every visible table on
// screen with a small padding. Triggered by F.
func (a *App) fitToTables() {
	view := a.currentView()

	tableIdx := make(map[string]*dbml.Table, len(a.schema.Tables))
	for i := range a.schema.Tables {
		tableIdx[a.schema.Tables[i].Name] = &a.schema.Tables[i]
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	any := false
	for name, p := range view.Tables {
		if p.Hidden || p.Orphaned {
			continue
		}
		t, ok := tableIdx[name]
		if !ok {
			continue
		}
		box := render.MeasureTable(t)
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X+box.W > maxX {
			maxX = p.X + box.W
		}
		if p.Y+box.H > maxY {
			maxY = p.Y + box.H
		}
		any = true
	}
	if !any {
		return
	}

	const pad = 80.0
	minX -= pad
	minY -= pad
	maxX += pad
	maxY += pad

	bw := maxX - minX
	bh := maxY - minY
	if bw <= 0 || bh <= 0 {
		return
	}

	availH := float64(a.height) - tabBarHeight
	if availH < 1 {
		availH = float64(a.height)
	}
	zoom := math.Min(float64(a.width)/bw, availH/bh)
	if zoom < 0.05 {
		zoom = 0.05
	}
	if zoom > 8.0 {
		zoom = 8.0
	}
	a.camera.Zoom = zoom

	worldCx := (minX + maxX) / 2
	worldCy := (minY + maxY) / 2
	a.camera.X = worldCx - float64(a.width)/(2*zoom)
	a.camera.Y = worldCy - (tabBarHeight+availH/2)/zoom
}

func (a *App) drainWatcher() {
	if a.watcher == nil {
		return
	}
	for {
		select {
		case ev, ok := <-a.watcher.Events():
			if !ok {
				return
			}
			switch ev.Kind {
			case watch.EventDBML:
				if err := a.reloadDBML(); err != nil {
					log.Printf("reload dbml: %v", err)
					return
				}
				log.Printf("reloaded dbml: tables=%d relationships=%d", len(a.schema.Tables), len(a.schema.Relationships))
				report := meta.Reconcile(a.doc, a.schema)
				if !report.Empty() {
					log.Printf("schema drift: +%d orphaned=%d restored=%d",
						len(report.AddedTables), len(report.OrphanedTables), len(report.RestoredTables))
					if err := a.persist(); err != nil {
						log.Printf("persist: %v", err)
					}
				}
			case watch.EventMeta:
				if !a.dirty {
					doc, err := meta.Load(a.metaPath)
					if err != nil {
						log.Printf("reload meta: %v", err)
						continue
					}
					if doc != nil {
						a.doc = doc
						meta.Reconcile(a.doc, a.schema)
					}
				}
			}
		default:
			return
		}
	}
}

func (a *App) tableAtWorld(wx, wy float64) string {
	view := a.currentView()
	for i := len(a.schema.Tables) - 1; i >= 0; i-- {
		t := &a.schema.Tables[i]
		p, ok := view.Tables[t.Name]
		if !ok || (p.Hidden) {
			continue
		}
		box := render.MeasureTable(t)
		if wx >= p.X && wx <= p.X+box.W && wy >= p.Y && wy <= p.Y+box.H {
			return t.Name
		}
	}
	return ""
}

// hitTestTable returns which sub-region of a table the world-space (wx, wy)
// lands in: header, a specific column's name area, or its type area.
func (a *App) hitTestTable(wx, wy float64) tableHit {
	view := a.currentView()
	for i := len(a.schema.Tables) - 1; i >= 0; i-- {
		t := &a.schema.Tables[i]
		p, ok := view.Tables[t.Name]
		if !ok || p.Hidden {
			continue
		}
		box := render.MeasureTable(t)
		if wx < p.X || wx > p.X+box.W || wy < p.Y || wy > p.Y+box.H {
			continue
		}
		localY := wy - p.Y
		if localY < render.TableHeaderH {
			return tableHit{Table: t.Name, Region: regionHeader}
		}
		rowIdx := int((localY - render.TableHeaderH) / render.TableRowH)
		if rowIdx < 0 || rowIdx >= len(t.Columns) {
			return tableHit{Table: t.Name, Region: regionHeader}
		}
		c := &t.Columns[rowIdx]
		typeW := render.TextWidth(c.Type)
		typeRegionLeft := p.X + box.W - render.TablePadX - typeW - 6
		if wx >= typeRegionLeft {
			return tableHit{Table: t.Name, Column: c.Name, Region: regionColumnType}
		}
		return tableHit{Table: t.Name, Column: c.Name, Region: regionColumnName}
	}
	return tableHit{}
}

func (a *App) Draw(screen *ebiten.Image) {
	screen.Fill(theme.ColorBackground)
	a.drawGrid(screen)
	a.drawGroups(screen)
	a.drawRelationships(screen)
	a.drawTables(screen)
	a.drawAnnotations(screen)
	a.drawCellEditOverlay(screen)
	a.drawGroupRenameOverlay(screen)
	a.drawSelectionBox(screen)
	a.drawTabBar(screen)
	if a.menu != nil {
		a.menu.Draw(screen)
	}
	if a.palette != nil {
		a.palette.Draw(screen)
	}
}

func (a *App) drawCellEditOverlay(screen *ebiten.Image) {
	e := a.cellEdit
	if !e.Active() {
		return
	}
	view := a.currentView()
	p, ok := view.Tables[e.Table]
	if !ok {
		return
	}
	var t *dbml.Table
	for i := range a.schema.Tables {
		if a.schema.Tables[i].Name == e.Table {
			t = &a.schema.Tables[i]
			break
		}
	}
	if t == nil {
		return
	}

	box := render.MeasureTable(t)
	scale := a.camera.Zoom

	display := e.Buffer
	if a.caretBlinkOn {
		display += "_"
	}
	tw := render.TextWidth(display)
	minTW := render.TextWidth("xxxxxx")
	if tw < minTW {
		tw = minTW
	}

	pad := 6 * scale

	switch e.Kind {
	case cellEditTableName:
		// Header text is at sx+TablePadX, vertical-centered in TableHeaderH.
		headerX := p.X + render.TablePadX
		headerYWorld := p.Y + (render.TableHeaderH-14)/2
		sx, sy := a.camera.WorldToScreen(headerX, headerYWorld)

		boxX := sx - pad
		boxY := sy - (4 * scale)
		boxW := tw*scale + pad*2
		boxH := (render.TableHeaderH - 4) * scale

		vector.FillRect(screen, float32(boxX), float32(boxY), float32(boxW), float32(boxH), theme.ColorSurface, false)
		vector.StrokeRect(screen, float32(boxX), float32(boxY), float32(boxW), float32(boxH), 1, theme.ColorAccent, false)
		render.DrawText(screen, display, sx, sy, scale, theme.ColorText)

	case cellEditColumnName, cellEditColumnType:
		rowIdx := -1
		for i, c := range t.Columns {
			if c.Name == e.Column {
				rowIdx = i
				break
			}
		}
		if rowIdx < 0 {
			return
		}
		rowYWorld := p.Y + render.TableHeaderH + float64(rowIdx)*render.TableRowH

		if e.Kind == cellEditColumnName {
			leftX := p.X + render.TablePadX
			sx, sy := a.camera.WorldToScreen(leftX, rowYWorld)
			boxX := sx - pad
			boxY := sy
			boxW := tw*scale + pad*2
			boxH := (render.TableRowH - 2) * scale
			vector.FillRect(screen, float32(boxX), float32(boxY), float32(boxW), float32(boxH), theme.ColorSurface, false)
			vector.StrokeRect(screen, float32(boxX), float32(boxY), float32(boxW), float32(boxH), 1, theme.ColorAccent, false)
			txtY := sy + (render.TableRowH-13)/2*scale
			render.DrawText(screen, display, sx, txtY, scale, theme.ColorText)
		} else {
			rightX := p.X + box.W - render.TablePadX
			sxRight, sy := a.camera.WorldToScreen(rightX, rowYWorld)
			boxX := sxRight - tw*scale - pad
			boxY := sy
			boxW := tw*scale + pad*2
			boxH := (render.TableRowH - 2) * scale
			vector.FillRect(screen, float32(boxX), float32(boxY), float32(boxW), float32(boxH), theme.ColorSurface, false)
			vector.StrokeRect(screen, float32(boxX), float32(boxY), float32(boxW), float32(boxH), 1, theme.ColorAccent, false)
			txtY := sy + (render.TableRowH-13)/2*scale
			render.DrawText(screen, display, sxRight-tw*scale, txtY, scale, theme.ColorText)
		}
	}
}

const (
	tabBarHeight = 28.0
	tabPadX      = 12.0
	tabGap       = 4.0
	tabPlusW     = 28.0
)

type tabKind int

const (
	tabKindView tabKind = iota
	tabKindPlus
	tabKindRearrange
)

type tabRect struct {
	id   string
	x, w float64
	kind tabKind
}

const rearrangeLabel = "Rearrange"

func (a *App) tabRects(screenW int) []tabRect {
	rects := make([]tabRect, 0, len(a.doc.Views)+2)
	x := 8.0
	for _, v := range a.doc.Views {
		label := v.Name
		if a.renamingView == v.ID {
			label = a.renameBuffer
			if a.caretBlinkOn {
				label += "_"
			}
		}
		w := render.TextWidth(label) + 2*tabPadX
		if w < 60 {
			w = 60
		}
		rects = append(rects, tabRect{id: v.ID, x: x, w: w, kind: tabKindView})
		x += w + tabGap
	}
	rects = append(rects, tabRect{x: x, w: tabPlusW, kind: tabKindPlus})

	rearrangeW := render.TextWidth(rearrangeLabel) + 2*tabPadX
	rearrangeX := float64(screenW) - 8 - rearrangeW
	rects = append(rects, tabRect{x: rearrangeX, w: rearrangeW, kind: tabKindRearrange})

	return rects
}

func (a *App) drawTabBar(screen *ebiten.Image) {
	w := screen.Bounds().Dx()
	bg := theme.ColorSurface
	bg.A = 0xee
	vector.FillRect(screen, 0, 0, float32(w), float32(tabBarHeight), bg, false)
	vector.StrokeLine(screen, 0, float32(tabBarHeight), float32(w), float32(tabBarHeight), 1, theme.ColorBorder, false)

	rects := a.tabRects(w)
	for _, r := range rects {
		fill := bg
		border := theme.ColorBorder
		var labelColor color.Color = theme.ColorTextMuted
		if r.kind == tabKindView && r.id == a.activeViewID {
			fill = theme.ColorBackground
			border = theme.ColorAccent
			labelColor = theme.ColorText
		}
		vector.FillRect(screen, float32(r.x), 4, float32(r.w), float32(tabBarHeight-4), fill, false)
		vector.StrokeRect(screen, float32(r.x), 4, float32(r.w), float32(tabBarHeight-4), 1, border, false)

		switch r.kind {
		case tabKindPlus:
			cx := r.x + r.w/2
			cy := 4 + (tabBarHeight-4)/2
			vector.StrokeLine(screen, float32(cx-5), float32(cy), float32(cx+5), float32(cy), 1.5, theme.ColorTextMuted, true)
			vector.StrokeLine(screen, float32(cx), float32(cy-5), float32(cx), float32(cy+5), 1.5, theme.ColorTextMuted, true)
		case tabKindRearrange:
			render.DrawText(screen, rearrangeLabel, r.x+tabPadX, 4+(tabBarHeight-4-13)/2, 1.0, labelColor)
		case tabKindView:
			var label string
			for _, v := range a.doc.Views {
				if v.ID == r.id {
					label = v.Name
					if a.renamingView == v.ID {
						label = a.renameBuffer
						if a.caretBlinkOn {
							label += "_"
						}
					}
					break
				}
			}
			render.DrawText(screen, label, r.x+tabPadX, 4+(tabBarHeight-4-13)/2, 1.0, labelColor)
		}
	}
}

func (a *App) handleTabBar(mx, my int) bool {
	if my >= tabBarHeight {
		return false
	}
	leftJust := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	rightJust := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	if !leftJust && !rightJust {
		return true
	}
	for _, r := range a.tabRects(a.width) {
		if float64(mx) < r.x || float64(mx) > r.x+r.w {
			continue
		}
		switch r.kind {
		case tabKindPlus:
			if leftJust {
				a.newView()
			}
		case tabKindRearrange:
			if leftJust {
				a.rearrangeCurrentView()
			}
		case tabKindView:
			if leftJust {
				a.activeViewID = r.id
			} else if rightJust {
				a.menu = a.buildViewMenu(r.id, mx, my)
			}
		}
		return true
	}
	return true
}

// rearrangeCurrentView re-runs force-directed layout on the active view,
// overwriting positions for tables that exist in the schema. Hidden state,
// colour, group memberships, and orphaned entries are preserved.
func (a *App) rearrangeCurrentView() {
	view := a.currentView()
	opts := layout.DefaultOptions()
	opts.SizeOf = a.tableSizeOf
	positions := layout.ForceDirected(a.schema, opts)
	for name, p := range positions {
		if existing, ok := view.Tables[name]; ok {
			existing.X = p.X
			existing.Y = p.Y
		} else {
			view.Tables[name] = &meta.TablePlacement{X: p.X, Y: p.Y}
		}
	}
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
	a.fitToTables()
}

func (a *App) isDoubleClick(hit tableHit) bool {
	if hit.Region == regionNone {
		return false
	}
	if a.lastLeftClickFrame == 0 {
		return false
	}
	if a.frameCount-a.lastLeftClickFrame > doubleClickFrames {
		return false
	}
	return a.lastClickHit == hit
}

func (a *App) recordClick(hit tableHit) {
	a.lastLeftClickFrame = a.frameCount
	a.lastClickHit = hit
}

func (a *App) startCellEdit(kind cellEditKind, table, column string) {
	var initial string
	for i := range a.schema.Tables {
		t := &a.schema.Tables[i]
		if t.Name != table {
			continue
		}
		switch kind {
		case cellEditTableName:
			initial = t.Name
		default:
			for _, c := range t.Columns {
				if c.Name != column {
					continue
				}
				if kind == cellEditColumnName {
					initial = c.Name
				} else {
					initial = c.Type
				}
				break
			}
		}
		break
	}
	a.cellEdit = cellEdit{Kind: kind, Table: table, Column: column, Buffer: initial}
	a.caretBlinkOn = true
	a.caretFrame = 0
}

func (a *App) commitCellEdit() {
	e := a.cellEdit
	a.cellEdit = cellEdit{}
	if e.Buffer == "" {
		return
	}
	var (
		res *dbmledit.Result
		err error
	)
	switch e.Kind {
	case cellEditTableName:
		res, err = dbmledit.RewriteTableName(a.dbmlPath, e.Table, e.Buffer)
	case cellEditColumnName:
		res, err = dbmledit.RewriteColumnName(a.dbmlPath, e.Table, e.Column, e.Buffer)
	case cellEditColumnType:
		res, err = dbmledit.RewriteColumnType(a.dbmlPath, e.Table, e.Column, e.Buffer)
	}
	if err != nil {
		log.Printf("edit %s.%s (%v): %v", e.Table, e.Column, e.Kind, err)
		return
	}
	if res != nil && res.Schema != nil {
		a.schema = res.Schema
	}
	// If the active selected/dragged table got renamed, follow the rename so
	// hover state stays meaningful.
	if e.Kind == cellEditTableName {
		if a.isTableSelected(e.Table) {
			delete(a.selectedTables, e.Table)
			if a.selectedTables == nil {
				a.selectedTables = map[string]bool{}
			}
			a.selectedTables[e.Buffer] = true
		}
		if a.hoveredTable == e.Table {
			a.hoveredTable = e.Buffer
		}
		if a.draggedTable == e.Table {
			a.draggedTable = e.Buffer
		}
		// Carry the position over in metadata: the rename creates a new
		// schema name; the placement under the old name will be soft-orphaned
		// on the next reconcile, and a new placement will be made by drift.
		// To avoid losing the position, copy it explicitly here.
		view := a.currentView()
		if oldP, ok := view.Tables[e.Table]; ok {
			view.Tables[e.Buffer] = &meta.TablePlacement{
				X: oldP.X, Y: oldP.Y, Hidden: oldP.Hidden, Color: oldP.Color,
			}
			delete(view.Tables, e.Table)
			a.dirty = true
			if err := a.persist(); err != nil {
				log.Printf("persist: %v", err)
			}
		}
	}
}

func (a *App) cancelCellEdit() {
	a.cellEdit = cellEdit{}
}

func (a *App) updateCellEditing() {
	chars := ebiten.AppendInputChars(nil)
	if len(chars) > 0 {
		a.cellEdit.Buffer += string(chars)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || keyRepeatPressed(ebiten.KeyBackspace) {
		if len(a.cellEdit.Buffer) > 0 {
			r := []rune(a.cellEdit.Buffer)
			a.cellEdit.Buffer = string(r[:len(r)-1])
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		a.commitCellEdit()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.cancelCellEdit()
		return
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		a.commitCellEdit()
	}
}

// tableSizeOf reports a table's rendered world-space size. Used by the layout
// engine for size-aware repulsion so tall tables don't overlap shorter ones.
func (a *App) tableSizeOf(name string) (float64, float64) {
	for i := range a.schema.Tables {
		t := &a.schema.Tables[i]
		if t.Name == name {
			box := render.MeasureTable(t)
			return box.W, box.H
		}
	}
	return 0, 0
}

func (a *App) buildViewMenu(viewID string, mx, my int) *widgets.Menu {
	canDelete := len(a.doc.Views) > 1
	items := []widgets.MenuItem{
		{Label: "Rename view", Action: func() { a.startRenamingView(viewID) }},
	}
	items = append(items, widgets.MenuItem{Sep: true})
	items = append(items, widgets.MenuItem{
		Label:    "Delete view",
		Disabled: !canDelete,
		Action:   func() { a.deleteView(viewID) },
	})
	return &widgets.Menu{X: float64(mx), Y: float64(my), Items: items}
}

func (a *App) newView() {
	source := a.currentView()
	v := &meta.View{
		ID:            meta.NewID(),
		Name:          nextViewName(a.doc.Views),
		Tables:        map[string]*meta.TablePlacement{},
		Relationships: map[string]*meta.RelationshipStyle{},
	}
	for name, p := range source.Tables {
		v.Tables[name] = &meta.TablePlacement{X: p.X, Y: p.Y, Hidden: p.Hidden}
	}
	for _, g := range a.schema.TableGroups {
		v.Groups = append(v.Groups, &meta.Group{
			ID:     meta.NewID(),
			Name:   g.Name,
			Tables: append([]string(nil), g.Members...),
		})
	}
	a.doc.Views = append(a.doc.Views, v)
	a.activeViewID = v.ID
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
}

func (a *App) deleteView(viewID string) {
	if len(a.doc.Views) <= 1 {
		return
	}
	out := a.doc.Views[:0]
	for _, v := range a.doc.Views {
		if v.ID != viewID {
			out = append(out, v)
		}
	}
	a.doc.Views = out
	if a.activeViewID == viewID {
		a.activeViewID = a.doc.DefaultView().ID
	}
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
}

func nextViewName(existing []*meta.View) string {
	used := map[string]bool{}
	for _, v := range existing {
		used[v.Name] = true
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("view %d", i)
		if !used[name] {
			return name
		}
	}
}

func (a *App) startRenamingView(viewID string) {
	for _, v := range a.doc.Views {
		if v.ID == viewID {
			a.renamingView = viewID
			a.renameBuffer = v.Name
			a.caretBlinkOn = true
			a.caretFrame = 0
			return
		}
	}
}

func (a *App) commitRenamingView() {
	for _, v := range a.doc.Views {
		if v.ID == a.renamingView {
			name := a.renameBuffer
			if name == "" {
				name = v.Name
			}
			v.Name = name
			a.dirty = true
			if err := a.persist(); err != nil {
				log.Printf("persist: %v", err)
			}
			break
		}
	}
	a.renamingView = ""
	a.renameBuffer = ""
}

func (a *App) cancelRenamingView() {
	a.renamingView = ""
	a.renameBuffer = ""
}

func (a *App) groupLabelAtWorld(wx, wy float64) string {
	view := a.currentView()
	tableIdx := make(map[string]*dbml.Table, len(a.schema.Tables))
	for i := range a.schema.Tables {
		tableIdx[a.schema.Tables[i].Name] = &a.schema.Tables[i]
	}
	for _, g := range view.Groups {
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		any := false
		for _, name := range g.Tables {
			p, ok := view.Tables[name]
			if !ok || p.Hidden {
				continue
			}
			t, ok := tableIdx[name]
			if !ok {
				continue
			}
			box := render.MeasureTable(t)
			if p.X < minX {
				minX = p.X
			}
			if p.Y < minY {
				minY = p.Y
			}
			if p.X+box.W > maxX {
				maxX = p.X + box.W
			}
			if p.Y+box.H > maxY {
				maxY = p.Y + box.H
			}
			any = true
		}
		if !any {
			continue
		}
		pad := render.GroupPadding
		header := render.GroupHeaderH
		// Hit area = the whole visible group rect (label header band + pad +
		// tables area + pad). Tables are checked earlier in the click path so
		// hitting a table inside the rect still goes to the table, not here.
		rectMinX := minX - pad
		rectMaxX := maxX + pad
		rectMinY := minY - pad - header
		rectMaxY := maxY + pad
		if wx >= rectMinX && wx <= rectMaxX && wy >= rectMinY && wy <= rectMaxY {
			return g.ID
		}
	}
	return ""
}

func (a *App) startRenamingGroup(groupID string) {
	for _, g := range a.currentView().Groups {
		if g.ID == groupID {
			a.renamingGroup = groupID
			a.renameGroupBuffer = g.Name
			a.caretBlinkOn = true
			a.caretFrame = 0
			return
		}
	}
}

func (a *App) commitRenamingGroup() {
	for _, g := range a.currentView().Groups {
		if g.ID == a.renamingGroup {
			if a.renameGroupBuffer != "" {
				g.Name = a.renameGroupBuffer
			}
			a.dirty = true
			if err := a.persist(); err != nil {
				log.Printf("persist: %v", err)
			}
			break
		}
	}
	a.renamingGroup = ""
	a.renameGroupBuffer = ""
}

func (a *App) cancelRenamingGroup() {
	a.renamingGroup = ""
	a.renameGroupBuffer = ""
}

func (a *App) updateGroupRenameEditing() {
	chars := ebiten.AppendInputChars(nil)
	if len(chars) > 0 {
		a.renameGroupBuffer += string(chars)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || keyRepeatPressed(ebiten.KeyBackspace) {
		if len(a.renameGroupBuffer) > 0 {
			r := []rune(a.renameGroupBuffer)
			a.renameGroupBuffer = string(r[:len(r)-1])
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		a.commitRenamingGroup()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.cancelRenamingGroup()
		return
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		a.commitRenamingGroup()
	}
}

// selectGroupMembers sets the current table selection to every member of the
// given group whose name is in the schema. Annotation selection is cleared.
func (a *App) selectGroupMembers(gid string) {
	for _, g := range a.currentView().Groups {
		if g.ID != gid {
			continue
		}
		sel := map[string]bool{}
		for _, m := range g.Tables {
			sel[m] = true
		}
		a.selectedTables = sel
		a.selectedAnnos = nil
		return
	}
}

func (a *App) drawGroupRenameOverlay(screen *ebiten.Image) {
	if a.renamingGroup == "" {
		return
	}
	view := a.currentView()
	var g *meta.Group
	for _, gg := range view.Groups {
		if gg.ID == a.renamingGroup {
			g = gg
			break
		}
	}
	if g == nil {
		return
	}

	tableIdx := make(map[string]*dbml.Table, len(a.schema.Tables))
	for i := range a.schema.Tables {
		tableIdx[a.schema.Tables[i].Name] = &a.schema.Tables[i]
	}
	minX, minY := math.Inf(1), math.Inf(1)
	any := false
	for _, name := range g.Tables {
		p, ok := view.Tables[name]
		if !ok || p.Hidden {
			continue
		}
		t, ok := tableIdx[name]
		if !ok {
			continue
		}
		box := render.MeasureTable(t)
		_ = box
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		any = true
	}
	if !any {
		return
	}

	pad := render.GroupPadding
	header := render.GroupHeaderH
	labelLeftWorld := minX - pad
	labelTopWorld := minY - pad - header

	sxLeft, sy := a.camera.WorldToScreen(labelLeftWorld+8, labelTopWorld+4)
	scale := a.camera.Zoom

	display := a.renameGroupBuffer
	if a.caretBlinkOn {
		display += "_"
	}
	tw := render.TextWidth(display)
	if tw < render.TextWidth("xxxxxx") {
		tw = render.TextWidth("xxxxxx")
	}

	padPx := 4 * scale
	boxW := tw*scale + padPx*2
	boxH := 16 * scale
	boxX := sxLeft - padPx
	boxY := sy - 2*scale

	vector.FillRect(screen, float32(boxX), float32(boxY), float32(boxW), float32(boxH), theme.ColorSurface, false)
	vector.StrokeRect(screen, float32(boxX), float32(boxY), float32(boxW), float32(boxH), 1, theme.ColorAccent, false)
	render.DrawText(screen, display, sxLeft, sy, scale, theme.ColorText)
}

func (a *App) buildGroupMenu(groupID string, mx, my int) *widgets.Menu {
	return &widgets.Menu{
		X: float64(mx), Y: float64(my),
		Items: []widgets.MenuItem{
			{Label: "Rename group", Action: func() { a.startRenamingGroup(groupID) }},
			{
				Label: "Set group colour…",
				Action: func() {
					a.openPalette(mx, my, func(c color.NRGBA, clear bool) {
						if clear {
							a.setGroupColor(groupID, "")
						} else {
							a.setGroupColor(groupID, toHex(c))
						}
					})
				},
			},
			{Sep: true},
			{Label: "Delete group", Action: func() { a.deleteGroup(groupID) }},
		},
	}
}

func (a *App) updateRenameEditing() {
	chars := ebiten.AppendInputChars(nil)
	if len(chars) > 0 {
		a.renameBuffer += string(chars)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || keyRepeatPressed(ebiten.KeyBackspace) {
		if len(a.renameBuffer) > 0 {
			r := []rune(a.renameBuffer)
			a.renameBuffer = string(r[:len(r)-1])
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		a.commitRenamingView()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.cancelRenamingView()
		return
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		a.commitRenamingView()
	}
}

func (a *App) openPalette(mx, my int, cb func(c color.NRGBA, clear bool)) {
	a.palette = &widgets.Palette{X: float64(mx), Y: float64(my)}
	a.paletteCallback = cb
}

func toHex(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func (a *App) setTableColor(name, hex string) {
	view := a.currentView()
	if p, ok := view.Tables[name]; ok {
		p.Color = hex
		a.dirty = true
		if err := a.persist(); err != nil {
			log.Printf("persist: %v", err)
		}
	}
}

func (a *App) setGroupColor(id, hex string) {
	view := a.currentView()
	for _, g := range view.Groups {
		if g.ID == id {
			g.Color = hex
			a.dirty = true
			if err := a.persist(); err != nil {
				log.Printf("persist: %v", err)
			}
			return
		}
	}
}

func (a *App) relationshipAtScreen(mx, my int) string {
	view := a.currentView()
	tableIdx := make(map[string]*dbml.Table, len(a.schema.Tables))
	for i := range a.schema.Tables {
		tableIdx[a.schema.Tables[i].Name] = &a.schema.Tables[i]
	}

	screenBox := func(name string) (routing.Box, bool) {
		p, ok := view.Tables[name]
		if !ok || p.Hidden {
			return routing.Box{}, false
		}
		t, ok := tableIdx[name]
		if !ok {
			return routing.Box{}, false
		}
		box := render.MeasureTable(t)
		sx, sy := a.camera.WorldToScreen(p.X, p.Y)
		return routing.Box{X: sx, Y: sy, W: box.W * a.camera.Zoom, H: box.H * a.camera.Zoom}, true
	}

	const threshold = 6.0
	bestKey := ""
	bestDist := threshold
	for _, r := range a.schema.Relationships {
		if r.FromTable == r.ToTable {
			continue
		}
		from, ok1 := screenBox(r.FromTable)
		to, ok2 := screenBox(r.ToTable)
		if !ok1 || !ok2 {
			continue
		}
		pts := routing.ZRoute(from, to)
		d := routing.DistanceToPolyline(float64(mx), float64(my), pts)
		if d < bestDist {
			bestDist = d
			bestKey = r.Key()
		}
	}
	return bestKey
}

func (a *App) buildRelationshipMenu(key string, mx, my int) *widgets.Menu {
	return &widgets.Menu{
		X: float64(mx), Y: float64(my),
		Items: []widgets.MenuItem{
			{
				Label: "Set relationship colour…",
				Action: func() {
					a.openPalette(mx, my, func(c color.NRGBA, clear bool) {
						if clear {
							a.setRelationshipColor(key, "")
						} else {
							a.setRelationshipColor(key, toHex(c))
						}
					})
				},
			},
		},
	}
}

func (a *App) setRelationshipColor(key, hex string) {
	view := a.currentView()
	if view.Relationships == nil {
		view.Relationships = map[string]*meta.RelationshipStyle{}
	}
	style, ok := view.Relationships[key]
	if !ok {
		style = &meta.RelationshipStyle{}
		view.Relationships[key] = style
	}
	style.Color = hex
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
}

func (a *App) setAnnotationColor(id, hex string) {
	an := a.findAnnotation(id)
	if an == nil {
		return
	}
	an.Color = hex
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
}

func (a *App) drawAnnotations(screen *ebiten.Image) {
	view := a.currentView()
	for _, an := range view.Annotations {
		sx, sy := a.camera.WorldToScreen(an.X, an.Y)
		sw := an.W * a.camera.Zoom
		sh := an.H * a.camera.Zoom

		clr := annotationColor(an)
		editing := a.editingAnno == an.ID
		caret := ""
		if editing && a.caretBlinkOn {
			caret = "_"
		}
		text := an.Text
		if editing {
			text = a.editBuffer
		}
		render.DrawAnnotation(screen, sx, sy, sw, sh, a.camera.Zoom, text, clr, a.isAnnoSelected(an.ID), editing, caret)
	}
}

func annotationColor(an *meta.Annotation) color.NRGBA {
	if an.Color != "" {
		if c, ok := parseHexColor(an.Color); ok {
			return c
		}
	}
	return theme.Palette[3]
}

func (a *App) annotationAt(wx, wy float64) (string, int) {
	view := a.currentView()
	for i := len(view.Annotations) - 1; i >= 0; i-- {
		an := view.Annotations[i]
		if wx < an.X || wx > an.X+an.W || wy < an.Y || wy > an.Y+an.H {
			continue
		}
		hxLo := an.X + an.W - render.AnnotationHandleSz
		hyLo := an.Y + an.H - render.AnnotationHandleSz
		if wx >= hxLo && wy >= hyLo {
			return an.ID, annoModeResize
		}
		return an.ID, annoModeMove
	}
	return "", 0
}

// captureGroupDragStart records the starting position of every currently
// selected table and annotation so a group drag can apply a uniform delta.
func (a *App) captureGroupDragStart() {
	view := a.currentView()
	a.dragStartTablePositions = map[string][2]float64{}
	for name := range a.selectedTables {
		if p, ok := view.Tables[name]; ok {
			a.dragStartTablePositions[name] = [2]float64{p.X, p.Y}
		}
	}
	a.dragStartAnnoPositions = map[string][2]float64{}
	for id := range a.selectedAnnos {
		for _, an := range view.Annotations {
			if an.ID == id {
				a.dragStartAnnoPositions[id] = [2]float64{an.X, an.Y}
				break
			}
		}
	}
}

// updateGroupDrag moves every selected table and annotation by the cursor
// delta from the drag start. Resizing is handled separately.
func (a *App) updateGroupDrag(wx, wy float64) {
	dx := wx - a.dragStartCursor[0]
	dy := wy - a.dragStartCursor[1]
	view := a.currentView()
	for name, start := range a.dragStartTablePositions {
		if p, ok := view.Tables[name]; ok {
			p.X = start[0] + dx
			p.Y = start[1] + dy
		}
	}
	for id, start := range a.dragStartAnnoPositions {
		for _, an := range view.Annotations {
			if an.ID == id {
				an.X = start[0] + dx
				an.Y = start[1] + dy
				break
			}
		}
	}
	a.dirty = true
}

func (a *App) startAnnotationDrag(id string, mode int, wx, wy float64) {
	an := a.findAnnotation(id)
	if an == nil {
		return
	}
	a.draggingAnno = id
	a.draggingAnnoMode = mode
	a.dragStartCursor = [2]float64{wx, wy}
	a.dragStartAnno = [4]float64{an.X, an.Y, an.W, an.H}
}

func (a *App) updateAnnotationDrag(wx, wy float64) {
	an := a.findAnnotation(a.draggingAnno)
	if an == nil {
		a.draggingAnno = ""
		return
	}
	dx := wx - a.dragStartCursor[0]
	dy := wy - a.dragStartCursor[1]
	if a.draggingAnnoMode == annoModeMove {
		an.X = a.dragStartAnno[0] + dx
		an.Y = a.dragStartAnno[1] + dy
	} else {
		w := a.dragStartAnno[2] + dx
		h := a.dragStartAnno[3] + dy
		if w < render.AnnotationMinWidth {
			w = render.AnnotationMinWidth
		}
		if h < render.AnnotationMinHeight {
			h = render.AnnotationMinHeight
		}
		an.W = w
		an.H = h
	}
	a.dirty = true
}

func (a *App) findAnnotation(id string) *meta.Annotation {
	if id == "" {
		return nil
	}
	for _, an := range a.currentView().Annotations {
		if an.ID == id {
			return an
		}
	}
	return nil
}

func (a *App) buildCanvasMenu(mx, my int, wx, wy float64) *widgets.Menu {
	return &widgets.Menu{
		X: float64(mx), Y: float64(my),
		Items: []widgets.MenuItem{
			{Label: "Add table here", Action: func() { a.addTable(wx, wy) }},
			{Label: "Add annotation here", Action: func() { a.newAnnotation(wx, wy) }},
		},
	}
}

func (a *App) addTable(wx, wy float64) {
	name := nextNewTableName(a.schema)
	res, err := dbmledit.AddTable(a.dbmlPath, name)
	if err != nil {
		log.Printf("add table: %v", err)
		return
	}
	if res != nil && res.Schema != nil {
		a.schema = res.Schema
	}
	view := a.currentView()
	view.Tables[name] = &meta.TablePlacement{X: wx, Y: wy}
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
	a.selectOnlyTable(name)
	a.startCellEdit(cellEditTableName, name, "")
}

func (a *App) addField(table string) {
	var t *dbml.Table
	for i := range a.schema.Tables {
		if a.schema.Tables[i].Name == table {
			t = &a.schema.Tables[i]
			break
		}
	}
	if t == nil {
		return
	}
	name := nextNewColumnName(t)
	res, err := dbmledit.AddColumn(a.dbmlPath, table, name, "text")
	if err != nil {
		log.Printf("add column to %s: %v", table, err)
		return
	}
	if res != nil && res.Schema != nil {
		a.schema = res.Schema
	}
	a.startCellEdit(cellEditColumnName, table, name)
}

func (a *App) removeField(table, column string) {
	res, err := dbmledit.RemoveColumn(a.dbmlPath, table, column)
	if err != nil {
		log.Printf("remove %s.%s: %v", table, column, err)
		return
	}
	if res != nil && res.Schema != nil {
		a.schema = res.Schema
	}
}

func (a *App) removeTable(table string) {
	res, err := dbmledit.RemoveTable(a.dbmlPath, table)
	if err != nil {
		log.Printf("remove table %s: %v", table, err)
		return
	}
	if res != nil && res.Schema != nil {
		a.schema = res.Schema
	}
	delete(a.selectedTables, table)
	if a.hoveredTable == table {
		a.hoveredTable = ""
	}
	if a.draggedTable == table {
		a.draggedTable = ""
	}
}

func nextNewTableName(s *dbml.Schema) string {
	for i := 1; ; i++ {
		name := fmt.Sprintf("new_table_%d", i)
		used := false
		for _, t := range s.Tables {
			if t.Name == name {
				used = true
				break
			}
		}
		if !used {
			return name
		}
	}
}

func nextNewColumnName(t *dbml.Table) string {
	for i := 1; ; i++ {
		name := fmt.Sprintf("new_field_%d", i)
		used := false
		for _, c := range t.Columns {
			if c.Name == name {
				used = true
				break
			}
		}
		if !used {
			return name
		}
	}
}

func (a *App) buildAnnotationMenu(id string, mx, my int) *widgets.Menu {
	multi := a.isAnnoSelected(id) && a.selectionSize() > 1

	colourLabel := "Set colour…"
	deleteLabel := "Delete annotation"
	if multi {
		colourLabel = fmt.Sprintf("Set colour for %d items…", a.selectionSize())
		deleteLabel = fmt.Sprintf("Delete %d annotations", len(a.selectedAnnos))
	}

	return &widgets.Menu{
		X: float64(mx), Y: float64(my),
		Items: []widgets.MenuItem{
			{Label: "Edit text", Action: func() { a.startEditing(id) }},
			{
				Label: colourLabel,
				Action: func() {
					a.openPalette(mx, my, func(c color.NRGBA, clear bool) {
						switch {
						case multi:
							a.applySelectionColor(toHex(c), clear)
						case clear:
							a.setAnnotationColor(id, "")
						default:
							a.setAnnotationColor(id, toHex(c))
						}
					})
				},
			},
			{Sep: true},
			{
				Label: deleteLabel,
				Action: func() {
					if multi {
						a.removeSelectedAnnotations()
					} else {
						a.deleteAnnotation(id)
					}
				},
			},
		},
	}
}

func (a *App) newAnnotation(wx, wy float64) {
	view := a.currentView()
	an := &meta.Annotation{
		ID:   meta.NewID(),
		X:    wx,
		Y:    wy,
		W:    160,
		H:    40,
		Text: "",
	}
	view.Annotations = append(view.Annotations, an)
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
	a.startEditing(an.ID)
}

func (a *App) deleteAnnotation(id string) {
	view := a.currentView()
	out := view.Annotations[:0]
	for _, an := range view.Annotations {
		if an.ID != id {
			out = append(out, an)
		}
	}
	view.Annotations = out
	delete(a.selectedAnnos, id)
	if a.editingAnno == id {
		a.editingAnno = ""
	}
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
}

func (a *App) startEditing(id string) {
	an := a.findAnnotation(id)
	if an == nil {
		return
	}
	a.editingAnno = id
	a.editBuffer = an.Text
	a.caretBlinkOn = true
	a.caretFrame = 0
}

func (a *App) commitEditing() {
	an := a.findAnnotation(a.editingAnno)
	if an != nil {
		an.Text = a.editBuffer
		a.dirty = true
		if err := a.persist(); err != nil {
			log.Printf("persist: %v", err)
		}
	}
	a.editingAnno = ""
	a.editBuffer = ""
}

func (a *App) cancelEditing() {
	a.editingAnno = ""
	a.editBuffer = ""
}

func (a *App) updateTextEditing() {
	chars := ebiten.AppendInputChars(nil)
	if len(chars) > 0 {
		a.editBuffer += string(chars)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || keyRepeatPressed(ebiten.KeyBackspace) {
		if len(a.editBuffer) > 0 {
			r := []rune(a.editBuffer)
			a.editBuffer = string(r[:len(r)-1])
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		a.commitEditing()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.cancelEditing()
		return
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		a.commitEditing()
	}
}

func keyRepeatPressed(key ebiten.Key) bool {
	d := inpututil.KeyPressDuration(key)
	return d >= 25 && d%4 == 0
}

func (a *App) tickCaret() {
	a.caretFrame++
	if a.caretFrame >= 30 {
		a.caretFrame = 0
		a.caretBlinkOn = !a.caretBlinkOn
	}
}

func (a *App) buildTableMenu(hit tableHit, mx, my int) *widgets.Menu {
	table := hit.Table
	view := a.currentView()
	memberOf := map[string]bool{}
	for _, g := range view.Groups {
		for _, m := range g.Tables {
			if m == table {
				memberOf[g.ID] = true
				break
			}
		}
	}

	multi := a.isTableSelected(table) && a.selectionSize() > 1
	selCount := a.selectionSize()
	tablesInSel := len(a.selectedTables)

	colourLabel := "Set table colour…"
	removeLabel := "Remove table " + table
	newGroupLabel := "New group with this table"
	if multi {
		colourLabel = fmt.Sprintf("Set colour for %d items…", selCount)
		removeLabel = fmt.Sprintf("Remove %d tables", tablesInSel)
		newGroupLabel = fmt.Sprintf("New group with %d tables", tablesInSel)
	}

	var items []widgets.MenuItem
	items = append(items, widgets.MenuItem{
		Label:  "Add field",
		Action: func() { a.addField(table) },
	})
	if hit.Column != "" {
		col := hit.Column
		items = append(items, widgets.MenuItem{
			Label:  "Remove field " + col,
			Action: func() { a.removeField(table, col) },
		})
	}
	items = append(items, widgets.MenuItem{Sep: true})
	items = append(items, widgets.MenuItem{
		Label: colourLabel,
		Action: func() {
			a.openPalette(mx, my, func(c color.NRGBA, clear bool) {
				switch {
				case multi:
					a.applySelectionColor(toHex(c), clear)
				case clear:
					a.setTableColor(table, "")
				default:
					a.setTableColor(table, toHex(c))
				}
			})
		},
	})
	items = append(items, widgets.MenuItem{
		Label: removeLabel,
		Action: func() {
			if multi {
				a.removeSelectedTables()
			} else {
				a.removeTable(table)
			}
		},
	})
	items = append(items, widgets.MenuItem{Sep: true})
	items = append(items, widgets.MenuItem{
		Label: newGroupLabel,
		Action: func() {
			if multi {
				a.newGroupFromSelection()
			} else {
				a.newGroupWith(table)
			}
		},
	})

	if len(view.Groups) > 0 {
		items = append(items, widgets.MenuItem{Sep: true})
		for _, g := range view.Groups {
			gID := g.ID
			gName := g.Name
			memberSet := map[string]bool{}
			for _, m := range g.Tables {
				memberSet[m] = true
			}
			if multi {
				// Count how many of the selected tables are/aren't already in this group.
				addable, removable := 0, 0
				for n := range a.selectedTables {
					if memberSet[n] {
						removable++
					} else {
						addable++
					}
				}
				if addable > 0 {
					items = append(items, widgets.MenuItem{
						Label:  fmt.Sprintf("Add %d selected to %s", addable, gName),
						Action: func() { a.addSelectionToGroup(gID) },
					})
				}
				if removable > 0 {
					items = append(items, widgets.MenuItem{
						Label:  fmt.Sprintf("Remove %d selected from %s", removable, gName),
						Action: func() { a.removeSelectionFromGroup(gID) },
					})
				}
				continue
			}
			if memberSet[table] {
				items = append(items, widgets.MenuItem{
					Label:  "Remove from " + gName,
					Action: func() { a.removeFromGroup(table, gID) },
				})
			} else {
				items = append(items, widgets.MenuItem{
					Label:  "Add to " + gName,
					Action: func() { a.addToGroup(table, gID) },
				})
			}
		}
		hasMembership := false
		for _, g := range view.Groups {
			if memberOf[g.ID] {
				hasMembership = true
				break
			}
		}
		if hasMembership {
			items = append(items, widgets.MenuItem{Sep: true})
			for _, g := range view.Groups {
				if !memberOf[g.ID] {
					continue
				}
				gID := g.ID
				gName := g.Name
				items = append(items, widgets.MenuItem{
					Label: "Set colour: " + gName,
					Action: func() {
						a.openPalette(mx, my, func(c color.NRGBA, clear bool) {
							if clear {
								a.setGroupColor(gID, "")
							} else {
								a.setGroupColor(gID, toHex(c))
							}
						})
					},
				})
			}
		}
		items = append(items, widgets.MenuItem{Sep: true})
		for _, g := range view.Groups {
			gID := g.ID
			gName := g.Name
			items = append(items, widgets.MenuItem{
				Label:  "Delete group " + gName,
				Action: func() { a.deleteGroup(gID) },
			})
		}
	}

	return &widgets.Menu{
		X:     float64(mx),
		Y:     float64(my),
		Items: items,
	}
}

func (a *App) newGroupWith(table string) {
	view := a.currentView()
	view.Groups = append(view.Groups, &meta.Group{
		ID:     meta.NewID(),
		Name:   nextGroupName(view.Groups),
		Tables: []string{table},
	})
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
}

func (a *App) addToGroup(table, groupID string) {
	view := a.currentView()
	for _, g := range view.Groups {
		if g.ID != groupID {
			continue
		}
		for _, m := range g.Tables {
			if m == table {
				return
			}
		}
		g.Tables = append(g.Tables, table)
		a.dirty = true
		if err := a.persist(); err != nil {
			log.Printf("persist: %v", err)
		}
		return
	}
}

func (a *App) removeFromGroup(table, groupID string) {
	view := a.currentView()
	for _, g := range view.Groups {
		if g.ID != groupID {
			continue
		}
		out := g.Tables[:0]
		for _, m := range g.Tables {
			if m != table {
				out = append(out, m)
			}
		}
		g.Tables = out
		a.dirty = true
		if err := a.persist(); err != nil {
			log.Printf("persist: %v", err)
		}
		return
	}
}

func (a *App) deleteGroup(groupID string) {
	view := a.currentView()
	out := view.Groups[:0]
	for _, g := range view.Groups {
		if g.ID != groupID {
			out = append(out, g)
		}
	}
	view.Groups = out
	a.dirty = true
	if err := a.persist(); err != nil {
		log.Printf("persist: %v", err)
	}
}

func nextGroupName(existing []*meta.Group) string {
	used := map[string]bool{}
	for _, g := range existing {
		used[g.Name] = true
	}
	for i := 1; ; i++ {
		name := fmt.Sprintf("group %d", i)
		if !used[name] {
			return name
		}
	}
}

func (a *App) drawGroups(screen *ebiten.Image) {
	view := a.currentView()
	tableIdx := make(map[string]*dbml.Table, len(a.schema.Tables))
	for i := range a.schema.Tables {
		tableIdx[a.schema.Tables[i].Name] = &a.schema.Tables[i]
	}
	for i, g := range view.Groups {
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		any := false
		for _, name := range g.Tables {
			p, ok := view.Tables[name]
			if !ok || p.Hidden {
				continue
			}
			t, ok := tableIdx[name]
			if !ok {
				continue
			}
			box := render.MeasureTable(t)
			if p.X < minX {
				minX = p.X
			}
			if p.Y < minY {
				minY = p.Y
			}
			if p.X+box.W > maxX {
				maxX = p.X + box.W
			}
			if p.Y+box.H > maxY {
				maxY = p.Y + box.H
			}
			any = true
		}
		if !any {
			continue
		}
		pad := render.GroupPadding
		header := render.GroupHeaderH
		minX -= pad
		minY -= pad + header
		maxX += pad
		maxY += pad

		sx, sy := a.camera.WorldToScreen(minX, minY)
		ex, ey := a.camera.WorldToScreen(maxX, maxY)

		clr := groupColor(g, i)
		active := g.ID == a.hoveredGroup || g.ID == a.renamingGroup || a.groupHasSelectedMembers(g)
		render.DrawGroup(screen, sx, sy, ex-sx, ey-sy, g.Name, clr, a.camera.Zoom, active)
	}
}

// groupHasSelectedMembers returns true if at least one of the group's members
// is in the current table selection. Used to light up groups whose contents
// are part of an active selection.
func (a *App) groupHasSelectedMembers(g *meta.Group) bool {
	if len(a.selectedTables) == 0 {
		return false
	}
	for _, m := range g.Tables {
		if a.selectedTables[m] {
			return true
		}
	}
	return false
}

func groupColor(g *meta.Group, idx int) color.NRGBA {
	if g.Color != "" {
		if c, ok := parseHexColor(g.Color); ok {
			return c
		}
	}
	return theme.Palette[idx%len(theme.Palette)]
}

// cardinalityKinds maps a DBML relationship kind to ER markers for each end.
//
//	`a > b` (ManyToOne):  many a → one b
//	`a < b` (OneToMany):  one a  → many b
//	`a - b` (OneToOne):   one    → one
func cardinalityKinds(k dbml.RelationshipKind) (render.CardinalityKind, render.CardinalityKind) {
	switch k {
	case dbml.RelManyToOne:
		return render.CardMany, render.CardOne
	case dbml.RelOneToMany:
		return render.CardOne, render.CardMany
	case dbml.RelOneToOne:
		return render.CardOne, render.CardOne
	}
	return render.CardNone, render.CardNone
}

func parseHexColor(s string) (color.NRGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, false
	}
	v := uint32(0)
	for i := 1; i < 7; i++ {
		c := s[i]
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return color.NRGBA{}, false
		}
		v = v<<4 | d
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, true
}

func (a *App) drawGrid(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	x0w, y0w := a.camera.ScreenToWorld(0, 0)
	x1w, y1w := a.camera.ScreenToWorld(float64(w), float64(h))

	startX := math.Floor(x0w/gridSpacing) * gridSpacing
	startY := math.Floor(y0w/gridSpacing) * gridSpacing

	for x := startX; x <= x1w; x += gridSpacing {
		sx, _ := a.camera.WorldToScreen(x, 0)
		vector.StrokeLine(screen, float32(sx), 0, float32(sx), float32(h), 1, theme.ColorGrid, false)
	}
	for y := startY; y <= y1w; y += gridSpacing {
		_, sy := a.camera.WorldToScreen(0, y)
		vector.StrokeLine(screen, 0, float32(sy), float32(w), float32(sy), 1, theme.ColorGrid, false)
	}
}

func (a *App) drawTables(screen *ebiten.Image) {
	view := a.currentView()
	for i := range a.schema.Tables {
		t := &a.schema.Tables[i]
		p, ok := view.Tables[t.Name]
		if !ok || p.Hidden {
			continue
		}
		sx, sy := a.camera.WorldToScreen(p.X, p.Y)
		var accent color.Color
		if c, ok := parseHexColor(p.Color); ok {
			accent = c
		}
		highlighted := t.Name == a.draggedTable ||
			a.isTableSelected(t.Name) ||
			a.isTableHovered(t.Name)
		render.DrawTable(screen, t, sx, sy, a.camera.Zoom, accent, highlighted)
	}
}

// relationshipShouldPulse returns true when the relationship is connected to
// any hovered (incl. via group hover) or selected table.
func (a *App) relationshipShouldPulse(r dbml.Relationship) bool {
	if a.isTableHovered(r.FromTable) || a.isTableHovered(r.ToTable) {
		return true
	}
	if a.isTableSelected(r.FromTable) || a.isTableSelected(r.ToTable) {
		return true
	}
	return false
}

func (a *App) drawRelationships(screen *ebiten.Image) {
	view := a.currentView()
	tableIdx := make(map[string]*dbml.Table, len(a.schema.Tables))
	for i := range a.schema.Tables {
		tableIdx[a.schema.Tables[i].Name] = &a.schema.Tables[i]
	}

	screenBox := func(name string) (render.TableBox, bool) {
		p, ok := view.Tables[name]
		if !ok || p.Hidden {
			return render.TableBox{}, false
		}
		t, ok := tableIdx[name]
		if !ok {
			return render.TableBox{}, false
		}
		box := render.MeasureTable(t)
		sx, sy := a.camera.WorldToScreen(p.X, p.Y)
		return render.TableBox{
			X: sx, Y: sy,
			W: box.W * a.camera.Zoom,
			H: box.H * a.camera.Zoom,
		}, true
	}

	for _, r := range a.schema.Relationships {
		if r.FromTable == r.ToTable {
			continue
		}
		from, ok1 := screenBox(r.FromTable)
		to, ok2 := screenBox(r.ToTable)
		if !ok1 || !ok2 {
			continue
		}
		var clr color.Color = theme.ColorLine
		if style, ok := view.Relationships[r.Key()]; ok {
			if style.Hidden {
				continue
			}
			if c, ok := parseHexColor(style.Color); ok {
				clr = c
			}
		}
		pulse := a.relationshipShouldPulse(r)
		fromCard, toCard := cardinalityKinds(r.Kind)
		render.DrawRelationship(screen, from, to, fromCard, toCard, a.camera.Zoom, a.frameCount, clr, a.width, a.height, pulse)
	}
}

func (a *App) Layout(outsideW, outsideH int) (int, int) {
	a.width = outsideW
	a.height = outsideH
	return outsideW, outsideH
}
