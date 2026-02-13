package server

import "strings"

func parseJobExecutionBuildMetadataFromOutput(output string) map[string]string {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	var buildVersion string
	var buildTarget string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "__CIWI_BUILD_SUMMARY__") {
			continue
		}
		meta := strings.TrimSpace(strings.TrimPrefix(line, "__CIWI_BUILD_SUMMARY__"))
		for _, field := range strings.Fields(meta) {
			kv := strings.SplitN(field, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			switch key {
			case "version":
				if val != "" {
					buildVersion = val
				}
			case "target":
				if val != "" {
					buildTarget = val
				}
			}
		}
	}
	patch := map[string]string{}
	if buildVersion != "" {
		patch["build_version"] = buildVersion
	}
	if buildTarget != "" {
		patch["build_target"] = buildTarget
	}
	if len(patch) == 0 {
		return nil
	}
	return patch
}
