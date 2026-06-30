//go:build integration

package integration_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestAddressBookListBooks(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	result := testutil.CallOptional(t, "address_book.list_books", func() (models.AddressBooksList, error) {
		return client.AddressBook.ListBooks(ctx)
	})
	if result.Books == nil {
		t.Fatal("expected books list")
	}
}
