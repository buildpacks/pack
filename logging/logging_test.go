package logging

import (
	"bytes"
	"io"
	"log"
	"testing"
)

type mockLogger struct {
	written string
}

func(m *mockLogger) Info(msg string) {
	m.written += msg
}

func TestNewWriter(t *testing.T) {
	logger := new(mockLogger)
	writer := NewWriter(logger.Info)
	testMsg := "hello, world!"
	buff := bytes.NewBufferString(testMsg)
	_, err := io.Copy(writer, buff)
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	if logger.written != testMsg {
		log.Fatalf("expected %q but got %q", testMsg, logger.written)
	}
}
