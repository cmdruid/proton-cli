package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/spf13/cobra"
)

func newAPICmd() *cobra.Command {
	var query []string
	var body string
	cmd := &cobra.Command{
		Use:   "api METHOD PATH",
		Short: "Make an authenticated raw API request",
		Long: `Send a raw authenticated request to the Proton API.

Examples:
  proton-cli api GET /calendar/v1
  proton-cli api GET /drive/volumes
  proton-cli api POST /calendar/v1 --body '{"Name":"Work","Color":"#7272a7","Display":1,"AddressID":"..."}'
  proton-cli api GET /mail/v4/messages --query Page=0 --query PageSize=10`,
		Args: cobra.ExactArgs(2),
		RunE: run([]Step{stepAuth}, func(c *Ctx) error {
			method, path := c.Args[0], c.Args[1]

			q := make(map[string][]string)
			for _, kv := range query {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid --query %q (expected key=value)", kv)
				}
				q[parts[0]] = append(q[parts[0]], parts[1])
			}
			if body != "" && !json.Valid([]byte(body)) {
				return fmt.Errorf("invalid JSON --body")
			}
			req := proton.Request{Method: method, Path: path, Query: q, Body: body}

			resp, err := c.App.API.Do(c.Ctx, req)
			if err != nil {
				var apiErr *proton.APIError
				if errors.As(err, &apiErr) {
					_ = c.R().JSON(apiErr.RawBody)
					return err
				}
				return err
			}
			return c.R().JSON(resp.Body)
		}),
	}
	cmd.Flags().StringArrayVar(&query, "query", nil, "Query parameter (key=value, repeatable)")
	cmd.Flags().StringVar(&body, "body", "", "JSON request body")
	return cmd
}
