package commands_test

import (
	"bytes"
	"errors"
	"regexp"
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/commands/fakes"
	"github.com/buildpacks/pack/internal/config"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

var (
	minimalLifecycleDescriptor = builder.LifecycleDescriptor{
		Info: builder.LifecycleInfo{Version: builder.VersionMustParse("3.4")},
		API: builder.LifecycleAPI{
			BuildpackVersion: api.MustParse("1.2"),
			PlatformVersion:  api.MustParse("2.3"),
		},
	}

	expectedLocalRunImages = []config.RunImage{
		{Image: "some/run-image", Mirrors: []string{"first/local", "second/local"}},
	}
	expectedLocalInfo = &pack.BuilderInfo{
		Description: "test-local-builder",
		Stack:       "local-stack",
		RunImage:    "local/image",
		Lifecycle:   minimalLifecycleDescriptor,
	}
	expectedRemoteInfo = &pack.BuilderInfo{
		Description: "test-remote-builder",
		Stack:       "remote-stack",
		RunImage:    "remote/image",
		Lifecycle:   minimalLifecycleDescriptor,
	}
	expectedLocalDisplay  = "Sample output for local builder"
	expectedRemoteDisplay = "Sample output for remote builder"
	expectedBuilderInfo   = commands.SharedBuilderInfo{
		Name:      "default/builder",
		Trusted:   false,
		IsDefault: true,
	}
)

func TestInspectBuilderCommand(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "Commands", testInspectBuilderCommand, spec.Parallel(), spec.Report(report.Terminal{}))
}

const inspectBuilderRemoteOutputSection = `
REMOTE:

Description: Some remote description

Created By:
  Name: Pack CLI
  Version: 1.2.3

Trusted: No

Stack:
  ID: test.stack.id

Lifecycle:
  Version: 6.7.8
  Buildpack APIs:
    Deprecated: (none)
    Supported: 1.2, 2.3
  Platform APIs:
    Deprecated: 0.1, 1.2
    Supported: 4.5

Run Images:
  first/local     (user-configured)
  second/local    (user-configured)
  some/run-image
  first/default
  second/default

Buildpacks:
  ID                     VERSION                        HOMEPAGE
  test.top.nested        test.top.nested.version        
  test.nested            test.nested.version            http://geocities.com/top-bp
  test.bp.one            test.bp.one.version            http://geocities.com/cool-bp
  test.bp.two            test.bp.two.version            
  test.bp.three          test.bp.three.version          

Detection Order:
 └ Group #1:
    ├ test.top.nested@test.top.nested.version    
    │  └ Group #1:
    │     ├ test.nested@test.nested.version    
    │     │  └ Group #1:
    │     │     └ test.bp.one@test.bp.one.version    (optional)
    │     └ test.bp.three@test.bp.three.version      (optional)
    └ test.bp.two                                    (optional)
`

const inspectBuilderLocalOutputSection = `
LOCAL:

Description: Some local description

Created By:
  Name: Pack CLI
  Version: 4.5.6

Trusted: No

Stack:
  ID: test.stack.id

Lifecycle:
  Version: 4.5.6
  Buildpack APIs:
    Deprecated: 4.5, 6.7
    Supported: 8.9, 10.11
  Platform APIs:
    Deprecated: (none)
    Supported: 7.8

Run Images:
  first/local     (user-configured)
  second/local    (user-configured)
  some/run-image
  first/local-default
  second/local-default

Buildpacks:
  ID                     VERSION                        HOMEPAGE
  test.top.nested        test.top.nested.version        
  test.nested            test.nested.version            http://geocities.com/top-bp
  test.bp.one            test.bp.one.version            http://geocities.com/cool-bp
  test.bp.two            test.bp.two.version            
  test.bp.three          test.bp.three.version          

Detection Order:
 └ Group #1:
    ├ test.top.nested@test.top.nested.version    
    │  └ Group #1:
    │     ├ test.nested@test.nested.version    
    │     │  └ Group #1:
    │     │     └ test.bp.one@test.bp.one.version    (optional)
    │     └ test.bp.three@test.bp.three.version      (optional)
    └ test.bp.two                                    (optional)
`

const stackLabelsSection = `
Stack:
  ID: test.stack.id
  Mixins:
    mixin1
    mixin2
    build:mixin3
    build:mixin4
`

const detectionOrderWithDepth = `Detection Order:
 └ Group #1:
    ├ test.top.nested@test.top.nested.version    
    │  └ Group #1:
    │     ├ test.nested@test.nested.version        
    │     └ test.bp.three@test.bp.three.version    (optional)
    └ test.bp.two                                  (optional)`

const detectionOrderWithCycle = `Detection Order:
 ├ Group #1:
 │  ├ test.top.nested@test.top.nested.version
 │  │  └ Group #1:
 │  │     └ test.nested@test.nested.version
 │  │        └ Group #1:
 │  │           └ test.top.nested@test.top.nested.version    [cyclic]
 │  └ test.bp.two                                            (optional)
 └ Group #2:
    └ test.nested@test.nested.version
       └ Group #1:
          └ test.top.nested@test.top.nested.version
             └ Group #1:
                └ test.nested@test.nested.version    [cyclic]
`
const selectDefaultBuilderOutput = `Please select a default builder with:

	pack set-default-builder <builder-image>`

func testInspectBuilderCommand(t *testing.T, when spec.G, it spec.S) {
	var (
		logger logging.Logger
		outBuf bytes.Buffer
		cfg    config.Config
	)

	it.Before(func() {
		cfg = config.Config{
			DefaultBuilder: "default/builder",
			RunImages:      expectedLocalRunImages,
		}
		logger = ilogging.NewLogWithWriters(&outBuf, &outBuf)
	})

	when("InspectBuilder", func() {
		var (
			assert = h.NewAssertionManager(t)
		)

		it("passes output of local and remote builders to correct writer", func() {
			builderInspector := newDefaultBuilderInspector()
			builderWriter := newDefaultBuilderWriter()
			builderWriterFactory := newWriterFactory(returnsForWriter(builderWriter))

			command := commands.InspectBuilder(logger, cfg, builderInspector, builderWriterFactory)
			command.SetArgs([]string{})
			err := command.Execute()
			assert.Nil(err)

			assert.Equal(builderWriter.ReceivedInfoForLocal, expectedLocalInfo)
			assert.Equal(builderWriter.ReceivedInfoForRemote, expectedRemoteInfo)
			assert.Equal(builderWriter.ReceivedBuilderInfo, expectedBuilderInfo)
			assert.Equal(builderWriter.ReceivedLocalRunImages, expectedLocalRunImages)
			assert.Equal(builderWriterFactory.ReceivedForKind, "human-readable")
			assert.Equal(builderInspector.ReceivedForLocalName, "default/builder")
			assert.Equal(builderInspector.ReceivedForRemoteName, "default/builder")
			assert.ContainsF(outBuf.String(), "LOCAL:\n%s", expectedLocalDisplay)
			assert.ContainsF(outBuf.String(), "REMOTE:\n%s", expectedRemoteDisplay)
		})

		when("image name is provided as first arg", func() {
			it("passes that image name to the inspector", func() {
				builderInspector := newDefaultBuilderInspector()
				writer := newDefaultBuilderWriter()
				command := commands.InspectBuilder(logger, cfg, builderInspector, newWriterFactory(returnsForWriter(writer)))
				command.SetArgs([]string{"some/image"})

				err := command.Execute()
				assert.Nil(err)

				assert.Equal(builderInspector.ReceivedForLocalName, "some/image")
				assert.Equal(builderInspector.ReceivedForRemoteName, "some/image")
				assert.Equal(writer.ReceivedBuilderInfo.IsDefault, false)
			})
		})

		when("depth flag is provided", func() {
			it("passes a modifier to the builder inspector", func() {
				builderInspector := newDefaultBuilderInspector()
				command := commands.InspectBuilder(logger, cfg, builderInspector, newDefaultWriterFactory())
				command.SetArgs([]string{"--depth", "5"})

				err := command.Execute()
				assert.Nil(err)

				assert.Equal(builderInspector.CalculatedConfigForLocal.OrderDetectionDepth, 5)
				assert.Equal(builderInspector.CalculatedConfigForRemote.OrderDetectionDepth, 5)
			})
		})

		when("output type is set to json", func() {
			it("passes json to the writer factory", func() {
				writerFactory := newDefaultWriterFactory()
				command := commands.InspectBuilder(logger, cfg, newDefaultBuilderInspector(), writerFactory)
				command.SetArgs([]string{"--output", "json"})

				err := command.Execute()
				assert.Nil(err)

				assert.Equal(writerFactory.ReceivedForKind, "json")
			})
		})

		when("output type is set to toml using the shorthand flag", func() {
			it("passes toml to the writer factory", func() {
				writerFactory := newDefaultWriterFactory()
				command := commands.InspectBuilder(logger, cfg, newDefaultBuilderInspector(), writerFactory)
				command.SetArgs([]string{"-o", "toml"})

				err := command.Execute()
				assert.Nil(err)

				assert.Equal(writerFactory.ReceivedForKind, "toml")
			})
		})

		when("builder inspector returns an error for local builder", func() {
			it("passes that error to the writer to handle appropriately", func() {
				baseError := errors.New("couldn't inspect local")

				builderInspector := newBuilderInspector(errorsForLocal(baseError))
				builderWriter := newDefaultBuilderWriter()
				builderWriterFactory := newWriterFactory(returnsForWriter(builderWriter))

				command := commands.InspectBuilder(logger, cfg, builderInspector, builderWriterFactory)
				command.SetArgs([]string{})
				err := command.Execute()
				assert.Nil(err)

				assert.ErrorWithMessage(builderWriter.ReceivedErrorForLocal, "couldn't inspect local")
			})
		})

		when("builder inspector returns an error remote builder", func() {
			it("passes that error to the writer to handle appropriately", func() {
				baseError := errors.New("couldn't inspect remote")

				builderInspector := newBuilderInspector(errorsForRemote(baseError))
				builderWriter := newDefaultBuilderWriter()
				builderWriterFactory := newWriterFactory(returnsForWriter(builderWriter))

				command := commands.InspectBuilder(logger, cfg, builderInspector, builderWriterFactory)
				command.SetArgs([]string{})
				err := command.Execute()
				assert.Nil(err)

				assert.ErrorWithMessage(builderWriter.ReceivedErrorForRemote, "couldn't inspect remote")
			})
		})

		when("image is trusted", func() {
			it("passes builder info with trusted true to the writer's `Print` method", func() {
				cfg.TrustedBuilders = []config.TrustedBuilder{
					{Name: "trusted/builder"},
				}
				writer := newDefaultBuilderWriter()

				command := commands.InspectBuilder(
					logger,
					cfg,
					newDefaultBuilderInspector(),
					newWriterFactory(returnsForWriter(writer)),
				)
				command.SetArgs([]string{"trusted/builder"})

				err := command.Execute()
				assert.Nil(err)

				assert.Equal(writer.ReceivedBuilderInfo.Trusted, true)
			})
		})

		when("default builder is configured and is the same as specified by the command", func() {
			it("passes builder info with isDefault true to the writer's `Print` method", func() {
				cfg.DefaultBuilder = "the/default-builder"
				writer := newDefaultBuilderWriter()

				command := commands.InspectBuilder(
					logger,
					cfg,
					newDefaultBuilderInspector(),
					newWriterFactory(returnsForWriter(writer)),
				)
				command.SetArgs([]string{"the/default-builder"})

				err := command.Execute()
				assert.Nil(err)

				assert.Equal(writer.ReceivedBuilderInfo.IsDefault, true)
			})
		})

		when("default builder is empty and no builder is specified in command args", func() {
			it("suggests builders and returns a soft error", func() {
				cfg.DefaultBuilder = ""

				command := commands.InspectBuilder(logger, cfg, newDefaultBuilderInspector(), newDefaultWriterFactory())
				command.SetArgs([]string{})

				err := command.Execute()
				assert.Error(err)
				if !errors.Is(err, pack.SoftError{}) {
					t.Fatalf("expect a pack.SoftError, got: %s", err)
				}

				assert.Contains(outBuf.String(), `Please select a default builder with:

	pack set-default-builder <builder-image>`)

				assert.Matches(outBuf.String(), regexp.MustCompile(`Paketo Buildpacks:\s+'paketobuildpacks/builder:base'`))
				assert.Matches(outBuf.String(), regexp.MustCompile(`Paketo Buildpacks:\s+'paketobuildpacks/builder:full'`))
				assert.Matches(outBuf.String(), regexp.MustCompile(`Heroku:\s+'heroku/buildpacks:18'`))
			})
		})

		when("print returns an error", func() {
			it("returns that error", func() {
				baseError := errors.New("couldn't write builder")

				builderWriter := newBuilderWriter(errorsForPrint(baseError))
				command := commands.InspectBuilder(
					logger,
					cfg,
					newDefaultBuilderInspector(),
					newWriterFactory(returnsForWriter(builderWriter)),
				)
				command.SetArgs([]string{})

				err := command.Execute()
				assert.ErrorWithMessage(err, "couldn't write builder")
			})
		})

		when("writer factory returns an error", func() {
			it("returns that error", func() {
				baseError := errors.New("invalid output format")

				writerFactory := newWriterFactory(errorsForWriter(baseError))
				command := commands.InspectBuilder(logger, cfg, newDefaultBuilderInspector(), writerFactory)
				command.SetArgs([]string{})

				err := command.Execute()
				assert.ErrorWithMessage(err, "invalid output format")
			})
		})
	})
}

func newDefaultBuilderInspector() *fakes.FakeBuilderInspector {
	return &fakes.FakeBuilderInspector{
		InfoForLocal:  expectedLocalInfo,
		InfoForRemote: expectedRemoteInfo,
	}
}

func newDefaultBuilderWriter() *fakes.FakeBuilderWriter {
	return &fakes.FakeBuilderWriter{
		PrintForLocal:  expectedLocalDisplay,
		PrintForRemote: expectedRemoteDisplay,
	}
}

func newDefaultWriterFactory() *fakes.FakeBuilderWriterFactory {
	return &fakes.FakeBuilderWriterFactory{
		ReturnForWriter: newDefaultBuilderWriter(),
	}
}

type BuilderWriterModifier func(w *fakes.FakeBuilderWriter)

func errorsForPrint(err error) BuilderWriterModifier {
	return func(w *fakes.FakeBuilderWriter) {
		w.ErrorForPrint = err
	}
}

func newBuilderWriter(modifiers ...BuilderWriterModifier) *fakes.FakeBuilderWriter {
	w := newDefaultBuilderWriter()

	for _, mod := range modifiers {
		mod(w)
	}

	return w
}

type WriterFactoryModifier func(f *fakes.FakeBuilderWriterFactory)

func returnsForWriter(writer commands.BuilderWriter) WriterFactoryModifier {
	return func(f *fakes.FakeBuilderWriterFactory) {
		f.ReturnForWriter = writer
	}
}

func errorsForWriter(err error) WriterFactoryModifier {
	return func(f *fakes.FakeBuilderWriterFactory) {
		f.ErrorForWriter = err
	}
}

func newWriterFactory(modifiers ...WriterFactoryModifier) *fakes.FakeBuilderWriterFactory {
	f := newDefaultWriterFactory()

	for _, mod := range modifiers {
		mod(f)
	}

	return f
}

type BuilderInspectorModifier func(i *fakes.FakeBuilderInspector)

func errorsForLocal(err error) BuilderInspectorModifier {
	return func(i *fakes.FakeBuilderInspector) {
		i.ErrorForLocal = err
	}
}

func errorsForRemote(err error) BuilderInspectorModifier {
	return func(i *fakes.FakeBuilderInspector) {
		i.ErrorForRemote = err
	}
}

func newBuilderInspector(modifiers ...BuilderInspectorModifier) *fakes.FakeBuilderInspector {
	i := newDefaultBuilderInspector()

	for _, mod := range modifiers {
		mod(i)
	}

	return i
}
