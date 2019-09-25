package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/app"
	"github.com/buildpack/pack/internal/fakes"
	h "github.com/buildpack/pack/testhelpers"
)

func TestApp(t *testing.T) {
	h.RequireDocker(t)
	color.Disable(true)
	defer func() { color.Disable(false) }()
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "app", testApp, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testApp(t *testing.T, when spec.G, it spec.S) {
	when("#Run", func() {
		var (
			subject *app.Image
			docker  *client.Client
			err     error
			errBuf  bytes.Buffer
			repo    string
		)

		it.Before(func() {
			docker, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)

			repo = "some-org/" + h.RandString(10)

			logger := fakes.NewFakeLogger(&errBuf)

			subject = &app.Image{
				RepoName: repo,
				Logger:   logger,
			}
		})

		it.After(func() {
			h.AssertNil(t, h.DockerRmi(docker, repo, "hashicorp/http-echo"))
		})

		when("there is no exposed or provided ports", func() {
			it.Before(func() {
				h.CreateImageOnLocal(
					t,
					docker,
					repo,
					"FROM hashicorp/http-echo\nCMD [\"-text=hello world\"]",
				)
			})

			it("runs an image", func() {
				assertOnRunningContainer(t, subject, nil, &errBuf, docker, func() bool {
					return strings.Contains(errBuf.String(), "listening")
				})
			})
		})

		when("a port is exposed", func() {
			var containerPort string

			it.Before(func() {
				containerPort, err = h.GetFreePort()
				h.AssertNil(t, err)
				h.CreateImageOnLocal(
					t,
					docker,
					repo,
					fmt.Sprintf(
						"FROM hashicorp/http-echo\nEXPOSE %s\nCMD [\"-listen=:%s\",\"-text=hello world\"]",
						containerPort,
						containerPort,
					),
				)
			})

			it("gets exposed ports from the image", func() {
				assertOnRunningContainer(t, subject, nil, &errBuf, docker, func() bool {
					resp, err := http.Get("http://localhost:" + containerPort)
					if err != nil {
						t.Log(err)
						return false
					}
					defer resp.Body.Close()

					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						t.Log(err)
						return false
					}
					if !strings.Contains(string(body), "hello world") {
						t.Log("got response body:", string(body))
						return false
					}
					return true
				})
			})
		})

		when("custom ports bindings are defined", func() {
			var (
				containerPort string
				err           error
			)

			it.Before(func() {
				containerPort, err = h.GetFreePort()
				h.AssertNil(t, err)
				h.CreateImageOnLocal(
					t,
					docker,
					repo,
					fmt.Sprintf("FROM hashicorp/http-echo\nCMD [\"-listen=:%s\",\"-text=hello world\"]", containerPort),
				)
			})

			it("binds simple ports from localhost to the container on the same port", func() {
				assertOnRunningContainer(t, subject, []string{containerPort}, &errBuf, docker, func() bool {
					resp, err := http.Get("http://localhost:" + containerPort)
					if err != nil {
						t.Log(err)
						return false
					}
					defer resp.Body.Close()

					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						t.Log(err)
						return false
					}
					if !strings.Contains(string(body), "hello world") {
						t.Log("got response body:", string(body))
						return false
					}
					return true
				})
			})

			it("binds each port to the container", func() {
				hostPort, err := h.GetFreePort()
				h.AssertNil(t, err)

				assertOnRunningContainer(
					t,
					subject,
					[]string{fmt.Sprintf("127.0.0.1:%s:%s/tcp", hostPort, containerPort)},
					&errBuf,
					docker,
					func() bool {
						resp, err := http.Get("http://localhost:" + hostPort)
						if err != nil {
							t.Log(err)
							return false
						}
						defer resp.Body.Close()

						body, err := ioutil.ReadAll(resp.Body)
						if err != nil {
							t.Log(err)
							return false
						}
						if !strings.Contains(string(body), "hello world") {
							t.Log("got response body:", string(body))
							return false
						}
						return true
					})
			})
		})
	})
}

func assertOnRunningContainer(t *testing.T, subject *app.Image, port []string, errBuf *bytes.Buffer, docker *client.Client, testFunc func() bool) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- subject.Run(ctx, docker, port)
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(time.Second * 5)
	defer timer.Stop()

loop:
	for {
		select {
		case <-timer.C:
			cancel()
			t.Fatalf("timed out: %s", errBuf.String())
		case <-ticker.C:
			if testFunc() {
				break loop
			}
		}
	}
	cancel()
	if err := <-done; !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected canceled context, failed with a different error: %s", err)
	}
}
