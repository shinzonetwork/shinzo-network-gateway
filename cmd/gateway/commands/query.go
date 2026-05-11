package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func (a *App) newQueryCommand() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "query <graphql-query>",
		Short: "sends GraphQL query to Gateway",
		Args:  cobra.ExactArgs(1),
		RunE:  a.query,
	}
	cmd.Flags().String(flagListen, defaultListenAddr, "Gateway HTTP address")
	cmd.Flags().Duration(flagTimeout, defaultTimeout, "Timeout for HTTP request")

	if err := a.v.BindPFlags(cmd.Flags()); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (a *App) query(cmd *cobra.Command, args []string) error {
	url := getEndpointURL(a.v.GetString(flagListen))
	timeout := a.v.GetDuration(flagTimeout)

	type query struct {
		Query string `json:"query"`
	}

	body, err := json.Marshal(query{Query: args[0]})
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/graphql-response+json, application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	return nil
}

func getEndpointURL(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/") + "/graphql"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	return "http://" + addr + "/graphql"
}
