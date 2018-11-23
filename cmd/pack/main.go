package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	"github.com/spf13/cobra"
)

var Version = "UNKNOWN"

func main() {
	rootCmd := &cobra.Command{Use: "pack"}
	for _, f := range [](func() *cobra.Command){
		buildCommand,
		runCommand,
		rebaseCommand,
		createBuilderCommand,
		addStackCommand,
		updateStackCommand,
		deleteStackCommand,
		setDefaultStackCommand,
		setDefaultBuilderCommand,
		versionCommand,
	} {
		rootCmd.AddCommand(f())
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildCommand() *cobra.Command {
	var buildFlags pack.BuildFlags
	buildCommand := &cobra.Command{
		Use:   "build <image-name>",
		Short: "Create runnable app image from source code using buildpacks",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			buildFlags.RepoName = args[0]
			bf, err := pack.DefaultBuildFactory()
			if err != nil {
				return err
			}
			b, err := bf.BuildConfigFromFlags(&buildFlags)
			if err != nil {
				return err
			}
			return b.Run()
		},
	}
	buildCommandFlags(buildCommand, &buildFlags)
	buildCommand.Flags().BoolVar(&buildFlags.Publish, "publish", false, "publish to registry")
	return buildCommand
}

func runCommand() *cobra.Command {
	var runFlags pack.RunFlags
	runCommand := &cobra.Command{
		Use:   "run",
		Short: "Create and immediately run an app image from source code using buildpacks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			bf, err := pack.DefaultBuildFactory()
			if err != nil {
				return err
			}
			r, err := bf.RunConfigFromFlags(&runFlags)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true
			return r.Run(makeStopChannelForSignals)
		},
	}

	buildCommandFlags(runCommand, &runFlags.BuildFlags)
	runCommand.Flags().StringVar(&runFlags.Port, "port", "", "comma separated ports to publish, defaults to ports exposed by the container")
	return runCommand
}

func buildCommandFlags(cmd *cobra.Command, buildFlags *pack.BuildFlags) {
	cmd.Flags().StringVarP(&buildFlags.AppDir, "path", "p", "current working directory", "path to app dir")
	cmd.Flags().StringVar(&buildFlags.Builder, "builder", "", "builder")
	cmd.Flags().StringVar(&buildFlags.RunImage, "run-image", "", "run image")
	cmd.Flags().StringVar(&buildFlags.EnvFile, "env-file", "", "env file")
	cmd.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, "don't pull images before use")
	cmd.Flags().StringArrayVar(&buildFlags.Buildpacks, "buildpack", []string{}, "buildpack ID or host directory path, \n\t\t repeat for each buildpack in order")
}

func rebaseCommand() *cobra.Command {
	var flags pack.RebaseFlags
	cmd := &cobra.Command{
		Use:   "rebase <image-name>",
		Short: "Update an app image to an new underlying stack",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			flags.RepoName = args[0]

			imageFactory, err := image.DefaultFactory()
			if err != nil {
				return err
			}
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			factory := pack.RebaseFactory{
				Log:          log.New(os.Stdout, "", log.LstdFlags),
				Config:       cfg,
				ImageFactory: imageFactory,
			}
			rebaseConfig, err := factory.RebaseConfigFromFlags(flags)
			if err != nil {
				return err
			}
			return factory.Rebase(rebaseConfig)
		},
	}
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "publish to registry")
	cmd.Flags().BoolVar(&flags.NoPull, "no-pull", false, "don't pull images before use")
	return cmd
}

func createBuilderCommand() *cobra.Command {
	flags := pack.CreateBuilderFlags{}
	createBuilderCommand := &cobra.Command{
		Use:   "create-builder <image-name> -b <path-to-builder-toml>",
		Short: "Compose several buildpacks into a builder image",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			flags.RepoName = args[0]

			if runtime.GOOS == "windows" {
				return fmt.Errorf("create builder is not implemented on windows")
			}

			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			imageFactory, err := image.DefaultFactory()
			if err != nil {
				return err
			}
			builderFactory := pack.BuilderFactory{
				FS:           &fs.FS{},
				Log:          log.New(os.Stdout, "", log.LstdFlags),
				Config:       cfg,
				ImageFactory: imageFactory,
			}
			builderConfig, err := builderFactory.BuilderConfigFromFlags(flags)
			if err != nil {
				return err
			}
			return builderFactory.Create(builderConfig)
		},
	}
	createBuilderCommand.Flags().BoolVar(&flags.NoPull, "no-pull", false, "don't pull stack image before use")
	createBuilderCommand.Flags().StringVarP(&flags.BuilderTomlPath, "builder-config", "b", "", "path to builder.toml file")
	createBuilderCommand.Flags().StringVarP(&flags.StackID, "stack", "s", "", "stack ID")
	createBuilderCommand.Flags().BoolVar(&flags.Publish, "publish", false, "publish to registry")
	return createBuilderCommand
}

func addStackCommand() *cobra.Command {
	flags := struct {
		BuildImages []string
		RunImages   []string
	}{}
	addStackCommand := &cobra.Command{
		Use:   "add-stack <stack-name> --run-image=<name> --build-image=<name>",
		Short: "Create a new stack with the provided build and run image(s)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			if err := cfg.Add(config.Stack{
				ID:          args[0],
				BuildImages: flags.BuildImages,
				RunImages:   flags.RunImages,
			}); err != nil {
				return err
			}
			fmt.Printf("%s successfully added\n", args[0])
			return nil
		},
	}
	addStackCommand.Flags().StringSliceVarP(&flags.BuildImages, "build-image", "b", []string{}, "build image to be used for bulder images built with the stack")
	addStackCommand.Flags().StringSliceVarP(&flags.RunImages, "run-image", "r", []string{}, "run image to be used for runnable images built with the stack")
	return addStackCommand
}

func setDefaultStackCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-default-stack <stack-name>",
		Short: "Set the default stack used by `pack create-builder`",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			err = cfg.SetDefaultStack(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%s is now the default stack\n", args[0])
			return nil
		},
	}
}

func setDefaultBuilderCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-default-builder <builder-name>",
		Short: "Set the default builder used by `pack build`",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			err = cfg.SetDefaultBuilder(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Successfully set '%s' as default builder.\n", args[0])
			return nil
		},
	}
}

func updateStackCommand() *cobra.Command {
	flags := struct {
		BuildImages []string
		RunImages   []string
	}{}
	updateStackCommand := &cobra.Command{
		Use:   "update-stack <stack-name> --run-image=<name> --build-image=<name>",
		Short: "Update a stack with the provided versions of build and run image(s)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			if err := cfg.Update(args[0], config.Stack{
				BuildImages: flags.BuildImages,
				RunImages:   flags.RunImages,
			}); err != nil {
				return err
			}
			fmt.Printf("%s successfully updated\n", args[0])
			return nil
		},
	}
	updateStackCommand.Flags().StringSliceVarP(&flags.BuildImages, "build-image", "b", []string{}, "build image to be used for builder images built with the stack")
	updateStackCommand.Flags().StringSliceVarP(&flags.RunImages, "run-image", "r", []string{}, "run image to be used for runnable images built with the stack")
	return updateStackCommand
}

func deleteStackCommand() *cobra.Command {
	addStackCommand := &cobra.Command{
		Use:   "delete-stack <stack-name>",
		Short: "Delete a named stack",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			if err := cfg.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("%s has been successfully deleted\n", args[0])
			return nil
		},
	}
	return addStackCommand
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display the version of the `pack` tool",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("VERSION: %s\n", strings.TrimSpace(Version))
		},
	}
}

func makeStopChannelForSignals() <-chan struct{} {
	sigsCh := make(chan os.Signal, 1)
	stopCh := make(chan struct{}, 1)
	signal.Notify(sigsCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		// convert chan os.Signal to chan struct{}
		for {
			<-sigsCh
			stopCh <- struct{}{}
		}
	}()
	return stopCh
}
