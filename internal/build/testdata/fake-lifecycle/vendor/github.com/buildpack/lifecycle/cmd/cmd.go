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
	DefaultLayersDir      = "/workspace"
	DefaultAppDir         = "/workspace/app"
	DefaultBuildpacksDir  = "/buildpacks"
	DefaultPlatformDir    = "/platform"
	DefaultOrderPath      = "/buildpacks/order.toml"
	DefaultGroupPath      = "./group.toml"
	DefaultPlanPath       = "./plan.toml"
	DefaultUseDaemon      = false
	DefaultUseCredHelpers = false

	EnvRunImage           = "PACK_RUN_IMAGE"
	EnvUID                = "PACK_USER_ID"
	EnvGID                = "PACK_GROUP_ID"
	EnvLayersDir          = "PACK_LAYERS_DIR"
	EnvAppDir             = "PACK_APP_DIR"
	EnvLegacyRegistryAuth = "PACK_REGISTRY_AUTH"
	EnvRegistryAuth       = "CNB_REGISTRY_AUTH"
)

func FlagLayersDir(dir *string) {
	flag.StringVar(dir, "layers", DefaultLayersDir, "path to layers directory")
}

func FlagAppDir(dir *string) {
	flag.StringVar(dir, "app", DefaultAppDir, "path to app directory")
}

func FlagBuildpacksDir(dir *string) {
	flag.StringVar(dir, "buildpacks", DefaultBuildpacksDir, "path to buildpacks directory")
}

func FlagPlatformDir(dir *string) {
	flag.StringVar(dir, "platform", DefaultPlatformDir, "path to platform directory")
}

func FlagOrderPath(path *string) {
	flag.StringVar(path, "order", DefaultOrderPath, "path to order.toml")
}

func FlagGroupPath(path *string) {
	flag.StringVar(path, "group", DefaultGroupPath, "path to group.toml")
}

func FlagPlanPath(path *string) {
	flag.StringVar(path, "plan", DefaultPlanPath, "path to plan.toml")
}

func FlagRunImage(image *string) {
	flag.StringVar(image, "image", os.Getenv(EnvRunImage), "reference to run image")
}

func FlagUseDaemon(use *bool) {
	flag.BoolVar(use, "daemon", DefaultUseDaemon, "export to docker daemon")
}

func FlagUseCredHelpers(use *bool) {
	flag.BoolVar(use, "helpers", DefaultUseCredHelpers, "use credential helpers")
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
	log.Printf("Error: %s\n", err)
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
