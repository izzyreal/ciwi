package giodom

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

type measurement struct {
	size     int
	lastUsed uint64
	index    int
}

type keyedViewportState struct {
	scroll           gesture.Scroll
	boundaryGate     scrollBoundaryGate
	anchor           Key
	anchorIndex      int
	anchorOffset     int
	measurements     map[Key]measurement
	clock            uint64
	atEnd            bool
	initialized      bool
	indexed          bool
	revision         uint64
	count            int
	estimate         int
	gap              int
	crossSize        int
	extents          []int
	prefixTree       []int
	scrollRevision   uint64
	forceEndRevision uint64
	resetRevision    uint64
	reachedStartKey  Key
	reachedEndKey    Key
}

// scrollBoundaryGate keeps a nested touch scroller from grabbing a gesture
// that starts by moving out through one of its exhausted boundaries. The
// pass-through parent can then claim the same drag. Interior drags continue to
// use Gio's gesture.Scroll, including its normal touch slop and fling behavior.
type scrollBoundaryGate struct {
	tracking bool
	delegate bool
	pid      pointer.ID
	last     float32
}

func (gate *scrollBoundaryGate) Add(ops *op.Ops) {
	event.Op(ops, gate)
}

func (gate *scrollBoundaryGate) Update(source input.Source, axis gesture.Axis, xRange, yRange pointer.ScrollRange) bool {
	delegated := gate.delegate
	filter := pointer.Filter{Target: gate, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel}
	for {
		raw, ok := source.Event(filter)
		if !ok {
			break
		}
		pointerEvent, ok := raw.(pointer.Event)
		if !ok {
			continue
		}
		switch pointerEvent.Kind {
		case pointer.Press:
			if pointerEvent.Source != pointer.Touch || gate.tracking {
				continue
			}
			gate.tracking, gate.delegate, gate.pid = true, false, pointerEvent.PointerID
			gate.last = scrollAxisPosition(axis, pointerEvent.Position.X, pointerEvent.Position.Y)
		case pointer.Drag:
			if !gate.tracking || gate.pid != pointerEvent.PointerID || gate.delegate || pointerEvent.Priority == pointer.Grabbed {
				continue
			}
			current := scrollAxisPosition(axis, pointerEvent.Position.X, pointerEvent.Position.Y)
			delta := gate.last - current
			gate.last = current
			rangeForAxis := yRange
			if axis == gesture.Horizontal {
				rangeForAxis = xRange
			}
			if (delta > 0 && rangeForAxis.Max <= 0) || (delta < 0 && rangeForAxis.Min >= 0) {
				gate.delegate = true
				delegated = true
			}
		case pointer.Release, pointer.Cancel:
			if gate.pid == pointerEvent.PointerID {
				gate.tracking, gate.delegate = false, false
			}
		}
	}
	return delegated
}

func scrollAxisPosition(axis gesture.Axis, x, y float32) float32 {
	if axis == gesture.Horizontal {
		return x
	}
	return y
}

type stockListState struct {
	list           layout.List
	revision       uint64
	scrollRevision uint64
}

type recordedViewportChild struct {
	call op.CallOp
	pos  image.Point
	size image.Point
}

// passThroughScrollContext is available only while a pass-through viewport
// lays out its children. Opt-in regions use a foremost hit gate to let this
// stationary viewport claim touch-axis drags before an editor without changing
// mouse or tap handling.
type passThroughScrollContext struct {
	axis           gesture.Axis
	xRange, yRange pointer.ScrollRange
	scroll         *gesture.Scroll
	delta          int
	updated        bool
}

type passThroughScrollRegionState struct {
	gate passThroughScrollGate
}

type passThroughScrollGate struct {
	tracking bool
	pid      pointer.ID
	scroll   float32
}

func (context *passThroughScrollContext) update(gtx layout.Context) {
	if context.updated {
		return
	}
	context.delta += context.scroll.Update(gtx.Metric, gtx.Source, gtx.Now, context.axis, context.xRange, context.yRange)
	context.updated = true
}

func (gate *passThroughScrollGate) Add(ops *op.Ops) {
	event.Op(ops, gate)
}

func (gate *passThroughScrollGate) Update(source input.Source, axis gesture.Axis, xRange, yRange pointer.ScrollRange) (bool, int) {
	prioritizeParent := gate.tracking
	scrollDelta := 0
	filter := pointer.Filter{
		Target: gate, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll | pointer.Cancel,
		ScrollX: xRange, ScrollY: yRange,
	}
	for {
		raw, ok := source.Event(filter)
		if !ok {
			break
		}
		pointerEvent, ok := raw.(pointer.Event)
		if !ok {
			continue
		}
		switch pointerEvent.Kind {
		case pointer.Press:
			if pointerEvent.Source == pointer.Touch && !gate.tracking {
				gate.tracking, gate.pid = true, pointerEvent.PointerID
				prioritizeParent = true
			}
		case pointer.Drag:
			if gate.tracking && gate.pid == pointerEvent.PointerID {
				prioritizeParent = true
			}
		case pointer.Release, pointer.Cancel:
			if gate.tracking && gate.pid == pointerEvent.PointerID {
				prioritizeParent = true
				gate.tracking = false
			}
		case pointer.Scroll:
			if axis == gesture.Horizontal {
				gate.scroll += pointerEvent.Scroll.X
			} else {
				gate.scroll += pointerEvent.Scroll.Y
			}
			delta := int(gate.scroll)
			gate.scroll -= float32(delta)
			scrollDelta += delta
		}
	}
	return prioritizeParent, scrollDelta
}

func (r *Runtime) layoutVirtualList(gtx layout.Context, element Element, identity string) layout.Dimensions {
	children := element.Children
	if children == nil {
		return layout.Dimensions{}
	}
	props := normalizedListProps(element.List)
	viewport := listViewportSize(gtx, props)
	if !r.validateDynamicKeys(identity, children) {
		return layout.Dimensions{Size: viewport}
	}
	state := r.useState(identity, "viewport", KindVirtualList, func() any {
		return &keyedViewportState{measurements: make(map[Key]measurement)}
	}).(*keyedViewportState)
	if props.ResetRevision != 0 && props.ResetRevision != state.resetRevision {
		state.anchor = ""
		state.anchorIndex = 0
		state.anchorOffset = 0
		state.atEnd = false
		state.initialized = false
		state.resetRevision = props.ResetRevision
	}

	mainViewport := axisMain(props.Axis, viewport)
	if r.rejectGeometry(identity, viewport.X, viewport.Y, mainViewport) || mainViewport == 0 {
		return layout.Dimensions{Size: viewport}
	}
	estimate := max(1, gtx.Dp(props.Estimate))
	gap := max(0, gtx.Dp(props.Gap))
	state.reconcileIndex(children, estimate, gap, axisCross(props.Axis, viewport))
	if props.ShrinkMain {
		viewport = r.intrinsicListViewport(gtx, props, viewport, children, state, identity, gap)
	}
	mainViewport = axisMain(props.Axis, viewport)
	if props.ScrollTo != "" && props.ScrollRevision != state.scrollRevision {
		for index := 0; index < children.Len(); index++ {
			if children.KeyAt(index) == props.ScrollTo {
				state.anchor = props.ScrollTo
				state.anchorIndex = index
				state.anchorOffset = 0
				break
			}
		}
		state.scrollRevision = props.ScrollRevision
	}
	forceEnd := props.ScrollToEnd && props.ForceEndRevision != 0 && props.ForceEndRevision != state.forceEndRevision
	followEnd := props.ScrollToEnd && (!state.initialized || state.atEnd || forceEnd)
	if followEnd {
		r.measureListTail(gtx, props, viewport, children, state, identity, gap)
	}
	total := state.totalExtent()

	anchorIndex := state.anchorIndex
	if state.anchor == "" {
		anchorIndex = -1
	}
	if anchorIndex < 0 && children.Len() > 0 {
		anchorIndex = min(max(0, state.anchorIndex), children.Len()-1)
		state.anchor = children.KeyAt(anchorIndex)
		state.anchorOffset = 0
	}
	offset := 0
	if anchorIndex >= 0 {
		offset = state.prefixAt(anchorIndex) + max(0, state.anchorOffset)
	}
	maxOffset := max(0, total-mainViewport)
	if followEnd {
		offset = maxOffset
	}
	if forceEnd {
		state.forceEndRevision = props.ForceEndRevision
	}
	offset = min(max(0, offset), maxOffset)

	var xRange, yRange pointer.ScrollRange
	scrollRange := pointer.ScrollRange{Min: -offset, Max: maxOffset - offset}
	axis := gesture.Vertical
	if props.Axis == layout.Horizontal {
		axis = gesture.Horizontal
		xRange = scrollRange
	} else {
		yRange = scrollRange
	}
	scrollDelta := 0
	if !props.PassThroughScroll {
		delegateTouch := false
		if props.NestedScroll {
			delegateTouch = state.boundaryGate.Update(gtx.Source, axis, xRange, yRange)
		}
		if delegateTouch {
			state.scroll = gesture.Scroll{}
		} else {
			scrollDelta = state.scroll.Update(gtx.Metric, gtx.Source, gtx.Now, axis, xRange, yRange)
		}
		offset += scrollDelta
		offset = min(max(0, offset), maxOffset)
		if props.NestedScroll && scrollDelta != 0 {
			r.nestedScrollClaimed = true
		}
	}

	firstVisible := state.firstVisible(offset)
	start := max(0, firstVisible-props.Overscan)
	endLimit := offset + mainViewport + props.Overscan*(estimate+gap)
	recorded := make([]recordedViewportChild, 0, props.Overscan*2+8)
	crossExtent := axisCross(props.Axis, gtx.Constraints.Min)
	previousPassThroughScroll := r.passThroughScroll
	var passThroughScroll *passThroughScrollContext
	if props.PassThroughScroll {
		passThroughScroll = &passThroughScrollContext{axis: axis, xRange: xRange, yRange: yRange, scroll: &state.scroll}
		r.passThroughScroll = passThroughScroll
	} else {
		r.passThroughScroll = nil
	}
	for index := start; index < children.Len(); index++ {
		position := state.prefixAt(index)
		if position >= endLimit && index > firstVisible {
			break
		}
		childIdentity, valid := r.childIdentity(identity, children, index)
		if !valid {
			continue
		}
		childContext := listChildContext(gtx, props.Axis, viewport, r.maxGeometryPixels, !props.ShrinkCross)
		macro := op.Record(gtx.Ops)
		dimensions := r.layoutElement(childContext, children.At(index), childIdentity)
		call := macro.Stop()
		mainSize := axisMain(props.Axis, dimensions.Size)
		if r.rejectGeometry(childIdentity, dimensions.Size.X, dimensions.Size.Y, mainSize) {
			continue
		}
		mainSize = max(1, mainSize)
		evictedIndex, evicted := state.remember(children.KeyAt(index), index, mainSize, props.MaxMeasured)
		state.setExtent(index, mainSize)
		if evicted && evictedIndex >= 0 {
			state.setExtent(evictedIndex, estimate)
		}
		position = state.prefixAt(index)
		pos := axisPoint(props.Axis, position-offset, 0)
		recorded = append(recorded, recordedViewportChild{call: call, pos: pos, size: dimensions.Size})
		crossExtent = max(crossExtent, axisCross(props.Axis, dimensions.Size))
	}
	r.passThroughScroll = previousPassThroughScroll
	if props.PassThroughScroll {
		passThroughScroll.update(gtx)
		parentDelta := passThroughScroll.delta
		if !r.nestedScrollClaimed && parentDelta != 0 {
			scrollDelta = parentDelta
			offset = min(max(0, offset+scrollDelta), maxOffset)
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	renderedViewport := viewport
	if props.ShrinkCross {
		crossExtent = min(axisCross(props.Axis, viewport), crossExtent)
		renderedViewport = setAxisCross(props.Axis, renderedViewport, crossExtent)
	}
	area := clip.Rect{Max: renderedViewport}.Push(gtx.Ops)
	if props.SemanticLabel != "" {
		semantic.DescriptionOp(props.SemanticLabel).Add(gtx.Ops)
	}
	if !props.NestedScroll {
		if props.PassThroughScroll {
			pass := pointer.PassOp{}.Push(gtx.Ops)
			state.scroll.Add(gtx.Ops)
			pass.Pop()
		} else {
			state.scroll.Add(gtx.Ops)
		}
	} else {
		// Keep both gesture observers below row controls. The boundary observer
		// is pass-through, while the scroll gesture still competes normally once
		// a tap turns into a drag.
		pass := pointer.PassOp{}.Push(gtx.Ops)
		state.boundaryGate.Add(gtx.Ops)
		pass.Pop()
		state.scroll.Add(gtx.Ops)
	}
	for _, child := range recorded {
		translation := op.Offset(child.pos).Push(gtx.Ops)
		child.call.Add(gtx.Ops)
		translation.Pop()
	}
	if props.PinnedOverlay != nil && firstVisible >= 0 && firstVisible < children.Len() {
		position := state.prefixAt(firstVisible)
		item := ListViewportItem{
			Key: children.KeyAt(firstVisible), Index: firstVisible,
			Offset:   gtx.Metric.PxToDp(offset - position),
			Extent:   gtx.Metric.PxToDp(state.extents[firstVisible]),
			Viewport: gtx.Metric.PxToDp(mainViewport),
		}
		if overlay := props.PinnedOverlay(item); overlay != nil {
			overlayContext := gtx
			overlayContext.Constraints = layout.Exact(renderedViewport)
			applyInsets(props.PinnedInsets, overlayContext, func(gtx layout.Context) layout.Dimensions {
				return props.PinnedAlignment.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutElement(gtx, *overlay, identity+"/pinned:"+identityPart(item.Key))
				})
			})
		}
	}
	area.Pop()

	anchorIndex = state.anchorAt(offset)
	if anchorIndex < children.Len() {
		state.anchor = children.KeyAt(anchorIndex)
		state.anchorIndex = anchorIndex
		state.anchorOffset = max(0, offset-state.prefixAt(anchorIndex))
	} else {
		state.anchor = ""
		state.anchorIndex = children.Len()
		state.anchorOffset = 0
	}
	maxOffset = max(0, state.totalExtent()-mainViewport)
	wasAtEnd := state.atEnd
	state.atEnd = offset >= maxOffset-1
	if props.OnReachStart != nil && children.Len() > 0 && offset <= 1 {
		key := children.KeyAt(0)
		if state.reachedStartKey != key {
			state.reachedStartKey = key
			props.OnReachStart()
		}
	}
	if props.OnReachEnd != nil && children.Len() > 0 && state.atEnd {
		key := children.KeyAt(children.Len() - 1)
		if state.reachedEndKey != key {
			state.reachedEndKey = key
			props.OnReachEnd()
		}
	}
	if scrollDelta != 0 && wasAtEnd && !state.atEnd && props.OnLeaveEnd != nil {
		props.OnLeaveEnd()
	}
	state.initialized = true
	r.stats.VisibleListItems += len(recorded)
	r.stats.MeasuredListItems += len(state.measurements)
	return layout.Dimensions{Size: renderedViewport}
}

// measureListTail resolves the variable-height children that can intersect a
// followed viewport before its end offset is calculated. Without this pass a
// newly appended log chunk is positioned using its estimate, then grows during
// visible layout and leaves the retained viewport hundreds of lines above the
// real end.
func (r *Runtime) measureListTail(
	gtx layout.Context,
	props ListProps,
	viewport image.Point,
	children Children,
	state *keyedViewportState,
	identity string,
	gap int,
) {
	remaining := axisMain(props.Axis, viewport)
	for index := children.Len() - 1; index >= 0 && remaining > 0; index-- {
		childIdentity, valid := r.childIdentity(identity, children, index)
		if !valid {
			continue
		}
		childContext := listChildContext(gtx, props.Axis, viewport, r.maxGeometryPixels, !props.ShrinkCross)
		childContext.Source = input.Source{}
		macro := op.Record(gtx.Ops)
		dimensions := r.layoutElement(childContext, children.At(index), childIdentity)
		_ = macro.Stop()
		mainSize := max(1, axisMain(props.Axis, dimensions.Size))
		if r.rejectGeometry(childIdentity, dimensions.Size.X, dimensions.Size.Y, mainSize) {
			continue
		}
		state.setExtent(index, mainSize)
		evictedIndex, evicted := state.remember(children.KeyAt(index), index, mainSize, props.MaxMeasured)
		if evicted && evictedIndex >= 0 {
			state.setExtent(evictedIndex, max(1, gtx.Dp(props.Estimate)))
		}
		remaining -= mainSize + gap
	}
}

// intrinsicListViewport measures only enough leading content to decide whether
// a bounded list fits below its cap. The measurement pass has no input source,
// so interactive children cannot consume a gesture before their visible pass.
func (r *Runtime) intrinsicListViewport(
	gtx layout.Context,
	props ListProps,
	viewport image.Point,
	children Children,
	state *keyedViewportState,
	identity string,
	gap int,
) image.Point {
	maximum := axisMain(props.Axis, viewport)
	if maximum <= 0 {
		return viewport
	}
	extent := 0
	for index := 0; index < children.Len() && extent < maximum; index++ {
		childIdentity, valid := r.childIdentity(identity, children, index)
		if !valid {
			continue
		}
		childContext := listChildContext(gtx, props.Axis, viewport, r.maxGeometryPixels, !props.ShrinkCross)
		childContext.Source = input.Source{}
		macro := op.Record(gtx.Ops)
		dimensions := r.layoutElement(childContext, children.At(index), childIdentity)
		_ = macro.Stop()
		mainSize := max(1, axisMain(props.Axis, dimensions.Size))
		if r.rejectGeometry(childIdentity, dimensions.Size.X, dimensions.Size.Y, mainSize) {
			continue
		}
		state.setExtent(index, mainSize)
		state.remember(children.KeyAt(index), index, mainSize, props.MaxMeasured)
		if index > 0 {
			extent += gap
		}
		extent += mainSize
	}
	minimum := min(maximum, max(0, gtx.Dp(props.MinimumViewport)))
	return setAxisMain(props.Axis, viewport, min(maximum, max(minimum, extent)))
}

func (r *Runtime) layoutPassThroughScrollRegion(gtx layout.Context, element Element, identity string) layout.Dimensions {
	context := r.passThroughScroll
	if context == nil {
		return r.layoutOnlyChild(gtx, element, identity)
	}
	state := r.useState(identity, "pass-through-scroll", KindPassThroughScrollRegion, func() any {
		return new(passThroughScrollRegionState)
	}).(*passThroughScrollRegionState)
	prioritizeParent, scrollDelta := state.gate.Update(gtx.Source, context.axis, context.xRange, context.yRange)
	context.delta += scrollDelta
	if prioritizeParent {
		context.update(gtx)
	}
	dimensions := r.layoutOnlyChild(gtx, element, identity)
	if dimensions.Size.X <= 0 || dimensions.Size.Y <= 0 {
		return dimensions
	}
	area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	state.gate.Add(gtx.Ops)
	pass.Pop()
	area.Pop()
	return dimensions
}

func (s *keyedViewportState) reconcileIndex(children Children, estimate, gap, crossSize int) {
	changed := !s.indexed || s.revision != children.Revision() || s.count != children.Len() ||
		s.estimate != estimate || s.gap != gap || s.crossSize != crossSize
	if !changed {
		return
	}
	previousAnchorIndex := s.anchorIndex
	anchorIndex := -1
	for key, measured := range s.measurements {
		measured.index = -1
		s.measurements[key] = measured
	}
	s.extents = resizeAndClear(s.extents, children.Len())
	s.prefixTree = resizeAndClear(s.prefixTree, children.Len()+1)
	for index := 0; index < children.Len(); index++ {
		key := children.KeyAt(index)
		if key == s.anchor {
			anchorIndex = index
		}
		size := estimate
		if measured, ok := s.measurements[key]; ok {
			size = max(1, measured.size)
			measured.index = index
			s.measurements[key] = measured
		}
		s.extents[index] = size
		s.addPrefix(index, size+gap)
	}
	if anchorIndex < 0 && children.Len() > 0 {
		anchorIndex = min(max(0, previousAnchorIndex), children.Len()-1)
	}
	s.anchorIndex = anchorIndex
	s.revision = children.Revision()
	s.count = children.Len()
	s.estimate = estimate
	s.gap = gap
	s.crossSize = crossSize
	s.indexed = true
}

func resizeAndClear(values []int, length int) []int {
	if cap(values) < length {
		return make([]int, length)
	}
	values = values[:length]
	clear(values)
	return values
}

func (s *keyedViewportState) addPrefix(index, delta int) {
	for treeIndex := index + 1; treeIndex < len(s.prefixTree); treeIndex += treeIndex & -treeIndex {
		s.prefixTree[treeIndex] += delta
	}
}

func (s *keyedViewportState) prefixAt(end int) int {
	end = min(max(0, end), len(s.extents))
	total := 0
	for end > 0 {
		total += s.prefixTree[end]
		end -= end & -end
	}
	return total
}

func (s *keyedViewportState) totalExtent() int {
	if len(s.extents) == 0 {
		return 0
	}
	return max(0, s.prefixAt(len(s.extents))-s.gap)
}

func (s *keyedViewportState) firstVisible(offset int) int {
	low, high := 0, len(s.extents)
	for low < high {
		middle := low + (high-low)/2
		if s.prefixAt(middle)+s.extents[middle] > offset {
			high = middle
		} else {
			low = middle + 1
		}
	}
	return low
}

// anchorAt returns the item whose leading edge owns offset. Unlike
// firstVisible, the preceding item continues to own the gap after it so an
// offset inside that gap can be reconstructed without snapping forward to the
// next item.
func (s *keyedViewportState) anchorAt(offset int) int {
	index := s.firstVisible(offset)
	if index > 0 && (index == len(s.extents) || s.prefixAt(index) > offset) {
		index--
	}
	return index
}

func (s *keyedViewportState) setExtent(index, size int) {
	if index < 0 || index >= len(s.extents) {
		return
	}
	size = max(1, size)
	delta := size - s.extents[index]
	if delta == 0 {
		return
	}
	s.extents[index] = size
	s.addPrefix(index, delta)
}

func (s *keyedViewportState) remember(key Key, index, size, limit int) (int, bool) {
	s.clock++
	s.measurements[key] = measurement{size: size, lastUsed: s.clock, index: index}
	if len(s.measurements) <= limit {
		return 0, false
	}
	var oldestKey Key
	var oldest measurement
	first := true
	for candidateKey, candidate := range s.measurements {
		if first || candidate.lastUsed < oldest.lastUsed {
			oldestKey, oldest, first = candidateKey, candidate, false
		}
	}
	delete(s.measurements, oldestKey)
	return oldest.index, true
}

func (r *Runtime) layoutStockList(gtx layout.Context, element Element, identity string) layout.Dimensions {
	children := element.Children
	if children == nil {
		return layout.Dimensions{}
	}
	props := normalizedListProps(element.List)
	viewport := listViewportSize(gtx, props)
	if !r.validateDynamicKeys(identity, children) {
		return layout.Dimensions{Size: viewport}
	}
	state := r.useState(identity, "stock-list", KindStockList, func() any {
		return &stockListState{list: layout.List{Axis: props.Axis}}
	}).(*stockListState)
	state.list.Axis = props.Axis
	state.list.Gap = gtx.Dp(props.Gap)
	state.list.ScrollToEnd = props.ScrollToEnd
	if props.ScrollTo != "" && props.ScrollRevision != state.scrollRevision {
		for index := 0; index < children.Len(); index++ {
			if children.KeyAt(index) != props.ScrollTo {
				continue
			}
			state.list.Position.First = index
			state.list.Position.Offset = 0
			state.list.Position.BeforeEnd = true
			break
		}
		state.scrollRevision = props.ScrollRevision
	}
	state.revision = children.Revision()
	listContext := gtx
	listContext.Constraints = layout.Exact(viewport)
	if props.SemanticLabel != "" {
		semantic.DescriptionOp(props.SemanticLabel).Add(gtx.Ops)
	}
	dimensions := state.list.Layout(listContext, children.Len(), func(gtx layout.Context, index int) layout.Dimensions {
		childIdentity, valid := r.childIdentity(identity, children, index)
		if !valid {
			return layout.Dimensions{}
		}
		r.stats.VisibleListItems++
		return r.layoutElement(gtx, children.At(index), childIdentity)
	})
	return dimensions
}

func normalizedListProps(props ListProps) ListProps {
	if props.Axis != layout.Horizontal {
		props.Axis = layout.Vertical
	}
	if props.Estimate <= 0 {
		props.Estimate = 48
	}
	if props.Overscan <= 0 {
		props.Overscan = 2
	}
	if props.MaxMeasured <= 0 {
		props.MaxMeasured = 2048
	}
	return props
}

func listViewportSize(gtx layout.Context, props ListProps) image.Point {
	size := gtx.Constraints.Max
	if props.Viewport > 0 {
		main := min(axisMain(props.Axis, size), gtx.Dp(props.Viewport))
		size = setAxisMain(props.Axis, size, main)
	}
	return gtx.Constraints.Constrain(size)
}

func listChildContext(gtx layout.Context, axis layout.Axis, viewport image.Point, maxGeometry int, stretchCross bool) layout.Context {
	if axis == layout.Horizontal {
		gtx.Constraints.Min = image.Pt(0, 0)
		if stretchCross {
			gtx.Constraints.Min.Y = viewport.Y
		}
		gtx.Constraints.Max = image.Pt(maxGeometry, viewport.Y)
	} else {
		gtx.Constraints.Min = image.Pt(0, 0)
		if stretchCross {
			gtx.Constraints.Min.X = viewport.X
		}
		gtx.Constraints.Max = image.Pt(viewport.X, maxGeometry)
	}
	return gtx
}

func axisMain(axis layout.Axis, point image.Point) int {
	if axis == layout.Horizontal {
		return point.X
	}
	return point.Y
}

func axisCross(axis layout.Axis, point image.Point) int {
	if axis == layout.Horizontal {
		return point.Y
	}
	return point.X
}

func setAxisMain(axis layout.Axis, point image.Point, value int) image.Point {
	if axis == layout.Horizontal {
		point.X = value
	} else {
		point.Y = value
	}
	return point
}

func setAxisCross(axis layout.Axis, point image.Point, value int) image.Point {
	if axis == layout.Horizontal {
		point.Y = value
	} else {
		point.X = value
	}
	return point
}

func axisPoint(axis layout.Axis, main, cross int) image.Point {
	if axis == layout.Horizontal {
		return image.Pt(main, cross)
	}
	return image.Pt(cross, main)
}
