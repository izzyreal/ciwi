//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"image"
	"math"
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
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

const maxDOMGraphButtons = 4096

type domGraphState struct {
	runtime   *giodom.Runtime
	mode      string
	scale     float32
	scaleSet  bool
	selection string
	viewport  graphViewportState
	buttons   map[string]*widget.Clickable
}

func newDOMGraphState(themeRuntime *screenDOMRenderer) *domGraphState {
	return &domGraphState{
		runtime:  giodom.NewRuntime(themeRuntime.theme, giodom.Options{MaxStateSlots: 4096}),
		viewport: graphViewportState{touches: map[pointer.ID]f32.Point{}},
		buttons:  map[string]*widget.Clickable{},
	}
}

func (s *domGraphState) button(key string) *widget.Clickable {
	if button := s.buttons[key]; button != nil {
		return button
	}
	if len(s.buttons) >= maxDOMGraphButtons {
		s.buttons = map[string]*widget.Clickable{}
	}
	button := new(widget.Clickable)
	s.buttons[key] = button
	return button
}

func (r *Renderer) layoutDOMGraphView(gtx layout.Context, node uidsl.Node, data any, path string, state *domGraphState) layout.Dimensions {
	graph := node.GraphView
	if graph == nil {
		return r.errorLabel(gtx, fmt.Errorf("graph-view configuration is missing"))
	}
	stateKey, err := uidsl.RenderText(data, uidsl.Text{Template: graph.StateKey})
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	stateKey = strings.TrimSpace(stateKey)
	if state.mode != "graph" && state.mode != "list" {
		state.mode = r.viewModes[stateKey]
		if state.mode != "graph" && state.mode != "list" {
			state.mode = graph.DefaultMode
			if state.mode != "list" {
				state.mode = "graph"
			}
		}
	}
	r.viewModes[stateKey] = state.mode
	r.persistentViews[stateKey] = true
	heading := "Structure"
	if node.Text != nil {
		if resolved, resolveErr := uidsl.RenderText(data, *node.Text); resolveErr == nil && strings.TrimSpace(resolved) != "" {
			heading = resolved
		}
	}
	graphButton, listButton := state.button("mode/graph"), state.button("mode/list")
	for graphButton.Clicked(gtx) {
		if state.mode != "graph" {
			state.mode = "graph"
			r.viewModes[stateKey] = state.mode
			r.notifyViewChange()
			r.requestFrame()
		}
	}
	for listButton.Clicked(gtx) {
		if state.mode != "list" {
			state.mode = "list"
			r.viewModes[stateKey] = state.mode
			r.notifyViewChange()
			r.requestFrame()
		}
	}
	header := func(gtx layout.Context) layout.Dimensions {
		title := r.materialTextLabel(heading, "heading", true)
		title.Color = r.palette.text
		graphIcon, listIcon := "", ""
		if state.mode == "graph" {
			graphIcon = "check"
		} else {
			listIcon = "check"
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, title.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutDOMControl(gtx, graphButton, "Graph", graphIcon, "", "accent")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutDOMControl(gtx, listButton, "List", listIcon, "", "accent")
				})
			}),
		)
	}
	body := func(gtx layout.Context) layout.Dimensions {
		if state.mode == "list" {
			children := r.compileDOMChildren(node.Children, data, path+"/list")
			return state.runtime.Layout(gtx, giodom.Column(giodom.Key(path+"/list-root"), r.metrics.spaceSmall, children...))
		}
		return r.layoutDOMDefinitionGraph(gtx, node, data, path+"/graph", state)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: r.metrics.spaceMedium}.Layout(gtx, body)
		}),
	)
}

func (r *Renderer) layoutDOMDefinitionGraph(gtx layout.Context, node uidsl.Node, data any, path string, state *domGraphState) layout.Dimensions {
	nodes, err := resolveDefinitionGraphNodes(*node.GraphView, data)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	if len(nodes) == 0 {
		label := r.materialTextLabel("No pipelines configured.", "body", false)
		label.Color = r.palette.muted
		return layout.UniformInset(r.metrics.spaceLarge).Layout(gtx, label.Layout)
	}
	selectedNode := definitionGraphNodeByID(nodes, state.selection)
	if selectedNode != nil && selectedNode.root {
		selectedNode, state.selection = nil, ""
	}
	if len(node.GraphView.Details) > 0 && selectedNode == nil {
		selectedNode = defaultDefinitionGraphNode(nodes)
		if selectedNode != nil {
			state.selection = selectedNode.id
		}
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
	actualScale := state.scale
	if !state.scaleSet || actualScale <= 0 {
		actualScale = fitScale
		state.viewport.center(actualScale, contentWidth, contentHeight, viewportWidth, viewportHeight)
	}
	fitButton, resetButton := state.button("fit"), state.button("reset")
	for fitButton.Clicked(gtx) {
		state.scale, state.scaleSet, actualScale = 0, false, fitScale
		state.viewport.center(actualScale, contentWidth, contentHeight, viewportWidth, viewportHeight)
		r.requestFrame()
	}
	for resetButton.Clicked(gtx) {
		state.scale, state.scaleSet, actualScale = 1, true, 1
		state.viewport.center(actualScale, contentWidth, contentHeight, viewportWidth, viewportHeight)
		r.requestFrame()
	}
	controls := func(gtx layout.Context) layout.Dimensions {
		percent := r.materialTextLabel(fmt.Sprintf("%d%%", int(actualScale*100+0.5)), "body", false)
		percent.Color = r.palette.muted
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutDOMControl(gtx, fitButton, "Fit", "", "", "accent")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutDOMControl(gtx, resetButton, "Reset", "", "", "accent")
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: r.metrics.spaceSmall}.Layout(gtx, percent.Layout)
			}),
		)
	}
	viewport := func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(gtx.Constraints.Max.X, viewportHeight)
		gtx.Constraints = layout.Exact(size)
		if state.viewport.update(gtx, &actualScale, minimumScale, definitionGraphMaxScale, contentWidth, contentHeight, size.X, size.Y) {
			state.scale, state.scaleSet = actualScale, true
			r.requestFrame()
		}
		state.viewport.clamp(actualScale, contentWidth, contentHeight, size.X, size.Y)
		paintDOMSurface(gtx, size, r.palette.surface, r.palette.border, gtx.Dp(1), gtx.Dp(r.metrics.controlRadius))
		stack := clip.Rect{Max: size}.Push(gtx.Ops)
		offset := op.Offset(image.Pt(-int(math.Round(float64(state.viewport.offset.X))), -int(math.Round(float64(state.viewport.offset.Y))))).Push(gtx.Ops)
		r.layoutDOMScaledDefinitionGraph(gtx, node, nodes, path, state, contentWidth, contentHeight, actualScale, nodeWidth, nodeHeight)
		offset.Pop()
		pass := pointer.PassOp{}.Push(gtx.Ops)
		event.Op(gtx.Ops, &state.viewport)
		pass.Pop()
		stack.Pop()
		return layout.Dimensions{Size: size}
	}
	children := []layout.FlexChild{
		layout.Rigid(controls),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: r.metrics.spaceSmall}.Layout(gtx, viewport)
		}),
	}
	if selectedNode != nil && len(node.GraphView.Details) > 0 {
		detailsData := selectedNode.data
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: r.metrics.spaceMedium}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := max(1, gtx.Dp(1))
				divider := image.Pt(gtx.Constraints.Max.X, height)
				paint.FillShape(gtx.Ops, r.palette.border, clip.Rect{Max: divider}.Op())
				return layout.Inset{Top: unit.Dp(float32(height) / gtx.Metric.PxPerDp)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					elements := r.compileDOMChildren(node.GraphView.Details, detailsData, path+"/details/"+state.selection)
					return state.runtime.Layout(gtx, giodom.Column(giodom.Key(path+"/details-root"), r.metrics.spaceMedium, elements...))
				})
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (r *Renderer) layoutDOMScaledDefinitionGraph(gtx layout.Context, owner uidsl.Node, nodes []*definitionGraphNode, path string, state *domGraphState, contentWidth, contentHeight int, scale float32, nodeWidth, nodeHeight int) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	raw := gtx
	raw.Constraints = layout.Exact(image.Pt(contentWidth, contentHeight))
	r.drawDOMDefinitionGraph(raw, owner, nodes, path, state, nodeWidth, nodeHeight)
	call := macro.Stop()
	transform := op.Affine(f32.AffineId().Scale(f32.Point{}, f32.Pt(scale, scale))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	transform.Pop()
	return layout.Dimensions{Size: image.Pt(int(float32(contentWidth)*scale+0.5), int(float32(contentHeight)*scale+0.5))}
}

func (r *Renderer) drawDOMDefinitionGraph(gtx layout.Context, owner uidsl.Node, nodes []*definitionGraphNode, path string, state *domGraphState, nodeWidth, nodeHeight int) {
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
		r.layoutDOMDefinitionGraphNode(nodeContext, owner, graphNode, path+"/node/"+graphNode.id, state, graphNode.id == state.selection)
		offset.Pop()
	}
}

func (r *Renderer) layoutDOMDefinitionGraphNode(gtx layout.Context, owner uidsl.Node, graphNode *definitionGraphNode, path string, state *domGraphState, selected bool) layout.Dimensions {
	selectable := !graphNode.root && len(owner.GraphView.Details) > 0
	selector, play := state.button(path+"/select"), state.button(path+"/run")
	actions := owner.Actions
	if graphNode.root {
		actions = owner.GraphView.Root.Actions
		if !conditionEnabled(owner.GraphView.Root.ActionVisible, graphNode.data) {
			actions = nil
		}
	}
	playActivated := false
	for play.Clicked(gtx) {
		playActivated = true
		if len(actions) > 0 {
			r.dispatch(actions[0], graphNode.data)
		}
	}
	for selector.Clicked(gtx) {
		if selectable && !playActivated {
			state.selection = graphNode.id
			r.requestFrame()
		}
	}
	border, background, borderWidth := definitionGraphNodeSurface(r.palette, selectable && selector.Hovered(), selected)
	if graphNode.root {
		border = r.palette.accent
		background = mixColorSRGB(r.palette.surface, r.palette.accent, .08)
	}
	content := func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paintDOMSurface(gtx, size, background, border, max(1, gtx.Dp(borderWidth)), gtx.Dp(r.metrics.controlRadius))
			return layout.Dimensions{Size: size}
		}, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				title := r.materialTextLabel(graphNode.label, "code-inline", true)
				title.MaxLines, title.Color = 1, r.palette.text
				meta := r.materialTextLabel(graphNode.meta, "badge", false)
				meta.MaxLines, meta.Color = 1, r.palette.muted
				copy := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(title.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Inset{Top: 6}.Layout(gtx, meta.Layout) }),
					)
				})
				children := []layout.FlexChild{copy}
				if len(actions) > 0 {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return r.layoutGraphPlayButton(gtx, play, "Run "+graphNode.label+" as a new execution")
						})
					}))
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
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
