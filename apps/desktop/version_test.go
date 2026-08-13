package main

import "testing"

func TestVersionUsesBuildValueAndFallback(t *testing.T) {
	originalProduct := productVersion
	original := buildVersion
	originalSource := sourceRevision
	t.Cleanup(func() {
		productVersion = originalProduct
		buildVersion = original
		sourceRevision = originalSource
	})

	app := &DesktopApp{}
	productVersion = "0.2.0"
	buildVersion = "2026.07.19.170102"
	sourceRevision = "a69657674503"
	if got, want := app.Version(), "0.2.0 · build 2026.07.19.170102 · source a69657674503"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}

	productVersion = "  "
	buildVersion = "  "
	sourceRevision = "unknown"
	if got := app.Version(); got != "dev · build dev" {
		t.Fatalf("empty Version() = %q, want dev build", got)
	}
}
