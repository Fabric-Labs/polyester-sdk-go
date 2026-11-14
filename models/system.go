package models

import (
	"strings"
	"time"
)

// ServerVersion is the exact build an environment serves, as
// Client.GetVersion reads it.
type ServerVersion struct {
	// Version of the build: a release reads MAJOR.MINOR.PATCH, such as
	// "0.10.0"; a build made after a release, which devnet may run, appends
	// the number of source revisions since that release and "g" with the
	// first 12 hex digits of its revision, such as "0.10.0-12-g4e21a2f09670".
	// It is not a semantic version: do not compare the part after "-" as a
	// pre-release.
	Version string `json:"version"`
	// Commit is the full source revision of the build.
	Commit string `json:"commit"`
	// Environment that answered: "devnet", "testnet", "mainnet", or "local"
	// for a development stack. It stays a plain string, so a value this SDK
	// does not know yet still reads.
	Environment string `json:"environment"`
}

// Release returns the release the build follows: Version up to its first
// "-", such as "0.10.0" for "0.10.0-12-g4e21a2f09670".
func (v ServerVersion) Release() string {
	release, _, _ := strings.Cut(v.Version, "-")
	return release
}

// Status event kinds, as StatusEvent.Kind reads them and
// ListStatusEventsOptions.Kinds takes them.
const (
	// StatusEventKindMaintenance: planned maintenance, announced ahead of its
	// window.
	StatusEventKindMaintenance = "maintenance"
	// StatusEventKindIncident: an unplanned disruption.
	StatusEventKindIncident = "incident"
)

// Status event states, as StatusEvent.State reads them and
// ListStatusEventsOptions.States takes them. A state follows the event's
// window at the time of the response: scheduled before StartTime,
// in_progress until EndTime, completed from EndTime on; a cancelled event is
// cancelled whatever its window.
const (
	StatusEventStateScheduled  = "scheduled"
	StatusEventStateInProgress = "in_progress"
	StatusEventStateCompleted  = "completed"
	StatusEventStateCancelled  = "cancelled"
)

// Incident impacts, as StatusEvent.Impact reads them. Impact is "" for
// maintenance and for an incident not yet assessed.
const (
	// StatusEventImpactDegraded: working, but slower or less reliable than
	// usual.
	StatusEventImpactDegraded = "degraded"
	// StatusEventImpactPartialOutage: unavailable for some users, markets or
	// assets.
	StatusEventImpactPartialOutage = "partial_outage"
	// StatusEventImpactMajorOutage: unavailable.
	StatusEventImpactMajorOutage = "major_outage"
)

// Affected services, as StatusEvent.Services reads them.
const (
	AffectedServiceTrading     = "trading"
	AffectedServiceMarketData  = "market_data"
	AffectedServiceDeposits    = "deposits"
	AffectedServiceWithdrawals = "withdrawals"
	AffectedServiceTransfers   = "transfers"
	AffectedServiceAccounts    = "accounts"
	AffectedServiceAPI         = "api"
	AffectedServiceQuickSwap   = "quickswap"
	AffectedServiceApp         = "app"
)

// ListStatusEventsOptions selects status events for Client.ListStatusEvents
// and Client.StatusEvents. Filters on different fields combine with AND;
// values within one field with OR.
type ListStatusEventsOptions struct {
	// States are StatusEventState* labels. Empty lists current and upcoming
	// events (scheduled and in_progress); the server applies that default.
	States []string
	// Kinds are StatusEventKind* labels. Empty lists every kind.
	Kinds []string
	// Limit is the page size; 0 means the server default of 50, at most 200.
	Limit int
	// PageToken continues from an earlier page's NextPageToken. Reuse it only
	// with the same States and Kinds; the server refuses it with other
	// filters.
	PageToken string
}

// StatusEventsPage is one page of status events, the latest start first.
type StatusEventsPage struct {
	Events []StatusEvent `json:"events"`
	// NextPageToken continues the list; empty on the last page.
	NextPageToken string `json:"next_page_token,omitempty"`
}

// StatusEvent is one scheduled maintenance or incident.
type StatusEvent struct {
	// ID is the event's stable public ID in base58. Match events by ID: an
	// event that changes state between two pages can be missing from both or
	// appear on both.
	ID string `json:"id"`
	// Kind is a StatusEventKind* label.
	Kind string `json:"kind"`
	// State is a StatusEventState* label, at the time of the response.
	State string `json:"state"`
	// Title is a short title, at most 200 characters.
	Title string `json:"title"`
	// Message is the plain-text summary; it can be revised. Progress notes
	// are in Updates.
	Message string `json:"message"`
	// StartTime is when the maintenance begins or the incident began. It no
	// longer changes once the window has started.
	StartTime *time.Time `json:"start_time,omitempty"`
	// EndTime is when the maintenance is expected to end, or when the
	// incident ended or is expected to end. Always set for maintenance; nil
	// for an incident with no expected end yet.
	EndTime *time.Time `json:"end_time,omitempty"`
	// Services are the affected parts of the platform, AffectedService*
	// labels. Never empty.
	Services []string `json:"services"`
	// Markets are the affected market symbols, such as "BTC-USDT", in
	// ascending order; empty when the event is not limited to markets.
	Markets []string `json:"markets,omitempty"`
	// Assets are the affected asset codes, such as "SOL", in ascending order;
	// empty when the event is not limited to assets.
	Assets []string `json:"assets,omitempty"`
	// Impact is a StatusEventImpact* label for an incident; "" for
	// maintenance and for an incident not yet assessed.
	Impact string `json:"impact,omitempty"`
	// UpdatedAt is the latest change to the event, a new progress note
	// included. A state change caused only by time passing does not move it:
	// compare UpdatedAt per ID to keep a banner current.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	// CancelledAt is when the event was cancelled; set only for the cancelled
	// state.
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	// Updates are the progress notes, oldest first; at most 50. Notes are
	// never edited or removed.
	Updates []StatusEventUpdate `json:"updates,omitempty"`
	// DetailsURL is an HTTPS link to a long-form announcement; nil when the
	// event has none.
	DetailsURL *string `json:"details_url,omitempty"`
}

// StatusEventUpdate is one progress note on a status event.
type StatusEventUpdate struct {
	// ID is the note's position on its event: 1 for the first, then
	// increasing by 1.
	ID uint32 `json:"id"`
	// Message is the plain-text note.
	Message string `json:"message"`
	// CreatedAt is when the note was posted.
	CreatedAt *time.Time `json:"created_at,omitempty"`
}
