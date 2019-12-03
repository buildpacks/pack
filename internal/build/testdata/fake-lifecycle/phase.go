package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"syscall"

	"github.com/buildpack/lifecycle/auth"
	"github.com/docker/docker/api/types"
	dockercli "github.com/docker/docker/client"
	v1remote "github.com/google/go-containerregistry/pkg/v1/remote"
)

func main() {
	fmt.Println("running some-lifecycle-phase")
	fmt.Printf("received args %+v\n", os.Args)
	if len(os.Args) > 3 && os.Args[1] == "write" {
		testWrite(os.Args[2], os.Args[3])
	}
	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		testDaemon()
	}
	if len(os.Args) > 1 && os.Args[1] == "registry" {
		testRegistryAccess(os.Args[2])
	}
	if len(os.Args) > 2 && os.Args[1] == "read" {
		testRead(os.Args[2])
	}
	if len(os.Args) > 2 && os.Args[1] == "delete" {
		testDelete(os.Args[2])
	}
	if len(os.Args) > 1 && os.Args[1] == "env" {
		testEnv()
	}
	if len(os.Args) > 1 && os.Args[1] == "buildpacks" {
		testBuildpacks()
	}
	if len(os.Args) > 1 && os.Args[1] == "proxy" {
		testProxy()
	}
	if len(os.Args) > 1 && os.Args[1] == "binds" {
		testBinds()
	}
	if len(os.Args) > 1 && os.Args[1] == "network" {
		testNetwork()
	}
	if len(os.Args) > 1 && os.Args[1] == "user" {
		testUser()
	}
}

func testWrite(filename, contents string) {
	fmt.Println("write test")
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("failed to create %s: %s\n", filename, err)
		os.Exit(1)
	}
	defer file.Close()
	_, err = file.Write([]byte(contents))
	if err != nil {
		fmt.Printf("failed to write to %s: %s\n", filename, err)
		os.Exit(2)
	}
}

func testDaemon() {
	fmt.Println("daemon test")
	cli, err := dockercli.NewClientWithOpts(dockercli.FromEnv, dockercli.WithVersion("1.38"))
	if err != nil {
		fmt.Printf("failed to create new docker client: %s\n", err)
		os.Exit(3)
	}
	_, err = cli.ContainerList(context.TODO(), types.ContainerListOptions{})
	if err != nil {
		fmt.Printf("failed to access docker daemon: %s\n", err)
		os.Exit(4)
	}
}

func testRegistryAccess(repoName string) {
	fmt.Println("registry test")
	ref, authenticator, err := auth.ReferenceForRepoName(auth.EnvKeychain("CNB_REGISTRY_AUTH"), repoName)
	if err != nil {
		fmt.Println("fail")
		os.Exit(5)
	}

	_, err = v1remote.Image(ref, v1remote.WithAuth(authenticator))
	if err != nil {
		fmt.Println("failed to access image:", err)
		os.Exit(6)
	}
}

func testRead(filename string) {
	fmt.Println("read test")
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Printf("failed to read file '%s'\n", filename)
		os.Exit(7)
	}
	fmt.Println("file contents:", string(contents))
	info, err := os.Stat(filename)
	stat := info.Sys().(*syscall.Stat_t)
	fmt.Printf("file uid/gid %d/%d\n", stat.Uid, stat.Gid)
}

func testEnv() {
	fmt.Println("env test")
	fis, err := ioutil.ReadDir("/platform/env")
	if err != nil {
		fmt.Printf("failed to read /plaform/env dir: %s\n", err)
		os.Exit(8)
	}
	for _, fi := range fis {
		contents, err := ioutil.ReadFile(filepath.Join("/", "platform", "env", fi.Name()))
		if err != nil {
			fmt.Printf("failed to read file /plaform/env/%s: %s\n", fi.Name(), err)
			os.Exit(9)
		}
		fmt.Printf("%s=%s\n", fi.Name(), string(contents))
	}
}

func testDelete(filename string) {
	fmt.Println("delete test")
	err := os.RemoveAll(filename)
	if err != nil {
		fmt.Printf("failed to delete file '%s'\n", filename)
		os.Exit(10)
	}
}

func testBuildpacks() {
	fmt.Println("buildpacks test")

	readDir("/buildpacks")
}

func testProxy() {
	fmt.Println("proxy test")
	fmt.Println("HTTP_PROXY=" + os.Getenv("HTTP_PROXY"))
	fmt.Println("HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"))
	fmt.Println("NO_PROXY=" + os.Getenv("NO_PROXY"))
	fmt.Println("http_proxy=" + os.Getenv("http_proxy"))
	fmt.Println("https_proxy=" + os.Getenv("https_proxy"))
	fmt.Println("no_proxy=" + os.Getenv("no_proxy"))
}

func testNetwork() {
	fmt.Println("testing network")
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Print(fmt.Errorf("localAddresses: %+v\n", err.Error()))
		return
	}
	fmt.Printf("iterating over %v interfaces\n", len(ifaces))
	for _, i := range ifaces {
		fmt.Printf("interface: %s\n", i.Name)
	}

	resp, err := http.Get("http://google.com")
	if err != nil {
		fmt.Printf("error connecting to internet: %s\n", err.Error())
		return
	}
	fmt.Printf("response status %s\n", resp.Status)
}

func testBinds() {
	fmt.Println("binds test")
	readDir("/mounted")
}

func readDir(dir string) {
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		fmt.Printf("failed to read %s dir: %s\n", dir, err)
		os.Exit(9)
	}
	for _, fi := range fis {
		absPath := filepath.Join(dir, fi.Name())
		stat := fi.Sys().(*syscall.Stat_t)
		fmt.Printf("%s %d/%d \n", absPath, stat.Uid, stat.Gid)
		if fi.IsDir() {
			readDir(absPath)
		}
	}
}

func testUser() {
	fmt.Println("user test")
	user, err := user.Current()
	if err != nil {
		fmt.Printf("failed to determine current user: %s\n", err)
		return
	}

	fmt.Printf("current user is %s\n", user.Name)
}
