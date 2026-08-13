package main

import "strings"

var (
	productVersion = "0.2.0"
	buildVersion   = "dev"
	sourceRevision = "unknown"
)

func (a *DesktopApp) Version() string {
	product := strings.TrimSpace(productVersion)
	if product == "" {
		product = "dev"
	}
	build := strings.TrimSpace(buildVersion)
	if build == "" {
		build = "dev"
	}
	version := product + " · build " + build
	if source := strings.TrimSpace(sourceRevision); source != "" && source != "unknown" {
		version += " · source " + source
	}
	return version
}
