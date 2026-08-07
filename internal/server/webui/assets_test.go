package webui

var (
	declarativeHTML = mustTestAsset("assets/pages/declarative.html")
	heartbeatJS     = mustTestAsset("assets/js/heartbeat.js")
	chromeCSS       = mustTestAsset("assets/css/chrome.css")
)

func mustTestAsset(path string) string {
	data, err := uiAssets.ReadFile(path)
	if err != nil {
		panic("read embedded UI test asset " + path + ": " + err.Error())
	}
	return string(data)
}
