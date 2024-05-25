package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/fatih/color"
	"github.com/kloudyuk/asso/sso"
	"github.com/spf13/cobra"
)

var red = color.New(color.FgRed)

// Flags
var (
	defaultRegion  string
	force          bool
	ssoRegion      string
	ssoSessionName string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:           "asso START_URL",
	Short:         "Build AWS config file from SSO login",
	SilenceErrors: true,
	SilenceUsage:  true,
	Args:          cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile := config.DefaultSharedConfigFilename()
		if !force {
			if _, err := os.Stat(configFile); err == nil {
				return fmt.Errorf("config found at %s\nuse --force to overwrite", configFile)
			}
		}
		startURL, ok := parseStartURL(args[0])
		if !ok {
			return fmt.Errorf("invalid START_URL: %s", args[0])
		}
		return sso.UpdateConfig(configFile, startURL, ssoSessionName, ssoRegion, defaultRegion)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		red.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&defaultRegion, "region", "r", "us-east-1", "default region to add to profiles")
	rootCmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite config if it already exists")
	rootCmd.Flags().StringVar(&ssoRegion, "sso-region", "us-east-1", "SSO region")
	rootCmd.Flags().StringVar(&ssoSessionName, "sso-session", "default", "SSO session name")
}

func parseStartURL(s string) (string, bool) {
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	u, err = url.Parse(u.String())
	if err != nil {
		return "", false
	}
	if u.Host == "" {
		return "", false
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/start/"
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return u.String(), true
}
