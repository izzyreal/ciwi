//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"image"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

const (
	maxNativeJobLogCacheBytes   = 4 * 1024 * 1024
	nativeJobLogDisplayRunesMax = 512
)

type nativeJobLogTextPosition struct {
	ChunkID int64
	Rune    int
}

type nativeJobLogTextSelection struct {
	Anchor, Focus nativeJobLogTextPosition
	HasAnchor     bool
}

func nativeJobLogKey(jobID, itemID string) string { return jobID + "\n" + itemID }

func (r *Renderer) ApplyJobLogPage(page jobLogStreamSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := nativeJobLogKey(page.JobID, page.ItemID)
	current := r.jobLogStreams[key]
	page.PageLoaded = true
	page.Terminal = page.Terminal || current.Terminal
	page.LatestChunkID = max(page.LatestChunkID, current.LatestChunkID)
	if page.SelectedChunkID == 0 {
		page.SelectedChunkID = current.SelectedChunkID
		page.SelectedStartRune = current.SelectedStartRune
		page.SelectedEndRune = current.SelectedEndRune
	}
	byID := make(map[int64]jobLogChunkSnapshot, len(current.Chunks)+len(page.Chunks))
	for _, chunk := range current.Chunks {
		byID[chunk.ID] = chunk
	}
	for _, chunk := range page.Chunks {
		byID[chunk.ID] = chunk
	}
	page.Chunks = page.Chunks[:0]
	for _, chunk := range byID {
		page.Chunks = append(page.Chunks, chunk)
	}
	sort.Slice(page.Chunks, func(i, j int) bool { return page.Chunks[i].ID < page.Chunks[j].ID })
	if len(page.Chunks) > 0 {
		page.LatestChunkID = max(page.LatestChunkID, page.Chunks[len(page.Chunks)-1].ID)
		page.HasAfter = page.HasAfter || page.Chunks[len(page.Chunks)-1].ID < page.LatestChunkID
	} else if page.LatestChunkID > 0 {
		page.HasAfter = true
	}
	r.jobLogStreams[key] = page
	for loadKey := range r.jobLogLoads {
		if strings.HasPrefix(loadKey, key+"\n") {
			delete(r.jobLogLoads, loadKey)
		}
	}
	r.trimJobLogCacheLocked(key)
	r.markDOMDirty()
	r.requestFrame()
}

func (r *Renderer) ApplyJobLogSearch(result jobLogSearchSnapshot) {
	r.outputSearch = result.Query
	r.outputMatch, r.outputTotalMatches = result.SelectedIndex, result.TotalMatches
	count := "0/0"
	if result.TotalMatches > 0 {
		count = fmt.Sprintf("%d/%d", result.SelectedIndex+1, result.TotalMatches)
	}
	r.SetRootBinding("jobDetails", "output_search_count", count)
	for key, stream := range r.jobLogStreams {
		if stream.JobID != result.JobID {
			continue
		}
		stream.SelectedChunkID = 0
		stream.SelectedStartRune = 0
		stream.SelectedEndRune = 0
		r.jobLogStreams[key] = stream
	}
	key := nativeJobLogKey(result.JobID, result.ItemID)
	stream := r.jobLogStreams[key]
	stream.SelectedChunkID = result.ChunkID
	stream.SelectedStartRune = result.StartRune
	stream.SelectedEndRune = result.EndRune
	r.jobLogStreams[key] = stream
	if result.ChunkID > 0 {
		r.setOutputTailing(false)
	}
	if result.ItemID != "" {
		if groups, err := resolveItems(r.data, "jobDetails.output_groups"); err == nil {
			for _, raw := range groups {
				group, ok := raw.(map[string]any)
				if ok && fmt.Sprint(group["id"]) == result.ItemID {
					r.setDisclosureState(fmt.Sprint(group["state_key"]), true, true)
					break
				}
			}
		}
		r.scrollOutputTo(result.ItemID)
	} else {
		r.outputScrollRevision++
	}
	r.markDOMDirty()
	r.requestFrame()
}

func (r *Renderer) clearJobLogSearchSelection(jobID string) {
	changed := false
	for key, stream := range r.jobLogStreams {
		if stream.JobID != jobID || stream.SelectedChunkID == 0 {
			continue
		}
		stream.SelectedChunkID = 0
		stream.SelectedStartRune = 0
		stream.SelectedEndRune = 0
		r.jobLogStreams[key] = stream
		changed = true
	}
	if changed {
		r.outputScrollRevision++
		r.markDOMDirty()
	}
}

func (r *Renderer) ApplyJobLogDescriptor(descriptor jobLogDescriptorSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, stream := range r.jobLogStreams {
		if stream.JobID == descriptor.JobID {
			stream.Terminal = descriptor.Terminal
			r.jobLogStreams[key] = stream
		}
	}
	for itemID, latest := range descriptor.Streams {
		key := nativeJobLogKey(descriptor.JobID, itemID)
		stream := r.jobLogStreams[key]
		stream.JobID, stream.ItemID, stream.Terminal = descriptor.JobID, itemID, descriptor.Terminal
		stream.LatestChunkID = max(stream.LatestChunkID, latest)
		if len(stream.Chunks) == 0 || stream.Chunks[len(stream.Chunks)-1].ID < latest {
			stream.HasAfter = true
		}
		r.jobLogStreams[key] = stream
	}
	r.markDOMDirty()
	r.requestFrame()
}

func (r *Renderer) FailJobLogPage(jobID, itemID, mode string) {
	r.mu.Lock()
	delete(r.jobLogLoads, nativeJobLogKey(jobID, itemID)+"\n"+mode)
	r.mu.Unlock()
	r.markDOMDirty()
	r.requestFrame()
}

func (r *Renderer) trimJobLogCacheLocked(activeKey string) {
	total := 0
	for _, stream := range r.jobLogStreams {
		for _, chunk := range stream.Chunks {
			total += len(chunk.Text)
		}
	}
	for total > maxNativeJobLogCacheBytes {
		trimmed := false
		for key, stream := range r.jobLogStreams {
			if len(stream.Chunks) <= 1 {
				continue
			}
			index := 0
			selectedIndex := -1
			for candidate, chunk := range stream.Chunks {
				if chunk.ID == stream.SelectedChunkID {
					selectedIndex = candidate
					break
				}
			}
			if key != activeKey || stream.LoadedMode == "before" || stream.LoadedMode == "head" || (selectedIndex >= 0 && selectedIndex < len(stream.Chunks)/2) {
				index = len(stream.Chunks) - 1
			}
			if r.nativeJobLogChunkSelected(key, stream.Chunks[index].ID) {
				alternative := -1
				for candidate := range stream.Chunks {
					if !r.nativeJobLogChunkSelected(key, stream.Chunks[candidate].ID) {
						alternative = candidate
						if candidate == 0 || candidate == len(stream.Chunks)-1 {
							break
						}
					}
				}
				if alternative < 0 {
					continue
				}
				index = alternative
			}
			total -= len(stream.Chunks[index].Text)
			stream.Chunks = append(stream.Chunks[:index], stream.Chunks[index+1:]...)
			if index == 0 {
				stream.HasBefore = true
			} else {
				stream.HasAfter = true
			}
			r.jobLogStreams[key] = stream
			trimmed = true
			if total <= maxNativeJobLogCacheBytes {
				break
			}
		}
		if !trimmed {
			break
		}
	}
}

func (r *Renderer) requestJobLogPage(jobID, itemID, mode string, cursor int64) {
	key := nativeJobLogKey(jobID, itemID) + "\n" + mode
	if r.jobLogLoads[key] || r.onAction == nil {
		return
	}
	r.jobLogLoads[key] = true
	r.onAction(uidsl.Action{On: "activate", Command: "load-job-log-page"}, map[string]string{
		"jobExecutionId": jobID, "itemId": itemID, "mode": mode, "cursor": strconv.FormatInt(cursor, 10),
	})
}

func (r *Renderer) compileDOMInteractiveOutputGroupBody(nodes []uidsl.Node, data any, path string, inherited domStyleContext) (giodom.Element, bool) {
	for nodeIndex, rawBody := range nodes {
		body, hidden := applyGioOverride(rawBody, r.compact)
		if hidden || body.Style.Role != "output-group-body" || !domNodeVisible(body, data) {
			continue
		}
		_, childStyle := r.resolveDOMStyle(body.Component, body.Style, data, inherited)
		bodyPath := fmt.Sprintf("%s/%d", path, nodeIndex)
		preambleChildren := make([]giodom.Element, 0, len(body.Children))
		var logNode *uidsl.Node
		logPath := ""
		for childIndex, rawChild := range body.Children {
			child, childHidden := applyGioOverride(rawChild, r.compact)
			childPath := fmt.Sprintf("%s/%d", bodyPath, childIndex)
			if childHidden || !domNodeVisible(child, data) {
				continue
			}
			if child.Component == "log-view" && child.LogView != nil {
				copy := child
				logNode, logPath = &copy, childPath
				continue
			}
			compiled := r.compileDOMNodeWithStyle(child, data, childPath, childStyle)
			if compiled != nil {
				preambleChildren = append(preambleChildren, *compiled)
			}
		}
		if logNode == nil {
			return giodom.Element{}, false
		}
		preamble := giodom.Element{
			Kind: giodom.KindFlex, Key: giodom.Key(bodyPath + "/preamble"),
			Flex: giodom.FlexProps{
				Axis: layout.Vertical, Alignment: layout.Start,
				Gap: r.spacing(body.Layout.Gap), Padding: giodom.UniformInsets(r.spacing(body.Layout.Padding)),
			},
			Children: giodom.Static(preambleChildren...),
		}
		compiled := r.compileDOMLogViewWithPreamble(*logNode, data, logPath, &preamble)
		compiled.Key = domNodeKey(*logNode, logPath)
		return *r.decorateDOMNode(compiled, *logNode, data, logPath), true
	}
	return giodom.Element{}, false
}

func (r *Renderer) compileDOMLogView(node uidsl.Node, data any, path string) giodom.Element {
	return r.compileDOMLogViewWithPreamble(node, data, path, nil)
}

func (r *Renderer) compileDOMLogViewWithPreamble(node uidsl.Node, data any, path string, preamble *giodom.Element) giodom.Element {
	if node.LogView == nil {
		return r.domMessage(giodom.Key(path+"/missing"), "Log view unavailable", r.palette.danger)
	}
	jobValue, jobErr := uidsl.Resolve(data, node.LogView.JobExecutionID)
	if jobErr != nil {
		return r.domError(path, jobErr)
	}
	jobID, itemID := fmt.Sprint(jobValue), ""
	if node.LogView.ItemID != "" {
		itemValue, err := uidsl.Resolve(data, node.LogView.ItemID)
		if err != nil {
			return r.domError(path, err)
		}
		itemID = fmt.Sprint(itemValue)
	}
	key := nativeJobLogKey(jobID, itemID)
	stream, known := r.jobLogStreams[key]
	children := make([]giodom.Element, 0, len(stream.Chunks)+1)
	if preamble != nil {
		children = append(children, *preamble)
	}
	selectionTarget := giodom.Key("")
	var selectableBlocks map[giodom.Key]nativeJobLogDisplayBlock
	if !known || !stream.PageLoaded {
		mode := "head"
		if r.outputTailing {
			mode = "tail"
		}
		r.requestJobLogPage(jobID, itemID, mode, 0)
		children = append(children, r.domMessage(giodom.Key(path+"/loading"), "Loading output…", r.palette.consoleMuted))
	} else if len(stream.Chunks) == 0 {
		if stream.HasAfter || stream.LatestChunkID > 0 {
			mode := "head"
			if r.outputTailing {
				mode = "tail"
			}
			r.requestJobLogPage(jobID, itemID, mode, 0)
			children = append(children, r.domMessage(giodom.Key(path+"/loading"), "Loading output…", r.palette.consoleMuted))
		} else {
			label := "Waiting for output…"
			if stream.Terminal {
				label = "(no output)"
			}
			children = append(children, r.domMessage(giodom.Key(path+"/empty"), label, r.palette.consoleMuted))
		}
	} else {
		blocks, target := nativeJobLogDisplayBlocks(stream, path)
		selectionTarget = target
		selectableBlocks = make(map[giodom.Key]nativeJobLogDisplayBlock, len(blocks))
		selection := r.jobLogSelections[key]
		for _, block := range blocks {
			selectableBlocks[block.Key] = block
			userStart, userEnd := nativeJobLogBlockSelectionRange(selection, block)
			children = append(children, r.compileDOMJobLogBlock(block, userStart, userEnd))
		}
	}
	props := giodom.ListProps{
		Axis: layout.Vertical, Viewport: r.domJobLogViewport(preamble != nil),
		MinimumViewport: unit.Dp(r.controls.LogView.MinimumHeight), ShrinkMain: true,
		NestedScroll: true, Estimate: 120, Overscan: 2, MaxMeasured: 128,
		ScrollToEnd: r.outputTailing, ForceEndRevision: r.outputTailRevision, SemanticLabel: "Execution output",
	}
	if r.outputTailing {
		props.OnLeaveEnd = func() {
			if !r.outputTailing {
				return
			}
			r.setOutputTailing(false)
			r.requestFrame()
		}
	}
	if selectionTarget != "" {
		props.ScrollTo = selectionTarget
		props.ScrollRevision = r.outputScrollRevision
	}
	if stream.HasBefore && len(stream.Chunks) > 0 {
		first := stream.Chunks[0].ID
		props.OnReachStart = func() { r.requestJobLogPage(jobID, itemID, "before", first) }
	}
	if stream.HasAfter && len(stream.Chunks) > 0 {
		last := stream.Chunks[len(stream.Chunks)-1].ID
		props.OnReachEnd = func() { r.requestJobLogPage(jobID, itemID, "after", last) }
	}
	if len(selectableBlocks) > 0 {
		props.TextSelection = &giodom.ListTextSelectionProps{
			HitTest: func(gtx layout.Context, blockKey giodom.Key, point image.Point) (int, bool) {
				block, ok := selectableBlocks[blockKey]
				if !ok {
					return 0, false
				}
				gtx.Constraints.Min.X = 0
				return domTextRuneAtPoint(gtx, r.theme.Shaper, r.nativeTextStyle("output-code", false), block.Text, point), true
			},
			Start: func(blockKey giodom.Key, runeOffset int, extend bool) {
				if block, ok := selectableBlocks[blockKey]; ok {
					r.startNativeJobLogSelection(key, block, runeOffset, extend)
				}
			},
			Extend: func(blockKey giodom.Key, runeOffset int) {
				if block, ok := selectableBlocks[blockKey]; ok {
					r.extendNativeJobLogSelection(key, block, runeOffset)
				}
			},
			CopyText:  func() string { return nativeJobLogSelectedText(stream, r.jobLogSelections[key]) },
			SelectAll: func() { r.selectAllNativeJobLog(key, stream) },
		}
	}
	return giodom.VirtualList(giodom.Key(path+"/log"), props, giodom.Keyed(domElementsRevision(children), children...))
}

func (r *Renderer) compileDOMJobLogBlock(block nativeJobLogDisplayBlock, selectionStart, selectionEnd int) giodom.Element {
	typography := r.nativeTextStyle("output-code", false)
	return giodom.Native(block.Key, giodom.NativeProps{Layout: func(gtx layout.Context, _ any) layout.Dimensions {
		pointer.CursorText.Add(gtx.Ops)
		semantic.LabelOp(block.Text).Add(gtx.Ops)
		if block.HighlightEnd > block.HighlightStart {
			highlight := r.palette.focus
			highlight.A = 0x90
			paintDOMTextHighlight(gtx, r.theme.Shaper, typography, block.Text, block.HighlightStart, block.HighlightEnd, highlight)
		}
		if selectionEnd > selectionStart {
			highlight := r.palette.focus
			highlight.A = 0xc0
			paintDOMTextHighlight(gtx, r.theme.Shaper, typography, block.Text, selectionStart, selectionEnd, highlight)
		}
		label := material.Label(r.theme, typography.size, block.Text)
		label.Font, label.LineHeightScale, label.Color = typography.font, typography.lineHeight, r.palette.consoleText
		return label.Layout(gtx)
	}})
}

func (r *Renderer) domJobLogViewport(inOutputGroup bool) unit.Dp {
	maximum := unit.Dp(r.controls.LogView.MaximumHeight)
	if !inOutputGroup {
		return maximum
	}
	headerStyle := r.nativeTextStyle("output-summary", true)
	lineHeightScale := headerStyle.lineHeight
	if lineHeightScale <= 0 {
		lineHeightScale = 1
	}
	fixedChrome := 4*r.metrics.sectionPadding + unit.Dp(float32(headerStyle.size)*lineHeightScale)
	available := r.domOutputGroupsViewport(660) - fixedChrome
	minimum := unit.Dp(r.controls.LogView.MinimumHeight)
	return max(minimum, min(maximum, available))
}

type nativeJobLogDisplayBlock struct {
	Key                          giodom.Key
	ChunkID                      int64
	StartRune                    int
	Text                         string
	HighlightStart, HighlightEnd int
}

func compareNativeJobLogTextPosition(left, right nativeJobLogTextPosition) int {
	if left.ChunkID < right.ChunkID {
		return -1
	}
	if left.ChunkID > right.ChunkID {
		return 1
	}
	if left.Rune < right.Rune {
		return -1
	}
	if left.Rune > right.Rune {
		return 1
	}
	return 0
}

func normalizedNativeJobLogSelection(selection nativeJobLogTextSelection) (nativeJobLogTextPosition, nativeJobLogTextPosition, bool) {
	if !selection.HasAnchor || compareNativeJobLogTextPosition(selection.Anchor, selection.Focus) == 0 {
		return nativeJobLogTextPosition{}, nativeJobLogTextPosition{}, false
	}
	if compareNativeJobLogTextPosition(selection.Anchor, selection.Focus) < 0 {
		return selection.Anchor, selection.Focus, true
	}
	return selection.Focus, selection.Anchor, true
}

func nativeJobLogBlockSelectionRange(selection nativeJobLogTextSelection, block nativeJobLogDisplayBlock) (int, int) {
	start, end, ok := normalizedNativeJobLogSelection(selection)
	if !ok || block.ChunkID < start.ChunkID || block.ChunkID > end.ChunkID {
		return 0, 0
	}
	blockRunes := utf8.RuneCountInString(block.Text)
	selectionStart, selectionEnd := block.StartRune, block.StartRune+blockRunes
	if block.ChunkID == start.ChunkID {
		selectionStart = max(selectionStart, start.Rune)
	}
	if block.ChunkID == end.ChunkID {
		selectionEnd = min(selectionEnd, end.Rune)
	}
	selectionStart = min(max(selectionStart, block.StartRune), block.StartRune+blockRunes)
	selectionEnd = min(max(selectionEnd, selectionStart), block.StartRune+blockRunes)
	return selectionStart - block.StartRune, selectionEnd - block.StartRune
}

func nativeJobLogBlockPosition(block nativeJobLogDisplayBlock, runeOffset int) nativeJobLogTextPosition {
	length := utf8.RuneCountInString(block.Text)
	return nativeJobLogTextPosition{ChunkID: block.ChunkID, Rune: block.StartRune + min(max(0, runeOffset), length)}
}

func (r *Renderer) startNativeJobLogSelection(key string, block nativeJobLogDisplayBlock, runeOffset int, extend bool) {
	position := nativeJobLogBlockPosition(block, runeOffset)
	selection := r.jobLogSelections[key]
	if extend && selection.HasAnchor {
		selection.Focus = position
	} else {
		selection = nativeJobLogTextSelection{Anchor: position, Focus: position, HasAnchor: true}
	}
	r.jobLogSelections[key] = selection
	r.setOutputTailing(false)
	r.markDOMDirty()
	r.requestFrame()
}

func (r *Renderer) extendNativeJobLogSelection(key string, block nativeJobLogDisplayBlock, runeOffset int) {
	selection := r.jobLogSelections[key]
	if !selection.HasAnchor {
		return
	}
	position := nativeJobLogBlockPosition(block, runeOffset)
	if compareNativeJobLogTextPosition(selection.Focus, position) == 0 {
		return
	}
	selection.Focus = position
	r.jobLogSelections[key] = selection
	r.markDOMDirty()
	r.requestFrame()
}

func (r *Renderer) selectAllNativeJobLog(key string, stream jobLogStreamSnapshot) {
	if len(stream.Chunks) == 0 {
		return
	}
	last := stream.Chunks[len(stream.Chunks)-1]
	r.jobLogSelections[key] = nativeJobLogTextSelection{
		Anchor:    nativeJobLogTextPosition{ChunkID: stream.Chunks[0].ID},
		Focus:     nativeJobLogTextPosition{ChunkID: last.ID, Rune: utf8.RuneCountInString(last.Text)},
		HasAnchor: true,
	}
	r.setOutputTailing(false)
	r.markDOMDirty()
	r.requestFrame()
}

func nativeJobLogSelectedText(stream jobLogStreamSnapshot, selection nativeJobLogTextSelection) string {
	start, end, ok := normalizedNativeJobLogSelection(selection)
	if !ok {
		return ""
	}
	var selected strings.Builder
	foundStart, foundEnd := false, false
	for _, chunk := range stream.Chunks {
		if chunk.ID < start.ChunkID || chunk.ID > end.ChunkID {
			continue
		}
		runes := []rune(chunk.Text)
		from, to := 0, len(runes)
		if chunk.ID == start.ChunkID {
			from = min(max(0, start.Rune), len(runes))
			foundStart = true
		}
		if chunk.ID == end.ChunkID {
			to = min(max(0, end.Rune), len(runes))
			foundEnd = true
		}
		if to > from {
			selected.WriteString(string(runes[from:to]))
		}
	}
	if !foundStart || !foundEnd {
		return ""
	}
	return selected.String()
}

func (r *Renderer) nativeJobLogChunkSelected(key string, chunkID int64) bool {
	start, end, ok := normalizedNativeJobLogSelection(r.jobLogSelections[key])
	return ok && chunkID >= start.ChunkID && chunkID <= end.ChunkID
}

func nativeJobLogDisplayBlocks(stream jobLogStreamSnapshot, path string) ([]nativeJobLogDisplayBlock, giodom.Key) {
	selections := jobLogSelectionRanges(stream)
	blocks := make([]nativeJobLogDisplayBlock, 0, len(stream.Chunks)*2)
	target := giodom.Key("")
	for _, chunk := range stream.Chunks {
		selection, selected := selections[chunk.ID]
		chunkBlocks := splitNativeJobLogDisplayChunk(chunk, path, selection, selected)
		for _, block := range chunkBlocks {
			if target == "" && block.HighlightEnd > block.HighlightStart {
				target = block.Key
			}
			blocks = append(blocks, block)
		}
	}
	return blocks, target
}

func splitNativeJobLogDisplayChunk(chunk jobLogChunkSnapshot, path string, selection [2]int, selected bool) []nativeJobLogDisplayBlock {
	runes := []rune(chunk.Text)
	if len(runes) == 0 {
		return nil
	}
	if selected {
		selection[0] = min(max(0, selection[0]), len(runes))
		selection[1] = min(max(selection[0], selection[1]), len(runes))
	}
	blocks := make([]nativeJobLogDisplayBlock, 0, len(runes)/nativeJobLogDisplayRunesMax+1)
	for start := 0; start < len(runes); {
		end := min(len(runes), start+nativeJobLogDisplayRunesMax)
		if end < len(runes) {
			for index := end - 1; index >= start; index-- {
				if runes[index] == '\n' {
					end = index + 1
					break
				}
			}
		}
		if end <= start {
			end = min(len(runes), start+nativeJobLogDisplayRunesMax)
		}
		block := nativeJobLogDisplayBlock{
			Key:     giodom.Key(fmt.Sprintf("%s/job-log-chunk:%d:block:%d", path, chunk.ID, start)),
			ChunkID: chunk.ID, StartRune: start, Text: string(runes[start:end]),
		}
		if selected {
			highlightStart := max(start, selection[0])
			highlightEnd := min(end, selection[1])
			if highlightEnd > highlightStart {
				block.HighlightStart = highlightStart - start
				block.HighlightEnd = highlightEnd - start
			}
		}
		blocks = append(blocks, block)
		start = end
	}
	return blocks
}

func jobLogSelectionRanges(stream jobLogStreamSnapshot) map[int64][2]int {
	selectedIndex := -1
	for index, chunk := range stream.Chunks {
		if chunk.ID == stream.SelectedChunkID {
			selectedIndex = index
			break
		}
	}
	if selectedIndex < 0 || stream.SelectedEndRune <= stream.SelectedStartRune {
		return nil
	}
	starts := make([]int, len(stream.Chunks))
	for index := selectedIndex - 1; index >= 0; index-- {
		starts[index] = starts[index+1] - utf8.RuneCountInString(stream.Chunks[index].Text)
	}
	for index := selectedIndex + 1; index < len(stream.Chunks); index++ {
		starts[index] = starts[index-1] + utf8.RuneCountInString(stream.Chunks[index-1].Text)
	}
	ranges := make(map[int64][2]int)
	for index, chunk := range stream.Chunks {
		chunkRunes := utf8.RuneCountInString(chunk.Text)
		chunkStart, chunkEnd := starts[index], starts[index]+chunkRunes
		start := max(stream.SelectedStartRune, chunkStart)
		end := min(stream.SelectedEndRune, chunkEnd)
		if end > start {
			ranges[chunk.ID] = [2]int{start - chunkStart, end - chunkStart}
		}
	}
	return ranges
}
