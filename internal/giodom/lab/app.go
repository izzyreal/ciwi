//go:build darwin || ios || linux || windows

// Package lab is an offline viability application for the giodom runtime.
package lab

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"runtime"
	"strconv"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/internal/giodom"
)

const (
	memoryWatchdogLimit = 400 << 20
	stressRows          = 10_000
)

var stressRowKeys = func() []giodom.Key {
	keys := make([]giodom.Key, stressRows)
	for index := range keys {
		keys[index] = giodom.Key("row-" + strconv.Itoa(index))
	}
	return keys
}()

type scenario string

const (
	scenarioMain     scenario = "main"
	scenarioJobKeyed scenario = "job-keyed"
	scenarioJobStock scenario = "job-stock"
	scenarioStress   scenario = "stress"
	scenarioSettings scenario = "settings"
)

type lifecycle uint8

const (
	lifecycleLoading lifecycle = iota
	lifecycleReady
	lifecycleError
)

type palette struct {
	page, surface, raised, border color.NRGBA
	text, muted, accent, danger   color.NRGBA
	progress, track, scrim        color.NRGBA
}

var labPalette = palette{
	page:     color.NRGBA{R: 0x0f, G: 0x14, B: 0x19, A: 0xff},
	surface:  color.NRGBA{R: 0x1a, G: 0x23, B: 0x2b, A: 0xff},
	raised:   color.NRGBA{R: 0x22, G: 0x2e, B: 0x38, A: 0xff},
	border:   color.NRGBA{R: 0x58, G: 0x75, B: 0x86, A: 0xff},
	text:     color.NRGBA{R: 0xee, G: 0xf4, B: 0xf6, A: 0xff},
	muted:    color.NRGBA{R: 0xaf, G: 0xbd, B: 0xc5, A: 0xff},
	accent:   color.NRGBA{R: 0x5b, G: 0xe1, B: 0xa8, A: 0xff},
	danger:   color.NRGBA{R: 0xff, G: 0x6b, B: 0x7d, A: 0xff},
	progress: color.NRGBA{R: 0x1f, G: 0xd9, B: 0x9a, A: 0x80},
	track:    color.NRGBA{R: 0x2a, G: 0x3a, B: 0x43, A: 0xff},
	scrim:    color.NRGBA{A: 0x99},
}

type model struct {
	scenario     scenario
	lifecycle    lifecycle
	stress       bool
	modal        bool
	query        string
	frame        uint64
	revision     uint64
	rowRevision  uint64
	rowShift     int
	rowCount     int
	lastMutation time.Time
	invalidate   func()
}

// Run opens the standalone lab window and blocks until it closes.
func Run() error {
	window := new(app.Window)
	window.Option(app.Title("Gio DOM Viability Lab"), app.Size(980, 760))
	theme := material.NewTheme()
	renderer := giodom.NewRuntime(theme, giodom.Options{MaxStateSlots: 4096, MaxGeometryPixels: 1_000_000})
	state := initialModel(window.Invalidate)
	runFor := environmentDuration("GIODOM_LAB_RUN_FOR")
	started := time.Now()
	stopWatchdog := startMemoryWatchdog()
	defer stopWatchdog()

	var operations op.Ops
	lastReport := time.Time{}
	for {
		event := window.Event()
		switch event := event.(type) {
		case app.DestroyEvent:
			return event.Err
		case app.FrameEvent:
			if runFor > 0 && time.Since(started) >= runFor {
				return nil
			}
			gtx := app.NewContext(&operations, event)
			state.advance(gtx.Now)
			renderer.Layout(gtx, state.root(renderer.Stats()))
			if state.stress {
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 60)})
			}
			if lastReport.IsZero() || gtx.Now.Sub(lastReport) >= time.Second {
				reportDiagnostics(state, renderer.Stats())
				lastReport = gtx.Now
			}
			event.Frame(&operations)
		}
	}
}

func initialModel(invalidate func()) *model {
	state := &model{scenario: scenarioMain, lifecycle: lifecycleReady, rowCount: stressRows, invalidate: invalidate}
	switch scenario(os.Getenv("GIODOM_LAB_SCENARIO")) {
	case scenarioMain, scenarioJobKeyed, scenarioJobStock, scenarioStress, scenarioSettings:
		state.scenario = scenario(os.Getenv("GIODOM_LAB_SCENARIO"))
	}
	stress, err := strconv.ParseBool(os.Getenv("GIODOM_LAB_STRESS"))
	if err == nil {
		state.stress = stress
	}
	return state
}

func environmentDuration(name string) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return 0
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		_, _ = fmt.Fprintf(os.Stderr, "giodom-lab: invalid %s=%q\n", name, value)
		return 0
	}
	return duration
}

func (m *model) advance(now time.Time) {
	m.frame++
	if !m.stress || (!m.lastMutation.IsZero() && now.Sub(m.lastMutation) < 100*time.Millisecond) {
		return
	}
	m.lastMutation = now
	m.revision++
	m.rowRevision++
	m.rowShift = (m.rowShift + 17) % stressRows
	if m.rowRevision%11 == 0 {
		m.rowCount = stressRows - 1
	} else {
		m.rowCount = stressRows
	}
	switch m.revision % 3 {
	case 0:
		m.lifecycle = lifecycleLoading
	case 1:
		m.lifecycle = lifecycleReady
	case 2:
		m.lifecycle = lifecycleError
	}
}

func (m *model) root(stats giodom.Stats) giodom.Element {
	header := m.labHeader(stats)
	var content giodom.Element
	switch m.scenario {
	case scenarioJobKeyed:
		content = m.jobPage(false)
	case scenarioJobStock:
		content = m.jobPage(true)
	case scenarioStress:
		content = m.stressPage()
	case scenarioSettings:
		content = m.settingsPage()
	default:
		content = m.mainPage()
	}
	content.Grow = true
	body := giodom.Column("lab-root", 12, header, content)
	body.Flex.Padding = giodom.UniformInsets(12)
	if !m.modal {
		return body
	}
	modal := card("modal", giodom.Column("modal-copy", 12,
		label("modal-title", "Standalone modal", 22, labPalette.text),
		label("modal-message", "Focus, overlay layout, and retained button state remain inside the keyed runtime.", 16, labPalette.muted),
		giodom.Button("close-modal", "Close", true, m.action(func() { m.modal = false })),
	))
	modal = giodom.Constrain("modal-width", giodom.ConstraintProps{MinWidth: 280, MaxWidth: 480}, modal)
	return giodom.Overlay("lab-overlay", giodom.OverlayProps{Scrim: labPalette.scrim}, body, modal)
}

func (m *model) labHeader(stats giodom.Stats) giodom.Element {
	navigation := giodom.Flow("lab-navigation", 8,
		giodom.Button("nav-main", "Main", true, m.setScenario(scenarioMain)),
		giodom.Button("nav-job-keyed", "Job · keyed", true, m.setScenario(scenarioJobKeyed)),
		giodom.Button("nav-job-stock", "Job · stock control", true, m.setScenario(scenarioJobStock)),
		giodom.Button("nav-stress", "10k rows", true, m.setScenario(scenarioStress)),
		giodom.Button("nav-settings", "Settings", true, m.setScenario(scenarioSettings)),
		giodom.Button("toggle-stress", stressLabel(m.stress), true, m.action(func() { m.stress = !m.stress })),
		giodom.Button("show-modal", "Modal", true, m.action(func() { m.modal = true })),
	)
	metrics := fmt.Sprintf("frame=%d  DOM=%d  states=%d  visible=%d  measured=%d  heap=%s  frame-time=%s  errors=%d",
		stats.Frame, stats.Elements, stats.LiveStates, stats.VisibleListItems, stats.MeasuredListItems,
		bytesLabel(stats.HeapAlloc), stats.FrameDuration.Round(time.Microsecond), stats.Errors)
	return card("lab-header", giodom.Column("lab-header-copy", 8,
		label("lab-title", "Gio DOM viability lab", 24, labPalette.text),
		navigation,
		label("lab-metrics", metrics, 13, labPalette.muted),
	))
}

func (m *model) mainPage() giodom.Element {
	hero := progressCard("main-hero", "Responsive CI dashboard", "Generated data only — no ciwi runtime, screens, or server.", true)
	projects := make([]giodom.Element, 0, 4)
	for index := 0; index < 4; index++ {
		id := strconv.Itoa(index)
		projects = append(projects, giodom.Constrain(giodom.Key("project-width-"+id), giodom.ConstraintProps{MinWidth: 250, MaxWidth: 360},
			card(giodom.Key("project-"+id), giodom.Column(giodom.Key("project-copy-"+id), 6,
				label(giodom.Key("project-title-"+id), "Project "+strconv.Itoa(index+1), 19, labPalette.accent),
				label(giodom.Key("project-meta-"+id), "main · 4 pipelines · healthy", 14, labPalette.muted),
			))))
	}
	wide := giodom.Flow("projects-wide", 12, projects...)
	compact := giodom.Column("projects-compact", 12, projects...)
	responsive := giodom.Responsive("projects-responsive", 560, compact, wide)
	open := giodom.Button("open-job", "Open representative job details", true, m.setScenario(scenarioJobKeyed))
	content := []giodom.Element{hero, responsive, open}
	return m.document("main-document", giodom.Keyed(m.revision, content...), false)
}

func (m *model) jobPage(stock bool) giodom.Element {
	children := []giodom.Element{
		progressCard("job-hero", "build / integration-tests", "Status: running · phase 3 of 8", true),
	}
	switch m.lifecycle {
	case lifecycleLoading:
		children = append(children, card("job-loading", giodom.Column("loading-copy", 8,
			label("loading-title", "Loading job details…", 20, labPalette.text),
			giodom.Spacer("loading-space", 0, 120),
		)))
	case lifecycleError:
		children = append(children, card("job-error", giodom.Column("error-copy", 8,
			label("error-title", "Could not load job details", 20, labPalette.danger),
			label("error-detail", "Synthetic lifecycle error", 15, labPalette.muted),
		)))
	case lifecycleReady:
		children = append(children, m.jobProperties(), m.jobOutput())
	}
	return m.document("job-document", giodom.Keyed(m.revision, children...), stock)
}

func (m *model) jobProperties() giodom.Element {
	property := func(key, title, value string) giodom.Element {
		return giodom.Constrain(giodom.Key(key+"-width"), giodom.ConstraintProps{MinWidth: 270, MaxWidth: 420},
			card(giodom.Key(key), giodom.Column(giodom.Key(key+"-copy"), 6,
				label(giodom.Key(key+"-title"), title, 18, labPalette.text),
				label(giodom.Key(key+"-value"), value, 15, labPalette.muted),
			)))
	}
	wide := giodom.Flow("properties-flow", 12,
		property("properties", "Job properties", "agent-bhakti · ordinary run · v0.2.36"),
		property("cache", "Cache statistics", "7 restored · 2 written · 91% hit rate"),
	)
	compact := giodom.Column("properties-column", 12,
		property("properties", "Job properties", "agent-bhakti · ordinary run · v0.2.36"),
		property("cache", "Cache statistics", "7 restored · 2 written · 91% hit rate"),
	)
	return giodom.Responsive("job-properties-responsive", 560, compact, wide)
}

func (m *model) jobOutput() giodom.Element {
	rows := m.outputRows(240)
	output := giodom.VirtualList("job-output-list", giodom.ListProps{
		Axis: layout.Vertical, Gap: 4, Viewport: 300, Estimate: 34, Overscan: 3, MaxMeasured: 512,
		SemanticLabel: "Synthetic job output",
	}, rows)
	search := giodom.Editor("output-search", giodom.EditorProps{
		Value: m.query, Placeholder: "Search output", SingleLine: true,
		OnChange: func(value string) { m.query = value; m.invalidate() },
	})
	return card("job-output", giodom.Column("job-output-copy", 8,
		label("output-title", "Output / Error", 20, labPalette.text), search, output,
	))
}

func (m *model) stressPage() giodom.Element {
	rows := m.outputRows(m.rowCount)
	return giodom.VirtualList("stress-list", giodom.ListProps{
		Axis: layout.Vertical, Gap: 5, Estimate: 42, Overscan: 4, MaxMeasured: 2048,
		SemanticLabel: "Ten thousand keyed rows",
	}, rows)
}

func (m *model) settingsPage() giodom.Element {
	hero := card("settings-hero", giodom.Column("settings-hero-copy", 6,
		label("settings-title", "Global settings", 25, labPalette.text),
		label("settings-meta", "Responsive controls and lifecycle content", 15, labPalette.muted),
	))
	sections := []giodom.Element{hero}
	if m.lifecycle == lifecycleLoading {
		sections = append(sections, card("settings-loading", giodom.Column("settings-loading-copy", 8,
			label("settings-loading-title", "Loading settings…", 19, labPalette.text),
			giodom.Spacer("settings-loading-space", 0, 140),
		)))
	} else if m.lifecycle == lifecycleError {
		sections = append(sections, card("settings-error", label("settings-error-copy", "Settings unavailable", 19, labPalette.danger)))
	} else {
		sections = append(sections,
			card("appearance", giodom.Column("appearance-copy", 8,
				label("appearance-title", "Appearance", 20, labPalette.text),
				label("appearance-detail", "Theme: standalone dark", 15, labPalette.muted),
			)),
			card("connection", giodom.Column("connection-copy", 8,
				label("connection-title", "Native connection", 20, labPalette.text),
				giodom.Editor("connection-address", giodom.EditorProps{Value: "tcp://example:8113", SingleLine: true}),
			)),
		)
	}
	return m.document("settings-document", giodom.Keyed(m.revision, sections...), false)
}

func (m *model) document(key giodom.Key, children giodom.Children, stock bool) giodom.Element {
	props := giodom.ListProps{Axis: layout.Vertical, Gap: 14, Estimate: 180, Overscan: 2, MaxMeasured: 256, SemanticLabel: string(key)}
	if stock {
		return giodom.StockList(key, props, children)
	}
	return giodom.VirtualList(key, props, children)
}

func (m *model) outputRows(count int) giodom.Children {
	revision, shift := m.rowRevision, m.rowShift
	return giodom.Lazy(revision, count, func(index int) giodom.Key {
		return stressRowKeys[(index+shift)%stressRows]
	}, func(index int) giodom.Element {
		rowID := (index + shift) % stressRows
		text := fmt.Sprintf("%05d  [integration] deterministic output line for keyed viewport", rowID)
		row := giodom.Surface(giodom.Key("row-surface-"+strconv.Itoa(rowID)), giodom.SurfaceProps{
			Fill: labPalette.raised, Border: labPalette.border, BorderWidth: 1, Radius: 7,
			Padding: giodom.UniformInsets(8),
		}, label(giodom.Key("row-text-"+strconv.Itoa(rowID)), text, 14, labPalette.text))
		return giodom.Constrain(giodom.Key("row-height-"+strconv.Itoa(rowID)), giodom.ConstraintProps{MinHeight: 38}, row)
	})
}

func card(key giodom.Key, child giodom.Element) giodom.Element {
	return giodom.Surface(key, giodom.SurfaceProps{
		Fill: labPalette.surface, Border: labPalette.border, BorderWidth: 1, Radius: 14,
		Padding: giodom.UniformInsets(16),
	}, child)
}

func progressCard(key giodom.Key, title, detail string, active bool) giodom.Element {
	content := giodom.Column(key+"-copy", 7,
		label(key+"-title", title, 25, labPalette.text),
		label(key+"-detail", detail, 16, labPalette.muted),
	)
	content.Flex.Padding = giodom.UniformInsets(15)
	progress := giodom.Progress(key+"-progress", giodom.ProgressProps{
		Fraction: .42, Indeterminate: active, Color: labPalette.progress, Track: labPalette.track, Radius: 14,
	}, content)
	return giodom.Surface(key, giodom.SurfaceProps{
		Fill: labPalette.track, Border: labPalette.border, BorderWidth: 1, Radius: 14,
		Padding: giodom.UniformInsets(1),
	}, progress)
}

func label(key giodom.Key, value string, size unit.Sp, ink color.NRGBA) giodom.Element {
	return giodom.Text(key, value, size, ink)
}

func (m *model) action(action func()) func() {
	return func() {
		action()
		m.revision++
		m.invalidate()
	}
}

func (m *model) setScenario(next scenario) func() {
	return m.action(func() {
		m.scenario = next
		m.lifecycle = lifecycleReady
	})
}

func stressLabel(active bool) string {
	if active {
		return "Stop churn"
	}
	return "Start churn"
}

func bytesLabel(value uint64) string {
	return fmt.Sprintf("%.1f MiB", float64(value)/(1024*1024))
}

func reportDiagnostics(model *model, stats giodom.Stats) {
	payload := struct {
		Scenario string       `json:"scenario"`
		Stress   bool         `json:"stress"`
		Stats    giodom.Stats `json:"stats"`
	}{Scenario: string(model.scenario), Stress: model.stress, Stats: stats}
	encoded, err := json.Marshal(payload)
	if err == nil {
		_, _ = fmt.Fprintln(os.Stderr, string(encoded))
	}
}

func startMemoryWatchdog() func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var memory runtime.MemStats
				runtime.ReadMemStats(&memory)
				if memory.HeapAlloc >= memoryWatchdogLimit {
					_, _ = fmt.Fprintf(os.Stderr, "giodom-lab watchdog: heap reached %s\n", bytesLabel(memory.HeapAlloc))
					os.Exit(86)
				}
			}
		}
	}()
	return func() { close(stop) }
}
