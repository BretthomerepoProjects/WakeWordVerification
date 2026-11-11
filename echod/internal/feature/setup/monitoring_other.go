//go:build !linux || (!spot && !firmwaremonitoring)

package setup

import "net/http"

func monitoringSection(http.ResponseWriter, string) {}
