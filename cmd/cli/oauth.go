// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Bojanni050/cortexiq/internal/oauth"
	"github.com/urfave/cli/v3"
)

// oauthClientsListCmd prints every registered OAuth client. It is the answer to
// "who has been granted access to my memory", which is otherwise only visible in
// the database.
func oauthClientsListCmd(ctx context.Context, cmd *cli.Command) error {
	store := getBootstrap(cmd).OAuth.Store()
	clients, err := store.ListClients(ctx)
	if err != nil {
		return err
	}
	if len(clients) == 0 {
		fmt.Println("No OAuth clients registered yet. A client registers itself on its first connection (DCR or CIMD).")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "CLIENT_ID\tNAME\tSOURCE\tAUTH\tSTATUS\tREDIRECT_URIS")
	for _, c := range clients {
		status := "active"
		if c.Disabled {
			status = "disabled"
		}
		redirects := "-"
		if len(c.RedirectURIs) > 0 {
			redirects = strings.Join(c.RedirectURIs, " ")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", c.ID, c.Name, c.Source, c.AuthMethod, status, redirects)
	}
	return w.Flush()
}

// oauthClientsAddCmd pre-registers a client for services that cannot register
// themselves. The secret is printed once and never stored in plaintext.
func oauthClientsAddCmd(ctx context.Context, cmd *cli.Command) error {
	name := strings.TrimSpace(cmd.String("name"))
	if name == "" {
		return errors.New("--name is required: it is what the consent page and the audit log show")
	}
	redirects := cmd.StringSlice("redirect")
	if len(redirects) == 0 {
		return errors.New("--redirect is required: without a registered callback URI the client cannot receive a code")
	}
	authMethod := strings.TrimSpace(cmd.String("auth"))
	switch authMethod {
	case "", "secret", "client_secret_post":
		authMethod = "client_secret_post"
	case "none":
		authMethod = "none"
	default:
		return fmt.Errorf("--auth %q: want none or secret", cmd.String("auth"))
	}

	id, secret, hash, err := oauth.NewClientCredentials(authMethod)
	if err != nil {
		return err
	}
	store := getBootstrap(cmd).OAuth.Store()
	err = store.CreateClient(ctx, oauth.Client{
		ID:           id,
		Name:         name,
		RedirectURIs: redirects,
		AuthMethod:   authMethod,
		SecretHash:   hash,
		Source:       "manual",
	})
	if err != nil {
		return err
	}

	fmt.Println("Client registered:")
	fmt.Println()
	fmt.Println("  client_id:     " + id)
	if secret != "" {
		fmt.Println("  client_secret: " + secret)
		fmt.Println()
		fmt.Println("The secret is shown once. Paste both into the client's MCP/OAuth configuration.")
	} else {
		fmt.Println()
		fmt.Println("This is a public client: it authenticates with PKCE only, no secret.")
	}
	fmt.Println()
	fmt.Println("Registered redirect URIs:")
	for _, r := range redirects {
		fmt.Println("  " + r)
	}
	return nil
}

// oauthClientsDisableCmd stops a client without deleting it, so its tokens stop
// working (the access-token lookup joins on disabled) while the history stays.
func oauthClientsDisableCmd(ctx context.Context, cmd *cli.Command) error {
	return setOAuthClientDisabled(ctx, cmd, true)
}

func oauthClientsEnableCmd(ctx context.Context, cmd *cli.Command) error {
	return setOAuthClientDisabled(ctx, cmd, false)
}

func setOAuthClientDisabled(ctx context.Context, cmd *cli.Command, disabled bool) error {
	id := strings.TrimSpace(cmd.Args().First())
	if id == "" {
		return errors.New("client_id is required")
	}
	store := getBootstrap(cmd).OAuth.Store()
	if err := store.SetClientDisabled(ctx, id, disabled); err != nil {
		if errors.Is(err, oauth.ErrUnknownClient) {
			return fmt.Errorf("no such client: %s", id)
		}
		return err
	}
	if disabled {
		fmt.Printf("Disabled %s. Its existing tokens are rejected from now on.\n", id)
	} else {
		fmt.Printf("Enabled %s.\n", id)
	}
	return nil
}

// oauthTokensListCmd shows issued tokens by metadata only. The tokens
// themselves are stored as digests, so this is the only view that exists.
func oauthTokensListCmd(ctx context.Context, cmd *cli.Command) error {
	store := getBootstrap(cmd).OAuth.Store()
	tokens, err := store.ListTokens(ctx, int(cmd.Int("limit")))
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		fmt.Println("No OAuth tokens issued yet.")
		return nil
	}
	now := time.Now()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "CLIENT\tKIND\tSCOPE\tPRINCIPAL\tEXPIRES\tSTATUS\tLAST_USED")
	for _, t := range tokens {
		status := "active"
		switch {
		case t.RevokedAt != nil:
			status = "revoked"
		case !t.ExpiresAt.After(now):
			status = "expired"
		}
		lastUsed := "-"
		if t.LastUsedAt != nil {
			lastUsed = t.LastUsedAt.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			t.ClientID, t.Kind, t.Scope, t.Principal, t.ExpiresAt.Format("2006-01-02 15:04"), status, lastUsed)
	}
	return w.Flush()
}

// oauthTokensRevokeCmd revokes every token of one client. Revoking a refresh
// token also drops its family, which is what makes a stolen token useless after
// the next rotation.
func oauthTokensRevokeCmd(ctx context.Context, cmd *cli.Command) error {
	clientID := strings.TrimSpace(cmd.String("client"))
	if clientID == "" {
		return errors.New("--client is required: revocation is per client, never global by accident")
	}
	store := getBootstrap(cmd).OAuth.Store()
	n, err := store.RevokeClient(ctx, clientID, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("Revoked %d token(s) for %s.\n", n, clientID)
	return nil
}
