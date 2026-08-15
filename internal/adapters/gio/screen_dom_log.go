//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

const maxNativeJobLogCacheBytes = 4 * 1024 * 1024

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
	r.requestFrame()
}

func (r *Renderer) ApplyJobLogSearch(result jobLogSearchSnapshot) {
	r.outputMatch, r.outputTotalMatches = result.SelectedIndex, result.TotalMatches
	count := "0/0"
	if result.TotalMatches > 0 {
		count = fmt.Sprintf("%d/%d", result.SelectedIndex+1, result.TotalMatches)
	}
	r.SetRootBinding("jobDetails", "output_search_count", count)
	key := nativeJobLogKey(result.JobID, result.ItemID)
	stream := r.jobLogStreams[key]
	stream.SelectedChunkID = result.ChunkID
	r.jobLogStreams[key] = stream
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
	}
	r.outputScrollRevision++
	r.requestFrame()
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
	r.requestFrame()
}

func (r *Renderer) FailJobLogPage(jobID, itemID, mode string) {
	r.mu.Lock()
	delete(r.jobLogLoads, nativeJobLogKey(jobID, itemID)+"\n"+mode)
	r.mu.Unlock()
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

func (r *Renderer) compileDOMLogView(node uidsl.Node, data any, path string) giodom.Element {
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
	if !known || !stream.PageLoaded {
		mode := "head"
		if r.outputTailing {
			mode = "tail"
		}
		r.requestJobLogPage(jobID, itemID, mode, 0)
		return r.domMessage(giodom.Key(path+"/loading"), "Loading output…", r.palette.consoleMuted)
	}
	if len(stream.Chunks) == 0 {
		if stream.HasAfter || stream.LatestChunkID > 0 {
			mode := "head"
			if r.outputTailing {
				mode = "tail"
			}
			r.requestJobLogPage(jobID, itemID, mode, 0)
			return r.domMessage(giodom.Key(path+"/loading"), "Loading output…", r.palette.consoleMuted)
		}
		label := "Waiting for output…"
		if stream.Terminal {
			label = "(no output)"
		}
		return r.domMessage(giodom.Key(path+"/empty"), label, r.palette.consoleMuted)
	}
	children := make([]giodom.Element, 0, len(stream.Chunks))
	for _, chunk := range stream.Chunks {
		chunkNode := uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: chunk.Text}, Style: uidsl.Style{Role: "output-code", Tone: "console-text"}}
		children = append(children, r.compileDOMText(chunkNode, data, path+"/job-log-chunk:"+strconv.FormatInt(chunk.ID, 10)))
	}
	first, last := stream.Chunks[0].ID, stream.Chunks[len(stream.Chunks)-1].ID
	props := giodom.ListProps{
		Axis: layout.Vertical, Viewport: unit.Dp(r.controls.LogView.MaximumHeight),
		MinimumViewport: unit.Dp(r.controls.LogView.MinimumHeight), ShrinkMain: true,
		NestedScroll: true, Estimate: 120, Overscan: 2, MaxMeasured: 128,
		ScrollToEnd: r.outputTailing, ForceEndRevision: r.outputTailRevision, SemanticLabel: "Execution output",
	}
	if stream.SelectedChunkID > 0 {
		props.ScrollTo = giodom.Key(path + "/job-log-chunk:" + strconv.FormatInt(stream.SelectedChunkID, 10))
		props.ScrollRevision = r.outputScrollRevision
	}
	if stream.HasBefore {
		props.OnReachStart = func() { r.requestJobLogPage(jobID, itemID, "before", first) }
	}
	if stream.HasAfter {
		props.OnReachEnd = func() { r.requestJobLogPage(jobID, itemID, "after", last) }
	}
	return giodom.VirtualList(giodom.Key(path+"/log"), props, giodom.Static(children...))
}
