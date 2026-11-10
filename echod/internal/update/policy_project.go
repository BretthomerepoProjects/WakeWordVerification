//go:build wakewordverification && !panel

package update

const (
	projectReleaseFeed = "https://github.com/BretthomerepoProjects/WakeWordVerification/releases"
	defaultReleaseKey  = "lCuutQGLLx1kSkHP8hQ9h0LNadOD0zQi68gE0EhviX8="

	// ProjectAutoUpdates uses an explicit project opt-in for idle-time updates.
	ProjectAutoUpdates = true
)
