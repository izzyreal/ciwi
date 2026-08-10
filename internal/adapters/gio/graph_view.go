//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

const (
	definitionGraphMinScale = float32(0.45)
	definitionGraphMaxScale = float32(1.5)
)

type graphViewportState struct {
	offset        f32.Point
	touches       map[pointer.ID]f32.Point
	gestureActive bool
	lastCentroid  f32.Point
	lastDistance  float32
}

func (s *graphViewportState) update(gtx layout.Context, scale *float32, minScale, maxScale float32, contentWidth, contentHeight, viewportWidth, viewportHeight int) bool {
	if s.touches == nil {
		s.touches = map[pointer.ID]f32.Point{}
	}
	changed := false
	scaledWidth := int(float32(contentWidth)**scale + 0.5)
	scaledHeight := int(float32(contentHeight)**scale + 0.5)
	xRange := graphScrollRange(s.offset.X, scaledWidth, viewportWidth)
	yRange := graphScrollRange(s.offset.Y, scaledHeight, viewportHeight)
	filter := pointer.Filter{
		Target:  s,
		Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
		ScrollX: xRange, ScrollY: yRange,
	}
	for {
		raw, ok := gtx.Event(filter)
		if !ok {
			break
		}
		e, ok := raw.(pointer.Event)
		if !ok {
			continue
		}
		if e.Kind == pointer.Scroll {
			s.offset.X += e.Scroll.X
			s.offset.Y += e.Scroll.Y
			changed = changed || e.Scroll.X != 0 || e.Scroll.Y != 0
			continue
		}
		if e.Source != pointer.Touch {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if s.pressTouch(e.PointerID, e.Position) {
				for id := range s.touches {
					gtx.Execute(pointer.GrabCmd{Tag: s, ID: id})
				}
			}
		case pointer.Drag:
			changed = s.dragTouch(e.PointerID, e.Position, scale, minScale, maxScale) || changed
		case pointer.Release, pointer.Cancel:
			s.endTouch(e.PointerID)
		}
	}
	s.clamp(*scale, contentWidth, contentHeight, viewportWidth, viewportHeight)
	return changed
}

func (s *graphViewportState) pressTouch(id pointer.ID, position f32.Point) bool {
	if s.touches == nil {
		s.touches = map[pointer.ID]f32.Point{}
	}
	if _, reused := s.touches[id]; reused {
		// iOS may reuse pointer IDs after ending one member of a multi-touch
		// sequence. A duplicate press marks the retained state as stale.
		s.resetTouches()
	}
	if len(s.touches) >= 2 {
		return false
	}
	s.touches[id] = position
	if len(s.touches) != 2 {
		return false
	}
	s.gestureActive = true
	s.lastCentroid, s.lastDistance = graphTouchGeometry(s.touches)
	return true
}

func (s *graphViewportState) dragTouch(id pointer.ID, position f32.Point, scale *float32, minScale, maxScale float32) bool {
	if !s.gestureActive {
		return false
	}
	if _, tracked := s.touches[id]; !tracked {
		return false
	}
	s.touches[id] = position
	if len(s.touches) != 2 {
		s.resetTouches()
		return false
	}
	centroid, distance := graphTouchGeometry(s.touches)
	changed := s.transformTouch(scale, centroid, distance, minScale, maxScale)
	s.lastCentroid, s.lastDistance = centroid, distance
	return changed
}

func (s *graphViewportState) endTouch(id pointer.ID) {
	if _, tracked := s.touches[id]; !tracked {
		return
	}
	if s.gestureActive {
		// A pinch is one atomic gesture. Keeping the other participant around
		// lets reused iOS pointer IDs turn a later one-finger drag into a pinch.
		s.resetTouches()
		return
	}
	delete(s.touches, id)
	s.lastCentroid = f32.Point{}
	s.lastDistance = 0
}

func (s *graphViewportState) resetTouches() {
	clear(s.touches)
	s.gestureActive = false
	s.lastCentroid = f32.Point{}
	s.lastDistance = 0
}

func (s *graphViewportState) transformTouch(scale *float32, centroid f32.Point, distance, minScale, maxScale float32) bool {
	if s.lastDistance <= 0 || distance <= 0 {
		return false
	}
	oldScale := *scale
	nextScale := min(maxScale, max(minScale, oldScale*distance/s.lastDistance))
	contentPoint := f32.Pt(
		(s.lastCentroid.X+s.offset.X)/oldScale,
		(s.lastCentroid.Y+s.offset.Y)/oldScale,
	)
	s.offset = f32.Pt(contentPoint.X*nextScale-centroid.X, contentPoint.Y*nextScale-centroid.Y)
	*scale = nextScale
	return true
}

func graphTouchGeometry(touches map[pointer.ID]f32.Point) (f32.Point, float32) {
	points := make([]f32.Point, 0, 2)
	for _, point := range touches {
		points = append(points, point)
		if len(points) == 2 {
			break
		}
	}
	if len(points) != 2 {
		return f32.Point{}, 0
	}
	centroid := f32.Pt((points[0].X+points[1].X)/2, (points[0].Y+points[1].Y)/2)
	dx, dy := points[0].X-points[1].X, points[0].Y-points[1].Y
	return centroid, float32(math.Hypot(float64(dx), float64(dy)))
}

func graphScrollRange(offset float32, content, viewport int) pointer.ScrollRange {
	if content <= viewport {
		return pointer.ScrollRange{}
	}
	return pointer.ScrollRange{Min: int(-offset), Max: int(float32(content-viewport) - offset)}
}

func (s *graphViewportState) clamp(scale float32, contentWidth, contentHeight, viewportWidth, viewportHeight int) {
	s.offset.X = clampGraphOffset(s.offset.X, float32(contentWidth)*scale, float32(viewportWidth))
	s.offset.Y = clampGraphOffset(s.offset.Y, float32(contentHeight)*scale, float32(viewportHeight))
}

func clampGraphOffset(offset, content, viewport float32) float32 {
	if content <= viewport {
		return -(viewport - content) / 2
	}
	return min(content-viewport, max(0, offset))
}

func (s *graphViewportState) center(scale float32, contentWidth, contentHeight, viewportWidth, viewportHeight int) {
	s.offset = f32.Pt(
		(float32(contentWidth)*scale-float32(viewportWidth))/2,
		(float32(contentHeight)*scale-float32(viewportHeight))/2,
	)
	s.clamp(scale, contentWidth, contentHeight, viewportWidth, viewportHeight)
}

type definitionGraphNode struct {
	id, label, meta string
	dependencies    []string
	data            any
	root            bool
	level, row      int
	x, y            int
}

func (r *Renderer) notifyViewChange() {
	if r.onViewChange == nil {
		return
	}
	states := make(map[string]string, len(r.persistentViews))
	for key := range r.persistentViews {
		states[key] = r.viewModes[key]
	}
	r.onViewChange(states)
}

func definitionGraphFitScale(viewportWidth, viewportHeight, contentWidth, contentHeight, padding int) float32 {
	scale := min(
		float32(max(1, viewportWidth-2*padding))/float32(max(1, contentWidth)),
		float32(max(1, viewportHeight-2*padding))/float32(max(1, contentHeight)),
	)
	return min(definitionGraphMaxScale, max(0.01, scale))
}

func definitionGraphNodeByID(nodes []*definitionGraphNode, id string) *definitionGraphNode {
	for _, node := range nodes {
		if node.id == id {
			return node
		}
	}
	return nil
}

func defaultDefinitionGraphNode(nodes []*definitionGraphNode) *definitionGraphNode {
	for _, node := range nodes {
		if node.root {
			continue
		}
		hasRegularDependency := false
		for _, dependency := range node.dependencies {
			if !strings.HasPrefix(dependency, "__root__:") {
				hasRegularDependency = true
				break
			}
		}
		if !hasRegularDependency {
			return node
		}
	}
	for _, node := range nodes {
		if !node.root {
			return node
		}
	}
	return nil
}

func resolveDefinitionGraphNodes(graph uidsl.GraphView, data any) ([]*definitionGraphNode, error) {
	raw, err := uidsl.Resolve(data, graph.Nodes)
	if err != nil {
		return nil, err
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("graph nodes binding %q is not a list", graph.Nodes)
	}
	nodes := make([]*definitionGraphNode, 0, len(items))
	for _, item := range items {
		nodeData := mergeData(data, graph.As, item)
		key, keyErr := uidsl.Resolve(nodeData, graph.NodeKey)
		if keyErr != nil {
			return nil, keyErr
		}
		label, labelErr := uidsl.RenderText(nodeData, graph.NodeLabel)
		if labelErr != nil {
			return nil, labelErr
		}
		meta := ""
		if graph.NodeMeta != (uidsl.Text{}) {
			meta, err = uidsl.RenderText(nodeData, graph.NodeMeta)
			if err != nil {
				return nil, err
			}
		}
		var dependencies any
		if graph.Dependencies != "" {
			dependencies, err = uidsl.Resolve(nodeData, graph.Dependencies)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, &definitionGraphNode{
			id: strings.TrimSpace(fmt.Sprint(key)), label: label, meta: meta,
			dependencies: stringSlice(dependencies), data: nodeData,
		})
	}
	if graph.Root != nil {
		rootValue, rootErr := uidsl.Resolve(data, graph.Root.Binding)
		if rootErr != nil {
			return nil, rootErr
		}
		rootData := mergeData(data, graph.Root.As, rootValue)
		key, keyErr := uidsl.Resolve(rootData, graph.Root.Key)
		if keyErr != nil {
			return nil, keyErr
		}
		keyText := strings.TrimSpace(fmt.Sprint(key))
		if keyText == "" {
			return nil, fmt.Errorf("graph root key %q resolved empty", graph.Root.Key)
		}
		label, labelErr := uidsl.RenderText(rootData, graph.Root.Label)
		if labelErr != nil {
			return nil, labelErr
		}
		meta := ""
		if graph.Root.Meta != (uidsl.Text{}) {
			meta, rootErr = uidsl.RenderText(rootData, graph.Root.Meta)
			if rootErr != nil {
				return nil, rootErr
			}
		}
		rootID := "__root__:" + keyText
		rootNode := &definitionGraphNode{id: rootID, label: label, meta: meta, data: rootData, root: true}
		regularIDs := make(map[string]bool, len(nodes))
		for _, regular := range nodes {
			regularIDs[regular.id] = true
		}
		for _, regular := range nodes {
			hasVisibleDependency := false
			for _, dependency := range regular.dependencies {
				if regularIDs[dependency] {
					hasVisibleDependency = true
					break
				}
			}
			if !hasVisibleDependency {
				regular.dependencies = append(regular.dependencies, rootID)
			}
		}
		nodes = append([]*definitionGraphNode{rootNode}, nodes...)
	}
	return nodes, nil
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if typed, typedOK := value.([]string); typedOK {
			return append([]string(nil), typed...)
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func layoutDefinitionGraph(nodes []*definitionGraphNode, nodeWidth, nodeHeight, gapX, gapY, padding int) (int, int) {
	byID := make(map[string]*definitionGraphNode, len(nodes))
	for _, node := range nodes {
		byID[node.id] = node
	}
	states := map[string]uint8{}
	var level func(*definitionGraphNode) int
	level = func(node *definitionGraphNode) int {
		if states[node.id] == 2 {
			return node.level
		}
		if states[node.id] == 1 {
			return 0
		}
		states[node.id] = 1
		for _, dependency := range node.dependencies {
			if parent := byID[dependency]; parent != nil {
				node.level = max(node.level, level(parent)+1)
			}
		}
		states[node.id] = 2
		return node.level
	}
	maxLevel := 0
	levels := map[int][]*definitionGraphNode{}
	for _, node := range nodes {
		maxLevel = max(maxLevel, level(node))
		levels[node.level] = append(levels[node.level], node)
	}
	maxRows := 1
	for _, levelNodes := range levels {
		maxRows = max(maxRows, len(levelNodes))
	}
	for column, levelNodes := range levels {
		sort.SliceStable(levelNodes, func(i, j int) bool { return levelNodes[i].id < levelNodes[j].id })
		topRows := (maxRows - len(levelNodes)) / 2
		for row, node := range levelNodes {
			node.row = row
			node.x = padding + column*(nodeWidth+gapX)
			node.y = padding + (topRows+row)*(nodeHeight+gapY)
		}
	}
	return 2*padding + (maxLevel+1)*nodeWidth + maxLevel*gapX,
		2*padding + maxRows*nodeHeight + (maxRows-1)*gapY
}

func definitionGraphNodeSurface(colors palette, hovered, selected bool) (border, background color.NRGBA, borderWidth unit.Dp) {
	border = colors.border
	background = colors.surface
	borderWidth = 1
	if hovered {
		border = colors.accent
		background = graphNodeHoverFill(colors.surface, colors.accent)
	}
	if selected {
		border = colors.focus
		borderWidth = 2
	}
	return border, background, borderWidth
}

func graphNodeHoverFill(surface, accent color.NRGBA) color.NRGBA {
	const accentWeight = uint32(30)
	const surfaceWeight = uint32(255) - accentWeight
	mix := func(base, tint uint8) uint8 {
		return uint8((uint32(base)*surfaceWeight + uint32(tint)*accentWeight + 127) / 255)
	}
	return color.NRGBA{
		R: mix(surface.R, accent.R),
		G: mix(surface.G, accent.G),
		B: mix(surface.B, accent.B),
		A: surface.A,
	}
}

func (r *Renderer) layoutGraphPlayButton(gtx layout.Context, button *widget.Clickable, description string) layout.Dimensions {
	icon := r.icons["player-play"]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", "player-play"))
	}
	background := r.palette.surface
	borderColor := r.palette.border
	if button.Hovered() {
		background = r.palette.subtle
		borderColor = r.palette.accent
	}
	if gtx.Focused(button) {
		borderColor = r.palette.focus
	}
	const size unit.Dp = 34
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(size), gtx.Dp(size)))
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		paint.FillShape(gtx.Ops, borderColor, clip.Ellipse(bounds).Op(gtx.Ops))
		borderWidth := max(1, gtx.Dp(1))
		inner := bounds.Inset(borderWidth)
		paint.FillShape(gtx.Ops, background, clip.Ellipse(inner).Op(gtx.Ops))
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}, func(gtx layout.Context) layout.Dimensions {
		return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp(description).Add(gtx.Ops)
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(17), gtx.Dp(17)))
				return icon.Layout(gtx, r.palette.accent)
			})
		})
	})
}

func clampGraphScale(scale float32) float32 {
	return min(definitionGraphMaxScale, max(definitionGraphMinScale, scale))
}
