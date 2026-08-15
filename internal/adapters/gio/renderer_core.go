//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"strings"
	"sync"
	"time"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/presentation/operations"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
)

type ActionHandler func(uidsl.Action, map[string]string)

// Renderer owns semantic application state and compiles the shared UI DSL to
// the keyed Gio DOM. Stateful Gio widgets belong to the DOM runtime; this
// struct deliberately contains no path-indexed widget or geometry caches.
type Renderer struct {
	mu                     sync.RWMutex
	screen                 *uidsl.ScreenDocument
	data                   any
	theme                  *material.Theme
	typography             uidsl.Typography
	controls               uidsl.Controls
	palette                palette
	inputPlaceholder       color.NRGBA
	metrics                visualMetrics
	themeName              string
	pendingTheme           *material.Theme
	pendingPalette         *palette
	pendingMetrics         *visualMetrics
	pendingThemeName       string
	disclosures            map[string]bool
	persistentDisclosures  map[string]bool
	onDisclosureChange     func(map[string]bool)
	viewModes              map[string]string
	persistentViews        map[string]bool
	onViewChange           func(map[string]string)
	projectStructureFilter string
	icons                  map[string]nativeIcon
	images                 map[string]paint.ImageOp
	pageBackgroundSize     image.Point
	pageBackground         paint.ImageOp
	pageBackgroundReady    bool
	surfaceBackground      paint.ImageOp
	surfaceBackgroundReady bool
	heroBackground         paint.ImageOp
	heroBackgroundSize     image.Point
	heroBackgroundReady    bool
	onAction               ActionHandler
	invalidate             func()
	pending                *pendingConfirmation
	alert                  *nativeAlert
	outputTailing          bool
	outputSearch           string
	outputMatch            int
	outputTotalMatches     int
	outputEditors          map[string]*widget.Editor
	pendingOutputSelection *outputSelection
	pendingOutputScroll    string
	outputScrollRevision   uint64
	outputResetRevision    uint64
	outputTailRevision     uint64
	jobLogStreams          map[string]jobLogStreamSnapshot
	jobLogLoads            map[string]bool
	pendingClipboard       *string
	renderedJobID          string
	activeOperations       map[string]operations.Operation
	actionCatalog          *uidsl.ActionCatalogDocument
	notice                 *nativeNotice
	noticeQueue            []nativeNotice
	pendingScrollSection   string
	compact                bool
	viewportSize           image.Point
	viewportHeight         unit.Dp
	domInteractionRevision uint64
	openSelectKey          giodom.Key
	dom                    *screenDOMRenderer
}

type outputSelection struct {
	itemID string
	start  int
	end    int
}

type jobOutputSnapshot struct {
	System    string
	Outputs   map[string]string
	Errors    map[string]string
	ExitCodes map[string]string
}

type jobLogChunkSnapshot struct {
	ID   int64
	Text string
}

type jobLogStreamSnapshot struct {
	JobID, ItemID       string
	Chunks              []jobLogChunkSnapshot
	HasBefore, HasAfter bool
	Terminal            bool
	PageLoaded          bool
	LatestChunkID       int64
	SelectedChunkID     int64
	LoadedMode          string
}

type jobLogSearchSnapshot struct {
	JobID, ItemID               string
	SelectedIndex, TotalMatches int
	ChunkID                     int64
}

type jobLogDescriptorSnapshot struct {
	JobID    string
	Terminal bool
	Streams  map[string]int64
}

type pendingConfirmation struct {
	action    uidsl.Action
	arguments map[string]string
	title     string
	message   string
}

type nativeAlert struct {
	title   string
	message string
}

type nativeNotice struct {
	message     string
	actionLabel string
	action      uidsl.Action
	arguments   map[string]string
	duration    time.Duration
	remaining   time.Duration
	paused      bool
	expires     time.Time
}

type palette struct {
	background, backgroundGlowA, backgroundGlowB                color.NRGBA
	surface, surfaceRaised, surfaceGlow, subtle                 color.NRGBA
	text, muted, accent, accentStrong, pillBackground, pillText color.NRGBA
	noticeBackground, noticeText, noticeBorder                  color.NRGBA
	awaitingSurface, awaitingBorder, awaitingText               color.NRGBA
	border, success, warning, danger, focus                     color.NRGBA
	consoleBackground, consoleSurface, consoleBorder            color.NRGBA
	consoleText, consoleMuted, consoleAccent, consoleSuccess    color.NRGBA
	pageGradient, heroGradient                                  nativeGradient
}

type nativeGradient struct {
	kind  string
	angle float64
	stops []nativeGradientStop
}

type nativeGradientStop struct {
	color    color.NRGBA
	position float64
}

type visualMetrics struct {
	spaceSmall, spaceMedium, spaceLarge, pageWidth, pageInset                        unit.Dp
	sectionPadding, cardPadding, heroPadding, surfaceRadius                          unit.Dp
	controlRadius, controlPaddingX, controlPaddingY                                  unit.Dp
	textBody, textControl, textCode, textBadge, textSubtitle, textHeading, textTitle unit.Sp
	textJobTitle                                                                     unit.Sp
	imageBrandWidth, imageBrandHeight                                                unit.Dp
}

func NewRenderer(screen *uidsl.ScreenDocument, theme *uidsl.ThemeDocument, onAction ActionHandler) (*Renderer, error) {
	if screen == nil || theme == nil {
		return nil, fmt.Errorf("screen and theme are required")
	}
	typographyDocument, err := sharedUI.LoadTypography()
	if err != nil {
		return nil, err
	}
	controlsDocument, err := sharedUI.LoadControls()
	if err != nil {
		return nil, err
	}
	inputPlaceholder, err := parseColor(controlsDocument.Controls.Input.PlaceholderColor)
	if err != nil {
		return nil, fmt.Errorf("input placeholder color: %w", err)
	}
	materialTheme, colors, err := rendererTheme(theme, typographyDocument.Typography)
	if err != nil {
		return nil, err
	}
	images, err := embeddedImages()
	if err != nil {
		return nil, err
	}
	return &Renderer{
		screen: screen, theme: materialTheme, typography: typographyDocument.Typography, controls: controlsDocument.Controls,
		palette: colors, inputPlaceholder: inputPlaceholder, metrics: metricsFromTheme(theme.Theme, typographyDocument.Typography),
		themeName: theme.Metadata.Name, onAction: onAction,
		disclosures: map[string]bool{}, persistentDisclosures: map[string]bool{},
		viewModes: map[string]string{}, persistentViews: map[string]bool{},
		icons: tablerIcons(), images: images, outputEditors: map[string]*widget.Editor{},
		activeOperations: map[string]operations.Operation{}, outputTailing: true,
		jobLogStreams: map[string]jobLogStreamSnapshot{}, jobLogLoads: map[string]bool{},
	}, nil
}

func (r *Renderer) SetOperations(snapshot []operations.Operation) {
	active := make(map[string]operations.Operation, len(snapshot))
	for _, operation := range snapshot {
		if !operation.State.Terminal() {
			active[operation.Fingerprint] = operation
		}
	}
	r.mu.Lock()
	r.activeOperations = active
	r.mu.Unlock()
}

func (r *Renderer) SetActionCatalog(catalog *uidsl.ActionCatalogDocument) {
	r.mu.Lock()
	r.actionCatalog = catalog
	r.mu.Unlock()
}

func (r *Renderer) SetData(data any) {
	r.mu.Lock()
	r.data = data
	r.mu.Unlock()
}

func (r *Renderer) SetScreenAndData(screen *uidsl.ScreenDocument, data any) {
	r.mu.Lock()
	screenChanged := r.screen == nil || screen == nil || r.screen.Metadata.Name != screen.Metadata.Name
	preserveTopLevelBinding(r.data, data, "client")
	if screen != nil && screen.Metadata.Name == "job-details" {
		preserveJobUIState(r.data, data)
	}
	if screen != nil && screen.Metadata.Name == "settings" {
		preserveSettingsUIState(r.data, data)
	}
	r.screen, r.data = screen, data
	if screenChanged {
		// Screen identity is the lifecycle boundary for its keyed widget and
		// viewport state. Recreating the runtime also guarantees a top scroll.
		r.dom = nil
		r.openSelectKey = ""
	}
	if screen != nil && screen.Metadata.Name == "project-details" && r.projectStructureFilter != "" {
		r.setProjectStructureFilterLocked(r.projectStructureFilter)
		if current, ok := r.data.(map[string]any); ok {
			if root, ok := current["projectDetails"].(map[string]any); ok {
				r.projectStructureFilter = fmt.Sprint(root["structure_filter"])
			}
		}
	}
	r.mu.Unlock()
}

func preserveTopLevelBinding(previous, next any, key string) {
	previousData, previousOK := previous.(map[string]any)
	nextData, nextOK := next.(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	if _, exists := nextData[key]; exists {
		return
	}
	if value, exists := previousData[key]; exists {
		nextData[key] = value
	}
}

func (r *Renderer) SetRepeatedItemBinding(root, collection, keyField, keyValue, field string, value any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	rootData, ok := data[root].(map[string]any)
	if !ok {
		return false
	}
	items, ok := rootData[collection].([]any)
	if !ok {
		return false
	}
	nextItems := append([]any(nil), items...)
	found := false
	for index, raw := range items {
		item, itemOK := raw.(map[string]any)
		if !itemOK || fmt.Sprint(item[keyField]) != keyValue {
			continue
		}
		nextItem := cloneAnyMap(item)
		nextItem[field] = value
		nextItems[index], found = nextItem, true
		break
	}
	if !found {
		return false
	}
	nextRoot := cloneAnyMap(rootData)
	nextRoot[collection] = nextItems
	nextData := cloneAnyMap(data)
	nextData[root] = nextRoot
	r.data = nextData
	return true
}

func (r *Renderer) ApplyJobOutput(snapshot jobOutputSnapshot) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	root, ok := data["jobDetails"].(map[string]any)
	if !ok {
		return false
	}
	groups, ok := root["output_groups"].([]any)
	if !ok {
		return false
	}
	nextGroups := make([]any, 0, len(groups))
	for _, raw := range groups {
		group, groupOK := raw.(map[string]any)
		if !groupOK {
			nextGroups = append(nextGroups, raw)
			continue
		}
		next := cloneAnyMap(group)
		itemID := fmt.Sprint(group["id"])
		next["output"] = snapshot.Outputs[itemID]
		if value := snapshot.Errors[itemID]; value != "" {
			next["error"], next["status"], next["status_label"] = value, "failed", "Failed"
		}
		if value := snapshot.ExitCodes[itemID]; value != "" {
			next["exit_code"] = value
		}
		nextGroups = append(nextGroups, next)
	}
	nextRoot := cloneAnyMap(root)
	nextRoot["system_output"], nextRoot["output_groups"] = snapshot.System, nextGroups
	nextRoot["output"] = structuredOutputPlainText(nextRoot, nextGroups, snapshot.System)
	nextData := cloneAnyMap(data)
	nextData["jobDetails"] = nextRoot
	if r.outputSearch != "" {
		matches := groupedOutputMatches(nextData, r.outputSearch)
		if len(matches) == 0 {
			r.outputMatch, nextRoot["output_search_count"] = 0, "0/0"
		} else {
			if r.outputMatch >= len(matches) {
				r.outputMatch = 0
			}
			nextRoot["output_search_count"] = fmt.Sprintf("%d/%d", r.outputMatch+1, len(matches))
		}
	}
	r.data = nextData
	return true
}

func structuredOutputPlainText(root map[string]any, groups []any, systemOutput string) string {
	var out strings.Builder
	out.WriteString("ciwi job log\n")
	out.WriteString("Job execution ID: " + fmt.Sprint(root["id"]) + "\n")
	out.WriteString("Status: " + fmt.Sprint(root["status"]) + "\n\n")
	if strings.TrimSpace(systemOutput) != "" {
		out.WriteString(strings.TrimRight(systemOutput, "\n") + "\n\n")
	}
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out.WriteString("--------------------------------------------------------------------------------\n")
		out.WriteString(fmt.Sprint(group["title"]) + "\n")
		out.WriteString("--------------------------------------------------------------------------------\n")
		if fmt.Sprint(group["reached"]) != "true" {
			out.WriteString("Status: Not reached\n")
		}
		for _, field := range []struct{ key, label string }{{"started", "Started"}, {"duration", "Duration"}, {"exit_code", "Exit code"}, {"error", "Error"}} {
			if value := strings.TrimSpace(fmt.Sprint(group[field.key])); value != "" {
				out.WriteString(field.label + ": " + value + "\n")
			}
		}
		out.WriteString("\n")
		if fmt.Sprint(group["kind"]) == "phase" {
			out.WriteString("Details:\n" + fmt.Sprint(group["details"]) + "\n")
		} else {
			out.WriteString("YAML literal:\n'''\n" + fmt.Sprint(group["yaml_literal"]) + "\n'''\n\n")
			out.WriteString("Expanded command:\n'''\n" + fmt.Sprint(group["expanded_command"]) + "\n'''\n")
		}
		out.WriteString("\nOutput:\n'''\n")
		output := fmt.Sprint(group["output"])
		if output == "" && fmt.Sprint(group["reached"]) != "true" {
			output = "(step was not reached)"
		}
		out.WriteString(output + "\n'''\n\n")
	}
	return out.String()
}

func (r *Renderer) ShowNotice(message, actionLabel string, action uidsl.Action, arguments map[string]string, duration time.Duration) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	r.mu.Lock()
	notice := nativeNotice{message: message, actionLabel: strings.TrimSpace(actionLabel), action: action, arguments: cloneStringMap(arguments), duration: duration}
	if r.notice != nil && sameNativeNotice(*r.notice, notice) {
		r.mu.Unlock()
		return
	}
	for _, queued := range r.noticeQueue {
		if sameNativeNotice(queued, notice) {
			r.mu.Unlock()
			return
		}
	}
	if r.notice == nil {
		r.activateNoticeLocked(notice, time.Now())
	} else {
		if len(r.noticeQueue) >= presentation.TransientNoticeCapacity-1 {
			copy(r.noticeQueue, r.noticeQueue[1:])
			r.noticeQueue = r.noticeQueue[:len(r.noticeQueue)-1]
		}
		r.noticeQueue = append(r.noticeQueue, notice)
	}
	r.mu.Unlock()
	r.requestFrame()
}

func (r *Renderer) ShowAlert(title, message string) {
	r.mu.Lock()
	r.alert = &nativeAlert{title: strings.TrimSpace(title), message: strings.TrimSpace(message)}
	r.mu.Unlock()
	r.requestFrame()
}

func (r *Renderer) ScrollToSection(section string) {
	r.mu.Lock()
	r.pendingScrollSection = strings.TrimSpace(section)
	r.mu.Unlock()
	r.requestFrame()
}

func (r *Renderer) activateNoticeLocked(notice nativeNotice, now time.Time) {
	next := notice
	next.remaining = next.duration
	if next.duration > 0 {
		next.expires = now.Add(next.duration)
	}
	r.notice = &next
}

func (r *Renderer) advanceNoticeLocked(now time.Time) {
	if len(r.noticeQueue) == 0 {
		r.notice = nil
		return
	}
	next := r.noticeQueue[0]
	r.noticeQueue = append(r.noticeQueue[:0], r.noticeQueue[1:]...)
	r.activateNoticeLocked(next, now)
}

func (r *Renderer) dismissNotice() {
	r.mu.Lock()
	r.advanceNoticeLocked(time.Now())
	r.mu.Unlock()
	r.requestFrame()
}

func sameNativeNotice(left, right nativeNotice) bool {
	if left.message != right.message || left.actionLabel != right.actionLabel || left.action.Command != right.action.Command || len(left.arguments) != len(right.arguments) {
		return false
	}
	for key, value := range left.arguments {
		if right.arguments[key] != value {
			return false
		}
	}
	return true
}

func (r *Renderer) SetTheme(theme *uidsl.ThemeDocument) error {
	materialTheme, colors, err := rendererTheme(theme, r.typography)
	if err != nil {
		return err
	}
	metrics := metricsFromTheme(theme.Theme, r.typography)
	r.mu.Lock()
	r.pendingTheme, r.pendingPalette, r.pendingMetrics = materialTheme, &colors, &metrics
	r.pendingThemeName = theme.Metadata.Name
	r.mu.Unlock()
	return nil
}

func (r *Renderer) ThemeName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.pendingThemeName != "" {
		return r.pendingThemeName
	}
	return r.themeName
}

func (r *Renderer) SetRootBinding(root, key string, value any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	rootData, ok := data[root].(map[string]any)
	if !ok {
		return false
	}
	nextRoot := cloneAnyMap(rootData)
	nextRoot[key] = value
	nextData := cloneAnyMap(data)
	nextData[root] = nextRoot
	r.data = nextData
	return true
}

func (r *Renderer) SetNestedBinding(root, objectKey, key string, value any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	rootData, ok := data[root].(map[string]any)
	if !ok {
		return false
	}
	object, ok := rootData[objectKey].(map[string]any)
	if !ok {
		return false
	}
	nextObject := cloneAnyMap(object)
	nextObject[key] = value
	nextRoot := cloneAnyMap(rootData)
	nextRoot[objectKey] = nextObject
	nextData := cloneAnyMap(data)
	nextData[root] = nextRoot
	r.data = nextData
	return true
}

func (r *Renderer) SetDataBinding(key string, value any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	next := cloneAnyMap(data)
	next[key] = value
	r.data = next
	return true
}

func (r *Renderer) SetProjectStructureFilter(filter string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.setProjectStructureFilterLocked(filter) {
		return false
	}
	if data, ok := r.data.(map[string]any); ok {
		if root, ok := data["projectDetails"].(map[string]any); ok {
			r.projectStructureFilter = fmt.Sprint(root["structure_filter"])
		}
	}
	return true
}

func (r *Renderer) setProjectStructureFilterLocked(filter string) bool {
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	root, ok := data["projectDetails"].(map[string]any)
	if !ok {
		return false
	}
	nextRoot := cloneAnyMap(root)
	if !applyProjectStructureFilter(nextRoot, filter) {
		return false
	}
	nextData := cloneAnyMap(data)
	nextData["projectDetails"] = nextRoot
	r.data = nextData
	return true
}

func (r *Renderer) SetInvalidate(invalidate func()) { r.invalidate = invalidate }

func (r *Renderer) SetDisclosureStates(states map[string]bool) {
	for key, expanded := range states {
		if len(r.disclosures) >= domSemanticStateLimit {
			break
		}
		if strings.TrimSpace(key) != "" {
			r.disclosures[key], r.persistentDisclosures[key] = expanded, true
		}
	}
}

func (r *Renderer) SetDisclosureChange(handler func(map[string]bool)) {
	r.onDisclosureChange = handler
}

func (r *Renderer) SetViewStates(states map[string]string) {
	for key, mode := range states {
		if len(r.viewModes) >= domSemanticStateLimit {
			break
		}
		if strings.TrimSpace(key) != "" && (mode == "graph" || mode == "list") {
			r.viewModes[key], r.persistentViews[key] = mode, true
		}
	}
}

func (r *Renderer) SetViewChange(handler func(map[string]string)) { r.onViewChange = handler }

func (r *Renderer) Layout(gtx layout.Context) layout.Dimensions {
	return r.layoutFrame(gtx)
}

func (r *Renderer) layoutFrame(gtx layout.Context) layout.Dimensions {
	r.viewportSize = gtx.Constraints.Max
	r.viewportHeight = gtx.Metric.PxToDp(r.viewportSize.Y)
	r.compact = compactLayoutForWidth(gtx, r.controls.Viewport.CompactMaximumWidth)
	r.mu.Lock()
	if r.notice != nil && !r.notice.expires.IsZero() && !gtx.Now.Before(r.notice.expires) {
		r.advanceNoticeLocked(gtx.Now)
	}
	if r.pendingTheme != nil && r.pendingPalette != nil && r.pendingMetrics != nil {
		r.theme, r.palette, r.metrics, r.themeName = r.pendingTheme, *r.pendingPalette, *r.pendingMetrics, r.pendingThemeName
		r.pendingTheme, r.pendingPalette, r.pendingMetrics, r.pendingThemeName = nil, nil, nil, ""
		r.pageBackgroundReady, r.surfaceBackgroundReady, r.heroBackgroundReady = false, false, false
		r.dom = nil
	}
	screen, data, pendingSection := r.screen, r.data, r.pendingScrollSection
	var notice *nativeNotice
	if r.notice != nil {
		copy := *r.notice
		copy.arguments = cloneStringMap(copy.arguments)
		notice = &copy
	}
	var alert *nativeAlert
	if r.alert != nil {
		copy := *r.alert
		alert = &copy
	}
	var clipboardValue *string
	if r.pendingClipboard != nil {
		value := *r.pendingClipboard
		clipboardValue = &value
		r.pendingClipboard = nil
	}
	r.mu.Unlock()
	if clipboardValue != nil {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(*clipboardValue))})
	}
	if notice != nil && !notice.expires.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: notice.expires})
	}

	if screen != nil && screen.Metadata.Name == "job-details" {
		jobID := bindingString(data, "jobDetails.id")
		if jobID != r.renderedJobID {
			r.renderedJobID, r.outputTailing, r.outputSearch, r.outputMatch = jobID, jobOutputStartsAtTail(bindingString(data, "jobDetails.status")), "", 0
			r.jobLogStreams, r.jobLogLoads = map[string]jobLogStreamSnapshot{}, map[string]bool{}
			r.outputResetRevision++
			if r.outputTailing {
				r.outputTailRevision++
			}
			r.pendingOutputSelection, r.pendingOutputScroll = nil, ""
		}
	}
	// This is only an index into widget state reached during the current frame.
	// Rebuilding it prevents off-screen output editors from accumulating.
	r.outputEditors = map[string]*widget.Editor{}
	r.paintPageBackground(gtx)
	if screen == nil {
		return r.errorLabel(gtx, fmt.Errorf("screen unavailable"))
	}
	if data == nil {
		label := r.materialTextLabel("Loading…", "body", false)
		label.Color = r.palette.muted
		return layout.Center.Layout(gtx, label.Layout)
	}
	return r.layoutScreenDOMFrame(gtx, screen, data, pendingSection, notice, alert)
}

func (r *Renderer) requestFrame() {
	if r.invalidate != nil {
		r.invalidate()
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneAnyMap(values map[string]any) map[string]any {
	result := make(map[string]any, len(values)+1)
	for key, value := range values {
		result[key] = value
	}
	return result
}

func jobOutputStartsAtTail(status string) bool {
	if protocol.IsActiveJobExecutionStatus(status) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "waiting", "in progress", "active":
		return true
	default:
		return false
	}
}
