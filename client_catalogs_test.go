package polyester

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForCatalogsReturnsHydrationError(t *testing.T) {
	done := make(chan struct{})
	close(done)
	client := &Client{
		catalogHydrationDone: done,
		catalogLastError:     errors.New("catalog spot hydrate: HTTP 500"),
	}
	err := client.WaitForCatalogs(context.Background())
	if err == nil || !strings.Contains(err.Error(), "catalog spot hydrate") {
		t.Fatalf("want hydration error, got %v", err)
	}
	if client.CatalogsLastError() == nil {
		t.Fatal("CatalogsLastError should expose last error")
	}
}

func TestWaitForCatalogsFailsWhenAPIUnreachable(t *testing.T) {
	client, err := New(Config{
		APIURL:          "http://127.0.0.1:1",
		HydrateCatalogs: true,
		Timeout:         300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = client.WaitForCatalogs(ctx)
	if err == nil {
		t.Fatal("expected WaitForCatalogs error when hydration fails")
	}
	if !strings.Contains(err.Error(), "catalog") {
		t.Fatalf("want catalog error, got %v", err)
	}
}
