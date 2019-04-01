package build

const (
	layersDir     = "/layers"
	buildpacksDir = "/buildpacks"
	platformDir   = "/platform"
	orderPath     = "/buildpacks/order.toml"
	groupPath     = `/layers/group.toml`
	planPath      = "/layers/plan.toml"
	appDir        = "/workspace"
)

func (l *Lifecycle) NewDetect() (*Phase, error) {
	 return l.NewPhase(
		"detector",
		WithArgs(
			"-buildpacks", buildpacksDir,
			"-order", orderPath,
			"-group", groupPath,
			"-plan", planPath,
			"-app", appDir,
		),
	)
}

func (l *Lifecycle) NewRestore(cacheImage string) (*Phase, error) {
	return l.NewPhase(
		"restorer",
		WithDaemonAccess(),
		WithArgs(
			"-image", cacheImage,
			"-group", groupPath,
			"-layers", layersDir,
		),
	)
}

func (l *Lifecycle) NewAnalyze(repoName string, publish bool) (*Phase, error) {
	if publish {
		return l.NewPhase(
			"analyzer",
			WithRegistryAccess(repoName),
			WithArgs(
				"-layers", layersDir,
				"-group", groupPath,
				repoName,
			),
		)
	} else {
		return l.NewPhase(
			"analyzer",
			WithDaemonAccess(),
			WithArgs(
				"-layers", layersDir,
				"-group", groupPath,
				"-daemon",
				repoName,
			),
		)
	}
}

func (l *Lifecycle) NewBuild() (*Phase, error) {
	return l.NewPhase(
		"builder",
		WithArgs(
			"-buildpacks", buildpacksDir,
			"-layers", layersDir,
			"-app", appDir,
			"-group", groupPath,
			"-plan", planPath,
			"-platform", platformDir,
		),
	)
}

func (l *Lifecycle) NewExport(repoName, runImage string, publish bool) (*Phase, error) {
	if publish {
		return l.NewPhase(
			"exporter",
			WithRegistryAccess(repoName, runImage),
			WithArgs(
				"-image", runImage,
				"-layers", layersDir,
				"-app", appDir,
				"-group", groupPath,
				repoName,
			),
		)
	} else {
		return l.NewPhase(
			"exporter",
			WithDaemonAccess(),
			WithArgs(
				"-image", runImage,
				"-layers", layersDir,
				"-app", appDir,
				"-group", groupPath,
				"-daemon",
				repoName,
			),
		)
	}
}


func (l *Lifecycle) NewCache(cacheImage string) (*Phase, error) {
	return l.NewPhase(
		"cacher",
		WithDaemonAccess(),
		WithArgs(
			"-image", cacheImage,
			"-group", groupPath,
			"-layers", layersDir,
		),
	)
}
