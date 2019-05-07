package logging

import (
	"bytes"
	"testing"
	"time"

	"github.com/sclevine/spec"

	"github.com/buildpack/pack/logging"
)

func TestPackCLILogger(t *testing.T) {
	spec.Run(t, "PackCLILogger", func(t *testing.T, when spec.G, it spec.S) {
		const testTime = "2019/05/15 01:01:01.000000"
		var log bytes.Buffer
		var logger logging.LoggerWithWriter
		var hnd *Handler

		it.Before(func() {
			hnd = NewLogHandler(&log)
			hnd.timer = func() time.Time {
				tm, _ := time.Parse(timeFmt, testTime)
				return tm
			}
			logger = NewLogWithWriter(hnd)
		})

		it.After(func() {
			log.Reset()
		})

		it("can enable time in logs", func() {
			hnd.WantTime = true
			logger.Info("test")
			expected := "2019/05/15 01:01:01.000000 \x1b[34mINFO  \x1b[0m test                     \n"
			if expected != log.String() {
				t.Fatalf("actual %q expected %q", log.String(), expected)
			}
		})

		it("it has no time and color by default", func() {
			logger.Info("test")
			expected := "\x1b[34mINFO  \x1b[0m test                     \n"
			if expected != log.String() {
				t.Fatalf("actual %q expected %q", log.String(), expected)
			}
		})

		it("can disable color logs", func() {
			hnd.NoColor = true
			logger.Info("test")
			expected := "INFO   test                     \n"
			if expected != log.String() {
				t.Fatalf("actual %q expected %q", log.String(), expected)
			}
		})

	})
}
