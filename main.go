package main

import (
	"flag"
	"fmt"
	"github.com/buildpack/pack/cmd"
	"log"
	"os"
)

var (
	appDir    string
	repoName  string
	useDaemon bool
)

func init() {
	wd, _ := os.Getwd()

	flag.BoolVar(&useDaemon, "daemon", false, "use local Docker daemon as repository")
	flag.StringVar(&appDir, "dir", wd, "path to app dir")
	flag.StringVar(&repoName, "name", "", "docker image repository name")
	flag.Parse()
}

func main() {
	fmt.Printf("ARGS: %+v. APP DIR: %s \n REPO NAME: %s \n", os.Args, appDir, repoName)
	flag.PrintDefaults()

	if len(os.Args) < 1 || appDir == "" || repoName == "" {
		log.Printf("USAGE: pack build -daemon [ -dir <app-dir> ] -name <image-repo-name>")
		os.Exit(1)
	}

	switch flag.Args()[0] {
	case "build":
		if err := cmd.Build(appDir, repoName, useDaemon); err != nil {
			log.Fatalln(err)
		}
	default:
		log.Fatalln("Unknown command:", flag.Args())
	}
}
