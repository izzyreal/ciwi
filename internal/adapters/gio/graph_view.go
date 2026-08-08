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
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
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
	level, row      int
	x, y            int
}

func (r *Renderer) layoutGraphView(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	graph := node.GraphView
	if graph == nil {
		return r.errorLabel(gtx, fmt.Errorf("graph-view configuration is missing"))
	}
	stateKey, err := uidsl.RenderText(data, uidsl.Text{Template: graph.StateKey})
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	stateKey = strings.TrimSpace(stateKey)
	mode := r.viewModes[stateKey]
	if mode != "graph" && mode != "list" {
		mode = graph.DefaultMode
		if mode != "list" {
			mode = "graph"
		}
		r.viewModes[stateKey] = mode
	}
	r.persistentViews[stateKey] = true

	setMode := func(next string) {
		if next == mode {
			return
		}
		mode = next
		r.viewModes[stateKey] = next
		r.notifyViewChange()
		r.requestFrame()
	}
	heading := "Structure"
	if node.Text != nil {
		if resolved, resolveErr := uidsl.RenderText(data, *node.Text); resolveErr == nil && strings.TrimSpace(resolved) != "" {
			heading = resolved
		}
	}
	graphButton := r.button(path + "/mode/graph")
	listButton := r.button(path + "/mode/list")
	for r.clicked(gtx, graphButton) {
		r.markInteraction(path + "/mode/graph")
		setMode("graph")
	}
	for r.clicked(gtx, listButton) {
		r.markInteraction(path + "/mode/list")
		setMode("list")
	}
	header := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				textNode := uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: heading}, Style: uidsl.Style{Role: "heading", Emphasis: "strong"}}
				return r.layoutText(gtx, textNode, data, path+"/heading")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				icon := ""
				if mode == "graph" {
					icon = "check"
				}
				return r.layoutControlButton(gtx, graphButton, "Graph", icon, true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					icon := ""
					if mode == "list" {
						icon = "check"
					}
					return r.layoutControlButton(gtx, listButton, "List", icon, true)
				})
			}),
		)
	}
	body := func(gtx layout.Context) layout.Dimensions {
		if mode == "list" {
			return r.layoutChildren(gtx, node, data, path+"/list")
		}
		return r.layoutDefinitionGraph(gtx, node, data, path+"/graph", stateKey)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: r.metrics.spaceMedium}.Layout(gtx, body)
		}),
	)
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

func (r *Renderer) layoutDefinitionGraph(gtx layout.Context, node uidsl.Node, data any, path, stateKey string) layout.Dimensions {
	nodes, err := resolveDefinitionGraphNodes(*node.GraphView, data)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	if len(nodes) == 0 {
		textNode := uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: "No pipelines configured."}, Style: uidsl.Style{Tone: "muted"}}
		return layout.UniformInset(r.metrics.spaceLarge).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutText(gtx, textNode, data, path+"/empty")
		})
	}
	selectedID := r.graphSelections[stateKey]
	selectedNode := definitionGraphNodeByID(nodes, selectedID)
	if len(node.GraphView.Details) > 0 && selectedNode == nil {
		selectedNode = defaultDefinitionGraphNode(nodes)
		selectedID = selectedNode.id
		r.graphSelections[stateKey] = selectedID
	}
	nodeWidth, nodeHeight := gtx.Dp(210), gtx.Dp(76)
	gapX, gapY, padding := gtx.Dp(58), gtx.Dp(24), gtx.Dp(16)
	contentWidth, contentHeight := layoutDefinitionGraph(nodes, nodeWidth, nodeHeight, gapX, gapY, padding)
	viewportWidth := max(1, gtx.Constraints.Max.X)
	viewportHeight := gtx.Dp(420)
	if r.viewportSize.Y > 0 {
		viewportHeight = min(viewportHeight, max(gtx.Dp(180), r.viewportSize.Y-gtx.Dp(220)))
	}
	if gtx.Constraints.Max.Y > 0 && gtx.Constraints.Max.Y < 1_000_000 {
		viewportHeight = min(viewportHeight, gtx.Constraints.Max.Y)
	}
	viewportHeight = max(1, viewportHeight)
	fitScale := definitionGraphFitScale(viewportWidth, viewportHeight, contentWidth, contentHeight, padding)
	minimumScale := min(definitionGraphMinScale, fitScale)
	requestedScale, exists := r.graphScales[stateKey]
	actualScale := requestedScale
	if !exists || requestedScale <= 0 {
		actualScale = fitScale
	}
	graphViewport := r.graphViewports[stateKey]
	if graphViewport == nil {
		graphViewport = &graphViewportState{touches: map[pointer.ID]f32.Point{}}
		r.graphViewports[stateKey] = graphViewport
	}
	if !exists || requestedScale <= 0 {
		graphViewport.center(actualScale, contentWidth, contentHeight, viewportWidth, viewportHeight)
	}

	fitButton := r.button(path + "/fit")
	resetButton := r.button(path + "/reset")
	for r.clicked(gtx, fitButton) {
		r.markInteraction(path + "/fit")
		r.graphScales[stateKey] = 0
		actualScale = fitScale
		graphViewport.center(actualScale, contentWidth, contentHeight, viewportWidth, viewportHeight)
		r.requestFrame()
	}
	for r.clicked(gtx, resetButton) {
		r.markInteraction(path + "/reset")
		r.graphScales[stateKey] = 1
		actualScale = 1
		graphViewport.center(actualScale, contentWidth, contentHeight, viewportWidth, viewportHeight)
		r.requestFrame()
	}
	controls := func(gtx layout.Context) layout.Dimensions {
		percent := fmt.Sprintf("%d%%", int(actualScale*100+0.5))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutControlButton(gtx, fitButton, "Fit", "", true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutControlButton(gtx, resetButton, "Reset", "", true)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					textNode := uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: percent}, Style: uidsl.Style{Tone: "muted"}}
					return r.layoutText(gtx, textNode, data, path+"/scale")
				})
			}),
		)
	}
	viewport := func(gtx layout.Context) layout.Dimensions {
		return r.layoutCachedBorder(gtx, r.palette.border, r.metrics.controlRadius, 1, func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Constraints.Max.X, viewportHeight)
			gtx.Constraints = layout.Exact(size)
			if graphViewport.update(gtx, &actualScale, minimumScale, definitionGraphMaxScale, contentWidth, contentHeight, size.X, size.Y) {
				r.graphScales[stateKey] = actualScale
				r.requestFrame()
			}
			graphViewport.clamp(actualScale, contentWidth, contentHeight, size.X, size.Y)
			stack := clip.Rect{Max: size}.Push(gtx.Ops)
			event.Op(gtx.Ops, graphViewport)
			offset := op.Offset(image.Pt(-int(math.Round(float64(graphViewport.offset.X))), -int(math.Round(float64(graphViewport.offset.Y))))).Push(gtx.Ops)
			pass := pointer.PassOp{}.Push(gtx.Ops)
			r.layoutScaledDefinitionGraph(gtx, node, nodes, data, path, stateKey, selectedID, contentWidth, contentHeight, actualScale, nodeWidth, nodeHeight)
			pass.Pop()
			offset.Pop()
			stack.Pop()
			return layout.Dimensions{Size: size}
		})
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart}.Layout(gtx, layout.Rigid(controls))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: r.metrics.spaceSmall}.Layout(gtx, viewport)
		}),
	}
	if selectedNode != nil && len(node.GraphView.Details) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: r.metrics.spaceMedium}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutGraphDetails(gtx, node.GraphView.Details, selectedNode.data, path+"/details/"+selectedNode.id)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
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
		if len(node.dependencies) == 0 {
			return node
		}
	}
	return nodes[0]
}

func (r *Renderer) layoutGraphDetails(gtx layout.Context, details []uidsl.Node, data any, path string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := max(1, gtx.Dp(1))
			size := image.Pt(gtx.Constraints.Max.X, height)
			paint.FillShape(gtx.Ops, r.palette.border, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: r.metrics.spaceMedium}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				container := uidsl.Node{Component: "column", Layout: uidsl.Layout{Direction: "vertical", Gap: "medium"}, Children: details}
				return r.layoutChildren(gtx, container, data, path)
			})
		}),
	)
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
		dependencies, dependencyErr := uidsl.Resolve(nodeData, graph.Dependencies)
		if dependencyErr != nil {
			return nil, dependencyErr
		}
		nodes = append(nodes, &definitionGraphNode{
			id: strings.TrimSpace(fmt.Sprint(key)), label: label, meta: meta,
			dependencies: stringSlice(dependencies), data: nodeData,
		})
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

func (r *Renderer) layoutScaledDefinitionGraph(
	gtx layout.Context,
	owner uidsl.Node,
	nodes []*definitionGraphNode,
	data any,
	path string,
	stateKey, selectedID string,
	contentWidth, contentHeight int,
	scale float32,
	nodeWidth, nodeHeight int,
) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	raw := gtx
	raw.Constraints = layout.Exact(image.Pt(contentWidth, contentHeight))
	r.drawDefinitionGraph(raw, owner, nodes, data, path, stateKey, selectedID, nodeWidth, nodeHeight)
	call := macro.Stop()
	transform := op.Affine(f32.AffineId().Scale(f32.Point{}, f32.Pt(scale, scale))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	transform.Pop()
	return layout.Dimensions{Size: image.Pt(int(float32(contentWidth)*scale+0.5), int(float32(contentHeight)*scale+0.5))}
}

func (r *Renderer) drawDefinitionGraph(gtx layout.Context, owner uidsl.Node, nodes []*definitionGraphNode, data any, path, stateKey, selectedID string, nodeWidth, nodeHeight int) {
	byID := make(map[string]*definitionGraphNode, len(nodes))
	for _, node := range nodes {
		byID[node.id] = node
	}
	for _, node := range nodes {
		for _, dependency := range node.dependencies {
			parent := byID[dependency]
			if parent == nil {
				continue
			}
			start := f32.Pt(float32(parent.x+nodeWidth), float32(parent.y+nodeHeight/2))
			end := f32.Pt(float32(node.x), float32(node.y+nodeHeight/2))
			middle := (start.X + end.X) / 2
			var edge clip.Path
			edge.Begin(gtx.Ops)
			edge.MoveTo(start)
			edge.CubeTo(f32.Pt(middle, start.Y), f32.Pt(middle, end.Y), end)
			paint.FillShape(gtx.Ops, r.palette.border, clip.Stroke{Path: edge.End(), Width: 2}.Op())
			var arrow clip.Path
			arrow.Begin(gtx.Ops)
			arrow.MoveTo(end)
			arrow.LineTo(f32.Pt(end.X-8, end.Y-5))
			arrow.LineTo(f32.Pt(end.X-8, end.Y+5))
			arrow.Close()
			paint.FillShape(gtx.Ops, r.palette.border, clip.Outline{Path: arrow.End()}.Op())
		}
	}
	for _, graphNode := range nodes {
		offset := op.Offset(image.Pt(graphNode.x, graphNode.y)).Push(gtx.Ops)
		nodeContext := gtx
		nodeContext.Constraints = layout.Exact(image.Pt(nodeWidth, nodeHeight))
		r.layoutDefinitionGraphNode(nodeContext, owner, graphNode, data, path+"/node/"+graphNode.id, stateKey, graphNode.id == selectedID)
		offset.Pop()
	}
}

func (r *Renderer) layoutDefinitionGraphNode(gtx layout.Context, owner uidsl.Node, graphNode *definitionGraphNode, data any, path, stateKey string, selected bool) layout.Dimensions {
	selectable := len(owner.GraphView.Details) > 0
	selector := r.button(path + "/select")
	play := r.button(path + "/run")
	playActivated := false
	if len(owner.Actions) > 0 {
		for r.clicked(gtx, play) {
			playActivated = true
			r.markInteraction(path + "/run")
			r.dispatch(owner.Actions[0], graphNode.data)
		}
	}
	if selectable {
		for r.clicked(gtx, selector) {
			r.markInteraction(path + "/select")
			if !playActivated {
				r.graphSelections[stateKey] = graphNode.id
				r.requestFrame()
			}
		}
	}
	borderColor, background, borderWidth := definitionGraphNodeSurface(r.palette, selectable && selector.Hovered(), selected)
	content := func(gtx layout.Context) layout.Dimensions {
		return r.layoutCachedBorder(gtx, borderColor, r.metrics.controlRadius, borderWidth, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				r.paintCachedRoundedFill(gtx, gtx.Constraints.Min, r.metrics.controlRadius, background)
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := []layout.FlexChild{layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								title := uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: graphNode.label}, Style: uidsl.Style{Role: "code-inline", Emphasis: "strong", Truncate: true}}
								return r.layoutText(gtx, title, graphNode.data, path+"/title")
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									meta := uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: graphNode.meta}, Style: uidsl.Style{Role: "badge", Tone: "muted", Truncate: true}}
									return r.layoutText(gtx, meta, graphNode.data, path+"/meta")
								})
							}),
						)
					})}
					if len(owner.Actions) > 0 {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return r.layoutGraphPlayButton(gtx, play, "Run "+graphNode.label+" as a new execution. Existing queued and running work is not interrupted.")
							})
						}))
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
			})
		})
	}
	if !selectable {
		return content(gtx)
	}
	return selector.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.DescriptionOp("Select " + graphNode.label).Add(gtx.Ops)
		return content(gtx)
	})
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
