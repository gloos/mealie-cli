package cli

import (
	"fmt"
	"io"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/internal/buildinfo"
)

type versionInfo struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	BuildDate    string `json:"build_date"`
	GoVersion    string `json:"go_version"`
	Platform     string `json:"platform"`
	TestedMealie string `json:"tested_mealie_version"`
	MinMealie    string `json:"min_mealie_version"`
}

func newVersionCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version and Mealie compatibility information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := f.Printer()
			if err != nil {
				return err
			}
			info := versionInfo{
				Version:      buildinfo.Version,
				Commit:       buildinfo.Commit,
				BuildDate:    buildinfo.Date,
				GoVersion:    runtime.Version(),
				Platform:     runtime.GOOS + "/" + runtime.GOARCH,
				TestedMealie: buildinfo.TestedMealieVersion,
				MinMealie:    buildinfo.MinMealieVersion,
			}
			return p.Emit(info, func(w io.Writer) error {
				fmt.Fprintf(w, "mealie %s\n", info.Version)
				fmt.Fprintf(w, "  commit:         %s\n", info.Commit)
				fmt.Fprintf(w, "  built:          %s\n", info.BuildDate)
				fmt.Fprintf(w, "  go:             %s\n", info.GoVersion)
				fmt.Fprintf(w, "  platform:       %s\n", info.Platform)
				fmt.Fprintf(w, "  tested against: Mealie %s (min %s)\n", info.TestedMealie, info.MinMealie)
				return nil
			})
		},
	}
}
