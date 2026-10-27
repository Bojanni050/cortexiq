// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/auth"
	"github.com/urfave/cli/v3"
)

// tokenSecretBytes is the entropy behind one token. 32 bytes is the same width
// as the digests the server compares with, so a token is not the weakest part
// of the check.
const tokenSecretBytes = 32

// tokenNewCmd prints a token and the configuration line it belongs in. It is
// the entry point the rest of the flow refers to: connect needs a secret, and
// the server needs the same secret in CORTEXIQ_API_TOKENS.
func tokenNewCmd(_ context.Context, cmd *cli.Command) error {
	name := strings.TrimSpace(cmd.String("name"))
	if name == "" {
		return errors.New("--name is required: it is what the server logs and what tells one app's writes from another's")
	}
	namespace := strings.TrimSpace(cmd.String("namespace"))
	if namespace != "" {
		return errors.New("--namespace is gone on purpose: a token is per app, and an app works in more than one project. The client is asked for the namespace on every write and finds it with list_namespaces.")
	}
	scope := strings.TrimSpace(cmd.String("scope"))
	switch scope {
	case "", "rw", "ro":
	default:
		return fmt.Errorf("--scope %q: want rw or ro", scope)
	}

	secret, err := newTokenSecret()
	if err != nil {
		return err
	}

	entry := auth.Token{Name: name, Secret: secret, Scope: scope}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	fmt.Println("Token (shown once; it is not stored anywhere):")
	fmt.Println()
	fmt.Println("  " + secret)
	fmt.Println()
	fmt.Println("Add this entry to CORTEXIQ_API_TOKENS on the server, then restart it:")
	fmt.Println()
	fmt.Println("  " + string(line))
	fmt.Println()
	fmt.Println("The token carries no namespace. A client works in more than one project, so it is")
	fmt.Println("asked where each memory belongs and answers with list_namespaces.")
	fmt.Println()
	fmt.Println("Point a client at the server with:")
	fmt.Println()
	fmt.Printf("  cortexiq connect --token %s --url <your server>/mcp\n", secret)
	return nil
}

// newTokenSecret returns a URL-safe secret. The alphabet matters: a token ends
// up in shell commands, JSON and HTTP headers, and base64url survives all three
// without quoting.
func newTokenSecret() (string, error) {
	buf := make([]byte, tokenSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
