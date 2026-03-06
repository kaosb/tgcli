package tg

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

type Client struct {
	client *telegram.Client
	api    *tg.Client
	sender *message.Sender
	selfID int64
	dir    string
}

type Options struct {
	StoreDir string
}

func New(opts Options) (*Client, error) {
	appID, appHash, err := loadCredentials(opts.StoreDir)
	if err != nil {
		return nil, err
	}

	sessionPath := filepath.Join(opts.StoreDir, "session.json")

	client := telegram.NewClient(appID, appHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{
			Path: sessionPath,
		},
	})

	return &Client{
		client: client,
		dir:    opts.StoreDir,
	}, nil
}

// Run executes fn inside an active Telegram connection.
// Authenticates if a session exists; errors if not authenticated.
func (c *Client) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return c.client.Run(ctx, func(ctx context.Context) error {
		c.api = c.client.API()
		c.sender = message.NewSender(c.api)

		status, err := c.client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("check auth status: %w", err)
		}
		if !status.Authorized {
			return fmt.Errorf("not authenticated; run 'tgcli login' first")
		}

		self, err := c.client.Self(ctx)
		if err != nil {
			return fmt.Errorf("get self: %w", err)
		}
		c.selfID = self.ID

		return fn(ctx)
	})
}

type LoginOptions struct {
	Phone    string
	Code     string
	Password string
}

// Login performs authentication (phone + code + optional 2FA).
// If Phone/Code are empty, prompts interactively via stdin.
func (c *Client) Login(ctx context.Context, opts LoginOptions) error {
	return c.client.Run(ctx, func(ctx context.Context) error {
		c.api = c.client.API()

		// Check if already authenticated.
		status, err := c.client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("check auth status: %w", err)
		}
		if status.Authorized {
			self, _ := c.client.Self(ctx)
			if self != nil {
				fmt.Printf("Already logged in as %s %s (ID: %d)\n", self.FirstName, self.LastName, self.ID)
			}
			return nil
		}

		reader := bufio.NewReader(os.Stdin)

		phone := opts.Phone
		if phone == "" {
			fmt.Print("Phone number (with country code, e.g. +56...): ")
			phone, err = reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("read phone: %w", err)
			}
			phone = strings.TrimSpace(phone)
		}
		if phone == "" {
			return fmt.Errorf("phone number is required")
		}

		a := &interactiveAuth{
			phone:    phone,
			code:     opts.Code,
			password: opts.Password,
			reader:   reader,
		}

		flow := auth.NewFlow(a, auth.SendCodeOptions{})
		if err := flow.Run(ctx, c.client.Auth()); err != nil {
			return fmt.Errorf("auth: %w", err)
		}

		self, err := c.client.Self(ctx)
		if err != nil {
			return fmt.Errorf("get self: %w", err)
		}
		c.selfID = self.ID

		fmt.Printf("Logged in as %s %s (ID: %d)\n", self.FirstName, self.LastName, self.ID)
		return nil
	})
}

// interactiveAuth implements auth.UserAuthenticator with interactive prompts.
// Pre-provided values (from flags) are used first; if empty, prompts stdin.
type interactiveAuth struct {
	phone    string
	code     string
	password string
	reader   *bufio.Reader
}

func (a *interactiveAuth) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (a *interactiveAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	if a.code != "" {
		code := a.code
		a.code = "" // only use once
		return code, nil
	}
	fmt.Print("Verification code (check Telegram): ")
	code, err := a.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read code: %w", err)
	}
	return strings.TrimSpace(code), nil
}

func (a *interactiveAuth) Password(_ context.Context) (string, error) {
	if a.password != "" {
		return a.password, nil
	}
	fmt.Print("2FA password: ")
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read 2FA password: %w", err)
	}
	return strings.TrimSpace(string(pwBytes)), nil
}

func (a *interactiveAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

func (a *interactiveAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up not supported; use an existing Telegram account")
}

// Logout terminates the current session on Telegram's servers and removes local session file.
func (c *Client) Logout(ctx context.Context) error {
	return c.client.Run(ctx, func(ctx context.Context) error {
		c.api = c.client.API()

		status, err := c.client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("check auth status: %w", err)
		}
		if !status.Authorized {
			return fmt.Errorf("not logged in")
		}

		if _, err := c.api.AuthLogOut(ctx); err != nil {
			return fmt.Errorf("logout: %w", err)
		}

		// Remove local session file.
		sessionPath := filepath.Join(c.dir, "session.json")
		os.Remove(sessionPath)

		return nil
	})
}

func (c *Client) SelfID() int64              { return c.selfID }
func (c *Client) API() *tg.Client            { return c.api }
func (c *Client) Sender() *message.Sender    { return c.sender }
func (c *Client) Uploader() *uploader.Uploader { return uploader.NewUploader(c.api) }
func (c *Client) Downloader() *downloader.Downloader { return downloader.NewDownloader() }

// ResolvePeer resolves a chat argument (username, phone, or numeric ID) to an InputPeer.
func (c *Client) ResolvePeer(ctx context.Context, chatArg string) (tg.InputPeerClass, error) {
	// Try as numeric ID first.
	if id, err := strconv.ParseInt(chatArg, 10, 64); err == nil {
		if id > 0 {
			return &tg.InputPeerUser{UserID: id}, nil
		}
		// Negative IDs: groups/channels.
		absID := -id
		if absID > 1000000000000 {
			// -100XXXXXXXXXX format for channels/supergroups.
			channelID := absID - 1000000000000
			return &tg.InputPeerChannel{ChannelID: channelID}, nil
		}
		return &tg.InputPeerChat{ChatID: absID}, nil
	}

	// Try as username (strip leading @).
	username := chatArg
	if len(username) > 0 && username[0] == '@' {
		username = username[1:]
	}

	// Try as phone number.
	if strings.HasPrefix(username, "+") {
		contacts, err := c.api.ContactsGetContacts(ctx, 0)
		if err != nil {
			return nil, fmt.Errorf("get contacts: %w", err)
		}
		if modified, ok := contacts.(*tg.ContactsContacts); ok {
			for _, u := range modified.Users {
				if user, ok := u.(*tg.User); ok && user.Phone == strings.TrimPrefix(username, "+") {
					return &tg.InputPeerUser{
						UserID:     user.ID,
						AccessHash: user.AccessHash,
					}, nil
				}
			}
		}
		return nil, fmt.Errorf("no contact found with phone %s", chatArg)
	}

	resolved, err := c.api.ContactsResolveUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", chatArg, err)
	}

	for _, u := range resolved.Users {
		if user, ok := u.(*tg.User); ok {
			return &tg.InputPeerUser{
				UserID:     user.ID,
				AccessHash: user.AccessHash,
			}, nil
		}
	}
	for _, ch := range resolved.Chats {
		switch v := ch.(type) {
		case *tg.Channel:
			return &tg.InputPeerChannel{
				ChannelID:  v.ID,
				AccessHash: v.AccessHash,
			}, nil
		case *tg.Chat:
			return &tg.InputPeerChat{ChatID: v.ID}, nil
		}
	}

	return nil, fmt.Errorf("could not resolve %q to a peer", chatArg)
}

type config struct {
	AppID   int    `json:"app_id"`
	AppHash string `json:"app_hash"`
}

// loadCredentials tries (in order): environment variables, config file, interactive prompt.
func loadCredentials(storeDir string) (int, string, error) {
	// 1. Environment variables (highest priority).
	appIDStr := os.Getenv("TGCLI_APP_ID")
	appHash := os.Getenv("TGCLI_APP_HASH")
	if appIDStr != "" && appHash != "" {
		appID, err := strconv.Atoi(appIDStr)
		if err != nil {
			return 0, "", fmt.Errorf("TGCLI_APP_ID must be a number: %w", err)
		}
		return appID, appHash, nil
	}

	// 2. Config file.
	configPath := filepath.Join(storeDir, "config.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var cfg config
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.AppID > 0 && cfg.AppHash != "" {
			return cfg.AppID, cfg.AppHash, nil
		}
	}

	// 3. Interactive prompt.
	fmt.Println("Telegram API credentials required (get them at https://my.telegram.org/apps)")
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("App ID: ")
	idLine, err := reader.ReadString('\n')
	if err != nil {
		return 0, "", fmt.Errorf("read app id: %w", err)
	}
	appID, err := strconv.Atoi(strings.TrimSpace(idLine))
	if err != nil {
		return 0, "", fmt.Errorf("app id must be a number: %w", err)
	}

	fmt.Print("App Hash: ")
	hashLine, err := reader.ReadString('\n')
	if err != nil {
		return 0, "", fmt.Errorf("read app hash: %w", err)
	}
	appHash = strings.TrimSpace(hashLine)
	if appHash == "" {
		return 0, "", fmt.Errorf("app hash is required")
	}

	// Save for next time.
	if err := os.MkdirAll(storeDir, 0700); err != nil {
		return 0, "", fmt.Errorf("create store dir: %w", err)
	}
	cfg := config{AppID: appID, AppHash: appHash}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", err)
	} else {
		fmt.Printf("Credentials saved to %s\n", configPath)
	}

	return appID, appHash, nil
}
