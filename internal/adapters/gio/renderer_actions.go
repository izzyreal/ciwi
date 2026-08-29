//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

type nativeSelectOption struct {
	value string
	label string
}

func (r *Renderer) disclosureStateKey(node uidsl.Node, data any, fallback string) (string, bool) {
	if node.Disclosure == nil || strings.TrimSpace(node.Disclosure.StateKey) == "" {
		return fallback, false
	}
	key, err := uidsl.RenderText(data, uidsl.Text{Template: node.Disclosure.StateKey})
	if err != nil || strings.TrimSpace(key) == "" {
		return fallback, false
	}
	return key, true
}

func (r *Renderer) setDisclosureState(key string, expanded, persistent bool) {
	r.disclosures[key] = expanded
	if persistent {
		r.persistentDisclosures[key] = true
		r.notifyDisclosureChange()
	}
	r.markDOMDirty()
	r.requestFrame()
}

func (r *Renderer) notifyDisclosureChange() {
	if r.onDisclosureChange == nil {
		return
	}
	states := make(map[string]bool, len(r.persistentDisclosures))
	for key := range r.persistentDisclosures {
		states[key] = r.disclosures[key]
	}
	r.onDisclosureChange(states)
}

func (r *Renderer) dispatchFromLayout(gtx layout.Context, action uidsl.Action, data any) {
	r.domInteractionRevision++
	r.dispatchAction(&gtx, action, data)
}

// dispatchRendererAction handles renderer-owned local actions independently of
// the component that emitted them. The optional layout context is only needed
// for Gio commands such as clipboard writes and immediate focus changes.
func (r *Renderer) dispatchRendererAction(gtx *layout.Context, command string, arguments map[string]string, data any) bool {
	switch command {
	case "select-timeline-item":
		r.setOutputTailing(false)
		root, ok := jobDetailsRoot(r.data)
		if !ok {
			r.ShowAlert("Timeline unavailable", "Job details are unavailable")
			return true
		}
		root["output_follow_latest"] = false
		selectJobOutputBinding(root, arguments["id"], false)
		r.outputMatch, r.outputTotalMatches = 0, 0
		root["output_search_count"] = "0/0"
		r.clearJobLogSearchSelection(bindingString(r.data, "jobDetails.id"))
		if utf8.RuneCountInString(r.outputSearch) >= 3 && r.onAction != nil {
			r.onAction(uidsl.Action{On: "activate", Command: "search-job-log"}, map[string]string{
				"jobExecutionId": bindingString(r.data, "jobDetails.id"),
				"itemId":         bindingString(r.data, "jobDetails.selected_output_group.id"),
				"query":          r.outputSearch, "selectedIndex": "0", "debounce": "false",
			})
		}
		r.pendingScrollSection = "job-output-viewer"
		r.outputResetRevision++
		r.markDOMDirty()
		r.requestFrame()
		return true
	case "change-output-search":
		r.outputSearch, r.outputMatch = arguments["query"], 0
		r.SetRootBinding("jobDetails", "output_search", r.outputSearch)
		if utf8.RuneCountInString(r.outputSearch) < 3 {
			r.outputTotalMatches = 0
			r.clearJobLogSearchSelection(bindingString(data, "jobDetails.id"))
			count := "0/0"
			if r.outputSearch != "" {
				count = "Enter 3+ characters"
			}
			r.SetRootBinding("jobDetails", "output_search_count", count)
			if r.onAction != nil {
				r.onAction(uidsl.Action{On: "activate", Command: "cancel-job-log-search"}, nil)
			}
		} else if r.onAction != nil {
			r.onAction(uidsl.Action{On: "activate", Command: "search-job-log"}, map[string]string{
				"jobExecutionId": bindingString(data, "jobDetails.id"),
				"itemId":         bindingString(data, "jobDetails.selected_output_group.id"),
				"query":          r.outputSearch, "selectedIndex": "0", "debounce": "true",
			})
		}
		r.requestFrame()
		return true
	case "find-output":
		direction := 1
		if arguments["direction"] == "previous" {
			direction = -1
		}
		query := arguments["query"]
		if query == "" {
			query = r.outputSearch
		}
		if utf8.RuneCountInString(query) >= 3 && r.onAction != nil {
			target := r.outputMatch
			if r.outputTotalMatches > 0 {
				target = (target + direction + r.outputTotalMatches) % r.outputTotalMatches
			}
			r.onAction(uidsl.Action{On: "activate", Command: "search-job-log"}, map[string]string{
				"jobExecutionId": bindingString(data, "jobDetails.id"),
				"itemId":         bindingString(data, "jobDetails.selected_output_group.id"),
				"query":          query, "selectedIndex": strconv.Itoa(target), "debounce": "false",
			})
		}
		return true
	case "copy-output":
		if r.onAction != nil {
			r.onAction(uidsl.Action{On: "activate", Command: "copy-full-job-log"}, map[string]string{
				"jobExecutionId": bindingString(data, "jobDetails.id"),
			})
			return true
		}
		if gtx == nil {
			r.ShowAlert("Action unavailable", "Clipboard access requires a direct control event.")
			return true
		}
		output, resolveErr := uidsl.Resolve(data, "jobDetails.output")
		if resolveErr != nil {
			r.ShowAlert("Output unavailable", resolveErr.Error())
			return true
		}
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(fmt.Sprint(output)))})
		r.ShowNotice("Output copied", "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
		return true
	case "copy-text":
		if gtx == nil {
			r.ShowAlert("Action unavailable", "Clipboard access requires a direct control event.")
			return true
		}
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(arguments["text"]))})
		r.ShowNotice("Copied", "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
		return true
	case "toggle-output-tailing":
		enabled := !r.outputTailing
		r.setOutputTailing(enabled)
		if enabled {
			r.armOutputTailing()
			if _, ok := jobDetailsRoot(r.data); ok {
				r.pendingScrollSection = "job-output-viewer"
				r.outputResetRevision++
			}
		}
		r.requestFrame()
		return true
	case "set-disclosures":
		prefix := arguments["prefix"]
		expanded, parseErr := strconv.ParseBool(arguments["expanded"])
		if parseErr != nil || prefix == "" {
			r.ShowAlert("Action unavailable", "Invalid disclosure group")
			return true
		}
		for key := range r.persistentDisclosures {
			if strings.HasPrefix(key, prefix) {
				r.disclosures[key] = expanded
			}
		}
		r.notifyDisclosureChange()
		r.markDOMDirty()
		r.requestFrame()
		return true
	default:
		return false
	}
}

func (r *Renderer) setOutputTailing(enabled bool) {
	if enabled && len(r.jobLogSelections) > 0 {
		r.jobLogSelections = map[string]nativeJobLogTextSelection{}
		r.markDOMDirty()
	}
	if r.outputTailing == enabled {
		return
	}
	r.outputTailing = enabled
	r.markDOMDirty()
	label, tone := "Tailing: Off", "accent"
	if enabled {
		label, tone = "Tailing: On", "success"
		r.outputTailRevision++
	}
	r.SetRootBinding("jobDetails", "tailing_label", label)
	r.SetRootBinding("jobDetails", "tailing_tone", tone)
	if !enabled {
		r.SetRootBinding("jobDetails", "output_follow_latest", false)
		r.SetRootBinding("jobDetails", "output_follow_anchor_id", "")
	}
}

func (r *Renderer) armOutputTailing() {
	if root, ok := jobDetailsRoot(r.data); ok {
		root["output_follow_latest"] = true
		root["output_follow_anchor_id"] = latestJobOutputBindingID(root)
	}
}

func (r *Renderer) resumeOutputTailingAtEnd() {
	if r.outputTailing || len(r.jobLogSelections) > 0 {
		return
	}
	r.setOutputTailing(true)
	r.armOutputTailing()
	r.requestFrame()
}

func jobDetailsRoot(data any) (map[string]any, bool) {
	root, ok := data.(map[string]any)
	if !ok {
		return nil, false
	}
	details, ok := root["jobDetails"].(map[string]any)
	return details, ok
}

func (r *Renderer) scrollOutputTo(itemID string) {
	if strings.TrimSpace(itemID) == "" {
		return
	}
	r.pendingOutputScroll = itemID
	r.outputScrollRevision++
	r.markDOMDirty()
}

func (r *Renderer) dispatch(action uidsl.Action, data any) {
	r.dispatchAction(nil, action, data)
}

func (r *Renderer) dispatchAction(gtx *layout.Context, action uidsl.Action, data any) {
	arguments, err := actionArguments(action, data)
	if err != nil {
		r.ShowAlert("Action unavailable", err.Error())
		return
	}
	if action.Confirm != nil {
		title, err := uidsl.RenderText(data, uidsl.Text{Template: action.Confirm.Title})
		if err != nil {
			r.ShowAlert("Action unavailable", err.Error())
			return
		}
		message, err := uidsl.RenderText(data, uidsl.Text{Template: action.Confirm.Message})
		if err != nil {
			r.ShowAlert("Action unavailable", err.Error())
			return
		}
		r.pending = &pendingConfirmation{action: action, arguments: arguments, title: title, message: message}
		r.requestFrame()
		return
	}
	if r.dispatchRendererAction(gtx, action.Command, arguments, data) {
		return
	}
	if r.onAction == nil {
		return
	}
	r.onAction(action, arguments)
}

func actionArguments(action uidsl.Action, data any) (map[string]string, error) {
	arguments := make(map[string]string, len(action.Arguments))
	for name, expression := range action.Arguments {
		value, err := uidsl.RenderText(data, uidsl.Text{Template: expression})
		if err != nil {
			return nil, err
		}
		arguments[name] = value
	}
	return arguments, nil
}
