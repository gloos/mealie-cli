package cli

import (
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/internal/config"
)

// saveProfile creates or updates a profile and persists the config file.
func (f *Factory) saveProfile(name, baseURL, token string, makeCurrent bool) (string, error) {
	path, err := f.configFilePath()
	if err != nil {
		return "", err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return "", err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*config.Profile{}
	}
	prof := cfg.Profiles[name]
	if prof == nil {
		prof = &config.Profile{}
		cfg.Profiles[name] = prof
	}
	if baseURL != "" {
		prof.BaseURL = baseURL
	}
	if token != "" {
		prof.Token = token
	}
	if makeCurrent || cfg.CurrentProfile == "" {
		cfg.CurrentProfile = name
	}
	if err := config.Save(path, cfg); err != nil {
		return "", err
	}
	return path, nil
}

func newConfigCmd(f *Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration and profiles",
	}
	cmd.AddCommand(newConfigPathCmd(f), newConfigListCmd(f), newConfigUseCmd(f), newConfigViewCmd(f))
	return cmd
}

func newConfigPathCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := f.Printer()
			if err != nil {
				return err
			}
			path, err := f.configFilePath()
			if err != nil {
				return err
			}
			return p.Emit(map[string]string{"path": path}, func(w io.Writer) error {
				fmt.Fprintln(w, path)
				return nil
			})
		},
	}
}

type profileInfo struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	Current  bool   `json:"current"`
	HasToken bool   `json:"has_token"`
}

func newConfigListCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List configured profiles",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := f.Printer()
			if err != nil {
				return err
			}
			path, err := f.configFilePath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			names := make([]string, 0, len(cfg.Profiles))
			for name := range cfg.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)
			infos := make([]profileInfo, 0, len(names))
			for _, name := range names {
				prof := cfg.Profiles[name]
				infos = append(infos, profileInfo{
					Name:     name,
					BaseURL:  prof.BaseURL,
					Current:  name == cfg.CurrentProfile,
					HasToken: prof.Token != "" || prof.TokenEnv != "",
				})
			}
			return p.Emit(infos, func(w io.Writer) error {
				tw := newTable(w, "CURRENT", "PROFILE", "SERVER", "TOKEN")
				for _, info := range infos {
					marker := ""
					if info.Current {
						marker = "*"
					}
					token := "no"
					if info.HasToken {
						token = "yes"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", marker, info.Name, dash(info.BaseURL), token)
				}
				return tw.Flush()
			})
		},
	}
}

func newConfigUseCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "use <profile>",
		Short: "Set the active profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := f.Printer()
			if err != nil {
				return err
			}
			name := args[0]
			path, err := f.configFilePath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			if _, ok := cfg.Profiles[name]; !ok {
				return newError(ExitNotFound, "not_found", fmt.Sprintf("no such profile %q", name),
					"run `mealie config list` to see available profiles")
			}
			cfg.CurrentProfile = name
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			p.Info("Active profile set to %q", name)
			return p.Emit(map[string]string{"current_profile": name}, func(w io.Writer) error {
				fmt.Fprintln(w, name)
				return nil
			})
		},
	}
}

func newConfigViewCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "Show the effective configuration after applying flags and environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := f.Printer()
			if err != nil {
				return err
			}
			res, _, path, err := f.resolved()
			if err != nil {
				return err
			}
			view := map[string]any{
				"profile":     res.Profile,
				"base_url":    res.BaseURL,
				"has_token":   res.Token != "",
				"config_path": path,
			}
			return p.Emit(view, func(w io.Writer) error {
				fmt.Fprintf(w, "Profile:     %s\n", dash(res.Profile))
				fmt.Fprintf(w, "Server:      %s\n", dash(res.BaseURL))
				fmt.Fprintf(w, "Token:       %s\n", boolWord(res.Token != "", "configured", "missing"))
				fmt.Fprintf(w, "Config file: %s\n", path)
				return nil
			})
		},
	}
}

func boolWord(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
