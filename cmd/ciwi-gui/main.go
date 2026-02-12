package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
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
	snap       snapshot
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
		theme:      material.NewTheme(),
		updates:    make(chan watchUpdate, 256),
		window:     w,
		phase:      phaseDisconnected,
		statusText: "Disconnected",
	}
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
			}
		default:
			return
		}
	}
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
