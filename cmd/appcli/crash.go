package main

// The crash report payload mirrors the JSON contract the backend
// accepts on POST /api/app/v1/crash-report (internal/reporting's
// CrashReport). The CLI keeps its own copy so it does not drag GORM
// and the reporting models into a binary that only needs to send a
// JSON body.
type CrashReport struct {
	Fatal      bool         `json:"fatal"`
	Error      string       `json:"error"`
	StackTrace string       `json:"stackTrace"`
	Context    string       `json:"context"`
	App        AppReport    `json:"app"`
	Device     DeviceReport `json:"device"`
}

type AppReport struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Build       string `json:"build"`
	Environment string `json:"environment"`
	Platform    string `json:"platform"`
}

type DeviceReport struct {
	OsVersion string `json:"osVersion"`
	Locale    string `json:"locale"`
}
