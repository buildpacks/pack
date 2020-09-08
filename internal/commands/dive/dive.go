package dive

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/jroimartin/gocui"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

var (
	once         sync.Once
	appSingleton *App
)

// CreateBuilder creates a builder image, based on a builder config
func Dive(logger logging.Logger, cfg config.Config, client commands.PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dive <image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "interactive exploration of image",
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			imgName := args[0]


			logger.Infof("Building structures for %s\n", imgName)
			diveResult, err := client.Dive(imgName, true)
			if err != nil {
				return err
			}
			// create a GUI
			g, err := gocui.NewGui(gocui.OutputNormal)
			if err != nil {
				return err
			}
			defer g.Close()

			logger.Info("starting app!")

			app, err := NewApp(AppOptions{
				DiveResult: diveResult,
				GUI:        g,
			})
			if err != nil {
				panic(err)
			}


			err = app.Run()
			if err != nil {
				panic(err)
			}

			return nil
		}),
	}
	return cmd
}


//
// Debug methods
//

func repeat(s string, c int) string {
	result := ""
	for i := 0; i < c; i++ {
		result += s
	}
	return result
}

func prettyPrint(input interface{}) (string, error) {
	buf := bytes.NewBuffer(nil)
	_, err := fmt.Fprintf(buf, "%+v", input)
	if err != nil {
		return "", err
	}
	inputString := buf.String()
	result := ""
	indent := 0
	for _, c := range inputString {
		character := string(c)
		if character == "{" || character == "[" {
			thisIndent := repeat("  ", indent)
			indent++
			nextIndent := repeat("  ", indent)
			result += fmt.Sprintf("\n%s%s\n%s", thisIndent, character, nextIndent)
		} else if character == " " {
			result += fmt.Sprintf("\n%s", repeat("  ", indent))
		} else if character == "}" || character == "]" {
			indent--
			nextIndent := repeat("  ", indent)
			result += fmt.Sprintf("\n%s%s%s", nextIndent, character, nextIndent)
		} else {
			result += character
		}
	}
	return result, nil
}
