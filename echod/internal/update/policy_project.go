//go:build wakewordverification && !panel

package update

const (
	projectReleaseFeed = "https://github.com/BretthomerepoProjects/WakeWordVerification/releases"
	defaultReleaseKey  = "lCuutQGLLx1kSkHP8hQ9h0LNadOD0zQi68gE0EhviX8="

	// ManualOnly prevents scheduled installs, even with a previously saved opt-in.
	ManualOnly = true
)
