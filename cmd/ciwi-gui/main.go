package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/internal/server/grpcapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

type connectionPhase string

const (
	phaseDisconnected connectionPhase = "disconnected"
	phaseConnecting   connectionPhase = "connecting"
	phaseConnected    connectionPhase = "connected"
	phaseReconnecting connectionPhase = "reconnecting"
)

type watchUpdate struct {
	phase      connectionPhase
	statusText string
	errText    string
	actionText string
	event      *grpcapi.WatchStateEvent
}

type snapshot struct {
	streamID      string
	seq           int64
	sentUTC       string
	serverName    string
	serverVer     string
	hostname      string
	projectCount  int
	pipelineCount int
	agentCount    int
	queuedCount   int
	historyCount  int
	queuedGroups  int
	historyGroups int
	agentIDs      []string
}

type pipelineView struct {
	dbID       int64
	project    string
	pipelineID string
	trigger    string
	sourceRepo string
	sourceRef  string
	dependsOn  []string
}

type projectView struct {
	name      string
	repoURL   string
	configRef string
	pipelines []pipelineView
}

type pipelineButtons struct {
	run    widget.Clickable
	dryRun widget.Clickable
}

type guiApp struct {
	theme *material.Theme
	ops   op.Ops

	addrEditor widget.Editor
	connectBtn widget.Clickable
	disconnBtn widget.Clickable

	window *app.Window

	watchMu     sync.Mutex
	watchCancel context.CancelFunc
	watching    bool

	updates chan watchUpdate

	phase      connectionPhase
	statusText string
	lastError  string
	lastAction string
	snap       snapshot
	projects   []projectView

	projectList     widget.List
	pipelineActions map[string]*pipelineButtons
}

func main() {
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("ciwi-gui"),
			app.Size(unit.Dp(980), unit.Dp(680)),
		)
		if err := run(w); err != nil {
			log.Printf("ciwi-gui: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window) error {
	model := &guiApp{
		theme:           material.NewTheme(),
		updates:         make(chan watchUpdate, 256),
		window:          w,
		phase:           phaseDisconnected,
		statusText:      "Disconnected",
		lastAction:      "none",
		pipelineActions: map[string]*pipelineButtons{},
	}
	model.projectList.List.Axis = layout.Vertical
	model.addrEditor.SingleLine = true
	model.addrEditor.Submit = true
	model.addrEditor.SetText(defaultServerAddr())
	model.startWatch()

	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			model.stopWatch("Disconnected")
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&model.ops, e)
			model.processUpdates()
			model.processActions(gtx)
			model.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func defaultServerAddr() string {
	if v := strings.TrimSpace(os.Getenv("CIWI_GUI_SERVER_ADDR")); v != "" {
		return normalizeServerAddr(v)
	}
	return "127.0.0.1:8113"
}

func normalizeServerAddr(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err == nil && strings.TrimSpace(u.Host) != "" {
			return strings.TrimSpace(u.Host)
		}
	}
	return raw
}

func (m *guiApp) processActions(gtx C) {
	for m.connectBtn.Clicked(gtx) {
		m.startWatch()
	}
	for m.disconnBtn.Clicked(gtx) {
		m.stopWatch("Disconnected")
	}
	for _, project := range m.projects {
		for _, pipeline := range project.pipelines {
			btns := m.pipelineButtonSet(pipeline.dbID)
			for btns.run.Clicked(gtx) {
				m.triggerPipelineRun(pipeline, false)
			}
			for btns.dryRun.Clicked(gtx) {
				m.triggerPipelineRun(pipeline, true)
			}
		}
	}
}

func (m *guiApp) startWatch() {
	addr := normalizeServerAddr(m.addrEditor.Text())
	if addr == "" {
		m.phase = phaseDisconnected
		m.statusText = "Enter a gRPC server address"
		m.lastError = "empty address"
		return
	}
	m.addrEditor.SetText(addr)

	m.watchMu.Lock()
	if m.watching {
		m.watchMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.watchCancel = cancel
	m.watching = true
	m.watchMu.Unlock()

	m.phase = phaseConnecting
	m.statusText = "Connecting"
	m.lastError = ""
	if m.window != nil {
		m.window.Invalidate()
	}

	go m.watchLoop(ctx, addr)
}

func (m *guiApp) stopWatch(reason string) {
	m.watchMu.Lock()
	cancel := m.watchCancel
	m.watchCancel = nil
	m.watching = false
	m.watchMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if strings.TrimSpace(reason) != "" {
		m.phase = phaseDisconnected
		m.statusText = reason
	}
	if m.window != nil {
		m.window.Invalidate()
	}
}

func (m *guiApp) watchLoop(ctx context.Context, addr string) {
	defer func() {
		m.enqueueUpdate(watchUpdate{phase: phaseDisconnected, statusText: "Disconnected"})
		m.watchMu.Lock()
		m.watchCancel = nil
		m.watching = false
		m.watchMu.Unlock()
	}()

	backoff := time.Second
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		attempt++
		phase := phaseConnecting
		statusText := fmt.Sprintf("Connecting to %s", addr)
		if attempt > 1 {
			phase = phaseReconnecting
			statusText = fmt.Sprintf("Reconnecting to %s", addr)
		}
		m.enqueueUpdate(watchUpdate{phase: phase, statusText: statusText})

		dialCtx, dialCancel := context.WithTimeout(ctx, 6*time.Second)
		conn, err := grpc.DialContext(dialCtx, addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		dialCancel()
		if err != nil {
			m.enqueueUpdate(watchUpdate{phase: phaseReconnecting, statusText: "Connection failed", errText: err.Error()})
			if !sleepWithContext(ctx, backoff) {
				return
			}
			if backoff < 10*time.Second {
				backoff *= 2
			}
			continue
		}

		client := grpcapi.NewCiwiServiceClient(conn)
		stream, err := client.WatchState(ctx, &grpcapi.WatchStateRequest{
			IntervalMs:         1000,
			IncludeProjects:    true,
			IncludeAgents:      true,
			IncludeJobsSummary: true,
			JobsLimit:          200,
		})
		if err != nil {
			_ = conn.Close()
			m.enqueueUpdate(watchUpdate{phase: phaseReconnecting, statusText: "Stream start failed", errText: err.Error()})
			if !sleepWithContext(ctx, backoff) {
				return
			}
			if backoff < 10*time.Second {
				backoff *= 2
			}
			continue
		}

		m.enqueueUpdate(watchUpdate{phase: phaseConnected, statusText: fmt.Sprintf("Connected to %s", addr)})
		backoff = time.Second

		for {
			evt, err := stream.Recv()
			if err != nil {
				_ = conn.Close()
				if ctx.Err() != nil {
					return
				}
				m.enqueueUpdate(watchUpdate{phase: phaseReconnecting, statusText: "Stream interrupted", errText: err.Error()})
				break
			}
			m.enqueueUpdate(watchUpdate{event: evt})
		}
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (m *guiApp) enqueueUpdate(u watchUpdate) {
	select {
	case m.updates <- u:
	default:
		select {
		case <-m.updates:
		default:
		}
		select {
		case m.updates <- u:
		default:
		}
	}
	if m.window != nil {
		m.window.Invalidate()
	}
}

func (m *guiApp) processUpdates() {
	for {
		select {
		case u := <-m.updates:
			if u.phase != "" {
				m.phase = u.phase
			}
			if strings.TrimSpace(u.statusText) != "" {
				m.statusText = u.statusText
			}
			if strings.TrimSpace(u.errText) != "" {
				m.lastError = u.errText
			}
			if u.event != nil {
				m.snap = snapshotFromEvent(u.event)
				m.projects = projectsFromEvent(u.event)
			}
			if strings.TrimSpace(u.actionText) != "" {
				m.lastAction = u.actionText
			}
		default:
			return
		}
	}
}

func (m *guiApp) pipelineButtonSet(dbID int64) *pipelineButtons {
	key := strconv.FormatInt(dbID, 10)
	if existing, ok := m.pipelineActions[key]; ok && existing != nil {
		return existing
	}
	created := &pipelineButtons{}
	m.pipelineActions[key] = created
	return created
}

func (m *guiApp) triggerPipelineRun(p pipelineView, dryRun bool) {
	addr := normalizeServerAddr(m.addrEditor.Text())
	if addr == "" {
		m.lastAction = "run request rejected: empty gRPC address"
		if m.window != nil {
			m.window.Invalidate()
		}
		return
	}
	mode := "run"
	if dryRun {
		mode = "dry run"
	}
	m.lastAction = fmt.Sprintf("Submitting %s for %s / %s", mode, p.project, p.pipelineID)
	if m.window != nil {
		m.window.Invalidate()
	}
	go m.runPipelineRPC(addr, p, dryRun)
}

func (m *guiApp) runPipelineRPC(addr string, p pipelineView, dryRun bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		m.enqueueUpdate(watchUpdate{actionText: fmt.Sprintf("Run failed for %s / %s: %v", p.project, p.pipelineID, err)})
		return
	}
	defer func() { _ = conn.Close() }()

	client := grpcapi.NewCiwiServiceClient(conn)
	resp, err := client.RunPipeline(ctx, &grpcapi.RunPipelineRequest{
		PipelineId: strconv.FormatInt(p.dbID, 10),
		DryRun:     dryRun,
	})
	if err != nil {
		m.enqueueUpdate(watchUpdate{actionText: fmt.Sprintf("Run failed for %s / %s: %v", p.project, p.pipelineID, err)})
		return
	}

	mode := "Run"
	if dryRun {
		mode = "Dry run"
	}
	m.enqueueUpdate(watchUpdate{
		actionText: fmt.Sprintf("%s queued for %s / %s: enqueued=%d jobs=%d",
			mode, p.project, p.pipelineID, resp.GetEnqueued(), len(resp.GetJobIds())),
	})
}

func snapshotFromEvent(evt *grpcapi.WatchStateEvent) snapshot {
	if evt == nil {
		return snapshot{}
	}
	s := snapshot{
		streamID: evt.GetStreamId(),
		seq:      evt.GetSeq(),
		sentUTC:  evt.GetSentUtc(),
	}
	if info := evt.GetServerInfo(); info != nil {
		s.serverName = info.GetName()
		s.serverVer = info.GetVersion()
		s.hostname = info.GetHostname()
	}
	if projects := evt.GetProjects(); projects != nil {
		s.projectCount = len(projects.GetProjects())
		for _, p := range projects.GetProjects() {
			s.pipelineCount += len(p.GetPipelines())
		}
	}
	if agents := evt.GetAgents(); agents != nil {
		s.agentCount = len(agents.GetAgents())
		for i, a := range agents.GetAgents() {
			if i >= 8 {
				break
			}
			s.agentIDs = append(s.agentIDs, a.GetAgentId())
		}
	}
	if jobs := evt.GetJobsSummary(); jobs != nil {
		s.queuedCount = int(jobs.GetQueuedCount())
		s.historyCount = int(jobs.GetHistoryCount())
		s.queuedGroups = int(jobs.GetQueuedGroupCount())
		s.historyGroups = int(jobs.GetHistoryGroupCount())
	}
	return s
}

func projectsFromEvent(evt *grpcapi.WatchStateEvent) []projectView {
	if evt == nil || evt.GetProjects() == nil {
		return nil
	}
	src := evt.GetProjects().GetProjects()
	if len(src) == 0 {
		return nil
	}
	out := make([]projectView, 0, len(src))
	for _, p := range src {
		if p == nil {
			continue
		}
		pv := projectView{
			name:      p.GetName(),
			repoURL:   p.GetRepoUrl(),
			configRef: p.GetConfigFile(),
		}
		if strings.TrimSpace(pv.configRef) == "" {
			pv.configRef = p.GetConfigPath()
		}
		for _, pl := range p.GetPipelines() {
			if pl == nil {
				continue
			}
			pv.pipelines = append(pv.pipelines, pipelineView{
				dbID:       pl.GetId(),
				project:    p.GetName(),
				pipelineID: pl.GetPipelineId(),
				trigger:    pl.GetTrigger(),
				sourceRepo: pl.GetSourceRepo(),
				sourceRef:  pl.GetSourceRef(),
				dependsOn:  append([]string(nil), pl.GetDependsOn()...),
			})
		}
		out = append(out, pv)
	}
	return out
}

func (m *guiApp) layout(gtx C) D {
	in := layout.UniformInset(unit.Dp(16))
	return in.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				label := material.H5(m.theme, "ciwi-gui")
				return label.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D {
				lbl := material.Body1(m.theme, "Live connection to ciwi gRPC WatchState stream")
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(m.layoutConnectionRow),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(m.layoutStatusPanel),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(m.layoutSnapshotPanel),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Flexed(1, m.layoutProjectsPanel),
		)
	})
}

func (m *guiApp) layoutConnectionRow(gtx C) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx C) D {
			ed := material.Editor(m.theme, &m.addrEditor, "127.0.0.1:8113")
			return ed.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx C) D {
			btn := material.Button(m.theme, &m.connectBtn, "Connect")
			return btn.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx C) D {
			btn := material.Button(m.theme, &m.disconnBtn, "Disconnect")
			return btn.Layout(gtx)
		}),
	)
}

func (m *guiApp) layoutStatusPanel(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			l := material.Body1(m.theme, "Status: "+string(m.phase)+" - "+m.statusText)
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx C) D {
			err := strings.TrimSpace(m.lastError)
			if err == "" {
				err = "none"
			}
			l := material.Body2(m.theme, "Last error: "+err)
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx C) D {
			action := strings.TrimSpace(m.lastAction)
			if action == "" {
				action = "none"
			}
			l := material.Body2(m.theme, "Last action: "+action)
			return l.Layout(gtx)
		}),
	)
}

func (m *guiApp) layoutSnapshotPanel(gtx C) D {
	s := m.snap
	agents := "(none)"
	if len(s.agentIDs) > 0 {
		agents = strings.Join(s.agentIDs, ", ")
	}
	server := strings.TrimSpace(s.serverName)
	if server == "" {
		server = "(unknown)"
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			l := material.H6(m.theme, "Latest WatchState Snapshot")
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx C) D {
			l := material.Body2(m.theme, fmt.Sprintf("stream=%s seq=%d sent=%s", s.streamID, s.seq, s.sentUTC))
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			l := material.Body2(m.theme, fmt.Sprintf("server=%s version=%s host=%s", server, s.serverVer, s.hostname))
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			l := material.Body2(m.theme, fmt.Sprintf("projects=%d pipelines=%d agents=%d", s.projectCount, s.pipelineCount, s.agentCount))
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			l := material.Body2(m.theme, fmt.Sprintf("jobs queued=%d history=%d groups queued=%d history=%d", s.queuedCount, s.historyCount, s.queuedGroups, s.historyGroups))
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			l := material.Body2(m.theme, "agents: "+agents)
			return l.Layout(gtx)
		}),
	)
}

func (m *guiApp) layoutProjectsPanel(gtx C) D {
	header := material.H6(m.theme, "Projects and Pipelines")
	if len(m.projects) == 0 {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(header.Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D {
				l := material.Body2(m.theme, "Waiting for project data from WatchState...")
				return l.Layout(gtx)
			}),
		)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header.Layout),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx C) D {
			list := material.List(m.theme, &m.projectList)
			return list.Layout(gtx, len(m.projects), func(gtx C, i int) D {
				project := m.projects[i]
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
					return m.layoutProjectBlock(gtx, project)
				})
			})
		}),
	)
}

func (m *guiApp) layoutProjectBlock(gtx C, p projectView) D {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx C) D {
			title := p.name
			if strings.TrimSpace(title) == "" {
				title = "(unnamed project)"
			}
			l := material.Body1(m.theme, "Project: "+title)
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			repo := strings.TrimSpace(p.repoURL)
			if repo == "" {
				repo = "(no repo)"
			}
			cfg := strings.TrimSpace(p.configRef)
			if cfg == "" {
				cfg = "(no config ref)"
			}
			l := material.Body2(m.theme, "repo="+repo+"  config="+cfg)
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
	}
	if len(p.pipelines) == 0 {
		children = append(children, layout.Rigid(func(gtx C) D {
			l := material.Body2(m.theme, "No pipelines")
			return l.Layout(gtx)
		}))
	} else {
		for _, pipeline := range p.pipelines {
			pl := pipeline
			children = append(children, layout.Rigid(func(gtx C) D {
				return m.layoutPipelineRow(gtx, pl)
			}))
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (m *guiApp) layoutPipelineRow(gtx C, p pipelineView) D {
	btns := m.pipelineButtonSet(p.dbID)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					label := strings.TrimSpace(p.pipelineID)
					if label == "" {
						label = "(unnamed pipeline)"
					}
					l := material.Body1(m.theme, "Pipeline: "+label)
					return l.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx C) D {
					b := material.Button(m.theme, &btns.run, "Run")
					return b.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx C) D {
					b := material.Button(m.theme, &btns.dryRun, "Dry Run")
					return b.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx C) D {
			trigger := strings.TrimSpace(p.trigger)
			if trigger == "" {
				trigger = "(none)"
			}
			sourceRepo := strings.TrimSpace(p.sourceRepo)
			if sourceRepo == "" {
				sourceRepo = "(none)"
			}
			sourceRef := strings.TrimSpace(p.sourceRef)
			if sourceRef == "" {
				sourceRef = "(none)"
			}
			deps := "(none)"
			if len(p.dependsOn) > 0 {
				deps = strings.Join(p.dependsOn, ",")
			}
			line := fmt.Sprintf("trigger=%s  source=%s@%s  depends_on=%s", trigger, sourceRepo, sourceRef, deps)
			l := material.Body2(m.theme, line)
			return l.Layout(gtx)
		}),
	)
}
