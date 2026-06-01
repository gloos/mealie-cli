package core

import (
	"context"
	"errors"
	"testing"
)

// fakePages turns a list of item batches into a fetch func that serves them by
// 1-based page, populating total/totalPages from the supplied values (zero means
// "not reported", exercising the heuristic stop). It records the pages fetched.
func fakePages(t *testing.T, batches [][]int, total, totalPages int, calls *[]int) func(page, perPage int) (*Page[int], error) {
	t.Helper()
	return func(page, perPage int) (*Page[int], error) {
		*calls = append(*calls, page)
		idx := page - 1
		if idx < 0 || idx >= len(batches) {
			t.Fatalf("fetched out-of-range page %d (have %d batches)", page, len(batches))
		}
		return &Page[int]{Page: page, PerPage: perPage, Total: total, TotalPages: totalPages, Items: batches[idx]}, nil
	}
}

func TestFetchAllWalksAllPages(t *testing.T) {
	var calls []int
	batches := [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8}}
	got, err := FetchAll(context.Background(), 3, fakePages(t, batches, 8, 3, &calls))
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4, 5, 6, 7, 8}
	if !equalInts(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if !equalInts(calls, []int{1, 2, 3}) {
		t.Fatalf("fetched pages = %v, want [1 2 3]", calls)
	}
}

// TestFetchAllExactMultipleStopsByTotal proves the count-based stop fires even
// when the server does not report total_pages, so an exact-multiple final page
// does not provoke a needless out-of-range request.
func TestFetchAllExactMultipleStopsByTotal(t *testing.T) {
	var calls []int
	batches := [][]int{{1, 2, 3}, {4, 5, 6}}
	got, err := FetchAll(context.Background(), 3, fakePages(t, batches, 6, 0, &calls))
	if err != nil {
		t.Fatal(err)
	}
	if !equalInts(got, []int{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("items = %v", got)
	}
	if !equalInts(calls, []int{1, 2}) {
		t.Fatalf("fetched pages = %v, want [1 2] (no out-of-range page 3)", calls)
	}
}

// TestFetchAllStopsOnEmptyPage covers the last-resort path: with neither total
// nor total_pages reported and a full final page, the walk continues until an
// empty page signals the end.
func TestFetchAllStopsOnEmptyPage(t *testing.T) {
	var calls []int
	batches := [][]int{{1, 2}, {}}
	got, err := FetchAll(context.Background(), 2, fakePages(t, batches, 0, 0, &calls))
	if err != nil {
		t.Fatal(err)
	}
	if !equalInts(got, []int{1, 2}) {
		t.Fatalf("items = %v, want [1 2]", got)
	}
	if !equalInts(calls, []int{1, 2}) {
		t.Fatalf("fetched pages = %v, want [1 2]", calls)
	}
}

func TestFetchAllDefaultsPageSize(t *testing.T) {
	var gotPerPage int
	_, err := FetchAll(context.Background(), 0, func(page, perPage int) (*Page[int], error) {
		gotPerPage = perPage
		return &Page[int]{Items: []int{1}, Total: 1}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPerPage != defaultPageSize {
		t.Fatalf("perPage = %d, want default %d", gotPerPage, defaultPageSize)
	}
}

func TestFetchAllRejectsNonPositivePageSize(t *testing.T) {
	_, err := FetchAll[int](context.Background(), -1, func(page, perPage int) (*Page[int], error) {
		t.Fatal("fetch must not be called for an invalid page size")
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected an error for pageSize < 1, got nil")
	}
}

func TestFetchAllPropagatesFetchError(t *testing.T) {
	boom := errors.New("boom")
	_, err := FetchAll(context.Background(), 2, func(page, perPage int) (*Page[int], error) {
		if page == 2 {
			return nil, boom
		}
		return &Page[int]{Items: []int{1, 2}, Total: 100}, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want boom", err)
	}
}

func TestFetchAllRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pages := 0
	_, err := FetchAll(ctx, 2, func(page, perPage int) (*Page[int], error) {
		pages++
		cancel() // cancel mid-walk; the next loop iteration must abort
		return &Page[int]{Items: []int{1, 2}, Total: 100}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if pages != 1 {
		t.Fatalf("fetched %d pages after cancellation, want 1", pages)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
