package lifecycle

import (
	"math/rand"
	"path/filepath"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/internal/buildkit/state"
	"github.com/buildpacks/pack/internal/paths"
)

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

func (l *LifecycleExecution) hasExtensions() bool {
	return len(l.opts.Builder.OrderExtensions()) > 0
}

func (l *LifecycleExecution) withLogLevel(args ...string) []string {
	if l.logger.IsVerbose() {
		return append([]string{"-log-level", "debug"}, args...)
	}
	return args
}

func withLayoutOperation(state *state.State) {
	layoutDir := filepath.Join(paths.RootDir, "layout-repo")
	newState := state.AddEnv("CNB_USE_LAYOUT", "true").AddEnv("CNB_LAYOUT_DIR", layoutDir).AddEnv("CNB_EXPERIMENTAL_MODE", "warn")
	state = &newState
}

func prependArg(arg string, args []string) []string {
	return append([]string{arg}, args...)
}

func addTags(flags, additionalTags []string) []string {
	for _, tag := range additionalTags {
		flags = append(flags, "-tag", tag)
	}
	return flags
}

func determineDefaultProcessType(platformAPI *api.Version, providedValue string) string {
	shouldSetForceDefault := platformAPI.Compare(api.MustParse("0.4")) >= 0 &&
		platformAPI.Compare(api.MustParse("0.6")) < 0
	if providedValue == "" && shouldSetForceDefault {
		return defaultProcessType
	}

	return providedValue
}
