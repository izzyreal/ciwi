// Package giodom provides a small keyed declarative runtime for Gio.
//
// It deliberately has no dependency on ciwi screens, presentation models, or
// application state. Applications build an immutable Element tree for each
// frame; Runtime owns only Gio widget and viewport state keyed by identity.
package giodom

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
)

// Key identifies an element among its siblings. Dynamic children must have
// non-empty, unique keys.
type Key string

// Kind identifies the layout and state behavior of an Element.
type Kind uint8

const (
	KindInvalid Kind = iota
	KindFlex
	KindSurface
	KindText
	KindButton
	KindEditor
	KindSpacer
	KindResponsive
	KindProgress
	KindVirtualList
	KindStockList
	KindOverlay
	KindConstrain
	KindInset
	KindAlign
	KindNative
)

func (k Kind) String() string {
	switch k {
	case KindFlex:
		return "flex"
	case KindSurface:
		return "surface"
	case KindText:
		return "text"
	case KindButton:
		return "button"
	case KindEditor:
		return "editor"
	case KindSpacer:
		return "spacer"
	case KindResponsive:
		return "responsive"
	case KindProgress:
		return "progress"
	case KindVirtualList:
		return "virtual-list"
	case KindStockList:
		return "stock-list"
	case KindOverlay:
		return "overlay"
	case KindConstrain:
		return "constrain"
	case KindInset:
		return "inset"
	case KindAlign:
		return "align"
	case KindNative:
		return "native"
	default:
		return "invalid"
	}
}

// Insets describes independent edge padding.
type Insets struct {
	Top, Right, Bottom, Left unit.Dp
}

// UniformInsets applies one value to all edges.
func UniformInsets(value unit.Dp) Insets {
	return Insets{Top: value, Right: value, Bottom: value, Left: value}
}

// FlexProps configures rows, columns, and wrapping flows.
type FlexProps struct {
	Axis      layout.Axis
	Alignment layout.Alignment
	Spacing   layout.Spacing
	Gap       unit.Dp
	Padding   Insets
	Wrap      bool
	Stretch   bool
}

// SurfaceProps configures a filled, optionally bordered rounded surface.
type SurfaceProps struct {
	Fill        color.NRGBA
	Border      color.NRGBA
	BorderWidth unit.Dp
	Radius      unit.Dp
	Padding     Insets
	// PaintBackground optionally paints the surface interior. Runtime clips the
	// callback to the safe rounded interior and rejects invalid geometry before
	// invoking it.
	PaintBackground func(layout.Context, image.Point)
}

// TextProps configures selectable presentation text.
type TextProps struct {
	Value           string
	Size            unit.Sp
	Color           color.NRGBA
	Font            font.Font
	LineHeightScale float32
	MaxLines        int
	Selectable      bool
}

// ButtonProps configures one native material button.
type ButtonProps struct {
	Label       string
	Description string
	Enabled     bool
	OnClick     func()
	Fill        color.NRGBA
	Border      color.NRGBA
	BorderWidth unit.Dp
	Radius      unit.Dp
	Padding     Insets
	MinHeight   unit.Dp
}

// EditorProps configures a controlled editor. Value remains the source of
// truth whenever the editor is not focused.
type EditorProps struct {
	Value       string
	Placeholder string
	SingleLine  bool
	OnChange    func(string)
}

// SpacerProps configures fixed or minimum empty space.
type SpacerProps struct {
	Width, Height unit.Dp
}

// ResponsiveProps selects one subtree from the current width constraint.
type ResponsiveProps struct {
	Breakpoint unit.Dp
	Compact    *Element
	Wide       *Element
}

// ProgressMode describes the visual state of a progress underlay.
type ProgressMode uint8

const (
	ProgressDeterminate ProgressMode = iota
	ProgressIndeterminate
	ProgressOverrun
	ProgressComplete
)

// ProgressProps configures an animated progress underlay around one child.
type ProgressProps struct {
	Mode     ProgressMode
	Fraction float32
	Animate  bool
	Color    color.NRGBA
	Track    color.NRGBA
	Radius   unit.Dp
	Phase    time.Duration
}

// ListProps configures stock and keyed scrolling lists.
type ListProps struct {
	Axis           layout.Axis
	Gap            unit.Dp
	Viewport       unit.Dp
	ShrinkCross    bool
	Estimate       unit.Dp
	Overscan       int
	MaxMeasured    int
	ScrollToEnd    bool
	ScrollTo       Key
	ScrollRevision uint64
	SemanticLabel  string
}

// OverlayProps configures a body with an optional centered modal child.
type OverlayProps struct {
	Scrim     color.NRGBA
	Alignment layout.Direction
	Align     bool
}

// ConstraintProps applies optional minimum and maximum dimensions.
type ConstraintProps struct {
	MinWidth, MaxWidth, MinHeight, MaxHeight unit.Dp
}

// AlignProps positions one child within the available constraints.
type AlignProps struct {
	Direction layout.Direction
}

// NativeProps is the deliberately narrow escape hatch for renderer-specific
// leaves such as platform icons, menus, and canvases. Runtime owns the leaf's
// optional state under the element identity, so applications do not need
// unbounded widget maps outside the DOM.
type NativeProps struct {
	NewState func() any
	Layout   func(layout.Context, any) layout.Dimensions
}

// Element is an immutable description produced by application code.
type Element struct {
	Kind       Kind
	Key        Key
	Grow       bool
	FitContent bool
	FlexWeight float32
	Flex       FlexProps
	Surface    SurfaceProps
	Text       TextProps
	Button     ButtonProps
	Editor     EditorProps
	Spacer     SpacerProps
	Responsive ResponsiveProps
	Progress   ProgressProps
	List       ListProps
	Overlay    OverlayProps
	Constraint ConstraintProps
	Inset      Insets
	Align      AlignProps
	Native     NativeProps
	Children   Children
}

// Children provides static or lazy child descriptions. Revision must change
// whenever a dynamic collection's keys or ordering changes.
type Children interface {
	Len() int
	KeyAt(index int) Key
	At(index int) Element
	Dynamic() bool
	Revision() uint64
}

type sliceChildren struct {
	elements []Element
	dynamic  bool
	revision uint64
}

func (c sliceChildren) Len() int             { return len(c.elements) }
func (c sliceChildren) KeyAt(index int) Key  { return c.elements[index].Key }
func (c sliceChildren) At(index int) Element { return c.elements[index] }
func (c sliceChildren) Dynamic() bool        { return c.dynamic }
func (c sliceChildren) Revision() uint64     { return c.revision }

// Static describes a structurally fixed child sequence. Unkeyed children use
// their position as identity.
func Static(elements ...Element) Children {
	return sliceChildren{elements: append([]Element(nil), elements...)}
}

// Keyed describes a dynamic child sequence whose elements carry explicit keys.
func Keyed(revision uint64, elements ...Element) Children {
	return sliceChildren{elements: append([]Element(nil), elements...), dynamic: true, revision: revision}
}

type lazyChildren struct {
	count    int
	revision uint64
	key      func(int) Key
	build    func(int) Element
}

func (c lazyChildren) Len() int             { return c.count }
func (c lazyChildren) KeyAt(index int) Key  { return c.key(index) }
func (c lazyChildren) At(index int) Element { return c.build(index) }
func (c lazyChildren) Dynamic() bool        { return true }
func (c lazyChildren) Revision() uint64     { return c.revision }

// Lazy describes a virtualized keyed collection without materializing every
// Element. A negative count is normalized to zero.
func Lazy(revision uint64, count int, key func(int) Key, build func(int) Element) Children {
	if count < 0 {
		count = 0
	}
	return lazyChildren{count: count, revision: revision, key: key, build: build}
}

// Column constructs a vertical flex container.
func Column(key Key, gap unit.Dp, children ...Element) Element {
	return Element{Kind: KindFlex, Key: key, Flex: FlexProps{Axis: layout.Vertical, Gap: gap}, Children: Static(children...)}
}

// Row constructs a horizontal flex container.
func Row(key Key, gap unit.Dp, children ...Element) Element {
	return Element{Kind: KindFlex, Key: key, Flex: FlexProps{Axis: layout.Horizontal, Gap: gap}, Children: Static(children...)}
}

// Flow constructs a horizontal wrapping flow container.
func Flow(key Key, gap unit.Dp, children ...Element) Element {
	return Element{Kind: KindFlex, Key: key, Flex: FlexProps{Axis: layout.Horizontal, Gap: gap, Wrap: true}, Children: Static(children...)}
}

// Surface constructs a rounded surface around one child.
func Surface(key Key, props SurfaceProps, child Element) Element {
	return Element{Kind: KindSurface, Key: key, Surface: props, Children: Static(child)}
}

// Text constructs selectable text.
func Text(key Key, value string, size unit.Sp, ink color.NRGBA) Element {
	return Element{Kind: KindText, Key: key, Text: TextProps{Value: value, Size: size, Color: ink, Selectable: true}}
}

// Button constructs a native material button.
func Button(key Key, label string, enabled bool, onClick func()) Element {
	return Element{Kind: KindButton, Key: key, Button: ButtonProps{Label: label, Enabled: enabled, OnClick: onClick}}
}

// Control constructs a styled clickable container around one child.
func Control(key Key, props ButtonProps, child Element) Element {
	return Element{Kind: KindButton, Key: key, Button: props, Children: Static(child)}
}

// Editor constructs a controlled native editor.
func Editor(key Key, props EditorProps) Element {
	return Element{Kind: KindEditor, Key: key, Editor: props}
}

// Spacer constructs fixed empty space.
func Spacer(key Key, width, height unit.Dp) Element {
	return Element{Kind: KindSpacer, Key: key, Spacer: SpacerProps{Width: width, Height: height}}
}

// Responsive selects compact at or below breakpoint and wide above it.
func Responsive(key Key, breakpoint unit.Dp, compact, wide Element) Element {
	return Element{Kind: KindResponsive, Key: key, Responsive: ResponsiveProps{Breakpoint: breakpoint, Compact: &compact, Wide: &wide}}
}

// Progress constructs an animated progress underlay around one child.
func Progress(key Key, props ProgressProps, child Element) Element {
	return Element{Kind: KindProgress, Key: key, Progress: props, Children: Static(child)}
}

// VirtualList constructs the custom keyed viewport under evaluation.
func VirtualList(key Key, props ListProps, children Children) Element {
	return Element{Kind: KindVirtualList, Key: key, List: props, Children: children}
}

// StockList constructs the control implementation backed by layout.List.
func StockList(key Key, props ListProps, children Children) Element {
	return Element{Kind: KindStockList, Key: key, List: props, Children: children}
}

// Overlay lays modal over body. Omitting modal leaves only body.
func Overlay(key Key, props OverlayProps, body Element, modal ...Element) Element {
	children := []Element{body}
	children = append(children, modal...)
	return Element{Kind: KindOverlay, Key: key, Overlay: props, Children: Static(children...)}
}

// Constrain applies explicit bounds around one child.
func Constrain(key Key, props ConstraintProps, child Element) Element {
	return Element{Kind: KindConstrain, Key: key, Constraint: props, Children: Static(child)}
}

// Inset applies independent edge padding around one child.
func Inset(key Key, insets Insets, child Element) Element {
	return Element{Kind: KindInset, Key: key, Inset: insets, Children: Static(child)}
}

// Align positions one child within the available constraints.
func Align(key Key, direction layout.Direction, child Element) Element {
	return Element{Kind: KindAlign, Key: key, Align: AlignProps{Direction: direction}, Children: Static(child)}
}

// Native constructs one runtime-owned renderer-specific leaf.
func Native(key Key, props NativeProps) Element {
	return Element{Kind: KindNative, Key: key, Native: props}
}
