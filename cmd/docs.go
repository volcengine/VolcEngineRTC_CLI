// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/affordance"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/errs"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/meta"
	"github.com/volcengine/VolcEngineRTC_CLI/internal/topicdocs"
)

type topicDocsClient interface {
	Search(context.Context, string, int) (topicdocs.SearchResult, error)
	Fetch(context.Context, string) (topicdocs.FetchResult, error)
	List(context.Context, string, int, int) (topicdocs.ListResult, error)
	Close(context.Context) error
}

var newTopicDocsClient = func() (topicDocsClient, error) {
	return topicdocs.New(meta.Version, topicdocs.WithRetryObserver(func(event topicdocs.RetryEvent) {
		out().Warn("retrying RTC docs %s (attempt %d, status %d, delay %dms)",
			event.Phase, event.Attempt, event.Status, event.DelayMS)
	}))
}

func newDocsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Search and read RTC documentation",
		Long: "Search and read the live RTC documentation index through its read-only MCP endpoint. " +
			"No project configuration or authentication is required; results are not cached or summarized.",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newDocsSearchCmd(), newDocsFetchCmd(), newDocsListCmd())
	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"You need current RTC documentation before implementing or diagnosing RTC behavior"},
		Avoid:  []string{"You need an offline answer or an SDK runtime error-code explanation"},
		Prereq: []string{"Network access to the built-in RTC documentation service"},
		Examples: []string{
			meta.BinName + " docs search \"audio mixing\"",
			meta.BinName + " docs fetch <doc-id>",
			meta.BinName + " docs list --query audio",
		},
	})
	return cmd
}

func newDocsSearchCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search RTC documents and preserve upstream ranking",
		Args:  exactlyOneDocsArg("search", "query", "<query>"),
		RunE: func(c *cobra.Command, args []string) error {
			client, err := newTopicDocsClient()
			if err != nil {
				return err
			}
			defer closeTopicDocsClient(client)
			result, err := client.Search(c.Context(), args[0], limit)
			if err != nil {
				return err
			}
			return out().Data(result)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, fmt.Sprintf("maximum results to return locally (1-%d)", topicdocs.MaxSearchLimit))
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"You know the concept or symptom but not the RTC document id"},
		Avoid:    []string{"You already have an exact document id; use docs fetch"},
		Prereq:   []string{"None beyond network access; --limit is applied locally after upstream ranking"},
		Examples: []string{meta.BinName + " docs search \"publish audio\"", meta.BinName + " docs search \"token\" --limit 5 --format json"},
	})
	return cmd
}

func newDocsFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <doc-id>",
		Short: "Fetch one RTC document as exact Markdown",
		Args:  exactlyOneDocsArg("fetch", "doc-id", "<doc-id>"),
		RunE: func(c *cobra.Command, args []string) error {
			client, err := newTopicDocsClient()
			if err != nil {
				return err
			}
			defer closeTopicDocsClient(client)
			result, err := client.Fetch(c.Context(), args[0])
			if err != nil {
				return err
			}
			return out().Data(result)
		},
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"You have an exact id from docs search/list and need the authoritative Markdown"},
		Avoid:    []string{"You only have keywords; use docs search first"},
		Prereq:   []string{"An exact RTC document id"},
		Examples: []string{meta.BinName + " docs fetch <doc-id>", meta.BinName + " docs fetch <doc-id> --format json"},
	})
	return cmd
}

func newDocsListCmd() *cobra.Command {
	var query string
	var offset, limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the RTC document index with local filtering and paging",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, err := newTopicDocsClient()
			if err != nil {
				return err
			}
			defer closeTopicDocsClient(client)
			result, err := client.List(c.Context(), query, offset, limit)
			if err != nil {
				return err
			}
			return out().Data(result)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "case-insensitive title/summary filter applied locally")
	cmd.Flags().IntVar(&offset, "offset", 0, "zero-based local result offset")
	cmd.Flags().IntVar(&limit, "limit", 20, fmt.Sprintf("maximum local page size (1-%d)", topicdocs.MaxListLimit))
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"You need to browse document ids or inspect a category without ranking"},
		Avoid:    []string{"You need relevance-ranked results; use docs search"},
		Prereq:   []string{"None beyond network access; filtering and paging are local"},
		Examples: []string{meta.BinName + " docs list", meta.BinName + " docs list --query audio --offset 20 --limit 20"},
	})
	return cmd
}

func exactlyOneDocsArg(action, param, placeholder string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errs.New("vertc.docs.invalid_argument", errs.TypeValidation,
				"docs %s requires exactly one argument: %s", action, placeholder).
				WithParam(param).
				WithHint("run `%s docs %s --help`", meta.BinName, action)
		}
		return nil
	}
}

func closeTopicDocsClient(client topicDocsClient) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Close(ctx); err != nil {
		out().Warn("could not clean up RTC docs session")
	}
}
