package commands

import (
	"os"

	"github.com/buildpack/pack/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

type genCmd struct {
	bc *cobra.Command
}

func GenCommand(logger logging.Logger) *cobra.Command {
	gc := &cobra.Command{
		Use:   "gen",
		Short: "Generate useful stuff",
	}

	gc.AddCommand(newDocCmd())

	return gc
}

type docCmd struct {
	genDocDir string
	cmd       *cobra.Command
}

func newDocCmd() *cobra.Command {
	const gendocFrontmatterTemplate = `---
	date: %s
	title: "%s"
	slug: %s
	url: %s
	---
	`
	dc := docCmd{}
	dc.cmd = &cobra.Command{
		Use:   "docs",
		Short: "Generate markdown docs of pack commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(dc.genDocDir); os.IsNotExist(err) {
				if err := os.MkdirAll(dc.genDocDir, 0777); err != nil {
					return err
				}
			}

			return doc.GenMarkdownTree(cmd.Root(), dc.genDocDir)
		},
	}

	dc.cmd.PersistentFlags().StringVar(&dc.genDocDir, "dir", "/tmp/packdocs/", "the directory to write the doc.")

	return dc.cmd
}
