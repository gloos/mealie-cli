package core

import (
	"context"
	"fmt"
)

const (
	// defaultPageSize is the per-request batch size FetchAll uses when the caller
	// does not specify one. It is deliberately bounded so a large instance is
	// walked in steady chunks rather than fetched as one giant page.
	defaultPageSize = 100
	// maxPages is a runaway guard: a misbehaving server that never signals the end
	// of a result set cannot drive FetchAll into an unbounded request loop.
	maxPages = 100_000
)

// FetchAll walks a paginated endpoint to completion and returns every item. It
// is the bounded, client-side replacement for the perPage=-1 "no pagination"
// sentinel: instead of asking the server for everything in a single response
// (which a large instance can fail to deliver under the response-size cap), it
// fetches steady pages until the result set is exhausted.
//
// pageSize is the per-request batch size: 0 falls back to defaultPageSize, and a
// value < 1 (such as the old -1 sentinel) is rejected with an error rather than
// being passed back to the server. fetch is invoked once per 1-based page with
// the resolved batch size and must return that page; any error it returns aborts
// the whole walk and is propagated unchanged.
func FetchAll[T any](ctx context.Context, pageSize int, fetch func(page, perPage int) (*Page[T], error)) ([]T, error) {
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	if pageSize < 1 {
		return nil, fmt.Errorf("page size must be a positive batch size, got %d", pageSize)
	}

	var all []T
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if page > maxPages {
			return nil, fmt.Errorf("pagination exceeded %d pages without completing; aborting", maxPages)
		}

		res, err := fetch(page, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, res.Items...)

		// Stop conditions, most reliable first. The spec populates Total on every
		// list response, so the count-based stop is preferred; TotalPages is the
		// fallback. The len(Items) < pageSize heuristic is used only when the
		// server reports neither total, so an exact-multiple final page never
		// triggers a needless out-of-range request.
		switch {
		case len(res.Items) == 0:
			return all, nil
		case res.Total > 0 && len(all) >= res.Total:
			return all, nil
		case res.TotalPages > 0 && page >= res.TotalPages:
			return all, nil
		case res.Total == 0 && res.TotalPages == 0 && len(res.Items) < pageSize:
			return all, nil
		}
	}
}
