package webui

// These aliases keep behavior-focused tests readable while their source of
// truth is now the embedded browser assets rather than Go string constants.
var (
	indexHTML        = mustTestAsset("assets/pages/index.html")
	settingsHTML     = mustTestAsset("assets/pages/settings.html")
	projectHTML      = mustTestAsset("assets/pages/project.html")
	vaultHTML        = mustTestAsset("assets/pages/vault.html")
	agentsHTML       = mustTestAsset("assets/pages/agents.html")
	agentHTML        = mustTestAsset("assets/pages/agent.html")
	jobExecutionHTML = mustTestAsset("assets/pages/job-execution.html")

	themeJS        = mustTestAsset("assets/js/theme.js")
	sharedJS       = mustTestAsset("assets/js/shared.js")
	pagesJS        = mustTestAsset("assets/js/pages.js")
	indexJS        = mustTestAsset("assets/js/index.js")
	settingsJS     = mustTestAsset("assets/js/settings.js")
	projectJS      = mustTestAsset("assets/js/project.js")
	jobExecutionJS = mustTestAsset("assets/js/job-execution.js")
	tablerIconsSVG = mustTestAsset("assets/tabler-icons.svg")

	chromeCSS       = mustTestAsset("assets/css/chrome.css")
	indexCSS        = mustTestAsset("assets/css/index.css")
	projectCSS      = mustTestAsset("assets/css/project.css")
	jobExecutionCSS = mustTestAsset("assets/css/job-execution.css")
)

func mustTestAsset(path string) string {
	data, err := uiAssets.ReadFile(path)
	if err != nil {
		panic("read embedded UI test asset " + path + ": " + err.Error())
	}
	return string(data)
}
