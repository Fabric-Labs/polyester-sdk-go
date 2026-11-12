package services

import (
	"testing"

	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// ListChanges filters on the assistant surface, and takes all 11 surfaces at
// once, one per surface.
func TestListChangesAcceptsEverySurfaceIncludingAssistant(t *testing.T) {
	req, err := listChangesRequest(models.ListChangelogChangesOptions{Surfaces: []string{models.ChangelogSurfaceAssistant}})
	if err != nil || len(req.GetSurfaces()) != 1 || req.GetSurfaces()[0] != changelogv1.Surface_ASSISTANT {
		t.Fatalf("req=%+v err=%v", req, err)
	}
	all := make([]string, 0, len(changelogv1.Surface_name))
	for value, name := range changelogv1.Surface_name {
		if value != 0 {
			all = append(all, name)
		}
	}
	if len(all) != maxChangelogSurfaces {
		t.Fatalf("contract has %d surfaces, SDK accepts %d", len(all), maxChangelogSurfaces)
	}
	if _, err := listChangesRequest(models.ListChangelogChangesOptions{Surfaces: all}); err != nil {
		t.Fatalf("every surface: %v", err)
	}
	if _, err := listChangesRequest(models.ListChangelogChangesOptions{Surfaces: append(all, "api")}); err == nil {
		t.Fatal("12 surfaces must be refused")
	}
}
