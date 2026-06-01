package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/internal/config"
	"github.com/gloos/mealie-cli/pkg/core"
)

func newAuthCmd(f *Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate against a Mealie server",
	}
	cmd.AddCommand(newAuthLoginCmd(f), newAuthStatusCmd(f), newAuthLogoutCmd(f))
	return cmd
}

func newAuthLoginCmd(f *Factory) *cobra.Command {
	var username, password, tokenName string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in and store credentials in a profile",
		Long: "Log in to a Mealie server and save the connection in a profile.\n\n" +
			"Provide an existing long-lived API token with --token, or log in with\n" +
			"--username/--password to mint one automatically. The server URL comes from\n" +
			"--url, the active profile, or an interactive prompt.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p, err := f.Printer()
			if err != nil {
				return err
			}

			if password != "" {
				fmt.Fprintln(f.Err, "warning: --password on the command line is visible to other users "+
					"via the process list and shell history; prefer interactive entry.")
			}
			if f.opts.token != "" {
				fmt.Fprintln(f.Err, "warning: --token on the command line is visible to other users "+
					"via the process list and shell history; prefer MEALIE_TOKEN or interactive entry.")
			}

			baseURL := f.opts.url
			if baseURL == "" {
				if res, _, _, rerr := f.resolved(); rerr == nil {
					baseURL = res.BaseURL
				}
			}
			if baseURL == "" {
				baseURL, err = f.promptLine("Mealie server URL: ")
				if err != nil {
					return err
				}
			}
			normURL, err := config.NormaliseBaseURL(baseURL)
			if err != nil {
				return usageError(err.Error())
			}
			if normURL == "" {
				return usageError("a server URL is required (use --url)")
			}

			// Warn before any credential leaves the machine — covers both the
			// password and token flows, and fires before the interactive password
			// prompt so the user learns the link is unencrypted before typing it.
			f.warnInsecureCredentials(normURL)

			token := f.opts.token
			if token == "" && username == "" && !f.opts.noInput {
				token, err = f.promptPassword("API token (press Enter to log in with username/password instead): ")
				if err != nil {
					return err
				}
			}

			if token == "" {
				if username == "" {
					username, err = f.promptLine("Username: ")
					if err != nil {
						return err
					}
				}
				if password == "" {
					password, err = f.promptPassword("Password: ")
					if err != nil {
						return err
					}
				}
				loginClient, cerr := core.New(normURL, "", f.coreOptions()...)
				if cerr != nil {
					return cerr
				}
				token, err = loginClient.Login(ctx, username, password, tokenName)
				if err != nil {
					return err
				}
			}

			client, cerr := core.New(normURL, token, f.coreOptions()...)
			if cerr != nil {
				return cerr
			}
			user, err := client.Whoami(ctx)
			if err != nil {
				return err
			}

			profileName := firstNonEmpty(f.opts.profile, config.DefaultProfileName)
			path, err := f.saveProfile(profileName, normURL, token, true)
			if err != nil {
				return err
			}

			p.Info("Logged in as %s on %s (profile %q, saved to %s)", user.Username, normURL, profileName, path)
			return p.Emit(map[string]any{
				"profile":  profileName,
				"base_url": normURL,
				"username": user.Username,
				"admin":    user.Admin,
			}, func(w io.Writer) error {
				fmt.Fprintf(w, "Logged in as %s\n", user.Username)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "", "username for password login")
	cmd.Flags().StringVar(&password, "password", "", "password for password login (prefer interactive entry)")
	cmd.Flags().StringVar(&tokenName, "name", "mealie-cli", "name for the long-lived API token minted on password login")
	return cmd
}

func newAuthStatusCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current authentication status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			client, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			user, err := client.Whoami(ctx)
			if err != nil {
				return err
			}
			res, _, _, _ := f.resolved()
			return p.Emit(map[string]any{
				"profile":  res.Profile,
				"base_url": res.BaseURL,
				"username": user.Username,
				"email":    user.Email,
				"admin":    user.Admin,
			}, func(w io.Writer) error {
				fmt.Fprintf(w, "Profile:  %s\n", res.Profile)
				fmt.Fprintf(w, "Server:   %s\n", res.BaseURL)
				fmt.Fprintf(w, "User:     %s\n", user.Username)
				if user.Email != "" {
					fmt.Fprintf(w, "Email:    %s\n", user.Email)
				}
				fmt.Fprintf(w, "Admin:    %t\n", user.Admin)
				return nil
			})
		},
	}
}

func newAuthLogoutCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored token for the active profile",
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
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			res, _, _, _ := f.resolved()
			prof := cfg.Profiles[res.Profile]
			if prof == nil || prof.Token == "" {
				p.Info("No stored token for profile %q", res.Profile)
			} else {
				prof.Token = ""
				if err := config.Save(path, cfg); err != nil {
					return err
				}
				p.Info("Removed stored token for profile %q", res.Profile)
			}
			return p.Emit(map[string]string{"profile": res.Profile, "status": "logged_out"}, func(w io.Writer) error {
				fmt.Fprintf(w, "Logged out of profile %q\n", res.Profile)
				return nil
			})
		},
	}
}
