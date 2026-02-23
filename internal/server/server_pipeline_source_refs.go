package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type sourceRefOptionView struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
}

type sourceRefsViewResponse struct {
	SourceRepo string                `json:"source_repo"`
	DefaultRef string                `json:"default_ref,omitempty"`
	Refs       []sourceRefOptionView `json:"refs"`
}

func normalizeSourceRef(selection *protocol.RunPipelineSelectionRequest) string {
	if selection == nil {
		return ""
	}
	return strings.TrimSpace(selection.SourceRef)
}

func shouldApplySourceRefOverride(pipelineRepo, overrideRepo string) bool {
	pipelineRepo = strings.TrimSpace(pipelineRepo)
	overrideRepo = strings.TrimSpace(overrideRepo)
	if pipelineRepo == "" || overrideRepo == "" {
		return false
	}
	return sameSourceRepo(pipelineRepo, overrideRepo)
}

func (s *stateStore) pipelineSourceRefsHandler(w http.ResponseWriter, p store.PersistedPipeline) {
	view, err := buildSourceRefsView(strings.TrimSpace(p.SourceRepo), strings.TrimSpace(p.SourceRef))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *stateStore) pipelineChainSourceRefsHandler(w http.ResponseWriter, ch store.PersistedPipelineChain) {
	if len(ch.Pipelines) == 0 {
		http.Error(w, "pipeline chain has no pipelines", http.StatusBadRequest)
		return
	}
	firstID := strings.TrimSpace(ch.Pipelines[0])
	if firstID == "" {
		http.Error(w, "pipeline chain has empty first pipeline id", http.StatusBadRequest)
		return
	}
	first, err := s.pipelineStore().GetPipelineByProjectAndID(ch.ProjectName, firstID)
	if err != nil {
		http.Error(w, fmt.Sprintf("load pipeline %q in chain %q: %v", firstID, ch.ChainID, err), http.StatusBadRequest)
		return
	}
	view, err := buildSourceRefsView(strings.TrimSpace(first.SourceRepo), strings.TrimSpace(first.SourceRef))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func buildSourceRefsView(sourceRepo, defaultRef string) (sourceRefsViewResponse, error) {
	if sourceRepo == "" {
		return sourceRefsViewResponse{}, fmt.Errorf("pipeline vcs_source.repo is empty")
	}
	options, err := listRemoteBranchRefs(sourceRepo)
	if err != nil {
		return sourceRefsViewResponse{}, err
	}
	resp := sourceRefsViewResponse{
		SourceRepo: sourceRepo,
		Refs:       options,
	}
	def := strings.TrimSpace(defaultRef)
	if def != "" {
		for _, opt := range options {
			if opt.Ref == def || opt.Name == def || ("refs/heads/"+opt.Name) == def {
				resp.DefaultRef = opt.Ref
				break
			}
		}
	}
	if resp.DefaultRef == "" && len(options) > 0 {
		for _, candidate := range []string{"main", "master"} {
			for _, opt := range options {
				if strings.EqualFold(opt.Name, candidate) {
					resp.DefaultRef = opt.Ref
					break
				}
			}
			if resp.DefaultRef != "" {
				break
			}
		}
	}
	if resp.DefaultRef == "" && len(options) > 0 {
		resp.DefaultRef = options[0].Ref
	}
	return resp, nil
}

func listRemoteBranchRefs(repoURL string) ([]sourceRefOptionView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := runCmd(ctx, "", "git", "ls-remote", "--heads", repoURL)
	if err != nil {
		return nil, fmt.Errorf("list source refs from %s: %w", repoURL, err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	options := make([]sourceRefOptionView, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ref := strings.TrimSpace(fields[1])
		if !strings.HasPrefix(ref, "refs/heads/") {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		name := strings.TrimPrefix(ref, "refs/heads/")
		options = append(options, sourceRefOptionView{Name: name, Ref: ref})
	}
	sort.Slice(options, func(i, j int) bool {
		a := strings.ToLower(options[i].Name)
		b := strings.ToLower(options[j].Name)
		if a == b {
			return options[i].Name < options[j].Name
		}
		return a < b
	})
	if len(options) == 0 {
		return nil, fmt.Errorf("no remote branches found in %s", repoURL)
	}
	return options, nil
}

func resolveSourceRefFromRepo(repoURL, sourceRef string) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	sourceRef = strings.TrimSpace(sourceRef)
	if repoURL == "" {
		return "", fmt.Errorf("resolve source ref: empty repo url")
	}
	if sourceRef == "" {
		return "", fmt.Errorf("resolve source ref: empty source ref")
	}
	tmpDir, err := os.MkdirTemp("", "ciwi-source-ref-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir for source ref resolution: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, err := runCmd(ctx, "", "git", "clone", "--depth", "1", repoURL, tmpDir); err != nil {
		return "", fmt.Errorf("clone source for source ref resolution: %w", err)
	}
	if _, err := runCmd(ctx, "", "git", "-C", tmpDir, "fetch", "--depth", "1", "origin", sourceRef); err != nil {
		return "", fmt.Errorf("fetch source ref %q: %w", sourceRef, err)
	}
	if _, err := runCmd(ctx, "", "git", "-C", tmpDir, "checkout", "--force", "FETCH_HEAD"); err != nil {
		return "", fmt.Errorf("checkout source ref %q: %w", sourceRef, err)
	}
	sha, err := runCmd(ctx, "", "git", "-C", tmpDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve source ref %q commit: %w", sourceRef, err)
	}
	return strings.TrimSpace(sha), nil
}
