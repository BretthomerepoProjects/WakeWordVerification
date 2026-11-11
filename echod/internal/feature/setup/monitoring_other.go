//go:build !spot || !linux

package setup

import "net/http"

func monitoringSection(http.ResponseWriter, string) {}
