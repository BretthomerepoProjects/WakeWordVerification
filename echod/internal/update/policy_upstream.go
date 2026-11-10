//go:build !spot || !wakewordverification

package update

const (
	projectReleaseFeed = ""
	defaultReleaseKey  = "KVUuQUbhyKwPBbIneqFEvXYSI+3Hkfu/heCTy5YNVMk="

	// ManualOnly prevents scheduled installs on builds that require it.
	ManualOnly = false
)
