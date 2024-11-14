package sso

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
	"gopkg.in/ini.v1"
)

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type CacheFile struct {
	StartURL    string    `json:"startUrl"`
	Region      string    `json:"region"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type Profile struct {
	Name         string
	SSOAccountID string
	SSORoleName  string
}

func UpdateConfig(configFile, startURL, ssoSessionName, ssoRegion, defaultRegion string) error {
	fmt.Println("Initialize config")
	configDir := filepath.Dir(configFile)
	// Create the AWS config dir if it doesn't exist
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		return err
	}
	// Clean up any old SSO cache
	fmt.Println("Remove SSO cache")
	ssoCache := filepath.Join(configDir, "sso")
	if err := os.RemoveAll(ssoCache); err != nil {
		return err
	}
	// Create initial config file
	fmt.Printf("Write config: %s\n", configFile)
	cfg := ini.Empty()
	ssoSession := cfg.Section(fmt.Sprintf("sso-session %s", ssoSessionName))
	ssoSession.Key("sso_region").SetValue(ssoRegion)
	ssoSession.Key("sso_start_url").SetValue(startURL)
	if err := cfg.SaveTo(configFile); err != nil {
		return err
	}
	// Do the initial login
	fmt.Println("Login")
	if err := login(ssoSessionName); err != nil {
		return err
	}
	// Get the access token
	fmt.Println("Fetch access token")
	token, err := getAccessToken(configDir, ssoSessionName)
	if err != nil {
		return err
	}
	// Create SSO client
	fmt.Println("Create SSO client")
	c := aws.NewConfig()
	c.Region = ssoRegion
	svc := sso.NewFromConfig(*c)
	// Create a slice to store profiles
	profiles := []Profile{}
	// Get a list of the AWS accounts
	fmt.Printf("Get AWS accounts & roles...\n\n")
	accounts, err := getAccounts(svc, token)
	if err != nil {
		return err
	}
	// For each account
	for _, a := range accounts {
		// Get a list of the AWS roles
		fmt.Printf("Account: %s (%s)\n", *a.AccountName, *a.AccountId)
		roles, err := getRoles(svc, token, *a.AccountId)
		if err != nil {
			return err
		}
		// Create a profile for each role
		sanitizedAccountName := nonAlphanumericRegex.ReplaceAllString(*a.AccountName, "_")
		fmt.Printf("Roles:\n")
		for _, r := range roles {
			fmt.Printf("  - %s\n", *r.RoleName)
			profiles = append(profiles, Profile{
				Name:         fmt.Sprintf("%s/%s", sanitizedAccountName, *r.RoleName),
				SSOAccountID: *a.AccountId,
				SSORoleName:  *r.RoleName,
			})
		}
		fmt.Println()
	}
	// Add each profile to the config
	for _, p := range profiles {
		s := cfg.Section(fmt.Sprintf("profile %s", p.Name))
		s.Key("sso_session").SetValue(ssoSessionName)
		s.Key("sso_account_id").SetValue(p.SSOAccountID)
		s.Key("sso_role_name").SetValue(p.SSORoleName)
		s.Key("region").SetValue(defaultRegion)
		fmt.Printf("Added profile '%s' to config\n", p.Name)
	}
	fmt.Println("Saving config to:", configFile)
	return cfg.SaveTo(configFile)
}

func login(ssoSessionName string) error {
	cmd := exec.Command("aws", "sso", "login", "--sso-session", ssoSessionName)
	cmd.Env = []string{}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getAccessToken(configDir, key string) (string, error) {
	s := sha1.Sum([]byte(key))
	sha := hex.EncodeToString(s[:])
	filename := fmt.Sprintf("%s.json", sha)
	tokenFile := filepath.Join(configDir, "sso", "cache", filename)
	b, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}
	cacheFile := &CacheFile{}
	if err := json.Unmarshal(b, cacheFile); err != nil {
		return "", err
	}
	return cacheFile.AccessToken, nil
}

func getAccounts(svc *sso.Client, token string) ([]types.AccountInfo, error) {
	accounts := []types.AccountInfo{}
	paginator := sso.NewListAccountsPaginator(
		svc,
		&sso.ListAccountsInput{
			AccessToken: &token,
		},
	)
	ctx := context.Background()
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, out.AccountList...)
	}
	return accounts, nil
}

func getRoles(svc *sso.Client, token, accountID string) ([]types.RoleInfo, error) {
	roles := []types.RoleInfo{}
	paginator := sso.NewListAccountRolesPaginator(
		svc,
		&sso.ListAccountRolesInput{
			AccessToken: &token,
			AccountId:   &accountID,
		},
	)
	ctx := context.Background()
	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		roles = append(roles, out.RoleList...)
	}
	return roles, nil
}
