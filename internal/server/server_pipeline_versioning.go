package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/store"
)

var semverCorePattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func resolvePipelineRunContextWithReporter(p store.PersistedPipeline, dep pipelineDependencyContext, report resolveStepReporter) (pipelineRunContext, error) {
	ctx := pipelineRunContext{}
	file := strings.TrimSpace(p.Versioning.File)
	tagPrefix := strings.TrimSpace(p.Versioning.TagPrefix)
	autoBump := strings.TrimSpace(p.Versioning.AutoBump)
	versioningEnabled := file != "" || tagPrefix != "" || autoBump != ""
	if versioningEnabled {
		ctx.VersionFile = file
		ctx.TagPrefix = tagPrefix
		ctx.AutoBump = autoBump
		if ctx.VersionFile == "" {
			ctx.VersionFile = "VERSION"
		}
		if ctx.TagPrefix == "" {
			ctx.TagPrefix = "v"
		}
	}

	if dep.Version != "" {
		if report != nil {
			report("version", "ok", fmt.Sprintf("using inherited dependency version %s", dep.Version))
		}
		ctx.Version = dep.Version
		ctx.VersionRaw = dep.VersionRaw
		ctx.SourceRefResolved = dep.SourceRefResolved
		return ctx, nil
	}
	if !versioningEnabled {
		if report != nil {
			report("version", "ok", "pipeline versioning not configured")
		}
		return ctx, nil
	}

	if strings.TrimSpace(p.SourceRepo) == "" {
		if report != nil {
			report("version", "error", "pipeline source.repo is empty; cannot resolve version")
		}
		return ctx, nil
	}
	if report != nil {
		report("version", "running", fmt.Sprintf("resolving from %s at ref %s", p.SourceRepo, strings.TrimSpace(p.SourceRef)))
	}
	versionRaw, sourceRefResolved, err := readVersionFromRepo(p.SourceRepo, p.SourceRef, ctx.VersionFile, report)
	if err != nil {
		if ctx.AutoBump != "" {
			if report != nil {
				report("version", "error", err.Error())
			}
			return pipelineRunContext{}, err
		}
		// Versioning is optional unless auto-bump was requested.
		if report != nil {
			report("version", "error", "version not resolved: "+err.Error())
		}
		return pipelineRunContext{}, nil
	}
	ctx.VersionRaw = versionRaw
	ctx.Version = ctx.TagPrefix + versionRaw
	ctx.SourceRefResolved = sourceRefResolved
	if report != nil {
		report("version", "ok", fmt.Sprintf("resolved version=%s (raw=%s)", ctx.Version, ctx.VersionRaw))
	}
	return ctx, nil
}

func resolvePipelineRunContext(p store.PersistedPipeline, dep pipelineDependencyContext) (pipelineRunContext, error) {
	return resolvePipelineRunContextWithReporter(p, dep, nil)
}

func readVersionFromRepo(repoURL, sourceRef, versionFile string, report resolveStepReporter) (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "ciwi-version-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir for version resolution: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if report != nil {
		report("checkout", "running", "cloning repository")
	}
	if _, err := runCmd(ctx, "", "git", "clone", "--depth", "1", repoURL, tmpDir); err != nil {
		return "", "", fmt.Errorf("clone source for version resolution: %w", err)
	}
	if strings.TrimSpace(sourceRef) != "" {
		if report != nil {
			report("checkout", "running", fmt.Sprintf("fetching source ref %q", sourceRef))
		}
		if _, err := runCmd(ctx, "", "git", "-C", tmpDir, "fetch", "--depth", "1", "origin", sourceRef); err != nil {
			return "", "", fmt.Errorf("fetch source ref %q for version resolution: %w", sourceRef, err)
		}
		if _, err := runCmd(ctx, "", "git", "-C", tmpDir, "checkout", "--force", "FETCH_HEAD"); err != nil {
			return "", "", fmt.Errorf("checkout source ref %q for version resolution: %w", sourceRef, err)
		}
	}
	if report != nil {
		report("checkout", "running", "resolving source commit")
	}
	sha, err := runCmd(ctx, "", "git", "-C", tmpDir, "rev-parse", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("resolve source commit for versioning: %w", err)
	}
	if report != nil {
		report("checkout", "ok", "resolved source commit "+strings.TrimSpace(sha))
	}
	rel := filepath.ToSlash(filepath.Clean(versionFile))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "..") || strings.Contains(rel, "/../") {
		return "", "", fmt.Errorf("invalid versioning.file: %q", versionFile)
	}
	if report != nil {
		report("version-file", "running", "reading "+rel)
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, filepath.FromSlash(rel)))
	if err != nil {
		return "", "", fmt.Errorf("read version file %q: %w", rel, err)
	}
	raw := strings.TrimSpace(string(data))
	if !semverCorePattern.MatchString(raw) {
		return "", "", fmt.Errorf("version file %q must contain semver core (x.y.z), got %q", rel, raw)
	}
	if report != nil {
		report("version-file", "ok", "validated "+rel+" as "+raw)
	}
	return raw, strings.TrimSpace(sha), nil
}

func buildAutoBumpStepScript(mode string) string {
	return fmt.Sprintf(`if [ "${CIWI_DRY_RUN:-0}" = "1" ]; then
  echo "[dry-run] skipping automatic version bump"
else
  case "%s" in
    patch|minor|major) ;;
    *) echo "unsupported auto bump mode: %s"; exit 1 ;;
  esac
  IFS='.' read -r MAJOR MINOR PATCH <<EOF
${CIWI_PIPELINE_VERSION_RAW}
EOF
  case "%s" in
    patch) NEXT_VERSION="${MAJOR}.${MINOR}.$((PATCH+1))" ;;
    minor) NEXT_VERSION="${MAJOR}.$((MINOR+1)).0" ;;
    major) NEXT_VERSION="$((MAJOR+1)).0.0" ;;
  esac
  printf '%%s\n' "$NEXT_VERSION" > "${CIWI_PIPELINE_VERSION_FILE}"
  git add "${CIWI_PIPELINE_VERSION_FILE}"
  git commit -m "chore: bump ${CIWI_PIPELINE_VERSION_FILE} to ${NEXT_VERSION} [skip ci]"
  BRANCH="$(git rev-parse --abbrev-ref HEAD)"
  if [ -z "$BRANCH" ] || [ "$BRANCH" = "HEAD" ]; then BRANCH="main"; fi
  git push origin "HEAD:${BRANCH}"
  echo "__CIWI_RELEASE_SUMMARY__ next_version=$NEXT_VERSION"
fi`, mode, mode, mode)
}
