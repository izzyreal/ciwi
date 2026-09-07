package server

import (
	"context"
	"debug/elf"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/agent"
	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

// Opt-in acceptance test using the real server handlers, scheduler, agent, CLI,
// repository jobs, report ingestion and artifact storage. No installed Ciwi
// server or agent is contacted. The runtime service must already be running.
func TestContainerLiveRepositoryJobs(t *testing.T) {
	backend := os.Getenv("CIWI_TEST_CONTAINER_RUNTIME")
	if backend == "" {
		t.Skip("set CIWI_TEST_CONTAINER_RUNTIME=apple or docker for real container acceptance")
	}
	if backend != "apple" && backend != "docker" {
		t.Fatal("unknown test runtime")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := t.TempDir()
	run := func(dir string, args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return out
	}
	paths := run(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	for _, name := range strings.Split(string(paths), "\x00") {
		if name == "" {
			continue
		}
		src := filepath.Join(root, name)
		info, err := os.Lstat(src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(snapshot, name)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, data, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	run(snapshot, "init", "-q")
	run(snapshot, "add", ".")
	run(snapshot, "-c", "user.name=Ciwi test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "Container acceptance snapshot")
	cfg, err := config.Load(filepath.Join(root, "ciwi-project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var pipelines []config.Pipeline
	for _, p := range cfg.Pipelines {
		if p.ID != "build" && p.ID != "build-desktop" {
			continue
		}
		var jobs []config.PipelineJobSpec
		for _, j := range p.Jobs {
			if j.ID != "integration-tests" && j.ID != "linux-amd64" {
				continue
			}
			if filter := os.Getenv("CIWI_TEST_CONTAINER_JOB"); filter != "" && filter != j.ID {
				continue
			}
			j.RunsOn["container_runtime"] = backend
			j.Needs = nil
			jobs = append(jobs, j)
		}
		if len(jobs) == 0 {
			continue
		}
		p.Jobs = jobs
		p.DependsOn = nil
		p.VCSSource = &config.Source{Repo: snapshot}
		pipelines = append(pipelines, p)
	}
	if len(pipelines) == 0 {
		t.Fatal("no selected jobs")
	}
	cfg.Pipelines = pipelines
	cfg.PipelineChains = nil
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()
	if err := s.db.LoadConfig(cfg, "ciwi-project.yaml", snapshot, "", "ciwi-project.yaml"); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	t.Cleanup(func() {
		// Go deliberately makes downloaded module directories read-only.
		_ = filepath.WalkDir(work, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0755)
			}
			return err
		})
	})
	t.Setenv("CIWI_SERVER_URL", ts.URL)
	t.Setenv("CIWI_AGENT_ID", "container-acceptance")
	t.Setenv("CIWI_AGENT_WORKDIR", work)
	t.Setenv("CIWI_AGENT_ENV_FILE", filepath.Join(work, "absent.env"))
	t.Setenv("CIWI_AGENT_LOG_FILE", filepath.Join(work, "agent.log"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- agent.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("agent did not stop")
		}
	}()
	deadline := time.Now().Add(90 * time.Second)
	registered := false
	for time.Now().Before(deadline) {
		for _, snap := range s.agentRegistry.snapshots() {
			if snap.ID == "container-acceptance" {
				if snap.State.Capabilities["container.runtime."+backend] == "" {
					t.Fatalf("runtime unavailable: %v", snap.State.Capabilities)
				}
				registered = true
			}
		}
		if registered {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !registered {
		t.Fatal("agent never registered")
	}
	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/agents/container-acceptance/actions", map[string]string{"action": "authorize"})
	resp.Body.Close()
	for _, p := range pipelines {
		repeats := 1
		if p.ID == "build-desktop" {
			repeats = 2
		}
		for iteration := 1; iteration <= repeats; iteration++ {
			t.Run(fmt.Sprintf("%s-%d", p.ID, iteration), func(t *testing.T) {
				persisted, err := s.db.GetPipelineByProjectAndID("ciwi", p.ID)
				if err != nil {
					t.Fatal(err)
				}
				enqueued, err := s.enqueuePersistedPipeline(persisted, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(enqueued.JobExecutionIDs) != 1 {
					t.Fatalf("enqueued %v", enqueued)
				}
				id := enqueued.JobExecutionIDs[0]
				deadline := time.Now().Add(time.Duration(p.Jobs[0].TimeoutSeconds+120) * time.Second)
				var job protocol.JobExecution
				for time.Now().Before(deadline) {
					job, err = s.db.GetJobExecution(id)
					if err != nil {
						t.Fatal(err)
					}
					if protocol.IsTerminalJobExecutionStatus(job.Status) {
						break
					}
					time.Sleep(time.Second)
				}
				if job.Status != protocol.JobExecutionStatusSucceeded {
					events, _ := s.db.ListJobExecutionEvents(id)
					for _, event := range events {
						if event.Output != "" {
							t.Log(event.Output)
						}
						if event.Message != "" {
							t.Log(event.Message)
						}
					}
					t.Fatalf("job %s: status=%s error=%s", id, job.Status, job.Error)
				}
				if job.RuntimeCapabilities["container.runtime"] != backend {
					t.Fatalf("wrong runtime: %v", job.RuntimeCapabilities)
				}
				artifacts, err := s.db.ListJobExecutionArtifacts(id)
				if err != nil || len(artifacts) == 0 {
					t.Fatalf("artifacts: %v %v", artifacts, err)
				}
				if p.ID == "build" {
					report, found, err := s.db.GetJobExecutionTestReport(id)
					if err != nil || !found {
						t.Fatalf("JUnit ingestion: %v %v", found, err)
					}
					if report.Total == 0 || report.Failed != 0 {
						t.Fatalf("invalid browser report: %+v", report)
					}
					t.Logf("browser report: passed=%d total=%d", report.Passed, report.Total)
				} else {
					var binary string
					err = filepath.WalkDir(s.artifactsDir, func(path string, d os.DirEntry, err error) error {
						if err == nil && !d.IsDir() && d.Name() == "ciwi" {
							binary = path
						}
						return err
					})
					if err != nil || binary == "" {
						t.Fatalf("desktop artifact missing: %v", err)
					}
					executable, err := elf.Open(binary)
					if err != nil {
						t.Fatal(err)
					}
					if executable.Machine != elf.EM_X86_64 {
						t.Fatalf("artifact is %v", executable.Machine)
					}
					executable.Close()
					if iteration == 2 {
						hit := false
						for _, stat := range job.CacheStats {
							if stat.Source == "hit" && stat.Files > 0 {
								hit = true
							}
						}
						if !hit {
							t.Fatal("second build did not reuse populated cache")
						}
					}
				}
				assertLiveResourceRemoved(t, backend, []string{"inspect", job.RuntimeCapabilities["container.name"]})
				if p.ID == "build-desktop" {
					assertLiveResourceRemoved(t, backend, []string{"image", "inspect", job.RuntimeCapabilities["container.image"]})
				}
				t.Logf("%s completed on %s in %s; artifacts=%d", p.Jobs[0].ID, backend, job.FinishedUTC.Sub(job.StartedUTC), len(artifacts))
			})
		}
	}
}

func assertLiveResourceRemoved(t *testing.T, backend string, args []string) {
	name := args[len(args)-1]
	t.Helper()
	if name == "" {
		t.Fatal("execution did not report container name")
	}
	command := "docker"
	if backend == "apple" {
		command = "container"
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
		cancel()
		if err != nil && (strings.Contains(strings.ToLower(string(out)), "not found") || strings.Contains(strings.ToLower(string(out)), "no such")) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("execution resource %s was not removed", name)
}
