package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/app"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestRun(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "run", testRun, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testRun(t *testing.T, when spec.G, it spec.S) {
	when("#Run", func() {
		var (
			subject        *app.Image
			docker         *client.Client
			err            error
			outBuf, errBuf bytes.Buffer
			repo           string
		)

		it.Before(func() {
			docker, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)

			repo = "some-org/" + h.RandString(10)
			logger := logging.NewLogger(&outBuf, &errBuf, true, false)
			subject = &app.Image{
				RepoName: repo,
				Logger:   logger,
			}
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
					return strings.Contains(errBuf.String(), "Server is listening")
				})
			})
		})

		when("a port is exposed", func() {
			var containerPort string

			it.Before(func() {
				containerPort, err = freePort()
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
				containerPort, err = freePort()
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
				hostPort, err := freePort()
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
	if err := <-done; errors.Cause(err) != context.Canceled {
		t.Fatalf("expected canceled context, failed with a different error: %s", err)
	}
}

func freePort() (string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return "", err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return "", err
	}
	defer l.Close()

	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port), nil
}
