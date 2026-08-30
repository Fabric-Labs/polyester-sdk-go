//go:build integration

package integration_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
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

func TestAddressBookGetView(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	view := testutil.CallOptional(t, "address_book.get_view", func() (models.AddressBookView, error) {
		return client.AddressBook.GetView(ctx, nil, nil, 10, 0)
	})
	if view.Raw == nil {
		t.Fatal("expected address book view")
	}
}

func TestAddressBookCreateEntryNewTagsAndAppend(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	_ = testutil.CallOptional(t, "address_book.list_books", func() (models.AddressBooksList, error) {
		return client.AddressBook.ListBooks(ctx)
	})
	_ = testutil.CallOptional(t, "address_book.get_view", func() (models.AddressBookView, error) {
		return client.AddressBook.GetView(ctx, nil, nil, 10, 0)
	})

	chainID := liveExternalChainID(t, client, ctx)
	label := fmt.Sprintf("sdk-go-ab-%d", time.Now().UnixNano())
	addr := uniqueHexAddress()
	stamp := time.Now().UnixNano() % 1_000_000
	createName := fmt.Sprintf("g%d", stamp)
	appendName := fmt.Sprintf("h%d", stamp)

	entry := testutil.CallOptional(t, "address_book.create_entry", func() (models.AddressBookEntry, error) {
		return client.AddressBook.CreateEntry(ctx, nil, nil, label, "", &addr, &chainID, nil, nil, []models.AddressBookTagInput{{Name: createName}})
	})

	entryID := entry.AddressBookEntryID
	leftover := tagIDs(entry.Tags)
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if entryID != "" {
			_ = client.AddressBook.DeleteEntry(cleanupCtx, entryID)
		}
		for _, id := range leftover {
			_ = client.AddressBook.DeleteTag(cleanupCtx, id)
		}
	})

	if !hasTagName(entry.Tags, createName) {
		t.Fatalf("create tags=%+v want name %q", entry.Tags, createName)
	}

	updated := testutil.CallOptional(t, "address_book.update_entry", func() (models.AddressBookEntry, error) {
		return client.AddressBook.UpdateEntry(ctx, entry.AddressBookEntryID, entry.Revision, nil, nil, nil, &[]models.AddressBookTagInput{{Name: appendName}})
	})
	leftover = tagIDs(updated.Tags)
	if !hasTagName(updated.Tags, createName) || !hasTagName(updated.Tags, appendName) {
		t.Fatalf("update tags=%+v want %q and %q", updated.Tags, createName, appendName)
	}
}

func TestSocialStartAcceptsAtHandle(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	handle := fmt.Sprintf("@t%d", time.Now().Unix()%1_000_000_000)
	_ = testutil.CallOptional(t, "social.start", func() (models.ApiData, error) {
		return client.SocialVerification.Start(ctx, "twitter", "profile", handle)
	})
}

func liveExternalChainID(t *testing.T, client *polyester.Client, ctx context.Context) uint32 {
	t.Helper()
	cfg := testutil.CallOptional(t, "zipper.get_deposit_withdraw_config", func() (models.DepositWithdrawConfig, error) {
		return client.Zipper.GetDepositWithdrawConfig(ctx)
	})
	for _, c := range cfg.Chains {
		if c.ChainID != 0 && (cfg.PolyesterChainID == 0 || c.ChainID != cfg.PolyesterChainID) {
			return c.ChainID
		}
	}
	for _, c := range cfg.Chains {
		if c.ChainID != 0 {
			return c.ChainID
		}
	}
	testutil.SoftSkip(t, "no zipper chain id for address-book create")
	return 0
}

func uniqueHexAddress() string {
	var b [20]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "0x" + hex.EncodeToString(b[:])
}

func hasTagName(tags []models.AddressBookTag, name string) bool {
	for _, tag := range tags {
		if tag.Name == name {
			return true
		}
	}
	return false
}

func tagIDs(tags []models.AddressBookTag) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag.TagID != "" {
			out = append(out, tag.TagID)
		}
	}
	return out
}
