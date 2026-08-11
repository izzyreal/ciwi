//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
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
		items, resolveErr := resolveItems(data, "jobDetails.timeline")
		if resolveErr != nil {
			r.ShowAlert("Timeline unavailable", resolveErr.Error())
			return true
		}
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if !ok || fmt.Sprint(itemMap["id"]) != arguments["id"] {
				continue
			}
			r.SetRootBinding("jobDetails", "selected_timeline_item", itemMap)
			if groups, groupErr := resolveItems(data, "jobDetails.output_groups"); groupErr == nil {
				for _, rawGroup := range groups {
					group, groupOK := rawGroup.(map[string]any)
					if !groupOK || fmt.Sprint(group["id"]) != arguments["id"] {
						continue
					}
					if stateKey := fmt.Sprint(group["state_key"]); stateKey != "" {
						r.setDisclosureState(stateKey, true, true)
					}
					r.scrollOutputTo(fmt.Sprint(group["id"]))
					break
				}
			}
			r.requestFrame()
			return true
		}
		return true
	case "change-output-search":
		r.outputSearch, r.outputMatch = arguments["query"], 0
		r.SetRootBinding("jobDetails", "output_search", r.outputSearch)
		r.selectGroupedOutputMatch(data, r.outputSearch, 0, true)
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
		r.selectGroupedOutputMatch(data, query, direction, true)
		if gtx != nil && r.pendingOutputSelection == nil {
			if matches := groupedOutputMatches(data, query); len(matches) > 0 {
				if editor := r.outputEditors[matches[r.outputMatch].itemID]; editor != nil {
					gtx.Execute(key.FocusCmd{Tag: editor})
				}
			}
		}
		r.requestFrame()
		return true
	case "copy-output":
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
		r.outputTailing = !r.outputTailing
		label, tone := "Tailing: Off", "warning"
		if r.outputTailing {
			label, tone = "Tailing: On", "success"
			r.outputTailRevision++
		}
		r.SetRootBinding("jobDetails", "tailing_label", label)
		r.SetRootBinding("jobDetails", "tailing_tone", tone)
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
		r.requestFrame()
		return true
	default:
		return false
	}
}

func (r *Renderer) scrollOutputTo(itemID string) {
	if strings.TrimSpace(itemID) == "" {
		return
	}
	r.pendingOutputScroll = itemID
	r.outputScrollRevision++
}

type groupedOutputMatch struct {
	itemID string
	index  int
	start  int
	end    int
}

func (r *Renderer) selectGroupedOutputMatch(data any, query string, direction int, selectMatch bool) {
	matches := groupedOutputMatches(data, query)
	if len(matches) == 0 {
		r.outputMatch = 0
		r.SetRootBinding("jobDetails", "output_search_count", "0/0")
		return
	}
	if direction > 0 {
		r.outputMatch = (r.outputMatch + 1) % len(matches)
	} else if direction < 0 {
		r.outputMatch = (r.outputMatch - 1 + len(matches)) % len(matches)
	} else if r.outputMatch >= len(matches) {
		r.outputMatch = 0
	}
	r.SetRootBinding("jobDetails", "output_search_count", fmt.Sprintf("%d/%d", r.outputMatch+1, len(matches)))
	if !selectMatch {
		return
	}
	match := matches[r.outputMatch]
	if match.itemID != "" {
		if groups, err := resolveItems(data, "jobDetails.output_groups"); err == nil {
			for _, raw := range groups {
				group, ok := raw.(map[string]any)
				if !ok || fmt.Sprint(group["id"]) != match.itemID {
					continue
				}
				if stateKey := fmt.Sprint(group["state_key"]); stateKey != "" {
					r.setDisclosureState(stateKey, true, true)
				}
				r.scrollOutputTo(match.itemID)
				break
			}
		}
	}
	if editor := r.outputEditors[match.itemID]; editor != nil {
		editor.SetCaret(match.start, match.end)
		r.pendingOutputSelection = nil
	} else {
		r.pendingOutputSelection = &outputSelection{itemID: match.itemID, start: match.start, end: match.end}
	}
}

func groupedOutputMatches(data any, query string) []groupedOutputMatch {
	if query == "" {
		return nil
	}
	sources := []struct{ itemID, text string }{}
	if system, err := uidsl.Resolve(data, "jobDetails.system_output"); err == nil && fmt.Sprint(system) != "" {
		sources = append(sources, struct{ itemID, text string }{"", fmt.Sprint(system)})
	}
	if groups, err := resolveItems(data, "jobDetails.output_groups"); err == nil {
		for _, raw := range groups {
			if group, ok := raw.(map[string]any); ok {
				sources = append(sources, struct{ itemID, text string }{fmt.Sprint(group["id"]), fmt.Sprint(group["output"])})
			}
		}
	}
	var matches []groupedOutputMatch
	for sourceIndex, source := range sources {
		for _, match := range outputMatches(source.text, query) {
			matches = append(matches, groupedOutputMatch{itemID: source.itemID, index: sourceIndex, start: match[0], end: match[1]})
		}
	}
	return matches
}

func outputMatches(output, query string) [][2]int {
	if query == "" {
		return nil
	}
	lowerOutput, lowerQuery := strings.ToLower(output), strings.ToLower(query)
	var matches [][2]int
	for offset := 0; offset <= len(lowerOutput)-len(lowerQuery); {
		index := strings.Index(lowerOutput[offset:], lowerQuery)
		if index < 0 {
			break
		}
		startByte, endByte := offset+index, offset+index+len(lowerQuery)
		matches = append(matches, [2]int{utf8.RuneCountInString(output[:startByte]), utf8.RuneCountInString(output[:endByte])})
		offset = endByte
	}
	return matches
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
