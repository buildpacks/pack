package cmd

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultLayersDir     = "/layers"
	DefaultAppDir        = "/workspace"
	DefaultBuildpacksDir = "/buildpacks"
	DefaultPlatformDir   = "/platform"
	DefaultOrderPath     = "/buildpacks/order.toml"
	DefaultGroupPath     = "./group.toml"
	DefaultStackPath     = "/buildpacks/stack.toml"
	DefaultPlanPath      = "./plan.toml"

	EnvLayersDir     = "CNB_LAYERS_DIR"
	EnvAppDir        = "CNB_APP_DIR"
	EnvBuildpacksDir = "CNB_BUILDPACKS_DIR"
	EnvPlatformDir   = "CNB_PLATFORM_DIR"
	EnvOrderPath     = "CNB_ORDER_PATH"
	EnvGroupPath     = "CNB_GROUP_PATH"
	EnvStackPath     = "CNB_STACK_PATH"
	EnvPlanPath      = "CNB_PLAN_PATH"
	EnvUseDaemon     = "CNB_USE_DAEMON"       // defaults to false
	EnvUseHelpers    = "CNB_USE_CRED_HELPERS" // defaults to false
	EnvRunImage      = "CNB_RUN_IMAGE"
	EnvCacheImage    = "CNB_CACHE_IMAGE"
	EnvUID           = "CNB_USER_ID"
	EnvGID           = "CNB_GROUP_ID"
	EnvRegistryAuth  = "CNB_REGISTRY_AUTH"
)

func FlagLayersDir(dir *string) {
	flag.StringVar(dir, "layers", envWithDefault(EnvLayersDir, DefaultLayersDir), "path to layers directory")
}

func FlagAppDir(dir *string) {
	flag.StringVar(dir, "app", envWithDefault(EnvAppDir, DefaultAppDir), "path to app directory")
}

func FlagBuildpacksDir(dir *string) {
	flag.StringVar(dir, "buildpacks", envWithDefault(EnvBuildpacksDir, DefaultBuildpacksDir), "path to buildpacks directory")
}

func FlagPlatformDir(dir *string) {
	flag.StringVar(dir, "platform", envWithDefault(EnvPlatformDir, DefaultPlatformDir), "path to platform directory")
}

func FlagOrderPath(path *string) {
	flag.StringVar(path, "order", envWithDefault(EnvOrderPath, DefaultOrderPath), "path to order.toml")
}

func FlagGroupPath(path *string) {
	flag.StringVar(path, "group", envWithDefault(EnvGroupPath, DefaultGroupPath), "path to group.toml")
}

func FlagStackPath(path *string) {
	flag.StringVar(path, "stack", envWithDefault(EnvStackPath, DefaultStackPath), "path to stack.toml")
}

func FlagPlanPath(path *string) {
	flag.StringVar(path, "plan", envWithDefault(EnvPlanPath, DefaultPlanPath), "path to plan.toml")
}

func FlagRunImage(image *string) {
	flag.StringVar(image, "image", os.Getenv(EnvRunImage), "reference to run image")
}

func FlagCacheImage(image *string) {
	flag.StringVar(image, "image", os.Getenv(EnvCacheImage), "cache image tag name")
}

func FlagUseDaemon(use *bool) {
	flag.BoolVar(use, "daemon", boolEnv(EnvUseDaemon), "export to docker daemon")
}

func FlagUseCredHelpers(use *bool) {
	flag.BoolVar(use, "helpers", boolEnv(EnvUseHelpers), "use credential helpers")
}

func FlagUID(uid *int) {
	flag.IntVar(uid, "uid", intEnv(EnvUID), "UID of user in the stack's build and run images")
}

func FlagGID(gid *int) {
	flag.IntVar(gid, "gid", intEnv(EnvGID), "GID of user's group in the stack's build and run images")
}

const (
	CodeFailed      = 1
	CodeInvalidArgs = iota + 2
	CodeInvalidEnv
	CodeNotFound
	CodeFailedDetect
	CodeFailedBuild
	CodeFailedLaunch
	CodeFailedUpdate
)

type ErrorFail struct {
	Err    error
	Code   int
	Action []string
}

func (e *ErrorFail) Error() string {
	message := "failed to " + strings.Join(e.Action, " ")
	if e.Err == nil {
		return message
	}
	return fmt.Sprintf("%s: %s", message, e.Err)
}

func FailCode(code int, action ...string) error {
	return FailErrCode(nil, code, action...)
}

func FailErr(err error, action ...string) error {
	code := CodeFailed
	if err, ok := err.(*ErrorFail); ok {
		code = err.Code
	}
	return FailErrCode(err, code, action...)
}

func FailErrCode(err error, code int, action ...string) error {
	return &ErrorFail{Err: err, Code: code, Action: action}
}

func Exit(err error) {
	if err == nil {
		os.Exit(0)
	}
	logger := log.New(os.Stderr, "", 0)
	logger.Printf("Error: %s\n", err)
	if err, ok := err.(*ErrorFail); ok {
		os.Exit(err.Code)
	}
	os.Exit(CodeFailed)
}

func intEnv(k string) int {
	v := os.Getenv(k)
	d, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return d
}

func boolEnv(k string) bool {
	v := os.Getenv(k)
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func envWithDefault(key string, defaultVal string) string {
	if envVal := os.Getenv(key); envVal != "" {
		return envVal
	}
	return defaultVal
}
