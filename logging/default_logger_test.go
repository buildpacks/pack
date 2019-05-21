package logging


import (
	"bytes"
	"regexp"
	"testing"

	"github.com/sclevine/spec"
)

const (
	debugMatcher = `^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}\.\d{6} DEBUG:  \w*\n$`
	infoMatcher  = `^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}\.\d{6} INFO:   \w*\n$`
	errorMatcher = `^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}\.\d{6} ERROR:  \w*\n$`
)

func TestDefaultLogger(t *testing.T) {
	spec.Run(t, "DefaultLogger", func(t *testing.T, when spec.G, it spec.S) {
		var w bytes.Buffer
		var logger Logger

		it.Before(func(){
			logger = New(&w)
		})

		it.After(func(){
			w.Reset()
		})

		it("should print debug messages properly", func(){
			logger.Debug("test")
			if f, _ := regexp.MatchString(debugMatcher, w.String()); !f {
				t.Fatalf("unexpected %q", w.String())
			}
		})

		it("should format debug messages properly", func(){
			logger.Debugf( "test%s", "foo")
			if f, _ := regexp.MatchString(debugMatcher, w.String()); !f {
				t.Fatalf("unexpected %q", w.String())
			}
		})

		it("should print info messages properly", func(){
			logger.Info("test")
			if f, _ := regexp.MatchString(infoMatcher, w.String()); !f {
				t.Fatalf("unexpected %q", w.String())
			}
		})

		it("should format info messages properly", func(){
			logger.Infof( "test%s", "foo")
			if f, _ := regexp.MatchString(infoMatcher, w.String()); !f {
				t.Fatalf("unexpected %q", w.String())
			}
		})

		it("should print error messages properly", func(){
			logger.Error("test")
			if f, _ := regexp.MatchString(errorMatcher, w.String()); !f {
				t.Fatalf("unexpected %q", w.String())
			}
		})

		it("should format error messages properly", func(){
			logger.Errorf( "test%s", "foo")
			if f, _ := regexp.MatchString(errorMatcher, w.String()); !f {
				t.Fatalf("unexpected %q", w.String())
			}
		})

		it("should not format writer messages", func(){
			_, _ = logger.Writer().Write([]byte("test"))
			if w.String() != "test" {
				t.Fatalf("expected %q but got %q", "test", w.String())
			}
		})
	})
}