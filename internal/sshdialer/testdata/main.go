package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// simple HTTP server to verify that tunneling works
func main() {
	if len(os.Args) != 3 {
		panic("exactly two positional parameters expected: path to unix socket and tcp port.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		cancel()
		<-sigs
		os.Exit(1)
	}()

	var handler http.HandlerFunc = func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		_, err := w.Write([]byte("Hello there!"))
		if err != nil {
			panic(err)
		}
	}

	serverUnix := http.Server{Handler: handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, _ := context.WithTimeout(context.Background(), time.Second*5)
		_ = serverUnix.Shutdown(shutdownCtx)
	}()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		unixListener, err := net.Listen("unix", os.Args[1])
		if err != nil {
			panic(err)
		}
		defer wg.Done()
		err = serverUnix.Serve(unixListener)
		if err != nil {
			panic(err)
		}
	}()

	serverTcp := http.Server{Handler: handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, _ := context.WithTimeout(context.Background(), time.Second*5)
		_ = serverTcp.Shutdown(shutdownCtx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tcpListener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%s", os.Args[2]))
		if err != nil {
			panic(err)
		}
		err = serverTcp.Serve(tcpListener)
		if err != nil {
			panic(err)
		}
	}()

	wg.Wait()
}
