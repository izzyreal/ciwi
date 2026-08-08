//go:build darwin || ios || linux || windows

package gio

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	giotext "gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/presentation/operations"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
	_ "golang.org/x/image/bmp"
)

type ActionHandler func(uidsl.Action, map[string]string)

type Renderer struct {
	mu                      sync.RWMutex
	screen                  *uidsl.ScreenDocument
	data                    any
	theme                   *material.Theme
	typography              uidsl.Typography
	controls                uidsl.Controls
	palette                 palette
	metrics                 visualMetrics
	themeName               string
	pendingTheme            *material.Theme
	pendingPalette          *palette
	pendingMetrics          *visualMetrics
	pendingThemeName        string
	list                    layout.List
	buttons                 map[string]*widget.Clickable
	disclosures             map[string]bool
	persistentDisclosures   map[string]bool
	onDisclosureChange      func(map[string]bool)
	viewModes               map[string]string
	persistentViews         map[string]bool
	graphScales             map[string]float32
	graphSelections         map[string]string
	graphViewports          map[string]*graphViewportState
	projectStructureFilter  string
	onViewChange            func(map[string]string)
	selectables             map[string]*widget.Selectable
	textEditors             map[string]*widget.Editor
	inputEditors            map[string]*widget.Editor
	selectOpen              map[string]bool
	selectDismiss           map[string]*widget.Clickable
	selectLists             map[string]*layout.List
	scrollers               map[string]*layout.List
	scrollGuards            map[string]*scrollGestureGuard
	icons                   map[string]nativeIcon
	images                  map[string]paint.ImageOp
	dynamicImages           map[string]dynamicImage
	pageBackgroundSize      image.Point
	pageBackground          paint.ImageOp
	pageBackgroundReady     bool
	surfaceBackgrounds      map[backgroundTextureKey]paint.ImageOp
	visualOps               *visualOpCache
	loaderTextures          map[loaderTextureKey]*loaderTextureEntry
	loaderTextureClock      uint64
	onAction                ActionHandler
	invalidate              func()
	pending                 *pendingConfirmation
	alert                   *nativeAlert
	activeSheet             *activeSheet
	resetScroll             bool
	outputTailing           bool
	outputSearch            string
	outputMatch             int
	outputEditors           map[string]*widget.Editor
	outputScroller          *layout.List
	pendingOutputSelection  *outputSelection
	renderedJobID           string
	activeOperations        map[string]operations.Operation
	actionCatalog           *uidsl.ActionCatalogDocument
	notice                  *nativeNotice
	noticeQueue             []nativeNotice
	pendingScrollSection    string
	activatedInteractions   map[string]bool
	pendingNodeActivations  []pendingNodeActivation
	compact                 bool
	viewportSize            image.Point
	suppressTouchActivation bool
}

type outputSelection struct {
	itemID string
	start  int
	end    int
}

type backgroundTextureKey struct {
	size            image.Point
	progressOpacity uint8
}

type jobOutputSnapshot struct {
	System    string
	Outputs   map[string]string
	Errors    map[string]string
	ExitCodes map[string]string
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

type activeSheet struct {
	path       string
	title      string
	node       uidsl.Node
	data       any
	stateKey   string
	persistent bool
	list       layout.List
	seen       bool
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

type pendingNodeActivation struct {
	path   string
	action uidsl.Action
	data   any
}

type dynamicImage struct {
	encoded string
	source  paint.ImageOp
}

type palette struct {
	background, backgroundStart, backgroundEnd, backgroundGlowA, backgroundGlowB color.NRGBA
	heroStart, heroEnd, surface, surfaceRaised, surfaceGlow, subtle              color.NRGBA
	text, muted, accent, accentStrong, pillBackground, pillText                  color.NRGBA
	noticeBackground, noticeText, noticeBorder                                   color.NRGBA
	awaitingSurface, awaitingBorder, awaitingText                                color.NRGBA
	border, success, warning, danger, focus                                      color.NRGBA
	consoleBackground, consoleSurface, consoleBorder                             color.NRGBA
	consoleText, consoleMuted, consoleAccent, consoleSuccess                     color.NRGBA
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
	materialTheme, colors, err := rendererTheme(theme, typographyDocument.Typography)
	if err != nil {
		return nil, err
	}
	iconSet := tablerIcons()
	imageSet, err := embeddedImages()
	if err != nil {
		return nil, err
	}
	renderer := &Renderer{
		screen: screen, theme: materialTheme, typography: typographyDocument.Typography, controls: controlsDocument.Controls, palette: colors,
		metrics: metricsFromTheme(theme.Theme, typographyDocument.Typography), themeName: theme.Metadata.Name, onAction: onAction,
		list: layout.List{Axis: layout.Vertical}, buttons: map[string]*widget.Clickable{}, disclosures: map[string]bool{},
		persistentDisclosures: map[string]bool{},
		viewModes:             map[string]string{}, persistentViews: map[string]bool{}, graphScales: map[string]float32{}, graphSelections: map[string]string{}, graphViewports: map[string]*graphViewportState{},
		selectables: map[string]*widget.Selectable{}, textEditors: map[string]*widget.Editor{}, inputEditors: map[string]*widget.Editor{},
		selectOpen: map[string]bool{}, selectDismiss: map[string]*widget.Clickable{}, selectLists: map[string]*layout.List{}, scrollers: map[string]*layout.List{}, scrollGuards: map[string]*scrollGestureGuard{}, outputEditors: map[string]*widget.Editor{},
		icons: iconSet, images: imageSet, dynamicImages: map[string]dynamicImage{},
		surfaceBackgrounds: map[backgroundTextureKey]paint.ImageOp{},
		visualOps:          newVisualOpCache(maxVisualOpCacheEntries),
		loaderTextures:     map[loaderTextureKey]*loaderTextureEntry{},
		activeOperations:   map[string]operations.Operation{},
	}
	return renderer, nil
}

// SetOperations replaces the renderer's view of in-flight work. The
// coordinator snapshot is the source of truth; widgets only derive their
// disabled/pending presentation from it.
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
	if screenChanged {
		r.resetScroll = true
		r.activeSheet = nil
	}
	r.screen = screen
	r.data = data
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
		nextItem := make(map[string]any, len(item)+1)
		for existingKey, existingValue := range item {
			nextItem[existingKey] = existingValue
		}
		nextItem[field] = value
		nextItems[index] = nextItem
		found = true
		break
	}
	if !found {
		return false
	}
	nextRoot := make(map[string]any, len(rootData))
	for existingKey, existingValue := range rootData {
		nextRoot[existingKey] = existingValue
	}
	nextRoot[collection] = nextItems
	nextData := make(map[string]any, len(data))
	for existingKey, existingValue := range data {
		nextData[existingKey] = existingValue
	}
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
	rootData, ok := data["jobDetails"].(map[string]any)
	if !ok {
		return false
	}
	groups, ok := rootData["output_groups"].([]any)
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
		nextGroup := make(map[string]any, len(group)+3)
		for key, value := range group {
			nextGroup[key] = value
		}
		itemID := fmt.Sprint(group["id"])
		nextGroup["output"] = snapshot.Outputs[itemID]
		if value := snapshot.Errors[itemID]; value != "" {
			nextGroup["error"] = value
			nextGroup["status"] = "failed"
			nextGroup["status_label"] = "Failed"
		}
		if value := snapshot.ExitCodes[itemID]; value != "" {
			nextGroup["exit_code"] = value
		}
		nextGroups = append(nextGroups, nextGroup)
	}
	nextRoot := make(map[string]any, len(rootData)+3)
	for key, value := range rootData {
		nextRoot[key] = value
	}
	nextRoot["system_output"] = snapshot.System
	nextRoot["output_groups"] = nextGroups
	nextRoot["output"] = structuredOutputPlainText(nextRoot, nextGroups, snapshot.System)
	nextData := make(map[string]any, len(data))
	for key, value := range data {
		nextData[key] = value
	}
	nextData["jobDetails"] = nextRoot
	if r.outputSearch != "" {
		matches := groupedOutputMatches(nextData, r.outputSearch)
		if len(matches) == 0 {
			r.outputMatch = 0
			nextRoot["output_search_count"] = "0/0"
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
	copy := notice
	copy.remaining = copy.duration
	copy.paused = false
	if copy.duration > 0 {
		copy.expires = now.Add(copy.duration)
	}
	r.notice = &copy
}

func (r *Renderer) setNoticePaused(paused bool, now time.Time) {
	r.mu.Lock()
	if r.notice == nil || r.notice.paused == paused || r.notice.duration <= 0 {
		r.mu.Unlock()
		return
	}
	if paused {
		r.notice.remaining = max(time.Duration(0), r.notice.expires.Sub(now))
		r.notice.expires = time.Time{}
		r.notice.paused = true
	} else {
		r.notice.expires = now.Add(r.notice.remaining)
		r.notice.paused = false
	}
	r.mu.Unlock()
	r.requestFrame()
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

func (r *Renderer) NoticeExpiry() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.notice != nil && !r.notice.expires.IsZero() {
		return r.notice.expires
	}
	return time.Time{}
}

func (r *Renderer) ClearExpiredNotice(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.notice != nil && !r.notice.expires.IsZero() && !now.Before(r.notice.expires) {
		r.advanceNoticeLocked(now)
		return true
	}
	return false
}

func (r *Renderer) SetTheme(theme *uidsl.ThemeDocument) error {
	materialTheme, colors, err := rendererTheme(theme, r.typography)
	if err != nil {
		return err
	}
	r.mu.Lock()
	metrics := metricsFromTheme(theme.Theme, r.typography)
	r.pendingTheme = materialTheme
	r.pendingPalette = &colors
	r.pendingMetrics = &metrics
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
	nextRoot := make(map[string]any, len(rootData)+1)
	for existingKey, existingValue := range rootData {
		nextRoot[existingKey] = existingValue
	}
	nextRoot[key] = value
	nextData := make(map[string]any, len(data))
	for existingKey, existingValue := range data {
		nextData[existingKey] = existingValue
	}
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
	nextObject := make(map[string]any, len(object)+1)
	for existingKey, existingValue := range object {
		nextObject[existingKey] = existingValue
	}
	nextObject[key] = value
	nextRoot := make(map[string]any, len(rootData))
	for existingKey, existingValue := range rootData {
		nextRoot[existingKey] = existingValue
	}
	nextRoot[objectKey] = nextObject
	nextData := make(map[string]any, len(data))
	for existingKey, existingValue := range data {
		nextData[existingKey] = existingValue
	}
	nextData[root] = nextRoot
	r.data = nextData
	return true
}

// SetDataBinding replaces or adds one top-level binding without disturbing the
// current screen data. Client-local state such as connectivity deliberately
// lives beside server-provided view models so every screen can consume it.
func (r *Renderer) SetDataBinding(key string, value any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	next := make(map[string]any, len(data)+1)
	for existingKey, existingValue := range data {
		next[existingKey] = existingValue
	}
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
	nextRoot := make(map[string]any, len(root)+2)
	for key, value := range root {
		nextRoot[key] = value
	}
	if !applyProjectStructureFilter(nextRoot, filter) {
		return false
	}
	nextData := make(map[string]any, len(data))
	for key, value := range data {
		nextData[key] = value
	}
	nextData["projectDetails"] = nextRoot
	r.data = nextData
	return true
}

func (r *Renderer) SetInvalidate(invalidate func()) {
	r.invalidate = invalidate
}

func (r *Renderer) SetDisclosureStates(states map[string]bool) {
	for key, expanded := range states {
		if strings.TrimSpace(key) != "" {
			r.disclosures[key] = expanded
		}
	}
}

func (r *Renderer) SetDisclosureChange(handler func(map[string]bool)) {
	r.onDisclosureChange = handler
}

func (r *Renderer) SetViewStates(states map[string]string) {
	for key, mode := range states {
		if strings.TrimSpace(key) != "" && (mode == "graph" || mode == "list") {
			r.viewModes[key] = mode
		}
	}
}

func (r *Renderer) SetViewChange(handler func(map[string]string)) {
	r.onViewChange = handler
}

func (r *Renderer) Layout(gtx layout.Context) layout.Dimensions {
	return r.layoutForPlatform(gtx, runtime.GOOS)
}

func (r *Renderer) layoutForPlatform(gtx layout.Context, platform string) layout.Dimensions {
	r.viewportSize = gtx.Constraints.Max
	r.suppressTouchActivation = false
	r.compact = compactLayoutForPlatform(gtx, platform)
	r.activatedInteractions = map[string]bool{}
	r.pendingNodeActivations = nil
	r.mu.Lock()
	if r.notice != nil && !r.notice.expires.IsZero() && !gtx.Now.Before(r.notice.expires) {
		r.advanceNoticeLocked(gtx.Now)
	}
	if r.pendingTheme != nil && r.pendingPalette != nil && r.pendingMetrics != nil {
		r.theme = r.pendingTheme
		r.palette = *r.pendingPalette
		r.metrics = *r.pendingMetrics
		r.themeName = r.pendingThemeName
		r.pendingTheme = nil
		r.pendingPalette = nil
		r.pendingMetrics = nil
		r.pendingThemeName = ""
		r.pageBackgroundReady = false
		r.surfaceBackgrounds = map[backgroundTextureKey]paint.ImageOp{}
		r.visualOps.reset()
		r.resetLoaderTextures()
		for _, icon := range r.icons {
			icon.resetVisualCache()
		}
	}
	screen, data, resetScroll := r.screen, r.data, r.resetScroll
	pendingScrollSection := r.pendingScrollSection
	var notice *nativeNotice
	if r.notice != nil {
		copy := *r.notice
		copy.arguments = cloneStringMap(r.notice.arguments)
		notice = &copy
	}
	var alert *nativeAlert
	if r.alert != nil {
		copy := *r.alert
		alert = &copy
	}
	r.resetScroll = false
	r.mu.Unlock()
	if resetScroll {
		r.list.Position = layout.Position{}
	}
	if screen != nil && screen.Metadata.Name == "job-details" {
		jobID := bindingString(data, "jobDetails.id")
		if jobID != r.renderedJobID {
			r.renderedJobID = jobID
			r.outputTailing = true
			r.outputSearch = ""
			r.outputMatch = 0
			r.outputEditors = map[string]*widget.Editor{}
			r.outputScroller = nil
			r.pendingOutputSelection = nil
		}
	}
	r.paintPageBackground(gtx)
	if data == nil {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutLoadingState(gtx)
		})
	}
	root, _ := applyGioOverride(screen.Screen.Root, r.compact)
	children := root.Children
	if pendingScrollSection != "" {
		for index := range children {
			if children[index].ID == pendingScrollSection {
				r.list.ScrollTo(index)
				// layout.List applies a programmatic scroll over its current and
				// following layout pass. Guarantee that follow-up pass even when
				// navigating to a section on the screen that is already visible.
				gtx.Execute(op.InvalidateCmd{})
				r.mu.Lock()
				if r.pendingScrollSection == pendingScrollSection {
					r.pendingScrollSection = ""
				}
				r.mu.Unlock()
				break
			}
		}
	}
	if r.compact && r.activeSheet != nil {
		r.activeSheet.seen = false
	}
	body := func(gtx layout.Context) layout.Dimensions {
		pageInset := r.pageInset()
		return layout.Inset{Left: pageInset, Right: pageInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if pageWidth := gtx.Dp(r.metrics.pageWidth); pageWidth > 0 && gtx.Constraints.Max.X > pageWidth {
				marginPixels := gtx.Constraints.Max.X - pageWidth
				margin := unit.Dp(float32(marginPixels) / (2 * gtx.Metric.PxPerDp))
				return layout.Inset{Left: margin, Right: margin}.Layout(gtx, r.layoutRootChildren(children, root, screen, data))
			}
			return r.layoutRootChildren(children, root, screen, data)(gtx)
		})
	}
	content := body
	if r.compact && r.activeSheet != nil {
		content = func(gtx layout.Context) layout.Dimensions {
			return r.layoutSheetOverlay(gtx, body)
		}
	}
	if notice != nil {
		content = func(underlay layout.Widget) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				return r.layoutNoticeOverlay(gtx, underlay, notice)
			}
		}(content)
	}
	if alert != nil {
		content = func(underlay layout.Widget) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				return layoutModalOverlay(gtx, underlay, r.layoutAlert)
			}
		}(content)
	}
	if r.pending != nil {
		content = func(underlay layout.Widget) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				return layoutModalOverlay(gtx, underlay, r.layoutConfirmation)
			}
		}(content)
	}
	dimensions := content(gtx)
	r.flushNodeActivations(gtx)
	if r.compact && r.activeSheet != nil && !r.activeSheet.seen {
		r.activeSheet = nil
	}
	return dimensions
}

func (r *Renderer) markInteraction(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if r.activatedInteractions == nil {
		r.activatedInteractions = map[string]bool{}
	}
	r.activatedInteractions[path] = true
}

func (r *Renderer) queueNodeActivation(path string, action uidsl.Action, data any) {
	r.pendingNodeActivations = append(r.pendingNodeActivations, pendingNodeActivation{path: path, action: action, data: data})
}

func (r *Renderer) flushNodeActivations(gtx layout.Context) {
	pending := r.pendingNodeActivations
	r.pendingNodeActivations = nil
	for _, candidate := range pending {
		prefix := candidate.path + "/"
		suppressed := false
		for path := range r.activatedInteractions {
			if strings.HasPrefix(path, prefix) {
				suppressed = true
				break
			}
		}
		if !suppressed {
			for _, other := range pending {
				if other.path != candidate.path && strings.HasPrefix(other.path, prefix) {
					suppressed = true
					break
				}
			}
		}
		if !suppressed {
			r.dispatchFromLayout(gtx, candidate.action, candidate.data)
		}
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

func (r *Renderer) paintPageBackground(gtx layout.Context) {
	viewport := gtx.Constraints.Max
	if viewport.X <= 0 || viewport.Y <= 0 {
		return
	}
	rect := image.Rectangle{Max: viewport}
	backgroundClip := clip.Rect(rect).Push(gtx.Ops)
	textureSize := gradientTextureSize(viewport)
	if !r.pageBackgroundReady || r.pageBackgroundSize != textureSize {
		r.pageBackground = paint.NewImageOp(renderPageBackground(textureSize, r.palette))
		r.pageBackground.Filter = paint.FilterLinear
		r.pageBackgroundSize = textureSize
		r.pageBackgroundReady = true
	}
	paintScaledImage(gtx, r.pageBackground, viewport)
	backgroundClip.Pop()
}

const maxGradientTextureDimension = 384

func gradientTextureSize(target image.Point) image.Point {
	maximum := max(target.X, target.Y)
	if maximum <= 0 {
		return image.Point{}
	}
	if maximum <= maxGradientTextureDimension {
		return target
	}
	scale := float64(maxGradientTextureDimension) / float64(maximum)
	return image.Pt(max(1, int(math.Round(float64(target.X)*scale))), max(1, int(math.Round(float64(target.Y)*scale))))
}

func paintScaledImage(gtx layout.Context, imageOp paint.ImageOp, target image.Point) {
	paintScaledImageOps(gtx.Ops, imageOp, target)
}

func paintScaledImageOps(ops *op.Ops, imageOp paint.ImageOp, target image.Point) {
	source := imageOp.Size()
	if source.X <= 0 || source.Y <= 0 || target.X <= 0 || target.Y <= 0 {
		return
	}
	scale := f32.Pt(float32(target.X)/float32(source.X), float32(target.Y)/float32(source.Y))
	transform := op.Affine(f32.AffineId().Scale(f32.Point{}, scale)).Push(ops)
	imageOp.Add(ops)
	paint.PaintOp{}.Add(ops)
	transform.Pop()
}

func cssGradientLine(rect image.Rectangle, angleDegrees float64) (f32.Point, f32.Point) {
	angle := angleDegrees * math.Pi / 180
	direction := f32.Pt(float32(math.Sin(angle)), float32(-math.Cos(angle)))
	width, height := float32(rect.Dx()), float32(rect.Dy())
	halfExtent := (float32(math.Abs(float64(direction.X)))*width + float32(math.Abs(float64(direction.Y)))*height) / 2
	center := f32.Pt(float32(rect.Min.X)+width/2, float32(rect.Min.Y)+height/2)
	return f32.Pt(center.X-direction.X*halfExtent, center.Y-direction.Y*halfExtent),
		f32.Pt(center.X+direction.X*halfExtent, center.Y+direction.Y*halfExtent)
}

func renderPageBackground(size image.Point, colors palette) *image.NRGBA {
	gradient := newThreeStopGradient(size, 145, colors.backgroundStart, .48, colors.background, colors.backgroundEnd)
	glowB := newRadialGlow(size, .90, .08, .34, colors.backgroundGlowB, .82)
	glowA := newRadialGlow(size, .12, -.10, .38, colors.backgroundGlowA, .86)
	return renderCSSBackground(size, func(x, y float64) color.NRGBA {
		base := gradient.pixel(x, y)
		base = glowB.composite(base, x, y)
		return glowA.composite(base, x, y)
	})
}

func renderSurfaceBackground(size image.Point, colors palette) *image.NRGBA {
	gradient := newThreeStopGradient(size, 145, colors.surface, 1, colors.subtle, colors.subtle)
	glow := newRadialGlow(size, 1, 0, .38, colors.surfaceGlow, 1)
	return renderCSSBackground(size, func(x, y float64) color.NRGBA {
		return glow.composite(gradient.pixel(x, y), x, y)
	})
}

func renderSurfaceProgressBackground(size image.Point, colors palette, progressWeight float64) *image.NRGBA {
	result := renderSurfaceBackground(size, colors)
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			result.SetNRGBA(x, y, mixColorSRGB(result.NRGBAAt(x, y), colors.success, progressWeight))
		}
	}
	return result
}

func renderCSSBackground(size image.Point, pixel func(x, y float64) color.NRGBA) *image.NRGBA {
	result := image.NewNRGBA(image.Rectangle{Max: size})
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			result.SetNRGBA(x, y, pixel(float64(x)+.5, float64(y)+.5))
		}
	}
	return result
}

type threeStopGradient struct {
	startX, startY, dx, dy, denominator, middlePosition float64
	start, middle, end                                  color.NRGBA
}

func newThreeStopGradient(size image.Point, angleDegrees float64, start color.NRGBA, middlePosition float64, middle, end color.NRGBA) threeStopGradient {
	lineStart, lineEnd := cssGradientLine(image.Rectangle{Max: size}, angleDegrees)
	dx, dy := float64(lineEnd.X-lineStart.X), float64(lineEnd.Y-lineStart.Y)
	return threeStopGradient{
		startX: float64(lineStart.X), startY: float64(lineStart.Y), dx: dx, dy: dy,
		denominator: dx*dx + dy*dy, middlePosition: max(0, min(middlePosition, 1)),
		start: start, middle: middle, end: end,
	}
}

func (gradient threeStopGradient) pixel(x, y float64) color.NRGBA {
	t := 0.0
	if gradient.denominator > 0 {
		t = ((x-gradient.startX)*gradient.dx + (y-gradient.startY)*gradient.dy) / gradient.denominator
	}
	t = max(0, min(t, 1))
	if t <= gradient.middlePosition && gradient.middlePosition > 0 {
		return mixColorSRGB(gradient.start, gradient.middle, t/gradient.middlePosition)
	}
	if gradient.middlePosition >= 1 {
		return gradient.middle
	}
	return mixColorSRGB(gradient.middle, gradient.end, (t-gradient.middlePosition)/(1-gradient.middlePosition))
}

type radialGlow struct {
	cx, cy, radius, opacity float64
	color                   color.NRGBA
}

func newRadialGlow(size image.Point, centerX, centerY, stopPosition float64, glow color.NRGBA, opacity float64) radialGlow {
	cx, cy := float64(size.X)*centerX, float64(size.Y)*centerY
	maxRadius := 0.0
	for _, corner := range [][2]float64{{0, 0}, {float64(size.X), 0}, {float64(size.X), float64(size.Y)}, {0, float64(size.Y)}} {
		maxRadius = max(maxRadius, math.Hypot(corner[0]-cx, corner[1]-cy))
	}
	return radialGlow{
		cx: cx, cy: cy, radius: maxRadius * stopPosition,
		opacity: opacity * float64(glow.A) / 255,
		color:   color.NRGBA{R: glow.R, G: glow.G, B: glow.B, A: 255},
	}
}

func (glow radialGlow) composite(background color.NRGBA, x, y float64) color.NRGBA {
	if glow.radius <= 0 || glow.opacity <= 0 {
		return background
	}
	alpha := glow.opacity * max(0, 1-math.Hypot(x-glow.cx, y-glow.cy)/glow.radius)
	return mixColorSRGB(background, glow.color, alpha)
}

func compactLayoutForPlatform(gtx layout.Context, platform string) bool {
	pxPerDp := gtx.Metric.PxPerDp
	if pxPerDp <= 0 {
		pxPerDp = 1
	}
	return compactViewport(platform, float32(gtx.Constraints.Max.X)/pxPerDp, float32(gtx.Constraints.Max.Y)/pxPerDp)
}

func compactViewport(platform string, width, height float32) bool {
	if width <= float32(compactLayoutWidth) {
		return true
	}
	return platform == "ios" && min(width, height) <= float32(compactLayoutWidth)
}

func (r *Renderer) pageInset() unit.Dp {
	if !r.compact {
		return r.metrics.pageInset
	}
	// Phone screens need the same breathing room as larger screens without
	// spending a significant fraction of their width on an ornamental gutter.
	return max(unit.Dp(2), r.metrics.pageInset*.2)
}

type scrollGestureGuard struct {
	position       layout.Position
	inertial       bool
	guardedTouches map[pointer.ID]struct{}
}

func (g *scrollGestureGuard) suppressActivations(gtx layout.Context) bool {
	suppress := len(g.guardedTouches) > 0
	filter := pointer.Filter{Target: g, Kinds: pointer.Press | pointer.Release | pointer.Cancel}
	for {
		raw, ok := gtx.Event(filter)
		if !ok {
			return suppress
		}
		e, ok := raw.(pointer.Event)
		if !ok || e.Source != pointer.Touch {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if !g.inertial {
				continue
			}
			if g.guardedTouches == nil {
				g.guardedTouches = map[pointer.ID]struct{}{}
			}
			g.guardedTouches[e.PointerID] = struct{}{}
			suppress = true
		case pointer.Release, pointer.Cancel:
			if _, guarded := g.guardedTouches[e.PointerID]; guarded {
				delete(g.guardedTouches, e.PointerID)
				suppress = true
			}
		}
	}
}

func (g *scrollGestureGuard) observe(list *layout.List) {
	position := list.Position
	moved := position.First != g.position.First || position.Offset != g.position.Offset
	g.inertial = moved && !list.Dragging()
	g.position = position
}

func (r *Renderer) layoutGuardedList(gtx layout.Context, key string, list *layout.List, length int, element layout.ListElement) layout.Dimensions {
	if r.scrollGuards == nil {
		r.scrollGuards = map[string]*scrollGestureGuard{}
	}
	guard := r.scrollGuards[key]
	if guard == nil {
		guard = &scrollGestureGuard{position: list.Position}
		r.scrollGuards[key] = guard
	}
	if guard.suppressActivations(gtx) {
		r.suppressTouchActivation = true
	}
	macro := op.Record(gtx.Ops)
	dimensions := list.Layout(gtx, length, element)
	call := macro.Stop()
	// The guard observes the touch without grabbing it. A tap that began during
	// inertia is still prevented from activating a control, while a drag remains
	// available to the list to take over scrolling and start a fresh fling.
	guard.observe(list)
	area := clip.Rect{Max: dimensions.Size}.Push(gtx.Ops)
	call.Add(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, guard)
	pass.Pop()
	area.Pop()
	return dimensions
}

func (r *Renderer) clicked(gtx layout.Context, button *widget.Clickable) bool {
	clicked := button.Clicked(gtx)
	return clicked && !r.suppressTouchActivation
}

func (r *Renderer) layoutRootChildren(children []uidsl.Node, root uidsl.Node, screen *uidsl.ScreenDocument, data any) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		pageInset := r.pageInset()
		itemCount := len(children)
		return r.layoutGuardedList(gtx, "root", &r.list, itemCount, func(gtx layout.Context, index int) layout.Dimensions {
			inset := layout.Inset{}
			if index == 0 {
				inset.Top = pageInset
			}
			if index < len(children)-1 {
				inset.Bottom = r.spacing(root.Layout.Gap)
			} else {
				inset.Bottom = pageInset
			}
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutNode(gtx, children[index], data, fmt.Sprintf("%s/root/%d", screen.Metadata.Name, index))
			})
		})
	}
}

func layoutModalOverlay(gtx layout.Context, body, confirmation layout.Widget) layout.Dimensions {
	viewportConstraints := gtx.Constraints
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Stack removes the minimum constraints from stacked children. Restore
			// the viewport constraints so showing a modal cannot reflow the page
			// underneath it.
			gtx.Constraints = viewportConstraints
			return body(gtx)
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, color.NRGBA{A: 0x70})
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(confirmation),
	)
}

func (r *Renderer) layoutNoticeOverlay(gtx layout.Context, body layout.Widget, notice *nativeNotice) layout.Dimensions {
	viewportConstraints := gtx.Constraints
	semantic.DescriptionOp(notice.message).Add(gtx.Ops)
	if !notice.expires.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: notice.expires})
	}
	actionButton := r.button("native-notice/action")
	dismissButton := r.button("native-notice/dismiss")
	r.setNoticePaused(actionButton.Hovered() || dismissButton.Hovered() || actionButton.Pressed() || dismissButton.Pressed(), gtx.Now)
	for r.clicked(gtx, actionButton) {
		r.dismissNotice()
		if r.onAction != nil && strings.TrimSpace(notice.action.Command) != "" {
			r.onAction(notice.action, cloneStringMap(notice.arguments))
		}
	}
	for r.clicked(gtx, dismissButton) {
		r.dismissNotice()
	}
	return layout.Stack{Alignment: layout.SE}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = viewportConstraints
			return body(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: 14, Bottom: 14, Left: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(480))
				return r.surfaceWithBorder(func(gtx layout.Context) layout.Dimensions {
					message := r.materialTextLabel(notice.message, "detail", false)
					message.Color = r.palette.noticeText
					actions := func(gtx layout.Context) layout.Dimensions {
						if r.compact {
							gtx.Constraints.Min.Y = max(gtx.Constraints.Min.Y, gtx.Dp(44))
						}
						children := make([]layout.FlexChild, 0, 2)
						if strings.TrimSpace(notice.actionLabel) != "" {
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return r.layoutControlButton(gtx, actionButton, notice.actionLabel, "", true)
							}))
						}
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.layoutControlButton(gtx, dismissButton, "Dismiss", "", true)
						}))
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(r.metrics.spaceSmall)}.Layout(gtx, children...)
					}
					if r.compact {
						return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(r.metrics.spaceSmall)}.Layout(gtx,
							layout.Rigid(message.Layout), layout.Rigid(actions),
						)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(r.metrics.spaceMedium)}.Layout(gtx,
						layout.Flexed(1, message.Layout), layout.Rigid(actions),
					)
				}, r.metrics.spaceMedium, r.palette.noticeBackground, r.palette.noticeBorder)(gtx)
			})
		}),
	)
}

func (r *Renderer) layoutSheetOverlay(gtx layout.Context, body layout.Widget) layout.Dimensions {
	viewportConstraints := gtx.Constraints
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = viewportConstraints
			return body(gtx)
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sheet := r.activeSheet
			if sheet == nil || !sheet.seen {
				return layout.Dimensions{}
			}
			r.paintPageBackground(gtx)
			return r.layoutFullScreenSheet(gtx, sheet)
		}),
	)
}

func (r *Renderer) layoutFullScreenSheet(gtx layout.Context, sheet *activeSheet) layout.Dimensions {
	closeButton := r.button("compact-sheet/close")
	for r.clicked(gtx, closeButton) {
		r.setDisclosureState(sheet.stateKey, false, sheet.persistent)
		r.activeSheet = nil
		r.requestFrame()
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	pageInset := r.pageInset()
	return layout.Inset{Top: pageInset, Right: pageInset, Bottom: pageInset, Left: pageInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = gtx.Constraints.Max
		return r.surface(func(gtx layout.Context) layout.Dimensions {
			imageItems := 0
			if sheet.node.Image != nil {
				imageItems = 1
			}
			return r.layoutGuardedList(gtx, "sheet:"+sheet.path, &sheet.list, len(sheet.node.Children)+1+imageItems, func(gtx layout.Context, index int) layout.Dimensions {
				if index == 0 {
					return layout.Inset{Bottom: r.metrics.spaceMedium}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						title := r.materialTextLabel(sheet.title, "heading", false)
						title.Color = r.palette.text
						title.State = r.selectable("compact-sheet/title")
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return r.layoutIconButton(gtx, closeButton, "arrow-left", "Close "+sheet.title)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{Width: r.metrics.spaceSmall}.Layout(gtx)
							}),
							layout.Flexed(1, title.Layout),
						)
					})
				}
				if imageItems > 0 && index == 1 {
					return layout.Inset{Bottom: r.metrics.spaceMedium}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return r.layoutImage(gtx, uidsl.Node{Image: sheet.node.Image, Style: uidsl.Style{Role: "project-icon"}}, sheet.data, "compact-sheet/"+sheet.path+"/image")
					})
				}
				index -= imageItems
				return layout.Inset{Bottom: r.metrics.spaceMedium}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutNode(gtx, sheet.node.Children[index-1], sheet.data, fmt.Sprintf("compact-sheet/%s/%d", sheet.path, index-1))
				})
			})
		}, r.metrics.sectionPadding, false, nil)(gtx)
	})
}

func (r *Renderer) openCompactSheet(path, title string, node uidsl.Node, data any, stateKey string, persistent bool) {
	if r.activeSheet != nil && r.activeSheet.path == path {
		r.activeSheet.title = title
		r.activeSheet.node = node
		r.activeSheet.data = data
		r.activeSheet.stateKey = stateKey
		r.activeSheet.persistent = persistent
		r.activeSheet.seen = true
		return
	}
	r.activeSheet = &activeSheet{
		path: path, title: title, node: node, data: data, stateKey: stateKey, persistent: persistent,
		list: layout.List{Axis: layout.Vertical}, seen: true,
	}
	r.requestFrame()
}

func (r *Renderer) layoutLoadingState(gtx layout.Context) layout.Dimensions {
	semantic.DescriptionOp("Loading content").Add(gtx.Ops)
	return r.layoutGlyph(gtx, "loader-2", "accent", 28)
}

func (r *Renderer) layoutSkeleton(gtx layout.Context) layout.Dimensions {
	semantic.DescriptionOp("Loading content").Add(gtx.Ops)
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 30)})
	size := gtx.Constraints.Min
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{Size: size}
	}
	const cycle = 2200 * time.Millisecond
	phase := float64(gtx.Now.UnixNano()%int64(cycle)) / float64(cycle)
	opacity := .35 + .55*(.5-.5*math.Cos(phase*2*math.Pi))
	fill := r.palette.border
	fill.A = uint8(math.Round(float64(fill.A) * opacity))
	radius := size.Y / 2
	stack := r.cachedRoundedClipPx(gtx.Ops, size, radius).Push(gtx.Ops)
	paint.Fill(gtx.Ops, fill)
	stack.Pop()
	return layout.Dimensions{Size: size}
}

func (r *Renderer) layoutNode(gtx layout.Context, raw uidsl.Node, data any, path string) layout.Dimensions {
	node, hidden := applyGioOverride(raw, r.compact)
	if hidden {
		return layout.Dimensions{}
	}
	if node.Visible != nil {
		value, err := uidsl.Resolve(data, node.Visible.Binding)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		equal := conditionEqual(node.Visible, value)
		if (!node.Visible.Not && !equal) || (node.Visible.Not && equal) {
			return layout.Dimensions{}
		}
	}
	if node.Component == "scroller" {
		if r.compact && node.ID == "job-output-groups" {
			// A vertical output list nested inside the page's vertical list makes
			// touch-drag ownership ambiguous. Phones use the page as the sole
			// vertical scroll owner so expanded output and following steps remain
			// reachable with one continuous gesture.
			node.Layout.MaxHeight = ""
		}
		content := func(gtx layout.Context) layout.Dimensions {
			return r.layoutScroller(gtx, node, data, path)
		}
		if node.ID == "job-output-groups" {
			content = r.surfaceWithBorder(content, r.metrics.spaceSmall, r.palette.consoleBackground, r.palette.consoleBorder)
		}
		return r.constrainNode(gtx, node, content)
	}
	if node.Repeat != nil {
		items, err := resolveItems(data, node.Repeat.Source)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		children := make([]layout.FlexChild, 0, len(items))
		for i, item := range items {
			itemData := mergeData(data, node.Repeat.As, item)
			clone := node
			clone.Repeat = nil
			itemPath := fmt.Sprintf("%s/%d", path, i)
			if key, err := uidsl.Resolve(itemData, node.Repeat.Key); err == nil {
				itemPath = path + "/" + fmt.Sprint(key)
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: r.spacing(node.Layout.Gap)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutNode(gtx, clone, itemData, itemPath)
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
	if node.Style.ToneBinding != "" {
		value, err := uidsl.Resolve(data, node.Style.ToneBinding)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		node.Style.Tone = semanticTone(fmt.Sprint(value))
	}
	if node.Style.Role == "skeleton" {
		return r.constrainNode(gtx, node, r.layoutSkeleton)
	}

	content := func(gtx layout.Context) layout.Dimensions {
		if node.Component == "disclosure" {
			return r.layoutDisclosure(gtx, node, data, path)
		}
		if node.Component == "graph-view" {
			return r.layoutGraphView(gtx, node, data, path)
		}
		if node.Component == "tree-view" {
			return r.layoutTreeView(gtx, node, data, path)
		}
		if node.Component == "text" {
			return r.layoutText(gtx, node, data, path)
		}
		if node.Component == "badge" {
			return r.layoutBadge(gtx, node, data, path)
		}
		if node.Component == "icon" {
			return r.layoutIcon(gtx, node, data)
		}
		if node.Component == "image" {
			return r.layoutImage(gtx, node, data, path)
		}
		if node.Component == "button" {
			return r.layoutButton(gtx, node, data, path)
		}
		if node.Component == "select" {
			return r.layoutSelect(gtx, node, data, path)
		}
		if node.Component == "input" {
			return r.layoutInput(gtx, node, data, path)
		}
		return r.layoutChildren(gtx, node, data, path)
	}
	var surfaceProgress *semanticProgress
	if node.Progress != nil {
		progress, active := activeSemanticProgress(data, node.Progress)
		useSurfaceProgress := usesSurfaceProgress(node, r.disclosureExpanded(node, data, path))
		if active && useSurfaceProgress {
			surfaceProgress = &progress
		} else {
			// Disclosures place expanded progress on their header in
			// layoutDisclosure. Wrapping the complete disclosure here would tint
			// its body as well and duplicate the header progress layer.
			if node.Component != "disclosure" {
				content = r.progressWidget(node, data, content)
			}
		}
	}
	if node.Style.Role == "execution-section-header" {
		headerContent := content
		content = func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, r.palette.subtle, clip.Rect(image.Rectangle{Max: gtx.Constraints.Min}).Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}, headerContent)
		}
	}
	if node.Component == "card" || node.Component == "disclosure" || node.Component == "section" || node.Component == "graph-view" || node.Style.Role == "hero" {
		padding := unit.Dp(0)
		if node.Component == "section" {
			padding = r.metrics.sectionPadding
			if node.Layout.Padding != "" {
				padding = 0
			}
		}
		if node.Component == "graph-view" {
			padding = r.metrics.sectionPadding
			if node.Layout.Padding != "" {
				padding = 0
			}
		}
		if node.Component == "card" {
			padding = r.metrics.cardPadding
			if node.Layout.Padding != "" {
				padding = 0
			}
		}
		if node.Component == "disclosure" {
			padding = r.metrics.sectionPadding
			if node.Layout.Padding != "" {
				padding = r.spacing(node.Layout.Padding)
			}
			// Output groups and execution rows put their declared inset inside the
			// header and body. Their progress layer can therefore meet the top and
			// side edges of the complete disclosure surface.
			if node.Style.Role == "output-group" || node.Style.Role == "execution-row" {
				padding = 0
			}
			if node.Style.Role == "project-row" {
				padding = 0
			}
		}
		if node.Style.Role == "hero" {
			padding = r.metrics.heroPadding
		}
		if node.Component == "disclosure" && node.Style.Role == "tree-branch" {
			// Recursive report trees use disclosures for expansion mechanics but
			// deliberately remain one visual hierarchy rather than nested cards.
		} else if node.Component == "disclosure" && node.Style.Role == "output-group" {
			content = r.surfaceWithBorderProgressColor(content, padding, r.palette.consoleSurface, r.palette.consoleBorder, surfaceProgress, r.palette.consoleSuccess)
		} else if node.Component == "disclosure" {
			content = r.surfaceWithFillProgress(content, padding, r.palette.surfaceRaised, surfaceProgress)
		} else if node.Component == "card" && node.Style.Role == "output-system" {
			content = r.surfaceWithBorder(content, padding, r.palette.consoleSurface, r.palette.consoleBorder)
		} else if node.Component == "card" && node.Style.Role == "scheduling-awaiting" {
			content = r.surfaceWithBorder(content, padding, r.palette.awaitingSurface, r.palette.awaitingBorder)
		} else {
			content = r.surface(content, padding, node.Style.Role == "hero", surfaceProgress)
		}
	}
	if node.Style.Role == "settings-project-row" {
		rowContent := content
		content = func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				height := max(1, gtx.Dp(1))
				paint.FillShape(gtx.Ops, r.palette.border, clip.Rect(image.Rect(0, 0, gtx.Constraints.Min.X, height)).Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 10, Bottom: 10}.Layout(gtx, rowContent)
			})
		}
	}
	if node.Style.Role == "queued-execution-job-row" || node.Style.Role == "history-execution-job-row" {
		content = r.surfaceWithBorderRadius(content, 0, r.palette.surfaceRaised, r.palette.border, r.metrics.controlRadius)
	}
	widgetFn := content
	if len(node.Actions) > 0 && !componentHandlesOwnActions(node.Component) {
		button := r.button(path)
		for r.clicked(gtx, button) {
			if !r.nodeHasSelection(path) {
				r.queueNodeActivation(path, node.Actions[0], data)
			}
		}
		widgetFn = func(gtx layout.Context) layout.Dimensions {
			child := content
			if node.Style.Role == "history-execution-job-row" {
				child = func(gtx layout.Context) layout.Dimensions {
					semantic.DescriptionOp("Open job details").Add(gtx.Ops)
					defer pointer.PassOp{}.Push(gtx.Ops).Pop()
					return content(gtx)
				}
			}
			return button.Layout(gtx, child)
		}
	}
	return r.constrainNode(gtx, node, widgetFn)
}

func usesSurfaceProgress(node uidsl.Node, disclosureExpanded bool) bool {
	_ = disclosureExpanded
	return node.Style.Role == "hero" ||
		node.Component == "card" && node.Style.Role != "output-system"
}

func componentHandlesOwnActions(component string) bool {
	switch component {
	case "button", "select", "input", "graph-view", "tree-view":
		return true
	default:
		return false
	}
}

func (r *Renderer) layoutTreeView(gtx layout.Context, node uidsl.Node, data any, nodePath string) layout.Dimensions {
	tree := node.TreeView
	if tree == nil {
		return layout.Dimensions{}
	}
	items, err := resolveItems(data, tree.Nodes)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	filter := ""
	if tree.Filter != "" {
		if value, resolveErr := uidsl.Resolve(data, tree.Filter); resolveErr == nil {
			filter = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	children := make([]layout.FlexChild, 0, len(items))
	for _, item := range items {
		itemData := mergeData(data, tree.As, item)
		if !treeEntryVisible(itemData, tree, filter) {
			continue
		}
		key, keyErr := uidsl.Resolve(itemData, tree.NodeKey)
		if keyErr != nil {
			return r.errorLabel(gtx, keyErr)
		}
		entryPath := fmt.Sprintf("%s/%v", nodePath, key)
		entryNode, entryErr := treeEntryNode(node, itemData, fmt.Sprint(key))
		if entryErr != nil {
			return r.errorLabel(gtx, entryErr)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutNode(gtx, entryNode, itemData, entryPath)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func treeEntryVisible(data any, tree *uidsl.TreeView, filter string) bool {
	if tree == nil || filter == "" || filter == "all" {
		return true
	}
	if tree.FilterValues != "" {
		if raw, err := uidsl.Resolve(data, tree.FilterValues); err == nil {
			if values, ok := raw.([]any); ok && len(values) > 0 {
				matched := false
				for _, value := range values {
					matched = matched || fmt.Sprint(value) == filter
				}
				if !matched {
					return false
				}
			}
		}
	}
	children, err := resolveItems(data, tree.Children)
	if err != nil || len(children) == 0 {
		return true
	}
	for _, child := range children {
		if treeEntryVisible(mergeData(data, tree.As, child), tree, filter) {
			return true
		}
	}
	return false
}

func treeEntryNode(node uidsl.Node, data any, key string) (uidsl.Node, error) {
	tree := node.TreeView
	label, err := uidsl.RenderText(data, tree.NodeLabel)
	if err != nil {
		return uidsl.Node{}, err
	}
	detail := ""
	if tree.NodeDetail != (uidsl.Text{}) {
		detail, err = uidsl.RenderText(data, tree.NodeDetail)
		if err != nil {
			return uidsl.Node{}, err
		}
	}
	tone := ""
	if tree.NodeTone != "" {
		if value, resolveErr := uidsl.Resolve(data, tree.NodeTone); resolveErr == nil {
			tone = semanticTone(fmt.Sprint(value))
		}
	}
	link := ""
	if tree.NodeLink != "" {
		if value, resolveErr := uidsl.Resolve(data, tree.NodeLink); resolveErr == nil {
			link = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	actionLabel := ""
	if tree.ActionLabel != (uidsl.Text{}) {
		actionLabel, err = uidsl.RenderText(data, tree.ActionLabel)
		if err != nil {
			return uidsl.Node{}, err
		}
	}
	labelNode := uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: label}, Layout: uidsl.Layout{Grow: true}, Style: uidsl.Style{Role: "code-inline", Emphasis: "strong"}}
	if link != "" {
		labelNode.Style.Tone = "accent"
		labelNode.Actions = []uidsl.Action{{On: "activate", Command: "open-url", Arguments: map[string]string{"url": link}}}
	}
	summary := []uidsl.Node{labelNode}
	if detail != "" {
		summary = append(summary, uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: detail}, Style: uidsl.Style{Role: "detail-small", Tone: tone}})
	}
	if actionLabel != "" && len(node.Actions) > 0 {
		summary = append(summary, uidsl.Node{Component: "button", Text: &uidsl.Text{Literal: actionLabel}, Style: uidsl.Style{Role: "tree-action"}, Actions: node.Actions})
	}
	children, _ := resolveItems(data, tree.Children)
	if len(children) == 0 {
		return uidsl.Node{Component: "row", Style: uidsl.Style{Role: "tree-row"}, Layout: uidsl.Layout{Direction: "horizontal", Gap: "small", Align: "center"}, Children: summary}, nil
	}
	defaultExpanded := false
	if tree.DefaultExpanded != "" {
		if value, resolveErr := uidsl.Resolve(data, tree.DefaultExpanded); resolveErr == nil {
			defaultExpanded, _ = value.(bool)
		}
	}
	childTree := *tree
	childTree.Nodes = tree.Children
	childNode := uidsl.Node{Component: "tree-view", TreeView: &childTree, Actions: node.Actions}
	return uidsl.Node{
		Component: "disclosure", Text: &uidsl.Text{Literal: label}, Style: uidsl.Style{Role: "tree-branch", Tone: tone},
		Disclosure: &uidsl.Disclosure{StateKey: tree.StateKey + ":" + key, DefaultExpanded: defaultExpanded, Summary: summary[1:]},
		Children:   []uidsl.Node{childNode}, Layout: uidsl.Layout{Direction: "vertical", Gap: "small", Padding: "small"},
	}, nil
}

func disclosureNavigationAction(disclosure *uidsl.Disclosure) (uidsl.Action, bool) {
	if disclosure == nil {
		return uidsl.Action{}, false
	}
	for _, summaryNode := range disclosure.Summary {
		for _, action := range summaryNode.Actions {
			if action.On == "activate" && action.Command == "navigate" {
				return action, true
			}
		}
	}
	return uidsl.Action{}, false
}

func (r *Renderer) layoutDisclosure(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	label := "Details"
	if node.Text != nil {
		resolved, err := uidsl.RenderText(data, *node.Text)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		label = resolved
	}
	sheetPresentation := r.compact && node.Disclosure != nil && node.Disclosure.CompactPresentation == "sheet"
	navigatePresentation := r.compact && node.Disclosure != nil && node.Disclosure.CompactPresentation == "navigate"
	navigateAction, hasNavigateAction := disclosureNavigationAction(node.Disclosure)
	sheetTitle := compactSheetTitle(node, data, label)
	stateKey, persistent := r.disclosureStateKey(node, data, path)
	if sheetPresentation && r.activeSheet != nil && r.activeSheet.path == path {
		r.openCompactSheet(path, sheetTitle, node, data, stateKey, persistent)
	}
	expanded := false
	if !navigatePresentation {
		var exists bool
		expanded, exists = r.disclosures[stateKey]
		if !exists {
			expanded = disclosureDefaultExpanded(node.Disclosure, data)
			r.disclosures[stateKey] = expanded
		}
		if persistent {
			r.persistentDisclosures[stateKey] = true
		}
		if sheetPresentation {
			expanded = false
		}
	}
	iconName := "chevron-right"
	if expanded {
		iconName = "chevron-down"
	}
	isProjectRow := node.Style.Role == "project-row"
	isOutputGroup := node.Style.Role == "output-group"
	isExecutionRow := node.Style.Role == "execution-row"
	isTreeBranch := node.Style.Role == "tree-branch"
	contentPadding := r.metrics.sectionPadding
	if node.Layout.Padding != "" {
		contentPadding = r.spacing(node.Layout.Padding)
	}
	headerToggleKey := path + "/disclosure-toggle"
	if isProjectRow {
		headerToggleKey = path + "/disclosure-header"
	}
	headerToggle := r.button(headerToggleKey)
	labelToggle := r.button(path + "/disclosure-label")
	summaryActionActivated := false
	labelToggleActivated := false
	for r.clicked(gtx, labelToggle) {
		labelToggleActivated = true
		r.markInteraction(path + "/disclosure-label")
		if r.selectable(path+"/label").SelectionLen() == 0 {
			if navigatePresentation && hasNavigateAction {
				r.dispatchFromLayout(gtx, navigateAction, data)
			} else if sheetPresentation {
				r.setDisclosureState(stateKey, true, persistent)
				r.openCompactSheet(path, sheetTitle, node, data, stateKey, persistent)
			} else {
				expanded = !expanded
				r.setDisclosureState(stateKey, expanded, persistent)
			}
		}
	}
	if node.Disclosure != nil {
		for index, summaryNode := range node.Disclosure.Summary {
			if len(summaryNode.Actions) == 0 {
				continue
			}
			summaryPath := fmt.Sprintf("%s/summary/%d", path, index)
			if summaryNode.Component == "button" {
				summaryCopy := summaryNode
				_, enabled := r.buttonNodeState(&summaryCopy, data)
				if r.handleButtonClicks(gtx, summaryNode, data, summaryPath, enabled) {
					summaryActionActivated = true
				}
				continue
			}
			if componentHandlesOwnActions(summaryNode.Component) {
				continue
			}
			actionButton := r.button(summaryPath)
			for r.clicked(gtx, actionButton) {
				summaryActionActivated = true
				r.markInteraction(summaryPath)
				if !r.nodeHasSelection(summaryPath) {
					r.dispatchFromLayout(gtx, summaryNode.Actions[0], data)
				}
			}
		}
	}
	for r.clicked(gtx, headerToggle) {
		r.markInteraction(headerToggleKey)
		if !summaryActionActivated && !labelToggleActivated && !r.disclosureHeaderHasSelection(path) {
			if navigatePresentation && hasNavigateAction {
				r.dispatchFromLayout(gtx, navigateAction, data)
			} else if sheetPresentation {
				r.setDisclosureState(stateKey, true, persistent)
				r.openCompactSheet(path, sheetTitle, node, data, stateKey, persistent)
			} else {
				expanded = !expanded
				r.setDisclosureState(stateKey, expanded, persistent)
			}
		}
	}
	header := func(gtx layout.Context) layout.Dimensions {
		toggleWidget := func(gtx layout.Context) layout.Dimensions {
			return r.layoutDisclosureIndicator(gtx, iconName)
		}
		layoutSummaryNode := func(gtx layout.Context, summaryNode uidsl.Node, index int) layout.Dimensions {
			if summaryNode.Component == "spacer" {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
			}
			return r.layoutNode(gtx, summaryNode, data, fmt.Sprintf("%s/summary/%d", path, index))
		}
		labelWidget := func(gtx layout.Context) layout.Dimensions {
			labelPath := path + "/label"
			layoutLabel := func(gtx layout.Context) layout.Dimensions {
				labelInset := unit.Dp(10)
				if isProjectRow {
					labelInset = 0
				} else if isTreeBranch {
					labelInset = 4
				}
				return layout.Inset{Left: labelInset, Right: labelInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					defer pointer.PassOp{}.Push(gtx.Ops).Pop()
					textNode := node
					textNode.Component = "text"
					textNode.Text = &uidsl.Text{Literal: label}
					if textNode.Style.Role == "" {
						textNode.Style.Role = "heading"
					}
					if textNode.Style.Role == "execution-row" {
						textNode.Style.Tone = ""
					}
					if textNode.Style.Role == "output-group" {
						textNode.Style.Role = "output-summary"
						textNode.Style.Tone = "console-accent"
					}
					return r.layoutText(gtx, textNode, data, labelPath)
				})
			}
			return layoutLabel(gtx)
		}
		summaryChildren := func() []layout.FlexChild {
			if node.Disclosure == nil {
				return nil
			}
			children := make([]layout.FlexChild, 0, len(node.Disclosure.Summary))
			for index := range node.Disclosure.Summary {
				summaryNode := node.Disclosure.Summary[index]
				widgetFn := func(gtx layout.Context) layout.Dimensions {
					left := unit.Dp(8)
					if node.Style.Role == "project-row" && index == 0 {
						left = 6
					}
					return layout.Inset{Left: left}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutSummaryNode(gtx, summaryNode, index)
					})
				}
				if summaryNode.Layout.Grow {
					children = append(children, layout.Flexed(1, widgetFn))
				} else {
					children = append(children, layout.Rigid(widgetFn))
				}
			}
			return children
		}
		if r.compact && !isProjectRow && node.Disclosure != nil && len(node.Disclosure.Summary) > 0 {
			mainChildren := make([]layout.FlexChild, 0, 4)
			if node.Style.Role == "execution-row" && node.Image != nil {
				mainChildren = append(mainChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return r.layoutImageSized(gtx, node.Image, 28, 28)
				}))
			}
			if node.Style.Role == "execution-row" {
				statusIcon := map[string]string{"success": "status-success", "danger": "status-danger", "warning": "status-waiting", "accent": "loader-2"}[node.Style.Tone]
				if statusIcon != "" {
					mainChildren = append(mainChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 9}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							iconTone := node.Style.Tone
							if statusIcon == "loader-2" {
								iconTone = "warning"
							}
							return r.layoutGlyph(gtx, statusIcon, iconTone, 18)
						})
					}))
				}
			}
			mainChildren = append(mainChildren, layout.Flexed(1, labelWidget), layout.Rigid(toggleWidget))
			main := func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, mainChildren...)
			}
			summaries := make([]layout.FlexChild, 0, len(node.Disclosure.Summary))
			for index := range node.Disclosure.Summary {
				summaryNode := node.Disclosure.Summary[index]
				widgetFn := func(gtx layout.Context) layout.Dimensions {
					if summaryNode.Component == "spacer" {
						return layoutSummaryNode(gtx, summaryNode, index)
					}
					return layout.Inset{Top: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = 0
						return layoutSummaryNode(gtx, summaryNode, index)
					})
				}
				if summaryNode.Layout.Grow {
					summaries = append(summaries, layout.Flexed(1, widgetFn))
				} else {
					summaries = append(summaries, layout.Rigid(widgetFn))
				}
			}
			description := "Expand " + label
			if sheetPresentation || navigatePresentation {
				description = "Open " + label
			}
			if expanded {
				description = "Collapse " + label
			}
			return headerToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(description).Add(gtx.Ops)
				defer pointer.PassOp{}.Push(gtx.Ops).Pop()
				if node.Style.Role == "execution-row" {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(main),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, summaries...)
						}),
					)
				}
				children := []layout.FlexChild{layout.Rigid(main)}
				children = append(children, summaries...)
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		}
		if node.Style.Role != "execution-row" {
			if isProjectRow {
				description := "Expand " + label
				if navigatePresentation {
					description = "Open " + label
				} else if expanded {
					description = "Collapse " + label
				}
				return headerToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					semantic.DescriptionOp(description).Add(gtx.Ops)
					defer pointer.PassOp{}.Push(gtx.Ops).Pop()
					return layout.UniformInset(contentPadding).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return r.layoutWrappedProjectSummary(gtx, node, data, path, toggleWidget)
					})
				})
			}
			labelChild := layout.Flexed(1, labelWidget)
			children := []layout.FlexChild{labelChild}
			if isTreeBranch {
				children = []layout.FlexChild{layout.Rigid(toggleWidget), labelChild}
				children = append(children, summaryChildren()...)
			} else {
				children = append(children, summaryChildren()...)
				children = append(children, layout.Rigid(toggleWidget))
			}
			layoutRow := func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			}
			description := "Expand " + label
			if expanded {
				description = "Collapse " + label
			}
			return headerToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(description).Add(gtx.Ops)
				// Project summary text remains selectable while pointer events also
				// reach the row-sized disclosure control behind it.
				defer pointer.PassOp{}.Push(gtx.Ops).Pop()
				return layoutRow(gtx)
			})
		}
		children := make([]layout.FlexChild, 0, 4)
		if node.Image != nil {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutImageSized(gtx, node.Image, 28, 28)
			}))
		}
		statusIcon := map[string]string{"success": "status-success", "danger": "status-danger", "warning": "status-waiting", "accent": "loader-2"}[node.Style.Tone]
		if statusIcon != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 9}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					iconTone := node.Style.Tone
					if statusIcon == "loader-2" {
						iconTone = "warning"
					}
					return r.layoutGlyph(gtx, statusIcon, iconTone, 18)
				})
			}))
		}
		children = append(children, layout.Flexed(1, labelWidget))
		children = append(children, summaryChildren()...)
		children = append(children, layout.Rigid(toggleWidget))
		layoutRow := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		}
		description := "Expand " + label
		if expanded {
			description = "Collapse " + label
		}
		return headerToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp(description).Add(gtx.Ops)
			defer pointer.PassOp{}.Push(gtx.Ops).Pop()
			return layoutRow(gtx)
		})
	}
	headerWidget := layout.Widget(header)
	if isOutputGroup || isExecutionRow {
		// The declared padding belongs to the header contents, while progress is
		// painted behind the resulting full-width, full-height header surface.
		headerInset := layout.Inset{
			Top: contentPadding, Right: contentPadding,
			Bottom: contentPadding, Left: contentPadding,
		}
		unpaddedHeader := headerWidget
		headerWidget = func(gtx layout.Context) layout.Dimensions {
			return headerInset.Layout(gtx, unpaddedHeader)
		}
	}
	if node.Progress != nil {
		headerWidget = r.progressWidget(node, data, headerWidget)
	}
	if !expanded {
		return headerWidget(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(headerWidget),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bodyInset := layout.Inset{Top: 12}
			if isTreeBranch {
				bodyInset.Top = r.metrics.spaceSmall
				bodyInset.Left = r.metrics.spaceMedium
			}
			if isOutputGroup || isExecutionRow {
				if isOutputGroup {
					bodyInset.Top = contentPadding
				}
				bodyInset.Right = contentPadding
				bodyInset.Bottom = contentPadding
				bodyInset.Left = contentPadding
			}
			if isProjectRow {
				bodyInset.Top = r.metrics.spaceSmall
				bodyInset.Right = contentPadding
				bodyInset.Bottom = contentPadding
				bodyInset.Left = contentPadding
			}
			body := func(gtx layout.Context) layout.Dimensions {
				return bodyInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					contentNode := node
					contentNode.Layout.Padding = ""
					if isOutputGroup {
						contentNode.Children = withDefaultConsoleText(node.Children)
					}
					return r.layoutChildren(gtx, contentNode, data, path+"/content")
				})
			}
			if !isTreeBranch {
				return body(gtx)
			}
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				guideX := gtx.Dp(r.metrics.spaceMedium / 2)
				guideWidth := max(1, gtx.Dp(1))
				paint.FillShape(gtx.Ops, r.palette.border, clip.Rect(image.Rect(guideX, 0, guideX+guideWidth, gtx.Constraints.Min.Y)).Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}, body)
		}),
	)
}

func disclosureDefaultExpanded(disclosure *uidsl.Disclosure, data any) bool {
	if disclosure == nil {
		return false
	}
	if binding := strings.TrimSpace(disclosure.DefaultExpandedBinding); binding != "" {
		value, err := uidsl.Resolve(data, binding)
		if err == nil {
			return boolValue(value)
		}
	}
	return disclosure.DefaultExpanded
}

type recordedProjectSummaryItem struct {
	size     image.Point
	call     op.CallOp
	trailing bool
}

func (r *Renderer) layoutWrappedProjectSummary(gtx layout.Context, node uidsl.Node, data any, path string, toggle layout.Widget) layout.Dimensions {
	const itemGap = 8
	maxWidth := max(1, gtx.Constraints.Max.X)
	items := make([]recordedProjectSummaryItem, 0, len(node.Disclosure.Summary)+1)
	trailing := false
	record := func(widget layout.Widget, trailing bool) {
		macro := op.Record(gtx.Ops)
		itemContext := gtx
		itemContext.Constraints.Min = image.Point{}
		itemContext.Constraints.Max.X = maxWidth
		dimensions := widget(itemContext)
		items = append(items, recordedProjectSummaryItem{size: dimensions.Size, call: macro.Stop(), trailing: trailing})
	}
	for index := range node.Disclosure.Summary {
		summaryNode := node.Disclosure.Summary[index]
		if summaryNode.Component == "spacer" {
			trailing = true
			continue
		}
		record(func(gtx layout.Context) layout.Dimensions {
			return r.layoutNode(gtx, summaryNode, data, fmt.Sprintf("%s/summary/%d", path, index))
		}, trailing)
	}
	record(toggle, true)

	type positionedItem struct {
		item recordedProjectSummaryItem
		x, y int
	}
	positioned := make([]positionedItem, 0, len(items))
	x, y, rowHeight, rowStart := 0, 0, 0, 0
	finishRow := func(end int) {
		firstTrailing := -1
		for index := rowStart; index < end; index++ {
			if positioned[index].item.trailing {
				firstTrailing = index
				break
			}
		}
		if firstTrailing >= 0 {
			used := 0
			if end > rowStart {
				last := positioned[end-1]
				used = last.x + last.item.size.X
			}
			shift := max(0, maxWidth-used)
			for index := firstTrailing; index < end; index++ {
				positioned[index].x += shift
			}
		}
	}
	for _, item := range items {
		width := min(item.size.X, maxWidth)
		if x > 0 && x+itemGap+width > maxWidth {
			finishRow(len(positioned))
			y += rowHeight + itemGap
			x, rowHeight, rowStart = 0, 0, len(positioned)
		}
		if x > 0 {
			x += itemGap
		}
		item.size.X = width
		positioned = append(positioned, positionedItem{item: item, x: x, y: y})
		x += width
		rowHeight = max(rowHeight, item.size.Y)
	}
	finishRow(len(positioned))
	for _, positionedItem := range positioned {
		offset := op.Offset(image.Pt(positionedItem.x, positionedItem.y)).Push(gtx.Ops)
		positionedItem.item.call.Add(gtx.Ops)
		offset.Pop()
	}
	return layout.Dimensions{Size: image.Pt(maxWidth, y+rowHeight)}
}

func withDefaultConsoleText(children []uidsl.Node) []uidsl.Node {
	result := make([]uidsl.Node, len(children))
	for index := range children {
		child := children[index]
		if child.Component == "text" && child.Style.Tone == "" && child.Style.ToneBinding == "" {
			child.Style.Tone = "console-text"
		}
		if len(child.Children) > 0 {
			child.Children = withDefaultConsoleText(child.Children)
		}
		result[index] = child
	}
	return result
}

func compactSheetTitle(node uidsl.Node, data any, fallback string) string {
	if node.Style.Role != "project-row" || node.Disclosure == nil || len(node.Disclosure.Summary) == 0 {
		return fallback
	}
	first := node.Disclosure.Summary[0]
	if first.Text == nil {
		return fallback
	}
	name, err := uidsl.RenderText(data, *first.Text)
	if err != nil || strings.TrimSpace(name) == "" {
		return fallback
	}
	if strings.TrimSpace(name) == strings.TrimSpace(fallback) {
		return strings.TrimSpace(fallback)
	}
	return strings.TrimSpace(fallback + " " + name)
}

func (r *Renderer) disclosureExpanded(node uidsl.Node, data any, path string) bool {
	stateKey, _ := r.disclosureStateKey(node, data, path)
	if expanded, exists := r.disclosures[stateKey]; exists {
		return expanded
	}
	return node.Disclosure != nil && node.Disclosure.DefaultExpanded
}

type semanticProgress struct {
	state          string
	fraction       float64
	snapshotUnixMS int64
	ratePerMS      float64
}

const (
	progressFrameInterval         = time.Second / 60
	determinateProgressLimit      = .999
	indeterminateProgressDuration = 4 * time.Second
	connectionPulseDuration       = 4 * time.Second
	connectionPulseMinimum        = .58
	heartbeatPulseDuration        = protocol.AgentHeartbeatFadeDuration
	heartbeatPulseMinimum         = .18
	compactLayoutWidth            = unit.Dp(520)
)

func (r *Renderer) progressWidget(node uidsl.Node, data any, content layout.Widget) layout.Widget {
	progress, active := activeSemanticProgress(data, node.Progress)
	if !active {
		return content
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				if size.X <= 0 || size.Y <= 0 {
					return layout.Dimensions{Size: size}
				}
				radius := r.metrics.controlRadius
				if node.Component == "disclosure" {
					radius = r.metrics.surfaceRadius
				}
				fill := r.palette.success
				var underlay *color.NRGBA
				if node.Style.Role == "output-group" {
					fill = r.palette.consoleSuccess
					underlay = &r.palette.consoleSurface
				} else if node.Style.Role == "execution-row" {
					underlay = &r.palette.surfaceRaised
				} else if node.Style.Role == "execution-section-header" {
					radius = r.metrics.controlRadius
					underlay = &r.palette.subtle
				}
				progressClip := r.cachedRoundedClip(gtx, size, radius).Push(gtx.Ops)
				r.paintSemanticProgress(gtx, progress, size, fill, underlay)
				progressClip.Pop()
				return layout.Dimensions{Size: size}
			},
			func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return content(gtx)
			},
		)
	}
}

func activeSemanticProgress(data any, binding *uidsl.Progress) (semanticProgress, bool) {
	progress, ok := resolveSemanticProgress(data, binding)
	return progress, ok && progress.state != "none" && progress.state != "waiting"
}

func (r *Renderer) paintSemanticProgress(gtx layout.Context, progress semanticProgress, size image.Point, base color.NRGBA, underlay *color.NRGBA) {
	rect, opacity, animated, ok := semanticProgressPaint(progress, size, gtx.Now)
	if !ok {
		return
	}
	if animated {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
	}
	fill := base
	// Browser progress uses color-mix(in srgb, ... 18%, transparent).
	// Gio's GPU pipeline blends translucent colors in linear space, which makes
	// bright progress colors visibly stronger. When the surface color is known,
	// precompose the browser's sRGB result and paint it opaquely instead.
	if underlay != nil {
		fill = mixColorSRGB(*underlay, fill, opacity)
	} else {
		fill.A = uint8(math.Round(255 * opacity))
	}
	paint.FillShape(gtx.Ops, fill, clip.Rect(rect).Op())
}

func semanticProgressPaint(progress semanticProgress, size image.Point, now time.Time) (image.Rectangle, float64, bool, bool) {
	state, fraction := evaluateSemanticProgress(progress, now)
	animated := state == "determinate" || state == "indeterminate" || state == "overrun"
	left, width := 0, int(float64(size.X)*fraction)
	const progressOpacity = .18
	opacity := progressOpacity
	switch state {
	case "indeterminate":
		width = max(1, int(float64(size.X)*.22))
		left = int(float64(size.X-width) * indeterminateProgressPosition(now))
	case "overrun":
		width = size.X
		pulse := float64(now.UnixMilli()%2000) / 2000
		if pulse > .5 {
			pulse = 1 - pulse
		}
		opacity *= .58 + .42*pulse*2
	case "complete":
		width = size.X
	}
	right := min(left+width, size.X)
	if width <= 0 || right <= left || size.Y <= 0 {
		return image.Rectangle{}, 0, animated, false
	}
	return image.Rect(max(0, left), 0, right, size.Y), opacity, animated, true
}

func mixColorSRGB(background, foreground color.NRGBA, foregroundWeight float64) color.NRGBA {
	weight := max(0, min(foregroundWeight, 1))
	mix := func(background, foreground uint8) uint8 {
		return uint8(math.Round(float64(background)*(1-weight) + float64(foreground)*weight))
	}
	return color.NRGBA{
		R: mix(background.R, foreground.R),
		G: mix(background.G, foreground.G),
		B: mix(background.B, foreground.B),
		A: 0xff,
	}
}

func indeterminateProgressPosition(now time.Time) float64 {
	cycle := float64(now.UnixNano()%int64(indeterminateProgressDuration)) / float64(indeterminateProgressDuration)
	return .5 - .5*math.Cos(2*math.Pi*cycle)
}

func connectionPulseOpacity(now time.Time) float32 {
	cycle := float64(now.UnixNano()%int64(connectionPulseDuration)) / float64(connectionPulseDuration)
	eased := .5 - .5*math.Cos(2*math.Pi*cycle)
	return float32(connectionPulseMinimum + (1-connectionPulseMinimum)*eased)
}

func heartbeatPulseOpacity(lastSeenUnixMS int64, now time.Time) float32 {
	if lastSeenUnixMS <= 0 {
		return heartbeatPulseMinimum
	}
	elapsed := now.Sub(time.UnixMilli(lastSeenUnixMS))
	if elapsed <= 0 {
		return 1
	}
	if elapsed >= heartbeatPulseDuration {
		return heartbeatPulseMinimum
	}
	remaining := 1 - float64(elapsed)/float64(heartbeatPulseDuration)
	return float32(heartbeatPulseMinimum + (1-heartbeatPulseMinimum)*remaining)
}

func heartbeatUnixMillis(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func resolveSemanticProgress(data any, binding *uidsl.Progress) (semanticProgress, bool) {
	if binding == nil || strings.TrimSpace(binding.Binding) == "" {
		return semanticProgress{}, false
	}
	resolve := func(suffix string) (any, bool) {
		value, err := uidsl.Resolve(data, binding.Binding+"."+suffix)
		return value, err == nil
	}
	stateValue, ok := resolve("state")
	if !ok {
		return semanticProgress{}, false
	}
	progress := semanticProgress{state: strings.ToLower(strings.TrimSpace(fmt.Sprint(stateValue)))}
	if value, ok := resolve("fraction"); ok {
		progress.fraction, _ = strconv.ParseFloat(fmt.Sprint(value), 64)
	}
	if value, ok := resolve("snapshot_unix_ms"); ok {
		progress.snapshotUnixMS, _ = strconv.ParseInt(fmt.Sprint(value), 10, 64)
	}
	if value, ok := resolve("rate_per_ms"); ok {
		progress.ratePerMS, _ = strconv.ParseFloat(fmt.Sprint(value), 64)
	}
	return progress, progress.state != ""
}

func evaluateSemanticProgress(progress semanticProgress, now time.Time) (string, float64) {
	state := progress.state
	fraction := max(0, min(progress.fraction, 1))
	if state == "determinate" {
		if progress.ratePerMS > 0 {
			elapsed := max(int64(0), now.UnixMilli()-progress.snapshotUnixMS)
			fraction += float64(elapsed) * progress.ratePerMS
		}
		// The server owns semantic state because it knows whether an aggregate
		// still contains unfinished jobs. Local interpolation must not turn a
		// determinate aggregate into an overrun pulse merely by reaching its
		// estimated duration between snapshots.
		fraction = max(0, min(determinateProgressLimit, fraction))
	}
	return state, fraction
}

func (r *Renderer) disclosureHeaderHasSelection(path string) bool {
	for key, selectable := range r.selectables {
		if key != path+"/label" && !strings.HasPrefix(key, path+"/summary/") {
			continue
		}
		if selectable.SelectionLen() != 0 {
			return true
		}
	}
	return false
}

func (r *Renderer) nodeHasSelection(path string) bool {
	for key, selectable := range r.selectables {
		if (key == path || strings.HasPrefix(key, path+"/")) && selectable.SelectionLen() != 0 {
			return true
		}
	}
	return false
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
	if !expanded && strings.HasPrefix(key, "job-output:") && r.outputScroller != nil {
		// A Gio list retains offsets measured against the formerly expanded row.
		// Once that row becomes shorter, the stale bottom-aligned position can
		// leave a viewport-sized blank area before the first output group.
		r.outputScroller.ScrollToEnd = false
		r.outputScroller.Position = layout.Position{}
	}
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

type recordedFlowItem struct {
	size image.Point
	call op.CallOp
}

func (r *Renderer) layoutWrappedNodeChildren(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	maxWidth := max(1, gtx.Constraints.Max.X)
	gap := gtx.Dp(r.spacing(node.Layout.Gap))
	type flowRow struct {
		items  []recordedFlowItem
		width  int
		height int
	}
	rows := make([]flowRow, 0, 2)
	current := flowRow{}
	finishRow := func() {
		if len(current.items) == 0 {
			return
		}
		rows = append(rows, current)
		current = flowRow{}
	}
	for index := range node.Children {
		child := node.Children[index]
		if !r.nodeOccupiesLayout(child, data) {
			continue
		}
		macro := op.Record(gtx.Ops)
		itemContext := gtx
		itemContext.Constraints.Min = image.Point{}
		itemContext.Constraints.Max.X = maxWidth
		dimensions := r.layoutNode(itemContext, child, data, fmt.Sprintf("%s/%d", path, index))
		call := macro.Stop()
		if dimensions.Size.X <= 0 || dimensions.Size.Y <= 0 {
			continue
		}
		dimensions.Size.X = min(dimensions.Size.X, maxWidth)
		nextWidth := dimensions.Size.X
		if len(current.items) > 0 {
			nextWidth += current.width + gap
		}
		if len(current.items) > 0 && nextWidth > maxWidth {
			finishRow()
			nextWidth = dimensions.Size.X
		}
		current.items = append(current.items, recordedFlowItem{size: dimensions.Size, call: call})
		current.width = nextWidth
		current.height = max(current.height, dimensions.Size.Y)
	}
	finishRow()

	y, usedWidth := 0, 0
	for rowIndex, row := range rows {
		x := 0
		for itemIndex, item := range row.items {
			if itemIndex > 0 {
				x += gap
			}
			offset := op.Offset(image.Pt(x, y+(row.height-item.size.Y)/2)).Push(gtx.Ops)
			item.call.Add(gtx.Ops)
			offset.Pop()
			x += item.size.X
		}
		usedWidth = max(usedWidth, row.width)
		y += row.height
		if rowIndex < len(rows)-1 {
			y += gap
		}
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(usedWidth, y))}
}

func (r *Renderer) layoutChildren(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	axis := layout.Vertical
	if node.Component == "row" || node.Layout.Direction == "horizontal" {
		axis = layout.Horizontal
	}
	compact := r.compact
	if compact && (node.Style.Role == "queued-execution-header" || node.Style.Role == "history-execution-header" || node.Style.Role == "agent-header") {
		// Desktop column labels become unreadable before the rows themselves do.
		// Compact execution details are rendered as vertical records instead.
		return layout.Dimensions{}
	}
	if compact && (node.Style.Role == "queued-execution-job-row" || node.Style.Role == "history-execution-job-row") {
		return r.layoutCompactExecutionRecord(gtx, node, data, path)
	}
	if compact && node.Style.Role == "agent-record" {
		return r.layoutCompactAgentRecord(gtx, node, data, path)
	}
	if compact && node.Style.Role == "compact-action-row" {
		return r.layoutCompactActionRow(gtx, node, data, path)
	}
	if compact && node.ID == "project-header" {
		return r.layoutCompactProjectHeader(gtx, node, data, path)
	}
	if axis == layout.Horizontal && node.Layout.Wrap && (node.Style.Role == "project-header-metadata" || node.Style.Role == "settings-project-summary") {
		return layout.Inset{
			Top: r.spacing(node.Layout.Padding), Right: r.spacing(node.Layout.Padding),
			Bottom: r.spacing(node.Layout.Padding), Left: r.spacing(node.Layout.Padding),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutWrappedNodeChildren(gtx, node, data, path)
		})
	}
	stackCompactRow := compact && axis == layout.Horizontal && (node.Style.Role == "hero" || node.Layout.Wrap || compactRowNeedsStack(node.Children) ||
		node.Style.Role == "queued-execution-job-row" || node.Style.Role == "history-execution-job-row")
	if node.Style.Role == "compact-toolbar" {
		stackCompactRow = false
	}
	if stackCompactRow {
		axis = layout.Vertical
	}
	children := make([]layout.FlexChild, 0, len(node.Children))
	gridWeights := executionGridWeights(node.Style.Role, len(node.Children))
	if stackCompactRow {
		gridWeights = nil
	}
	for i := range node.Children {
		child := node.Children[i]
		if gridWeights == nil && !r.nodeOccupiesLayout(child, data) {
			continue
		}
		widgetFn := func(gtx layout.Context) layout.Dimensions {
			if node.Style.Role == "report-stack" && axis == layout.Vertical && gtx.Constraints.Max.X > 0 {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
			}
			return r.layoutNode(gtx, child, data, fmt.Sprintf("%s/%d", path, i))
		}
		if gridWeights != nil {
			children = append(children, layout.Flexed(gridWeights[i], widgetFn))
		} else if child.Layout.Grow && !stackCompactRow {
			children = append(children, layout.Flexed(1, widgetFn))
		} else {
			children = append(children, layout.Rigid(widgetFn))
		}
	}
	return layout.Inset{
		Top: r.spacing(node.Layout.Padding), Right: r.spacing(node.Layout.Padding),
		Bottom: r.spacing(node.Layout.Padding), Left: r.spacing(node.Layout.Padding),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		row := func(gtx layout.Context) layout.Dimensions {
			alignment := flexAlignment(axis, node.Layout.Align, gridWeights != nil)
			if stackCompactRow && node.Style.Role == "settings-project-row" {
				alignment = layout.Start
			}
			return layout.Flex{Axis: axis, Alignment: alignment, Gap: gtx.Dp(r.spacing(node.Layout.Gap))}.Layout(gtx, children...)
		}
		if node.Style.Role == "queued-execution-job-row" || node.Style.Role == "history-execution-job-row" {
			return layout.Inset{Top: 7, Right: r.metrics.spaceSmall, Bottom: 7, Left: r.metrics.spaceSmall}.Layout(gtx, row)
		}
		return row(gtx)
	})
}

func (r *Renderer) layoutCompactProjectHeader(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	logoIndex, copyIndex, backIndex := -1, -1, -1
	for index := range node.Children {
		switch node.Children[index].Style.Role {
		case "project-icon":
			logoIndex = index
		case "project-header-copy":
			copyIndex = index
		case "project-header-back":
			backIndex = index
		}
	}
	if logoIndex < 0 || copyIndex < 0 || backIndex < 0 {
		return r.errorLabel(gtx, fmt.Errorf("compact project header roles are incomplete"))
	}
	logo := node.Children[logoIndex]
	copy := node.Children[copyIndex]
	titleIndex, metadataIndex := -1, -1
	for index := range copy.Children {
		switch copy.Children[index].Style.Role {
		case "title":
			titleIndex = index
		case "project-header-metadata":
			metadataIndex = index
		}
	}
	if titleIndex < 0 || metadataIndex < 0 {
		return r.errorLabel(gtx, fmt.Errorf("compact project header content roles are incomplete"))
	}
	title := copy.Children[titleIndex]
	metadata := copy.Children[metadataIndex]
	back := node.Children[backIndex]
	back.Style.Role = "icon-button"
	top := func(gtx layout.Context) layout.Dimensions {
		return layoutCompactProjectHeaderRow(gtx, gtx.Dp(r.metrics.spaceMedium),
			func(gtx layout.Context) layout.Dimensions {
				return r.layoutNode(gtx, back, data, fmt.Sprintf("%s/%d", path, backIndex))
			},
			func(gtx layout.Context) layout.Dimensions {
				return r.layoutNode(gtx, title, data, fmt.Sprintf("%s/%d/%d", path, copyIndex, titleIndex))
			},
			func(gtx layout.Context) layout.Dimensions {
				return r.layoutNode(gtx, logo, data, fmt.Sprintf("%s/%d", path, logoIndex))
			},
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(top),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutNode(gtx, metadata, data, fmt.Sprintf("%s/%d/%d", path, copyIndex, metadataIndex))
			})
		}),
	)
}

func layoutCompactProjectHeaderRow(gtx layout.Context, gap int, back, title, logo layout.Widget) layout.Dimensions {
	// Let the 72dp logo establish the row height. Forcing that minimum onto
	// every child top-aligns text inside an artificially tall text box.
	gtx.Constraints.Min.Y = 0
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gap}.Layout(gtx,
		layout.Rigid(back),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			return title(gtx)
		}),
		layout.Rigid(logo),
	)
}

func (r *Renderer) nodeOccupiesLayout(raw uidsl.Node, data any) bool {
	node, hidden := applyGioOverride(raw, r.compact)
	if hidden {
		return false
	}
	if node.Visible == nil {
		return true
	}
	value, err := uidsl.Resolve(data, node.Visible.Binding)
	if err != nil {
		// Preserve the binding error that layoutNode renders in place of the node.
		return true
	}
	equal := conditionEqual(node.Visible, value)
	return (node.Visible.Not && !equal) || (!node.Visible.Not && equal)
}

func (r *Renderer) layoutCompactActionRow(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	var content, actions []layout.FlexChild
	for index := range node.Children {
		child := node.Children[index]
		if child.Component == "spacer" {
			continue
		}
		widgetFn := func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			return r.layoutNode(gtx, child, data, fmt.Sprintf("%s/%d", path, index))
		}
		if child.Component == "button" {
			actions = append(actions, layout.Rigid(widgetFn))
		} else {
			content = append(content, layout.Rigid(widgetFn))
		}
	}
	return layout.Inset{
		Top: r.spacing(node.Layout.Padding), Right: r.spacing(node.Layout.Padding),
		Bottom: r.spacing(node.Layout.Padding), Left: r.spacing(node.Layout.Padding),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 2)
		if len(content) > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start, Gap: gtx.Dp(r.metrics.spaceSmall)}.Layout(gtx, content...)
			}))
		}
		if len(actions) > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(r.metrics.spaceSmall)}.Layout(gtx, actions...)
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
	})
}

func (r *Renderer) layoutCompactExecutionRecord(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	labels := []string{"Job", "Status", "Pipeline", "Build", "Agent", "Created", "Reason", "Actions"}
	if node.Style.Role == "history-execution-job-row" {
		labels = []string{"Job", "Status", "Pipeline", "Build", "Agent", "Created", "Duration"}
	}
	rows := make([]layout.FlexChild, 0, len(node.Children))
	for index := range node.Children {
		child := node.Children[index]
		if index >= len(labels) || !compactNodeHasContent(child, data) {
			continue
		}
		label := labels[index]
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				key := r.materialTextLabel(label, "table-header", false)
				key.Color = r.palette.muted
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(70)
						return key.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = 0
						return r.layoutNode(gtx, child, data, fmt.Sprintf("%s/%d", path, index))
					}),
				)
			})
		}))
	}
	return layout.Inset{Top: 7, Right: r.metrics.spaceSmall, Bottom: 7, Left: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, rows...)
	})
}

func (r *Renderer) layoutCompactAgentRecord(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	labels := []string{"Agent ID", "Host", "Platform", "Version", "Heartbeat", "Health", "Run mode"}
	rows := make([]layout.FlexChild, 0, len(labels))
	for index := range node.Children {
		if index >= len(labels) || !compactNodeHasContent(node.Children[index], data) {
			continue
		}
		child := node.Children[index]
		label := labels[index]
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: r.metrics.spaceSmall}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				key := r.materialTextLabel(label, "table-header", false)
				key.Color = r.palette.muted
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(88)
						return key.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = 0
						return r.layoutNode(gtx, child, data, fmt.Sprintf("%s/%d", path, index))
					}),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, rows...)
}

func compactNodeHasContent(node uidsl.Node, data any) bool {
	if node.Visible != nil {
		value, err := uidsl.Resolve(data, node.Visible.Binding)
		if err != nil {
			return false
		}
		equal := conditionEqual(node.Visible, value)
		if (!node.Visible.Not && !equal) || (node.Visible.Not && equal) {
			return false
		}
	}
	if node.Text != nil {
		value, err := uidsl.RenderText(data, *node.Text)
		return err == nil && strings.TrimSpace(value) != ""
	}
	if len(node.Children) > 0 {
		for _, child := range node.Children {
			if compactNodeHasContent(child, data) {
				return true
			}
		}
		return false
	}
	return node.Component != "spacer"
}

func compactRowNeedsStack(children []uidsl.Node) bool {
	buttons := 0
	for _, child := range children {
		if child.Component == "button" {
			buttons++
		}
	}
	return buttons >= 2
}

func flexAlignment(axis layout.Axis, align string, executionGrid bool) layout.Alignment {
	switch strings.ToLower(strings.TrimSpace(align)) {
	case "center", "middle":
		return layout.Middle
	case "end":
		return layout.End
	case "start":
		return layout.Start
	}
	if executionGrid {
		return layout.Start
	}
	if axis == layout.Horizontal {
		return layout.Middle
	}
	return layout.Start
}

func executionGridWeights(role string, childCount int) []float32 {
	var weights []float32
	switch role {
	case "queued-execution-header", "queued-execution-job-row":
		weights = []float32{2.0, 1.0, 1.25, 1.1, 1.2, 1.35, 2.25, 0.85}
	case "history-execution-header", "history-execution-job-row":
		weights = []float32{2.0, 1.0, 1.25, 1.1, 1.2, 1.35, 1.0}
	case "agent-header":
		weights = []float32{1.4, 1.2, 1.1, 0.8, 1.0, 0.9, 0.8}
	case "agent-record":
		weights = []float32{1.4, 1.2, 1.1, 0.8, 1.0, 0.9, 0.8, 0.2}
	default:
		return nil
	}
	if len(weights) != childCount {
		return nil
	}
	return weights
}

func (r *Renderer) constrainNode(gtx layout.Context, node uidsl.Node, content layout.Widget) layout.Dimensions {
	constraints := gtx.Constraints
	if value, ok := r.layoutDimension(node.Layout.MinWidth, gtx); ok {
		constraints.Min.X = min(max(value, constraints.Min.X), constraints.Max.X)
	}
	if value, ok := r.layoutDimension(node.Layout.MaxWidth, gtx); ok {
		constraints.Max.X = max(constraints.Min.X, min(value, constraints.Max.X))
	}
	if value, ok := r.layoutDimension(node.Layout.MinHeight, gtx); ok {
		constraints.Min.Y = min(max(value, constraints.Min.Y), constraints.Max.Y)
	}
	if value, ok := r.layoutDimension(node.Layout.MaxHeight, gtx); ok {
		constraints.Max.Y = max(constraints.Min.Y, min(value, constraints.Max.Y))
	}
	gtx.Constraints = constraints
	return content(gtx)
}

func (r *Renderer) layoutDimension(value string, gtx layout.Context) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if value == "page" {
		return gtx.Dp(r.metrics.pageWidth), true
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return gtx.Dp(unit.Dp(parsed)), true
}

func (r *Renderer) layoutText(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	text := "ciwi"
	if node.Text != nil {
		resolved, err := uidsl.RenderText(data, *node.Text)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		text = resolved
	}
	role := r.typographyRole(node.Style.Role)
	strong := node.Style.Emphasis == "strong"
	typography := r.nativeTextStyle(role, strong)
	if role == "code" || role == "code-inline" || role == "output-code" {
		editor := r.textEditors[path]
		if editor == nil {
			editor = &widget.Editor{ReadOnly: true}
			r.textEditors[path] = editor
		}
		// Compact code labels may still need to wrap, such as a long pipeline
		// chain. Truncated labels (graph node identifiers) stay on one line.
		editor.SingleLine = role == "code-inline" && node.Style.Truncate
		outputChanged := editor.Text() != text
		if outputChanged {
			editor.SetText(text)
			if node.ID == "job-output-text" && r.outputTailing {
				runeCount := utf8.RuneCountInString(text)
				editor.SetCaret(runeCount, runeCount)
			}
		}
		outputItemID := ""
		if node.ID == "job-output-group-text" {
			if value, resolveErr := uidsl.Resolve(data, "outputGroup.id"); resolveErr == nil {
				outputItemID = fmt.Sprint(value)
				r.outputEditors[outputItemID] = editor
				if outputChanged && r.outputTailing {
					runeCount := utf8.RuneCountInString(text)
					editor.SetCaret(runeCount, runeCount)
				}
			}
		} else if node.ID == "job-output-system-text" {
			r.outputEditors[""] = editor
		}
		if pending := r.pendingOutputSelection; pending != nil && pending.itemID == outputItemID && (node.ID == "job-output-group-text" || node.ID == "job-output-system-text") {
			editor.SetCaret(pending.start, pending.end)
			r.pendingOutputSelection = nil
		}
		style := material.Editor(r.theme, editor, "")
		style.Font = typography.font
		style.TextSize = typography.size
		style.LineHeightScale = typography.lineHeight
		style.Color = r.palette.text
		if tone, ok := r.toneColor(node.Style.Tone); ok {
			style.Color = tone
		}
		style.SelectionColor = r.palette.focus
		style.SelectionColor.A = 0xc0
		if role == "code-inline" || role == "output-code" {
			return style.Layout(gtx)
		}
		return layout.UniformInset(12).Layout(gtx, style.Layout)
	}
	label := material.Label(r.theme, typography.size, text)
	label.Font = typography.font
	label.LineHeightScale = typography.lineHeight
	if role == "table-header" {
		label.Color = r.palette.muted
	}
	if tone, ok := r.toneColor(node.Style.Tone); ok {
		label.Color = tone
	}
	if node.Style.Truncate || role == "badge" || role == "table-header" || node.Style.Role == "execution-row" {
		label.MaxLines = 1
	}
	label.State = r.selectable(path)
	return label.Layout(gtx)
}

type nativeTextStyle struct {
	font       font.Font
	size       unit.Sp
	lineHeight float32
}

func (r *Renderer) typographyRole(role string) string {
	switch role {
	case "execution-row":
		return "control"
	case "":
		return "body"
	}
	if _, ok := r.typography.Roles[role]; ok {
		return role
	}
	return "body"
}

func (r *Renderer) materialTextLabel(text, role string, strong bool) material.LabelStyle {
	typography := r.nativeTextStyle(role, strong)
	label := material.Label(r.theme, typography.size, text)
	label.Font = typography.font
	label.LineHeightScale = typography.lineHeight
	return label
}

func (r *Renderer) nativeTextStyle(role string, strong bool) nativeTextStyle {
	role = r.typographyRole(role)
	definition := r.typography.Roles[role]
	weightName := definition.Weight
	if strong {
		weightName = "strong"
	}
	weight := r.typography.Weights[weightName].Native
	return nativeTextStyle{
		font: font.Font{
			Typeface: font.Typeface(r.typography.Families[definition.Family]),
			Weight:   font.Weight(weight - 400),
		},
		size:       unit.Sp(definition.Size),
		lineHeight: definition.LineHeight,
	}
}

func (r *Renderer) layoutBadge(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	// Badges are intrinsically sized pills even when their parent is a flexed
	// column with an exact cross-axis constraint.
	gtx.Constraints.Min.X = 0
	tone, ok := r.toneColor(node.Style.Tone)
	if !ok {
		tone = r.palette.accent
	}
	background := tone
	border := tone
	borderWidth := unit.Dp(1)
	if node.Style.Tone == "muted" {
		// Browser metadata pills use the theme's colorful secondary accent,
		// not a translucent version of muted body text. Both are semantic
		// theme tokens, so this stays consistent across every theme.
		node.Style.Tone = "pill"
		background = r.palette.pillBackground
		border = color.NRGBA{}
		borderWidth = 0
		if node.Style.Emphasis == "strong" {
			border = r.palette.border
			borderWidth = 1
		}
	} else {
		background.A = 0x24
		border.A = 0x90
	}
	node.Style.Role = "badge"
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rect := image.Rectangle{Max: gtx.Constraints.Min}
		radius := rect.Dy() / 2
		if borderWidth > 0 {
			r.paintCachedRoundedFillPx(gtx.Ops, rect.Size(), radius, border)
			inset := max(1, gtx.Dp(borderWidth))
			inner := rect.Inset(inset)
			if !inner.Empty() {
				offset := op.Offset(inner.Min).Push(gtx.Ops)
				r.paintCachedRoundedFillPx(gtx.Ops, inner.Size(), max(0, radius-inset), background)
				offset.Pop()
			}
		} else {
			r.paintCachedRoundedFillPx(gtx.Ops, rect.Size(), radius, background)
		}
		return layout.Dimensions{Size: rect.Size()}
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 2, Right: 8, Bottom: 2, Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutText(gtx, node, data, path+"/text")
		})
	})
}

func (r *Renderer) toneColor(tone string) (color.NRGBA, bool) {
	switch tone {
	case "muted":
		return r.palette.muted, true
	case "accent":
		return r.palette.accent, true
	case "accent-strong":
		return r.palette.accentStrong, true
	case "pill":
		return r.palette.pillText, true
	case "success":
		return r.palette.success, true
	case "warning":
		return r.palette.warning, true
	case "awaiting":
		return r.palette.awaitingText, true
	case "danger":
		return r.palette.danger, true
	case "focus":
		return r.palette.focus, true
	case "console-text":
		return r.palette.consoleText, true
	case "console-muted":
		return r.palette.consoleMuted, true
	case "console-accent":
		return r.palette.consoleAccent, true
	default:
		return color.NRGBA{}, false
	}
}

func (r *Renderer) layoutImage(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	if node.Image == nil {
		return r.errorLabel(gtx, fmt.Errorf("image source is missing"))
	}
	if node.Image.Binding != "" {
		value, err := uidsl.Resolve(data, node.Image.Binding)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		encoded := strings.TrimSpace(fmt.Sprint(value))
		if encoded == "" {
			return layout.Dimensions{}
		}
		dynamic, ok := r.dynamicImages[path]
		if !ok || dynamic.encoded != encoded {
			payload, decodeErr := base64.StdEncoding.DecodeString(encoded)
			if decodeErr != nil {
				return r.errorLabel(gtx, fmt.Errorf("decode bound image: %w", decodeErr))
			}
			decoded, _, decodeErr := image.Decode(bytes.NewReader(payload))
			if decodeErr != nil {
				return r.errorLabel(gtx, fmt.Errorf("decode bound project image: %w", decodeErr))
			}
			dynamic = dynamicImage{encoded: encoded, source: paint.NewImageOp(decoded)}
			dynamic.source.Filter = paint.FilterNearest
			r.dynamicImages[path] = dynamic
		}
		width, height := r.metrics.imageBrandWidth, r.metrics.imageBrandHeight
		if node.Style.Role == "project-icon" {
			width, height = 72, 72
		} else if node.Style.Role == "job-header-icon" {
			width, height = 100, 100
		}
		return r.layoutImageSource(gtx, dynamic.source, node.Image.Description, width, height)
	}
	return r.layoutImageSized(gtx, node.Image, r.metrics.imageBrandWidth, r.metrics.imageBrandHeight)
}

func (r *Renderer) layoutIcon(gtx layout.Context, node uidsl.Node, data any) layout.Dimensions {
	if strings.TrimSpace(node.Icon) == "" {
		return r.errorLabel(gtx, fmt.Errorf("icon name is missing"))
	}
	tone := node.Style.Tone
	if tone == "" {
		tone = "accent"
	}
	if node.Pulse == nil {
		return r.layoutGlyph(gtx, node.Icon, tone, 21)
	}
	value, err := uidsl.Resolve(data, node.Pulse.Binding)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	opacity := heartbeatPulseOpacity(heartbeatUnixMillis(value), gtx.Now)
	if opacity > heartbeatPulseMinimum {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
	}
	semantic.DescriptionOp("Heartbeat").Add(gtx.Ops)
	fade := paint.PushOpacity(gtx.Ops, opacity)
	dimensions := r.layoutGlyph(gtx, node.Icon, tone, 21)
	fade.Pop()
	return dimensions
}

func (r *Renderer) layoutImageSized(gtx layout.Context, description *uidsl.Image, width, height unit.Dp) layout.Dimensions {
	if description == nil {
		return r.errorLabel(gtx, fmt.Errorf("image source is missing"))
	}
	source, ok := r.images[description.Asset]
	if !ok {
		return r.errorLabel(gtx, fmt.Errorf("image asset %q is unavailable", description.Asset))
	}
	return r.layoutImageSource(gtx, source, description.Description, width, height)
}

func (r *Renderer) layoutImageSource(gtx layout.Context, source paint.ImageOp, description string, width, height unit.Dp) layout.Dimensions {
	semantic.DescriptionOp(description).Add(gtx.Ops)
	size := gtx.Constraints.Constrain(image.Pt(gtx.Dp(width), gtx.Dp(height)))
	gtx.Constraints = layout.Exact(size)
	return widget.Image{Src: source, Fit: widget.Contain, Position: layout.Center, Scale: 1}.Layout(gtx)
}

func (r *Renderer) layoutDisclosureToggle(gtx layout.Context, button *widget.Clickable, iconName, description string) layout.Dimensions {
	icon := r.icons[iconName]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", iconName))
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.DescriptionOp(description).Add(gtx.Ops)
		return layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(14), gtx.Dp(14)))
			return icon.Layout(gtx, r.palette.muted)
		})
	})
}

func (r *Renderer) layoutDisclosureIndicator(gtx layout.Context, iconName string) layout.Dimensions {
	icon := r.icons[iconName]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", iconName))
	}
	return layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(14), gtx.Dp(14)))
		return icon.Layout(gtx, r.palette.muted)
	})
}

func (r *Renderer) layoutGlyph(gtx layout.Context, iconName, tone string, size unit.Dp) layout.Dimensions {
	icon := r.icons[iconName]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", iconName))
	}
	iconColor, ok := r.toneColor(tone)
	if !ok {
		iconColor = r.palette.accent
	}
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(size), gtx.Dp(size)))
	if iconName == "loader-2" {
		return r.layoutAnimatedLoader(gtx, iconColor)
	}
	return icon.Layout(gtx, iconColor)
}

func (r *Renderer) layoutButton(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	if !node.Layout.Grow {
		gtx.Constraints.Min.X = 0
	}
	label, enabled := r.buttonNodeState(&node, data)
	r.handleButtonClicks(gtx, node, data, path, enabled)
	button := r.button(path)
	if node.Style.Role == "icon-button" && node.Icon != "" {
		return r.layoutIconButton(gtx, button, node.Icon, label)
	}
	if node.Style.Role == "tailing-toggle" && node.Icon != "" {
		return r.layoutTonedIconButton(gtx, button, node.Icon, label, node.Style.Tone)
	}
	if node.Style.Role == "connection-pulse" {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
		opacity := paint.PushOpacity(gtx.Ops, connectionPulseOpacity(gtx.Now))
		dimensions := r.layoutControlButton(gtx, button, label, node.Icon, enabled)
		opacity.Pop()
		return dimensions
	}
	return r.layoutControlButtonReserved(gtx, button, label, r.longestButtonLabel(node, data, label), node.Icon, enabled)
}

func (r *Renderer) longestButtonLabel(node uidsl.Node, data any, displayed string) string {
	longest := displayed
	if node.Text != nil {
		if ordinary, err := uidsl.RenderText(data, *node.Text); err == nil && utf8.RuneCountInString(ordinary) > utf8.RuneCountInString(longest) {
			longest = ordinary
		}
	}
	if len(node.Actions) == 0 {
		return longest
	}
	r.mu.RLock()
	catalog := r.actionCatalog
	r.mu.RUnlock()
	if catalog != nil {
		if spec, ok := catalog.Spec(node.Actions[0].Command); ok && utf8.RuneCountInString(spec.Pending) > utf8.RuneCountInString(longest) {
			longest = spec.Pending
		}
	}
	return longest
}

func (r *Renderer) buttonNodeState(node *uidsl.Node, data any) (string, bool) {
	label := "Run"
	if node.Text != nil {
		if resolved, err := uidsl.RenderText(data, *node.Text); err == nil {
			label = resolved
		}
	}
	enabled := conditionEnabled(node.Enabled, data)
	pending := operations.Operation{}
	// SetOperations replaces this map instead of mutating it, so the snapshot
	// remains safe to read after releasing the lock. Most frames have no active
	// operations and can skip argument rendering and fingerprint hashing.
	r.mu.RLock()
	activeOperations := r.activeOperations
	r.mu.RUnlock()
	if len(activeOperations) > 0 && len(node.Actions) > 0 {
		if arguments, err := actionArguments(node.Actions[0], data); err == nil {
			if fingerprint, err := operations.Fingerprint(node.Actions[0].Command, arguments); err == nil {
				pending = activeOperations[fingerprint]
			}
		}
	}
	if pending.ID != "" {
		enabled = false
		if strings.TrimSpace(pending.PendingLabel) != "" {
			label = pending.PendingLabel
		}
		node.Icon = "loader-2"
	}
	return label, enabled
}

func (r *Renderer) handleButtonClicks(gtx layout.Context, node uidsl.Node, data any, path string, enabled bool) bool {
	button := r.button(path)
	clicked := false
	for r.clicked(gtx, button) {
		clicked = true
		r.markInteraction(path)
		if enabled && len(node.Actions) > 0 {
			r.dispatchFromLayout(gtx, node.Actions[0], data)
		}
	}
	return clicked
}

func (r *Renderer) layoutInput(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	if node.Input == nil {
		return r.errorLabel(gtx, fmt.Errorf("input configuration is missing"))
	}
	value, err := uidsl.Resolve(data, node.Input.Value)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	editor := r.inputEditors[path]
	if editor == nil {
		editor = &widget.Editor{}
		editor.SetText(fmt.Sprint(value))
		r.inputEditors[path] = editor
	} else if !gtx.Focused(editor) && editor.Text() != fmt.Sprint(value) {
		editor.SetText(fmt.Sprint(value))
	}
	editor.SingleLine = !node.Input.Multiline
	editor.Submit = !node.Input.Multiline
	changed := false
	submitted := false
	for {
		event, ok := editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.ChangeEvent); ok {
			changed = true
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	if (changed || submitted) && len(node.Actions) > 0 {
		inputData := mergeData(data, "input", map[string]any{"value": editor.Text()})
		r.dispatchFromLayout(gtx, node.Actions[0], inputData)
	}
	return r.layoutCachedBorder(gtx, r.palette.border, r.metrics.controlRadius, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: r.metrics.controlPaddingY, Right: r.metrics.controlPaddingX, Bottom: r.metrics.controlPaddingY, Left: r.metrics.controlPaddingX}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if node.Input.Multiline && node.Input.MinLines > 1 {
				minimum := gtx.Dp(unit.Dp(float32(node.Input.MinLines) * 24))
				if gtx.Constraints.Min.Y < minimum {
					gtx.Constraints.Min.Y = minimum
				}
			}
			style := material.Editor(r.theme, editor, node.Input.Placeholder)
			role := "control"
			if node.Style.Role == "code" || node.Style.Role == "code-inline" {
				role = "code"
			}
			typography := r.nativeTextStyle(role, false)
			style.Font = typography.font
			style.TextSize = typography.size
			style.LineHeightScale = typography.lineHeight
			style.Color = r.palette.text
			style.HintColor = r.palette.muted
			return style.Layout(gtx)
		})
	})
}

func (r *Renderer) layoutScroller(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	if node.Repeat == nil {
		return r.errorLabel(gtx, fmt.Errorf("scroller repeat configuration is missing"))
	}
	items, err := resolveItems(data, node.Repeat.Source)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	if node.ID == "job-output-groups" && (r.compact || !r.anyOutputGroupExpanded(items)) {
		r.outputScroller = nil
		return r.layoutInlineScrollerItems(gtx, node, data, path, items)
	}
	list := r.scrollers[path]
	if list == nil {
		axis := layout.Horizontal
		if node.Layout.Direction == "vertical" {
			axis = layout.Vertical
		}
		list = &layout.List{Axis: axis}
		r.scrollers[path] = list
	}
	if node.ID == "job-output-groups" {
		r.outputScroller = list
		// Gio bottom-aligns a short list when ScrollToEnd is true. Only tail a
		// list known from its previous layout to exceed the available viewport.
		list.ScrollToEnd = r.outputTailing && list.Position.Length > gtx.Constraints.Max.Y
	}
	content := func(gtx layout.Context) layout.Dimensions {
		return r.layoutGuardedList(gtx, "scroller:"+path, list, len(items), func(gtx layout.Context, index int) layout.Dimensions {
			if list.Axis == layout.Vertical {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
			}
			itemData := mergeData(data, node.Repeat.As, items[index])
			itemPath := fmt.Sprintf("%s/%d", path, index)
			if key, resolveErr := uidsl.Resolve(itemData, node.Repeat.Key); resolveErr == nil {
				itemPath = path + "/" + fmt.Sprint(key)
			}
			inset := layout.Inset{Right: r.spacing(node.Layout.Gap)}
			component := "row"
			if list.Axis == layout.Vertical {
				inset = layout.Inset{Bottom: r.spacing(node.Layout.Gap)}
				component = "column"
			}
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				container := node
				container.Component = component
				container.Repeat = nil
				container.Actions = nil
				return r.layoutChildren(gtx, container, itemData, itemPath)
			})
		})
	}
	if node.ID != "job-output-groups" {
		return content(gtx)
	}
	stateKey, expanded := r.visibleOutputGroupState(items, list.Position.First)
	if !expanded {
		return content(gtx)
	}
	collapse := r.button(path + "/floating-collapse")
	for r.clicked(gtx, collapse) {
		r.markInteraction(path + "/floating-collapse")
		r.setDisclosureState(stateKey, false, true)
	}
	return layout.Stack{Alignment: layout.NE}.Layout(gtx,
		layout.Stacked(content),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutControlButton(gtx, collapse, "Collapse", "chevron-up", true)
			})
		}),
	)
}

func (r *Renderer) anyOutputGroupExpanded(items []any) bool {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		stateKey := strings.TrimSpace(fmt.Sprint(item["state_key"]))
		if stateKey != "" && r.disclosures[stateKey] {
			return true
		}
	}
	return false
}

func (r *Renderer) layoutInlineScrollerItems(gtx layout.Context, node uidsl.Node, data any, path string, items []any) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(items))
	for index := range items {
		itemData := mergeData(data, node.Repeat.As, items[index])
		itemPath := fmt.Sprintf("%s/%d", path, index)
		if key, err := uidsl.Resolve(itemData, node.Repeat.Key); err == nil {
			itemPath = path + "/" + fmt.Sprint(key)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Inset{Bottom: r.spacing(node.Layout.Gap)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				container := node
				container.Component = "column"
				container.Repeat = nil
				container.Actions = nil
				return r.layoutChildren(gtx, container, itemData, itemPath)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (r *Renderer) visibleOutputGroupState(items []any, index int) (string, bool) {
	if index < 0 || index >= len(items) {
		return "", false
	}
	item, ok := items[index].(map[string]any)
	if !ok {
		return "", false
	}
	stateKey := strings.TrimSpace(fmt.Sprint(item["state_key"]))
	if stateKey == "" {
		return "", false
	}
	return stateKey, r.disclosures[stateKey]
}

type nativeSelectOption struct{ value, label string }

func (r *Renderer) layoutSelect(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	if node.Select == nil {
		return r.errorLabel(gtx, fmt.Errorf("select configuration is missing"))
	}
	if !node.Layout.Grow {
		gtx.Constraints.Min.X = 0
	}
	enabled := conditionEnabled(node.Enabled, data)
	value, err := uidsl.Resolve(data, node.Select.Value)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	items, err := resolveItems(data, node.Select.Options)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	options := make([]nativeSelectOption, 0, len(items))
	selectedValue := fmt.Sprint(value)
	selectedLabel := selectedValue
	for _, item := range items {
		itemData := mergeData(data, node.Select.As, item)
		optionValue, valueErr := uidsl.Resolve(itemData, node.Select.OptionValue)
		optionLabel, labelErr := uidsl.Resolve(itemData, node.Select.OptionLabel)
		if valueErr != nil {
			return r.errorLabel(gtx, valueErr)
		}
		if labelErr != nil {
			return r.errorLabel(gtx, labelErr)
		}
		entry := nativeSelectOption{value: fmt.Sprint(optionValue), label: fmt.Sprint(optionLabel)}
		options = append(options, entry)
		if entry.value == selectedValue {
			selectedLabel = entry.label
		}
	}
	for _, entry := range options {
		choice := r.button(path + "/option/" + entry.value)
		for r.clicked(gtx, choice) {
			r.markInteraction(path + "/option/" + entry.value)
			r.selectOpen[path] = false
			if len(node.Actions) > 0 && entry.value != selectedValue {
				selectionData := mergeData(data, "selection", map[string]any{"value": entry.value, "label": entry.label})
				r.dispatch(node.Actions[0], selectionData)
			}
			r.requestFrame()
		}
	}
	toggle := r.button(path + "/select-toggle")
	for r.clicked(gtx, toggle) {
		r.markInteraction(path + "/select-toggle")
		if enabled {
			r.selectOpen[path] = !r.selectOpen[path]
			r.requestFrame()
		}
	}
	if !enabled {
		r.selectOpen[path] = false
	}
	dismiss := r.selectDismiss[path]
	if dismiss == nil {
		dismiss = new(widget.Clickable)
		r.selectDismiss[path] = dismiss
	}
	for r.clicked(gtx, dismiss) {
		r.selectOpen[path] = false
		r.requestFrame()
	}
	widest := r.selectControlWidth(gtx, options, selectedLabel)
	header := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, widest)
		icon := "chevron-down"
		if r.selectOpen[path] {
			icon = "chevron-up"
		}
		visuals := r.controls.Select
		return r.layoutControlButtonPositioned(gtx, toggle, selectedLabel, selectedLabel, icon, enabled,
			visuals.ChevronPosition, unit.Dp(visuals.ChevronSize), unit.Dp(visuals.ChevronGap), unit.Dp(visuals.MinimumHeight))
	}
	if !r.selectOpen[path] {
		return header(gtx)
	}
	headerMacro := op.Record(gtx.Ops)
	headerDims := header(gtx)
	headerCall := headerMacro.Stop()
	overlayMacro := op.Record(gtx.Ops)
	// A deferred transparent scrim makes an outside tap dismiss the menu while
	// keeping the menu independent of surrounding layout.
	scrimContext := gtx
	scrimContext.Constraints = layout.Exact(gtx.Constraints.Max)
	dismiss.Layout(scrimContext, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	})
	menuMacro := op.Record(gtx.Ops)
	menuContext := gtx
	menuContext.Constraints.Min = image.Point{}
	menuContext.Constraints.Max.X = min(menuContext.Constraints.Max.X, widest)
	menuContext.Constraints.Min.X = menuContext.Constraints.Max.X
	menuContext.Constraints.Max.Y = min(max(menuContext.Constraints.Max.Y, gtx.Dp(unit.Dp(r.controls.Select.MenuMinimumHeight))), gtx.Dp(unit.Dp(r.controls.Select.MenuMaximumHeight)))
	optionList := r.selectLists[path]
	if optionList == nil {
		optionList = &layout.List{Axis: layout.Vertical}
		r.selectLists[path] = optionList
	}
	menu := r.surface(func(gtx layout.Context) layout.Dimensions {
		return optionList.Layout(gtx, len(options), func(gtx layout.Context, optionIndex int) layout.Dimensions {
			entry := options[optionIndex]
			choice := r.button(path + "/option/" + entry.value)
			bottom := unit.Dp(0)
			if optionIndex < len(options)-1 {
				bottom = unit.Dp(r.controls.Select.MenuItemGap)
			}
			return layout.Inset{Bottom: bottom}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutSelectOption(gtx, choice, entry.label, entry.value == selectedValue)
			})
		})
	}, unit.Dp(r.controls.Select.MenuPadding), false, nil)
	menuDims := menu(menuContext)
	menuCall := menuMacro.Stop()
	menuGap := gtx.Dp(unit.Dp(r.controls.Select.MenuGap))
	menuY := headerDims.Size.Y + menuGap
	if menuY+menuDims.Size.Y > gtx.Constraints.Max.Y && menuDims.Size.Y+menuGap < gtx.Constraints.Max.Y {
		menuY = -menuDims.Size.Y - menuGap
	}
	op.Offset(image.Pt(0, menuY)).Add(gtx.Ops)
	menuCall.Add(gtx.Ops)
	overlayCall := overlayMacro.Stop()
	op.Defer(gtx.Ops, overlayCall)
	headerCall.Add(gtx.Ops)
	return headerDims
}

func (r *Renderer) selectControlWidth(gtx layout.Context, options []nativeSelectOption, selected string) int {
	labels := make([]string, 0, len(options)+1)
	labels = append(labels, selected)
	for _, option := range options {
		labels = append(labels, option.label)
	}
	maxWidth := 0
	var measureOps op.Ops
	measureContext := gtx
	measureContext.Ops = &measureOps
	measureContext.Constraints.Min = image.Point{}
	for _, label := range labels {
		style := r.materialTextLabel(label, "control", false)
		style.MaxLines = 1
		if width := style.Layout(measureContext).Size.X; width > maxWidth {
			maxWidth = width
		}
	}
	visuals := r.controls.Select
	width := maxWidth + gtx.Dp(r.metrics.controlPaddingX*2) + gtx.Dp(unit.Dp(visuals.ChevronSize+visuals.ChevronGap))
	width = max(width, gtx.Dp(unit.Dp(visuals.MenuMinimumWidth)))
	return min(gtx.Constraints.Max.X, width)
}

func (r *Renderer) layoutControlButton(gtx layout.Context, button *widget.Clickable, label, iconName string, enabled bool) layout.Dimensions {
	return r.layoutControlButtonReserved(gtx, button, label, label, iconName, enabled)
}

func (r *Renderer) layoutControlButtonReserved(gtx layout.Context, button *widget.Clickable, label, reservedLabel, iconName string, enabled bool) layout.Dimensions {
	visuals := r.controls.Button
	return r.layoutControlButtonPositioned(gtx, button, label, reservedLabel, iconName, enabled,
		visuals.IconPosition, unit.Dp(visuals.IconSize), unit.Dp(visuals.IconGap), 0)
}

func (r *Renderer) layoutControlButtonPositioned(gtx layout.Context, button *widget.Clickable, label, reservedLabel, iconName string, enabled bool, iconPosition string, iconSize, iconGap, minimumHeight unit.Dp) layout.Dimensions {
	iconTrailing := iconPosition == "trailing"
	if reservedLabel != "" {
		var measureOps op.Ops
		measureContext := gtx
		measureContext.Ops = &measureOps
		measureContext.Constraints.Min = image.Point{}
		style := r.materialTextLabel(reservedLabel, "control", false)
		style.MaxLines = 1
		minimum := style.Layout(measureContext).Size.X + gtx.Dp(r.metrics.controlPaddingX*2)
		if iconName != "" {
			minimum += gtx.Dp(iconSize + iconGap)
		}
		gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, max(gtx.Constraints.Min.X, minimum))
	}
	if minimumHeight > 0 {
		gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, max(gtx.Constraints.Min.Y, gtx.Dp(minimumHeight)))
	}
	background := r.palette.surface
	borderColor := r.palette.border
	if enabled && button.Hovered() {
		background = r.palette.subtle
		borderColor = r.palette.accent
	}
	if enabled && gtx.Focused(button) {
		borderColor = r.palette.focus
	}
	radius := r.metrics.controlRadius
	return r.layoutCachedBorder(gtx, borderColor, radius, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			r.paintCachedRoundedFill(gtx, gtx.Constraints.Min, radius, background)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: r.metrics.controlPaddingY, Right: r.metrics.controlPaddingX, Bottom: r.metrics.controlPaddingY, Left: r.metrics.controlPaddingX}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := make([]layout.FlexChild, 0, 2)
					labelWidget := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						labelStyle := r.materialTextLabel(label, "control", false)
						labelStyle.MaxLines = 1
						labelStyle.Color = r.palette.accent
						if !enabled {
							labelStyle.Color = r.palette.muted
						}
						return labelStyle.Layout(gtx)
					})
					icon := r.icons[iconName]
					iconWidget := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						inset := layout.Inset{Right: iconGap}
						if iconTrailing {
							inset = layout.Inset{Left: iconGap}
						}
						return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(iconSize), gtx.Dp(iconSize)))
							iconColor := r.palette.accent
							if !enabled {
								iconColor = r.palette.muted
							}
							if iconName == "loader-2" {
								return r.layoutAnimatedLoader(gtx, iconColor)
							}
							return icon.Layout(gtx, iconColor)
						})
					})
					if icon != nil {
						if iconTrailing {
							children = append(children, labelWidget, iconWidget)
						} else {
							children = append(children, iconWidget, labelWidget)
						}
					} else {
						children = append(children, labelWidget)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
			})
		})
	})
}

func (r *Renderer) layoutSelectOption(gtx layout.Context, button *widget.Clickable, label string, selected bool) layout.Dimensions {
	visuals := r.controls.Select
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, max(gtx.Constraints.Min.Y, gtx.Dp(unit.Dp(visuals.OptionMinimumHeight))))
	background := color.NRGBA{}
	borderColor := color.NRGBA{}
	if button.Hovered() || gtx.Focused(button) {
		background = r.palette.subtle
		borderColor = r.palette.accent
	}
	radius := max(unit.Dp(0), r.metrics.controlRadius-2)
	return r.layoutCachedBorder(gtx, borderColor, radius, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if background.A != 0 {
				r.paintCachedRoundedFill(gtx, gtx.Constraints.Min, radius, background)
			}
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Top: unit.Dp(visuals.OptionPaddingY), Right: unit.Dp(visuals.OptionPaddingX),
					Bottom: unit.Dp(visuals.OptionPaddingY), Left: unit.Dp(visuals.OptionPaddingX),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					indicator := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						width := gtx.Dp(unit.Dp(visuals.SelectionIndicatorWidth))
						gtx.Constraints = layout.Exact(image.Pt(width, gtx.Dp(unit.Dp(visuals.ChevronSize))))
						if !selected {
							return layout.Dimensions{Size: gtx.Constraints.Min}
						}
						icon := r.icons["check"]
						if icon == nil {
							return layout.Dimensions{Size: gtx.Constraints.Min}
						}
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							size := min(width, gtx.Dp(unit.Dp(visuals.ChevronSize)))
							gtx.Constraints = layout.Exact(image.Pt(size, size))
							return icon.Layout(gtx, r.palette.accent)
						})
					})
					copy := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						style := r.materialTextLabel(label, "control", false)
						style.MaxLines = 1
						style.Color = r.palette.text
						if selected {
							style.Color = r.palette.accent
						}
						return style.Layout(gtx)
					})
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
						indicator,
						layout.Rigid(layout.Spacer{Width: unit.Dp(visuals.OptionGap)}.Layout),
						copy,
					)
				})
			})
		})
	})
}

func conditionEnabled(condition *uidsl.Condition, data any) bool {
	if condition == nil {
		return true
	}
	value, err := uidsl.Resolve(data, condition.Binding)
	if err != nil {
		return false
	}
	equal := conditionEqual(condition, value)
	if condition.Not {
		return !equal
	}
	return equal
}

func conditionEqual(condition *uidsl.Condition, value any) bool {
	if condition != nil && condition.Empty {
		return fmt.Sprint(value) == ""
	}
	return fmt.Sprint(value) == defaultString(condition.Equals, "true")
}

func (r *Renderer) layoutIconButton(gtx layout.Context, button *widget.Clickable, iconName, description string) layout.Dimensions {
	icon := r.icons[iconName]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", iconName))
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
	radius := r.metrics.controlRadius
	return r.layoutCachedBorder(gtx, borderColor, radius, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			r.paintCachedRoundedFill(gtx, gtx.Constraints.Min, radius, background)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(description).Add(gtx.Ops)
				return layout.UniformInset(9).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(19), gtx.Dp(19)))
					return icon.Layout(gtx, r.palette.accent)
				})
			})
		})
	})
}

func (r *Renderer) layoutTonedIconButton(gtx layout.Context, button *widget.Clickable, iconName, description, tone string) layout.Dimensions {
	ink, ok := r.toneColor(tone)
	if !ok {
		ink = r.palette.accent
	}
	icon := r.icons[iconName]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", iconName))
	}
	background := mixColorSRGB(r.palette.surface, ink, .12)
	borderColor := mixColorSRGB(r.palette.surface, ink, .55)
	if button.Hovered() {
		background = mixColorSRGB(r.palette.surface, ink, .18)
		borderColor = ink
	}
	if gtx.Focused(button) {
		borderColor = r.palette.focus
	}
	radius := r.metrics.controlRadius
	return r.layoutCachedBorder(gtx, borderColor, radius, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			r.paintCachedRoundedFill(gtx, gtx.Constraints.Min, radius, background)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(description).Add(gtx.Ops)
				return layout.UniformInset(9).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(19), gtx.Dp(19)))
					return icon.Layout(gtx, ink)
				})
			})
		})
	})
}

func (r *Renderer) surface(content layout.Widget, padding unit.Dp, hero bool, progress *semanticProgress) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		radius := r.metrics.surfaceRadius
		return r.layoutCachedBorder(gtx, r.palette.border, radius, 1, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rect := image.Rectangle{Max: gtx.Constraints.Min}
				r.paintSurfaceBackground(gtx, rect, radius, hero, 0)
				if progress != nil {
					r.paintSurfaceProgress(gtx, *progress, rect, radius, hero)
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(padding).Layout(gtx, content)
			})
		})
	}
}

func (r *Renderer) paintSurfaceProgress(gtx layout.Context, progress semanticProgress, surfaceRect image.Rectangle, radius unit.Dp, hero bool) {
	progressRect, opacity, animated, ok := semanticProgressPaint(progress, surfaceRect.Size(), gtx.Now)
	if !ok {
		return
	}
	if animated {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
	}
	surfaceClip := r.cachedRoundedClip(gtx, surfaceRect.Size(), radius).Push(gtx.Ops)
	progressClip := clip.Rect(progressRect).Push(gtx.Ops)
	if r.palette.surfaceGlow.A != 0 {
		// Browser progress is an sRGB color-mix over the hero texture. Gio's GPU
		// alpha blending is linear and makes bright success colors much stronger,
		// so cache the equivalent precomposed texture and reveal only its progress
		// portion. Quantizing animated opacity keeps the cache bounded.
		textureSize := gradientTextureSize(surfaceRect.Size())
		opacityByte := int(math.Round(opacity * 255))
		opacityByte = min(255, max(0, ((opacityByte+1)/2)*2))
		key := backgroundTextureKey{size: textureSize, progressOpacity: uint8(opacityByte)}
		background, exists := r.surfaceBackgrounds[key]
		if !exists {
			if len(r.surfaceBackgrounds) >= 32 {
				r.surfaceBackgrounds = map[backgroundTextureKey]paint.ImageOp{}
			}
			background = paint.NewImageOp(renderSurfaceProgressBackground(textureSize, r.palette, float64(opacityByte)/255))
			background.Filter = paint.FilterLinear
			r.surfaceBackgrounds[key] = background
		}
		paintScaledImageOps(gtx.Ops, background, surfaceRect.Size())
		progressClip.Pop()
		surfaceClip.Pop()
		return
	}
	progressColor := r.palette.success
	progressColor.A = uint8(math.Round(float64(progressColor.A) * opacity))
	paint.Fill(gtx.Ops, progressColor)
	progressClip.Pop()
	surfaceClip.Pop()
}

func (r *Renderer) paintSurfaceBackground(gtx layout.Context, rect image.Rectangle, radius unit.Dp, hero bool, _ float64) {
	radiusPx := gtx.Dp(radius)
	variant := "surface"
	if hero {
		variant = "hero"
	}
	key := visualOpKey{
		kind: "surface-background", variant: variant, size: rect.Size(), radius: radiusPx,
		color1: r.palette.surface, color2: r.palette.surfaceGlow,
	}
	r.visualOps.add(gtx.Ops, key, func(ops *op.Ops) {
		r.paintSurfaceBackgroundOps(ops, image.Rectangle{Max: rect.Size()}, radiusPx, hero)
	})
}

func (r *Renderer) paintSurfaceBackgroundOps(ops *op.Ops, rect image.Rectangle, radiusPx int, hero bool) {
	if r.palette.surfaceGlow.A != 0 {
		stack := clip.UniformRRect(rect, radiusPx).Push(ops)
		textureSize := gradientTextureSize(rect.Size())
		key := backgroundTextureKey{size: textureSize}
		background, ok := r.surfaceBackgrounds[key]
		if !ok {
			if len(r.surfaceBackgrounds) >= 32 {
				r.surfaceBackgrounds = map[backgroundTextureKey]paint.ImageOp{}
			}
			background = paint.NewImageOp(renderSurfaceBackground(textureSize, r.palette))
			background.Filter = paint.FilterLinear
			r.surfaceBackgrounds[key] = background
		}
		paintScaledImageOps(ops, background, rect.Size())
		stack.Pop()
	} else if hero {
		stack := clip.UniformRRect(rect, radiusPx).Push(ops)
		paint.LinearGradientOp{
			Stop1: f32.Pt(0, 0), Color1: r.palette.heroStart,
			Stop2: f32.Pt(float32(rect.Dx()), float32(rect.Dy())), Color2: r.palette.heroEnd,
		}.Add(ops)
		paint.PaintOp{}.Add(ops)
		stack.Pop()
	} else {
		paint.FillShape(ops, r.palette.surface, clip.UniformRRect(rect, radiusPx).Op(ops))
	}
}

func (r *Renderer) surfaceWithFill(content layout.Widget, padding unit.Dp, fill color.NRGBA) layout.Widget {
	return r.surfaceWithFillProgress(content, padding, fill, nil)
}

func (r *Renderer) surfaceWithFillProgress(content layout.Widget, padding unit.Dp, fill color.NRGBA, progress *semanticProgress) layout.Widget {
	return r.surfaceWithBorderProgress(content, padding, fill, r.palette.border, progress)
}

func (r *Renderer) surfaceWithBorder(content layout.Widget, padding unit.Dp, fill, border color.NRGBA) layout.Widget {
	return r.surfaceWithBorderRadius(content, padding, fill, border, r.metrics.surfaceRadius)
}

func (r *Renderer) surfaceWithBorderRadius(content layout.Widget, padding unit.Dp, fill, border color.NRGBA, radius unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return r.layoutCachedBorder(gtx, border, radius, 1, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				r.paintCachedRoundedFill(gtx, size, radius, fill)
				return layout.Dimensions{Size: size}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(padding).Layout(gtx, content)
			})
		})
	}
}

func (r *Renderer) surfaceWithBorderProgress(content layout.Widget, padding unit.Dp, fill, border color.NRGBA, progress *semanticProgress) layout.Widget {
	return r.surfaceWithBorderProgressColor(content, padding, fill, border, progress, r.palette.success)
}

func (r *Renderer) surfaceWithBorderProgressColor(content layout.Widget, padding unit.Dp, fill, border color.NRGBA, progress *semanticProgress, progressColor color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		radius := r.metrics.surfaceRadius
		return r.layoutCachedBorder(gtx, border, radius, 1, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				r.paintCachedRoundedFill(gtx, size, radius, fill)
				if progress != nil {
					stack := r.cachedRoundedClip(gtx, size, radius).Push(gtx.Ops)
					r.paintSemanticProgress(gtx, *progress, size, progressColor, &fill)
					stack.Pop()
				}
				return layout.Dimensions{Size: size}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(padding).Layout(gtx, content)
			})
		})
	}
}

func (r *Renderer) dispatchFromLayout(gtx layout.Context, action uidsl.Action, data any) {
	arguments, err := actionArguments(action, data)
	if err != nil {
		r.ShowAlert("Action unavailable", err.Error())
		return
	}
	switch action.Command {
	case "select-timeline-item":
		items, resolveErr := resolveItems(data, "jobDetails.timeline")
		if resolveErr != nil {
			r.ShowAlert("Timeline unavailable", resolveErr.Error())
			return
		}
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if ok && fmt.Sprint(itemMap["id"]) == arguments["id"] {
				r.SetRootBinding("jobDetails", "selected_timeline_item", itemMap)
				if groups, groupErr := resolveItems(data, "jobDetails.output_groups"); groupErr == nil {
					for index, rawGroup := range groups {
						group, groupOK := rawGroup.(map[string]any)
						if !groupOK || fmt.Sprint(group["id"]) != arguments["id"] {
							continue
						}
						stateKey := fmt.Sprint(group["state_key"])
						if stateKey != "" {
							r.setDisclosureState(stateKey, true, true)
						}
						if r.outputScroller != nil {
							r.outputScroller.ScrollTo(index)
						}
						break
					}
				}
				r.ShowNotice("Selected "+fmt.Sprint(itemMap["title"]), "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
				r.requestFrame()
				return
			}
		}
	case "change-output-search":
		r.outputSearch = arguments["query"]
		r.outputMatch = 0
		r.SetRootBinding("jobDetails", "output_search", r.outputSearch)
		r.selectGroupedOutputMatch(data, r.outputSearch, 0, true)
		r.requestFrame()
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
		if pending := r.pendingOutputSelection; pending == nil {
			if matches := groupedOutputMatches(data, query); len(matches) > 0 {
				if editor := r.outputEditors[matches[r.outputMatch].itemID]; editor != nil {
					gtx.Execute(key.FocusCmd{Tag: editor})
				}
			}
		}
		r.requestFrame()
	case "copy-output":
		output, resolveErr := uidsl.Resolve(data, "jobDetails.output")
		if resolveErr != nil {
			r.ShowAlert("Output unavailable", resolveErr.Error())
			return
		}
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(fmt.Sprint(output)))})
		r.ShowNotice("Output copied", "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
	case "copy-text":
		text := arguments["text"]
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
		r.ShowNotice("Copied", "", uidsl.Action{}, nil, presentation.TransientNoticeDuration)
	case "toggle-output-tailing":
		r.outputTailing = !r.outputTailing
		label := "Tailing: Off"
		tone := "warning"
		if r.outputTailing {
			label = "Tailing: On"
			tone = "success"
			if r.outputScroller != nil {
				r.outputScroller.ScrollToEnd = true
			}
		}
		r.SetRootBinding("jobDetails", "tailing_label", label)
		r.SetRootBinding("jobDetails", "tailing_tone", tone)
		r.requestFrame()
	case "set-disclosures":
		prefix := arguments["prefix"]
		expanded, parseErr := strconv.ParseBool(arguments["expanded"])
		if parseErr != nil || prefix == "" {
			r.ShowAlert("Action unavailable", "Invalid disclosure group")
			return
		}
		for key := range r.persistentDisclosures {
			if strings.HasPrefix(key, prefix) {
				r.disclosures[key] = expanded
			}
		}
		r.notifyDisclosureChange()
		r.requestFrame()
	default:
		r.dispatch(action, data)
	}
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
	if selectMatch {
		match := matches[r.outputMatch]
		if match.itemID != "" {
			if groups, err := resolveItems(data, "jobDetails.output_groups"); err == nil {
				for index, raw := range groups {
					group, ok := raw.(map[string]any)
					if !ok || fmt.Sprint(group["id"]) != match.itemID {
						continue
					}
					stateKey := fmt.Sprint(group["state_key"])
					if stateKey != "" {
						r.setDisclosureState(stateKey, true, true)
					}
					if r.outputScroller != nil {
						r.outputScroller.ScrollTo(index)
					}
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
			group, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			sources = append(sources, struct{ itemID, text string }{fmt.Sprint(group["id"]), fmt.Sprint(group["output"])})
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
	lowerOutput := strings.ToLower(output)
	lowerQuery := strings.ToLower(query)
	var matches [][2]int
	for offset := 0; offset <= len(lowerOutput)-len(lowerQuery); {
		index := strings.Index(lowerOutput[offset:], lowerQuery)
		if index < 0 {
			break
		}
		startByte := offset + index
		endByte := startByte + len(lowerQuery)
		matches = append(matches, [2]int{utf8.RuneCountInString(output[:startByte]), utf8.RuneCountInString(output[:endByte])})
		offset = endByte
	}
	return matches
}

func (r *Renderer) dispatch(action uidsl.Action, data any) {
	if r.onAction == nil {
		return
	}
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

func (r *Renderer) layoutConfirmation(gtx layout.Context) layout.Dimensions {
	pending := r.pending
	if pending == nil {
		return layout.Dimensions{}
	}
	if gtx.Constraints.Max.X > gtx.Dp(560) {
		gtx.Constraints.Max.X = gtx.Dp(560)
	}
	gtx.Constraints.Min = image.Point{}
	confirm := r.button("confirmation/confirm")
	cancel := r.button("confirmation/cancel")
	for r.clicked(gtx, cancel) {
		r.pending = nil
		r.requestFrame()
	}
	for r.clicked(gtx, confirm) {
		r.pending = nil
		r.onAction(pending.action, pending.arguments)
		r.requestFrame()
	}
	return r.surface(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 22, Right: 22, Bottom: 22, Left: 22}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			title := r.materialTextLabel(pending.title, "heading", false)
			title.State = r.selectable("confirmation/title")
			message := r.materialTextLabel(pending.message, "body", false)
			message.State = r.selectable("confirmation/message")
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(title.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 12, Bottom: 20}.Layout(gtx, message.Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.layoutControlButton(gtx, cancel, "Cancel", "", true)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return r.layoutControlButton(gtx, confirm, "Confirm", "", true)
							})
						}),
					)
				}),
			)
		})
	}, 14, false, nil)(gtx)
}

func (r *Renderer) layoutAlert(gtx layout.Context) layout.Dimensions {
	r.mu.RLock()
	alert := r.alert
	r.mu.RUnlock()
	if alert == nil {
		return layout.Dimensions{}
	}
	if gtx.Constraints.Max.X > gtx.Dp(560) {
		gtx.Constraints.Max.X = gtx.Dp(560)
	}
	gtx.Constraints.Min = image.Point{}
	ok := r.button("alert/ok")
	for r.clicked(gtx, ok) {
		r.mu.Lock()
		r.alert = nil
		r.mu.Unlock()
		r.requestFrame()
	}
	semantic.DescriptionOp(alert.title + ". " + alert.message).Add(gtx.Ops)
	return r.surface(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 22, Right: 22, Bottom: 22, Left: 22}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			title := r.materialTextLabel(alert.title, "heading", false)
			title.State = r.selectable("alert/title")
			message := r.materialTextLabel(alert.message, "body", false)
			message.State = r.selectable("alert/message")
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(title.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 12, Bottom: 20}.Layout(gtx, message.Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return r.layoutControlButton(gtx, ok, "OK", "", true)
						}),
					)
				}),
			)
		})
	}, 14, false, nil)(gtx)
}

func (r *Renderer) requestFrame() {
	if r.invalidate != nil {
		r.invalidate()
	}
}

func (r *Renderer) button(key string) *widget.Clickable {
	if button := r.buttons[key]; button != nil {
		return button
	}
	button := new(widget.Clickable)
	r.buttons[key] = button
	return button
}

func (r *Renderer) selectable(key string) *widget.Selectable {
	if r.selectables == nil {
		r.selectables = map[string]*widget.Selectable{}
	}
	if selectable := r.selectables[key]; selectable != nil {
		return selectable
	}
	selectable := new(widget.Selectable)
	r.selectables[key] = selectable
	return selectable
}

func (r *Renderer) errorLabel(gtx layout.Context, err error) layout.Dimensions {
	label := r.materialTextLabel(err.Error(), "detail-small", false)
	label.Color = r.palette.danger
	label.State = r.selectable("error/" + err.Error())
	return label.Layout(gtx)
}

func applyGioOverride(node uidsl.Node, compact bool) (uidsl.Node, bool) {
	hidden := false
	apply := func(override uidsl.Override) {
		if override.Hidden {
			hidden = true
		}
		if override.Layout != (uidsl.Layout{}) {
			node.Layout = mergeLayout(node.Layout, override.Layout)
		}
		if override.Style != (uidsl.Style{}) {
			node.Style = mergeStyle(node.Style, override.Style)
		}
	}
	if override, ok := node.Overrides["gio"]; ok {
		apply(override)
	}
	if compact {
		if override, ok := node.Overrides["compact"]; ok {
			apply(override)
		}
	}
	return node, hidden
}

func mergeLayout(base, override uidsl.Layout) uidsl.Layout {
	if override.Direction != "" {
		base.Direction = override.Direction
	}
	if override.Gap != "" {
		base.Gap = override.Gap
	}
	if override.Padding != "" {
		base.Padding = override.Padding
	}
	if override.Align != "" {
		base.Align = override.Align
	}
	if override.Justify != "" {
		base.Justify = override.Justify
	}
	if override.MinWidth != "" {
		base.MinWidth = override.MinWidth
	}
	if override.MaxWidth != "" {
		base.MaxWidth = override.MaxWidth
	}
	if override.MinHeight != "" {
		base.MinHeight = override.MinHeight
	}
	if override.MaxHeight != "" {
		base.MaxHeight = override.MaxHeight
	}
	if override.Wrap {
		base.Wrap = true
	}
	if override.Grow {
		base.Grow = true
	}
	return base
}

func mergeStyle(base, override uidsl.Style) uidsl.Style {
	if override.Role != "" {
		base.Role = override.Role
	}
	if override.Emphasis != "" {
		base.Emphasis = override.Emphasis
	}
	if override.Tone != "" {
		base.Tone = override.Tone
	}
	if override.ToneBinding != "" {
		base.ToneBinding = override.ToneBinding
	}
	if override.Truncate {
		base.Truncate = true
	}
	return base
}

func mergeData(root any, name string, value any) map[string]any {
	result := map[string]any{}
	if existing, ok := root.(map[string]any); ok {
		for key, item := range existing {
			result[key] = item
		}
	}
	result[name] = value
	return result
}

func preserveJobUIState(previous, next any) {
	if bindingString(previous, "jobDetails.id") == "" || bindingString(previous, "jobDetails.id") != bindingString(next, "jobDetails.id") {
		return
	}
	previousData, previousOK := previous.(map[string]any)
	nextData, nextOK := next.(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	previousRoot, previousOK := previousData["jobDetails"].(map[string]any)
	nextRoot, nextOK := nextData["jobDetails"].(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	for _, key := range []string{"output_search", "output_search_count", "tailing_label", "tailing_tone"} {
		if value, exists := previousRoot[key]; exists {
			nextRoot[key] = value
		}
	}
	selected, ok := previousRoot["selected_timeline_item"].(map[string]any)
	if !ok {
		return
	}
	selectedID := fmt.Sprint(selected["id"])
	if timeline, ok := nextRoot["timeline"].([]any); ok {
		for _, item := range timeline {
			entry, entryOK := item.(map[string]any)
			if entryOK && fmt.Sprint(entry["id"]) == selectedID {
				nextRoot["selected_timeline_item"] = entry
				return
			}
		}
	}
}

func preserveSettingsUIState(previous, next any) {
	previousData, previousOK := previous.(map[string]any)
	nextData, nextOK := next.(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	previousRoot, previousOK := previousData["settings"].(map[string]any)
	nextRoot, nextOK := nextData["settings"].(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	for _, field := range []string{
		"import_repo_url", "import_repo_ref", "import_config_file",
		"update_versions", "selected_update_version", "rollback_versions", "selected_rollback_version",
		"update_result", "update_result_tone", "rollback_result", "rollback_result_tone",
	} {
		if value, exists := previousRoot[field]; exists {
			nextRoot[field] = value
		}
	}
	selectedTarget := strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(previousRoot["selected_update_version"])), "v")
	if selectedTarget == "" {
		selectedTarget = strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(previousRoot["selected_rollback_version"])), "v")
	}
	currentVersion := strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(nextRoot["update_current_version"])), "v")
	if selectedTarget != "" && currentVersion == selectedTarget {
		nextRoot["update_result"] = "Update successful."
		nextRoot["update_result_tone"] = "success"
		nextRoot["rollback_result"] = ""
		nextRoot["selected_update_version"] = ""
		nextRoot["selected_rollback_version"] = ""
		nextRoot["update_versions"] = []any{map[string]any{"value": "", "label": "Check for updates"}}
		nextRoot["rollback_versions"] = []any{map[string]any{"value": "", "label": "Refresh versions"}}
	}
	statuses := map[string]map[string]any{}
	if projects, ok := previousRoot["projects"].([]any); ok {
		for _, raw := range projects {
			project, projectOK := raw.(map[string]any)
			if !projectOK || strings.TrimSpace(fmt.Sprint(project["action_status"])) == "" {
				continue
			}
			statuses[fmt.Sprint(project["id"])] = map[string]any{
				"status": project["action_status"], "tone": project["action_tone"],
			}
		}
	}
	if projects, ok := nextRoot["projects"].([]any); ok {
		for _, raw := range projects {
			project, projectOK := raw.(map[string]any)
			if !projectOK {
				continue
			}
			if status, exists := statuses[fmt.Sprint(project["id"])]; exists {
				project["action_status"] = status["status"]
				project["action_tone"] = status["tone"]
			}
		}
	}
}

func bindingString(data any, path string) string {
	value, err := uidsl.Resolve(data, path)
	if err != nil {
		return ""
	}
	return fmt.Sprint(value)
}

func resolveItems(root any, path string) ([]any, error) {
	value, err := uidsl.Resolve(root, path)
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("binding %q is not a list", path)
	}
	return items, nil
}

func paletteFromTheme(theme uidsl.Theme) (palette, error) {
	get := func(name string) (color.NRGBA, error) { return parseColor(theme.Colors[name]) }
	var p palette
	var err error
	for name, target := range map[string]*color.NRGBA{
		"background": &p.background, "surface": &p.surface, "surface-subtle": &p.subtle,
		"text": &p.text, "text-muted": &p.muted, "accent": &p.accent, "accent-strong": &p.accentStrong,
		"border": &p.border, "success": &p.success, "warning": &p.warning,
		"danger": &p.danger, "focus": &p.focus,
	} {
		*target, err = get(name)
		if err != nil {
			return palette{}, fmt.Errorf("theme color %s: %w", name, err)
		}
	}
	p.backgroundStart = p.background
	p.backgroundEnd = p.background
	p.heroStart = p.surface
	p.heroEnd = p.subtle
	p.surfaceRaised = p.subtle
	p.pillBackground = p.subtle
	p.pillText = p.accentStrong
	p.noticeBackground = p.surfaceRaised
	p.noticeText = p.text
	p.noticeBorder = p.border
	p.awaitingSurface = p.surfaceRaised
	p.awaitingBorder = p.warning
	p.awaitingText = p.warning
	if gradient, ok := theme.Gradients["page"]; ok && len(gradient.Stops) >= 2 {
		p.backgroundStart, err = parseColor(gradient.Stops[0].Color)
		if err != nil {
			return palette{}, fmt.Errorf("page gradient start: %w", err)
		}
		p.backgroundEnd, err = parseColor(gradient.Stops[len(gradient.Stops)-1].Color)
		if err != nil {
			return palette{}, fmt.Errorf("page gradient end: %w", err)
		}
	}
	for name, target := range map[string]*color.NRGBA{
		"background-start": &p.backgroundStart, "background-end": &p.backgroundEnd,
		"background-glow-a": &p.backgroundGlowA, "background-glow-b": &p.backgroundGlowB,
		"surface-raised": &p.surfaceRaised, "surface-glow": &p.surfaceGlow,
		"pill-background": &p.pillBackground, "pill-text": &p.pillText,
		"notice-background": &p.noticeBackground, "notice-text": &p.noticeText, "notice-border": &p.noticeBorder,
		"awaiting-surface": &p.awaitingSurface, "awaiting-border": &p.awaitingBorder,
		"awaiting-text":      &p.awaitingText,
		"console-background": &p.consoleBackground, "console-surface": &p.consoleSurface,
		"console-border": &p.consoleBorder, "console-text": &p.consoleText,
		"console-muted": &p.consoleMuted, "console-accent": &p.consoleAccent,
		"console-success": &p.consoleSuccess,
	} {
		value := strings.TrimSpace(theme.Colors[name])
		if value == "" {
			continue
		}
		*target, err = parseColor(value)
		if err != nil {
			return palette{}, fmt.Errorf("theme color %s: %w", name, err)
		}
	}
	if gradient, ok := theme.Gradients["hero"]; ok && len(gradient.Stops) >= 2 {
		p.heroStart, err = parseColor(gradient.Stops[0].Color)
		if err != nil {
			return palette{}, fmt.Errorf("hero gradient start: %w", err)
		}
		p.heroEnd, err = parseColor(gradient.Stops[len(gradient.Stops)-1].Color)
		if err != nil {
			return palette{}, fmt.Errorf("hero gradient end: %w", err)
		}
	}
	return p, nil
}

func rendererTheme(document *uidsl.ThemeDocument, typography uidsl.Typography) (*material.Theme, palette, error) {
	if document == nil {
		return nil, palette{}, fmt.Errorf("theme is required")
	}
	colors, err := paletteFromTheme(document.Theme)
	if err != nil {
		return nil, palette{}, err
	}
	theme := material.NewTheme()
	fonts, err := ciwiFontCollection()
	if err != nil {
		return nil, palette{}, err
	}
	// Ciwi Mono is an explicit alias backed by the exact same font files served
	// to browsers. Avoid relying on platform family lookup or parsed font-weight
	// metadata, both of which can otherwise select a visually different face.
	theme.Shaper = giotext.NewShaper(giotext.WithCollection(fonts))
	// Match the browser chrome's body font stack exactly. The shaper resolves
	// Avenir Next on macOS, Segoe UI on Windows, and the same generic fallback
	// used by the browser elsewhere.
	theme.Face = font.Typeface(typography.Families["body"])
	theme.Palette.Fg = colors.text
	theme.Palette.Bg = colors.background
	theme.Palette.ContrastBg = colors.accent
	theme.Palette.ContrastFg = colors.surface
	return theme, colors, nil
}

func ciwiFontCollection() ([]font.FontFace, error) {
	collection := append([]font.FontFace(nil), gofont.Collection()...)
	for _, source := range []struct {
		path   string
		weight font.Weight
	}{
		{path: "assets/GeistMono-Regular.ttf", weight: font.Normal},
		{path: "assets/GeistMono-Medium.ttf", weight: font.Medium},
		{path: "assets/GeistMono-Bold.ttf", weight: font.Bold},
	} {
		payload, err := sharedUI.Read(source.path)
		if err != nil {
			return nil, fmt.Errorf("load native monospace font: %w", err)
		}
		faces, err := opentype.ParseCollection(payload)
		if err != nil {
			return nil, fmt.Errorf("parse native monospace font %q: %w", source.path, err)
		}
		if len(faces) == 0 {
			return nil, fmt.Errorf("parse native monospace font %q: collection is empty", source.path)
		}
		face := faces[0]
		face.Font.Typeface = font.Typeface("Ciwi Mono")
		face.Font.Weight = source.weight
		collection = append(collection, face)
	}
	return collection, nil
}

func semanticTone(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "succeeded", "success", "passed", "complete", "completed", "online":
		return "success"
	case "failed", "failure", "error", "cancelled", "canceled", "offline":
		return "danger"
	case "warning", "queued", "waiting", "pending", "not reached", "stale", "deactivated":
		return "warning"
	case "accent", "running", "leased", "in progress", "active":
		return "accent"
	case "muted":
		return "muted"
	default:
		return "muted"
	}
}

func embeddedImages() (map[string]paint.ImageOp, error) {
	payload, err := sharedUI.Read("assets/ciwi-logo.png")
	if err != nil {
		return nil, err
	}
	decoded, err := png.Decode(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("decode ciwi logo: %w", err)
	}
	logo := paint.NewImageOp(decoded)
	logo.Filter = paint.FilterNearest
	return map[string]paint.ImageOp{"ciwi-logo": logo}, nil
}

func parseColor(value string) (color.NRGBA, error) {
	value = strings.TrimPrefix(value, "#")
	if len(value) != 6 && len(value) != 8 {
		return color.NRGBA{}, fmt.Errorf("invalid color")
	}
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return color.NRGBA{}, err
	}
	if len(value) == 6 {
		return color.NRGBA{R: byte(parsed >> 16), G: byte(parsed >> 8), B: byte(parsed), A: 0xff}, nil
	}
	return color.NRGBA{R: byte(parsed >> 24), G: byte(parsed >> 16), B: byte(parsed >> 8), A: byte(parsed)}, nil
}

func metricsFromTheme(theme uidsl.Theme, typography uidsl.Typography) visualMetrics {
	value := func(name string, fallback float32) float32 {
		raw := strings.TrimSpace(theme.Dimensions[name])
		if raw == "" {
			return fallback
		}
		parsed, err := strconv.ParseFloat(raw, 32)
		if err != nil || parsed < 0 {
			return fallback
		}
		return float32(parsed)
	}
	// Keep the shared theme as the source of truth. A small adapter calibration
	// remains for layout spacing, while type sizes use the exact CSS-equivalent
	// values: once both clients use the same face, scaling type independently
	// makes the native hierarchy look visibly smaller than the browser's.
	const density = float32(0.88)
	dense := func(name string, fallback float32) float32 { return value(name, fallback) * density }
	return visualMetrics{
		spaceSmall:       unit.Dp(dense("small", 8)),
		spaceMedium:      unit.Dp(dense("medium", 16)),
		spaceLarge:       unit.Dp(dense("large", 24)),
		pageWidth:        unit.Dp(value("page", 1150)),
		pageInset:        unit.Dp(value("page-inset", 16)),
		sectionPadding:   unit.Dp(value("section-padding", 14)),
		cardPadding:      unit.Dp(value("card-padding", 16)),
		heroPadding:      unit.Dp(value("hero-padding", 16)),
		surfaceRadius:    unit.Dp(value("surface-radius", 12)),
		controlRadius:    unit.Dp(value("control-radius", 8)),
		controlPaddingX:  unit.Dp(value("control-padding-x", 12)),
		controlPaddingY:  unit.Dp(value("control-padding-y", 8)),
		textBody:         typographySize(typography, "body", 16),
		textControl:      typographySize(typography, "control", 14),
		textCode:         typographySize(typography, "code", 13),
		textBadge:        typographySize(typography, "badge", 12),
		textSubtitle:     typographySize(typography, "subtitle", 16),
		textHeading:      typographySize(typography, "heading", 18),
		textTitle:        typographySize(typography, "title", 28),
		textJobTitle:     typographySize(typography, "job-title", 20),
		imageBrandWidth:  unit.Dp(value("image-brand-width", 110)),
		imageBrandHeight: unit.Dp(value("image-brand-height", 91)),
	}
}

func typographySize(typography uidsl.Typography, role string, fallback float32) unit.Sp {
	if style, ok := typography.Roles[role]; ok && style.Size > 0 {
		return unit.Sp(style.Size)
	}
	return unit.Sp(fallback)
}

func (r *Renderer) spacing(value string) unit.Dp {
	if value == "" {
		return 0
	}
	switch value {
	case "small":
		return r.metrics.spaceSmall
	case "medium":
		return r.metrics.spaceMedium
	case "large":
		return r.metrics.spaceLarge
	case "section-padding":
		return r.metrics.sectionPadding
	}
	if parsed, err := strconv.ParseFloat(value, 32); err == nil {
		return unit.Dp(parsed)
	}
	return 0
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
