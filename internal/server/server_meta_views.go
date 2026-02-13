package server

type serverInfoResponse struct {
	Name       string `json:"name"`
	APIVersion int    `json:"api_version"`
	Version    string `json:"version"`
	Hostname   string `json:"hostname,omitempty"`
}

type healthzResponse struct {
	Status string `json:"status"`
}
