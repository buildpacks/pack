package logging

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
)

const testTime = "2019/05/15 01:01:01.000000"

func newTestLogger(stdout, stderr io.Writer) *logWithWriters {
	hnd := &handler{
		writer: stdout,
		timer: func() time.Time {
			tm, _ := time.Parse(timeFmt, testTime)
			return tm
		},
	}
	var lw logWithWriters
	lw.handler = hnd
	lw.out = hnd.writer
	lw.errOut = stderr
	lw.Logger.Handler = hnd
	lw.Logger.Level = log.DebugLevel
	return &lw
}

func TestPackCLILogger(t *testing.T) {
	spec.Run(t, "PackCLILogger", func(t *testing.T, when spec.G, it spec.S) {
		var log, errlog bytes.Buffer
		var logger *logWithWriters

		it.Before(func() {
			color.NoColor = false
			logger = newTestLogger(&log, &errlog)
		})

		it.After(func() {
			log.Reset()
			errlog.Reset()
		})

		it("can enable time in logs", func() {
			logger.WantTime(true)
			logger.Error("test")
			expected := "2019/05/15 01:01:01.000000 \x1b[31;1mERROR  \x1b[0mtest                     \n"
			if expected != log.String() {
				t.Fatalf("actual %q expected %q", log.String(), expected)
			}
		})

		it("it has no time and color by default", func() {
			logger.Error("test")
			expected := "\x1b[31;1mERROR  \x1b[0mtest                     \n"
			if expected != log.String() {
				t.Log(log.String())
				t.Fatalf("actual %q expected %q", log.String(), expected)
			}
		})

		it("can disable color logs", func() {
			color.NoColor = true
			logger.Error("test")
			expected := "ERROR  test                     \n"
			if expected != log.String() {
				t.Fatalf("actual %q expected %q", log.String(), expected)
			}
		})

		it("non-error levels not shown", func() {
			logger.Info("test")
			expected := "test                     \n"
			if expected != log.String() {
				t.Fatalf("actual %q expected %q", log.String(), expected)
			}
		})

		it("will not show verbose messages if quiet", func() {
			logger.WantQuiet(true)
			logger.Debug("hello")
			logger.Debugf("there")
			if log.String() != "" {
				t.Fatal("should not be a string in quiet mode")
			}
			logger.Info("test")
			expected := "test                     \n"
			if log.String() != expected {
				t.Fatalf("actual %q expected %q", log.String(), expected)
			}

			testOut := logger.Writer()
			if testOut != ioutil.Discard {
				t.Fatal("writer should be /dev/null")
			}

			testOut = logger.ErrorWriter()
			if testOut != ioutil.Discard {
				t.Fatal("error writer should be /dev/null")
			}
		})

		it("will return correct writers", func() {
			testOut := logger.Writer()
			if testOut != &log {
				t.Fatal("incorrect writer")
			}

			testOut = logger.ErrorWriter()
			if testOut != &errlog {
				t.Fatal("incorrect error writer")
			}
		})

	})
}
