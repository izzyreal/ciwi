package server

import (
	"strings"

	serverupdate "github.com/izzyreal/ciwi/internal/server/update"
	"runtime"
)

func isLinuxSystemUpdaterEnabled() bool {
	return serverupdate.IsLinuxSystemUpdaterEnabled(runtime.GOOS, envOrDefault("CIWI_LINUX_SYSTEM_UPDATER", "true"))
}

func stageLinuxUpdateBinary(targetVersion string, info latestUpdateInfo, newBinPath string) error {
	stagingDir := strings.TrimSpace(envOrDefault("CIWI_UPDATE_STAGING_DIR", "/var/lib/ciwi/updates"))
	manifestPath := strings.TrimSpace(envOrDefault("CIWI_UPDATE_STAGED_MANIFEST", ""))
	return serverupdate.StageLinuxUpdateBinary(targetVersion, info.Asset.Name, newBinPath, serverupdate.StageLinuxOptions{
		StagingDir:   stagingDir,
		ManifestPath: manifestPath,
	})
}

func triggerLinuxSystemUpdater() error {
	return serverupdate.TriggerLinuxSystemUpdater(
		envOrDefault("CIWI_SYSTEMCTL_PATH", "/bin/systemctl"),
		envOrDefault("CIWI_UPDATER_UNIT", "ciwi-updater.service"),
	)
}

func fileSHA256(path string) (string, error) {
	return serverupdate.FileSHA256(path)
}

func sanitizeVersionToken(v string) string {
	return serverupdate.SanitizeVersionToken(v)
}
