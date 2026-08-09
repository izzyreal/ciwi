package giodom

import (
	"image"

	"gioui.org/gesture"
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
	scroll       gesture.Scroll
	anchor       Key
	anchorIndex  int
	anchorOffset int
	measurements map[Key]measurement
	clock        uint64
	atEnd        bool
	initialized  bool
	indexed      bool
	revision     uint64
	count        int
	estimate     int
	gap          int
	crossSize    int
	extents      []int
	prefixTree   []int
}

type stockListState struct {
	list     layout.List
	revision uint64
}

type recordedViewportChild struct {
	call op.CallOp
	pos  image.Point
	size image.Point
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

	mainViewport := axisMain(props.Axis, viewport)
	if r.rejectGeometry(identity, viewport.X, viewport.Y, mainViewport) || mainViewport == 0 {
		return layout.Dimensions{Size: viewport}
	}
	estimate := max(1, gtx.Dp(props.Estimate))
	gap := max(0, gtx.Dp(props.Gap))
	state.reconcileIndex(children, estimate, gap, axisCross(props.Axis, viewport))
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
	if props.ScrollToEnd && (!state.initialized || state.atEnd) {
		offset = maxOffset
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
	offset += state.scroll.Update(gtx.Metric, gtx.Source, gtx.Now, axis, xRange, yRange)
	offset = min(max(0, offset), maxOffset)

	firstVisible := state.firstVisible(offset)
	start := max(0, firstVisible-props.Overscan)
	endLimit := offset + mainViewport + props.Overscan*(estimate+gap)
	recorded := make([]recordedViewportChild, 0, props.Overscan*2+8)
	for index := start; index < children.Len(); index++ {
		position := state.prefixAt(index)
		if position >= endLimit && index > firstVisible {
			break
		}
		childIdentity, valid := r.childIdentity(identity, children, index)
		if !valid {
			continue
		}
		childContext := listChildContext(gtx, props.Axis, viewport, r.maxGeometryPixels)
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
	}

	area := clip.Rect{Max: viewport}.Push(gtx.Ops)
	if props.SemanticLabel != "" {
		semantic.DescriptionOp(props.SemanticLabel).Add(gtx.Ops)
	}
	state.scroll.Add(gtx.Ops)
	for _, child := range recorded {
		translation := op.Offset(child.pos).Push(gtx.Ops)
		child.call.Add(gtx.Ops)
		translation.Pop()
	}
	area.Pop()

	if firstVisible < children.Len() {
		state.anchor = children.KeyAt(firstVisible)
		state.anchorIndex = firstVisible
		state.anchorOffset = max(0, offset-state.prefixAt(firstVisible))
	} else {
		state.anchor = ""
		state.anchorIndex = children.Len()
		state.anchorOffset = 0
	}
	maxOffset = max(0, state.totalExtent()-mainViewport)
	state.atEnd = offset >= maxOffset-1
	state.initialized = true
	r.stats.VisibleListItems += len(recorded)
	r.stats.MeasuredListItems += len(state.measurements)
	return layout.Dimensions{Size: viewport}
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

func listChildContext(gtx layout.Context, axis layout.Axis, viewport image.Point, maxGeometry int) layout.Context {
	if axis == layout.Horizontal {
		gtx.Constraints.Min = image.Pt(0, viewport.Y)
		gtx.Constraints.Max = image.Pt(maxGeometry, viewport.Y)
	} else {
		gtx.Constraints.Min = image.Pt(viewport.X, 0)
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

func axisPoint(axis layout.Axis, main, cross int) image.Point {
	if axis == layout.Horizontal {
		return image.Pt(main, cross)
	}
	return image.Pt(cross, main)
}
