package giodom

import (
	"fmt"
	"image"
	"net/url"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget/material"
)

const (
	defaultMaxStateSlots     = 4096
	defaultMaxGeometryPixels = 1_000_000
)

// Options configures hard runtime resource limits.
type Options struct {
	MaxStateSlots     int
	MaxGeometryPixels int
}

// Stats is a bounded diagnostic snapshot for the most recently laid-out frame.
type Stats struct {
	Frame             uint64
	Elements          int
	MountedStates     int
	UnmountedStates   int
	LiveStates        int
	VisibleListItems  int
	MeasuredListItems int
	GeometryRejects   int
	Errors            int
	LastError         string
	FrameDuration     time.Duration
}

type stateEntry struct {
	kind     Kind
	slot     string
	value    any
	lastSeen uint64
}

type keyValidationState struct {
	revision uint64
	count    int
	valid    bool
	ready    bool
}

// Runtime reconciles immutable elements with bounded Gio widget state.
type Runtime struct {
	theme               *material.Theme
	states              map[string]*stateEntry
	frame               uint64
	maxStateSlots       int
	maxGeometryPixels   int
	stats               Stats
	animationSource     input.Source
	nestedScrollClaimed bool
	passThroughScroll   *passThroughScrollContext
	controlClicks       uint64
}

// NewRuntime constructs an independent DOM runtime.
func NewRuntime(theme *material.Theme, options Options) *Runtime {
	if theme == nil {
		theme = material.NewTheme()
	}
	if options.MaxStateSlots <= 0 {
		options.MaxStateSlots = defaultMaxStateSlots
	}
	if options.MaxGeometryPixels <= 0 {
		options.MaxGeometryPixels = defaultMaxGeometryPixels
	}
	return &Runtime{
		theme: theme, states: make(map[string]*stateEntry),
		maxStateSlots: options.MaxStateSlots, maxGeometryPixels: options.MaxGeometryPixels,
	}
}

// Layout reconciles and lays out one immutable frame.
func (r *Runtime) Layout(gtx layout.Context, root Element) layout.Dimensions {
	started := time.Now()
	r.frame++
	r.animationSource = gtx.Source
	r.nestedScrollClaimed = false
	r.stats = Stats{Frame: r.frame}
	identity := "root"
	if root.Key != "" {
		identity += "/key:" + identityPart(root.Key)
	}
	dimensions := r.layoutElement(gtx, root, identity)
	r.sweepStates()
	r.stats.LiveStates = len(r.states)
	r.stats.FrameDuration = time.Since(started)
	return dimensions
}

// Reset drops all retained Gio state.
func (r *Runtime) Reset() {
	r.states = make(map[string]*stateEntry)
	r.frame = 0
	r.controlClicks = 0
	r.stats = Stats{}
}

// Stats returns the most recent diagnostic snapshot.
func (r *Runtime) Stats() Stats { return r.stats }

func (r *Runtime) requestAnimationFrame(gtx layout.Context) {
	if r.animationSource.Enabled() {
		r.animationSource.Execute(op.InvalidateCmd{})
		return
	}
	gtx.Execute(op.InvalidateCmd{})
}

func (r *Runtime) layoutElement(gtx layout.Context, element Element, identity string) layout.Dimensions {
	r.stats.Elements++
	if element.FitContent {
		gtx.Constraints.Min = image.Point{}
	}
	switch element.Kind {
	case KindFlex:
		return r.layoutFlex(gtx, element, identity)
	case KindSurface:
		return r.layoutSurface(gtx, element, identity)
	case KindText:
		return r.layoutText(gtx, element, identity)
	case KindButton:
		return r.layoutButton(gtx, element, identity)
	case KindEditor:
		return r.layoutEditor(gtx, element, identity)
	case KindSpacer:
		return r.layoutSpacer(gtx, element)
	case KindResponsive:
		return r.layoutResponsive(gtx, element, identity)
	case KindProgress:
		return r.layoutProgress(gtx, element, identity)
	case KindVirtualList:
		return r.layoutVirtualList(gtx, element, identity)
	case KindStockList:
		return r.layoutStockList(gtx, element, identity)
	case KindOverlay:
		return r.layoutOverlay(gtx, element, identity)
	case KindConstrain:
		return r.layoutConstrain(gtx, element, identity)
	case KindInset:
		return applyInsets(element.Inset, gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutOnlyChild(gtx, element, identity)
		})
	case KindAlign:
		return element.Align.Direction.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutOnlyChild(gtx, element, identity)
		})
	case KindNative:
		return r.layoutNative(gtx, element, identity)
	case KindPassThroughScrollRegion:
		return r.layoutPassThroughScrollRegion(gtx, element, identity)
	default:
		r.recordError(fmt.Errorf("%s: invalid element kind", identity))
		return layout.Dimensions{}
	}
}

func (r *Runtime) layoutNative(gtx layout.Context, element Element, identity string) layout.Dimensions {
	if element.Native.Layout == nil {
		r.recordError(fmt.Errorf("%s: native layout is missing", identity))
		return layout.Dimensions{}
	}
	var state any
	if element.Native.NewState != nil {
		state = r.useState(identity, "native", KindNative, element.Native.NewState)
	}
	interactionRevision := uint64(0)
	if element.Native.InteractionRevision != nil {
		interactionRevision = element.Native.InteractionRevision()
	}
	dimensions := element.Native.Layout(gtx, state)
	if element.Native.InteractionRevision != nil && element.Native.InteractionRevision() != interactionRevision {
		r.controlClicks++
	}
	if r.rejectGeometry(identity, dimensions.Size.X, dimensions.Size.Y) {
		return layout.Dimensions{}
	}
	return dimensions
}

func (r *Runtime) useState(identity, slot string, kind Kind, factory func() any) any {
	key := identity + "/state:" + slot
	if entry := r.states[key]; entry != nil {
		if entry.kind != kind || entry.slot != slot {
			r.recordError(fmt.Errorf("%s: state kind changed from %s to %s", identity, entry.kind, kind))
			delete(r.states, key)
		} else {
			entry.lastSeen = r.frame
			return entry.value
		}
	}
	if len(r.states) >= r.maxStateSlots {
		r.evictOldestState()
	}
	entry := &stateEntry{kind: kind, slot: slot, value: factory(), lastSeen: r.frame}
	r.states[key] = entry
	r.stats.MountedStates++
	return entry.value
}

func (r *Runtime) evictOldestState() {
	var oldestKey string
	var oldestFrame uint64
	first := true
	for key, entry := range r.states {
		if first || entry.lastSeen < oldestFrame {
			oldestKey, oldestFrame, first = key, entry.lastSeen, false
		}
	}
	if !first {
		delete(r.states, oldestKey)
		r.stats.UnmountedStates++
	}
}

func (r *Runtime) sweepStates() {
	for key, entry := range r.states {
		if entry.lastSeen != r.frame {
			delete(r.states, key)
			r.stats.UnmountedStates++
		}
	}
}

func (r *Runtime) childIdentity(parent string, children Children, index int) (string, bool) {
	key := children.KeyAt(index)
	if children.Dynamic() {
		if key == "" {
			r.recordError(fmt.Errorf("%s: dynamic child %d has an empty key", parent, index))
			return parent + fmt.Sprintf("/invalid:%d", index), false
		}
		return parent + "/key:" + identityPart(key), true
	}
	if key != "" {
		return parent + "/key:" + identityPart(key), true
	}
	return parent + fmt.Sprintf("/index:%d", index), true
}

func (r *Runtime) validateDynamicKeys(parent string, children Children) bool {
	if children == nil || !children.Dynamic() {
		return true
	}
	validation := r.useState(parent, "key-validation", KindInvalid, func() any { return new(keyValidationState) }).(*keyValidationState)
	if validation.ready && validation.revision == children.Revision() && validation.count == children.Len() {
		return validation.valid
	}
	seen := make(map[Key]struct{}, children.Len())
	valid := true
	for index := 0; index < children.Len(); index++ {
		key := children.KeyAt(index)
		if key == "" {
			r.recordError(fmt.Errorf("%s: dynamic child %d has an empty key", parent, index))
			valid = false
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			r.recordError(fmt.Errorf("%s: duplicate dynamic key %q", parent, key))
			valid = false
		}
		seen[key] = struct{}{}
	}
	validation.revision = children.Revision()
	validation.count = children.Len()
	validation.valid = valid
	validation.ready = true
	return valid
}

func (r *Runtime) recordError(err error) {
	if err == nil {
		return
	}
	r.stats.Errors++
	r.stats.LastError = err.Error()
}

func (r *Runtime) rejectGeometry(identity string, values ...int) bool {
	for _, value := range values {
		if value < 0 || value > r.maxGeometryPixels {
			r.stats.GeometryRejects++
			r.recordError(fmt.Errorf("%s: geometry %d is outside 0..%d", identity, value, r.maxGeometryPixels))
			return true
		}
	}
	return false
}

func identityPart(key Key) string {
	return url.PathEscape(string(key))
}
