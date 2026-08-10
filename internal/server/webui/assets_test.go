package webui

import "bytes"

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

func browserRendererSource() ([]byte, error) {
	var source bytes.Buffer
	for _, path := range []string{
		"assets/js/view-bindings.js",
		"assets/js/select-control.js",
		"assets/js/graph-view.js",
		"assets/js/tree-view.js",
		"assets/js/dom-reconciler.js",
		"assets/js/declarative.js",
	} {
		payload, err := uiAssets.ReadFile(path)
		if err != nil {
			return nil, err
		}
		source.Write(payload)
		source.WriteByte('\n')
	}
	return source.Bytes(), nil
}
