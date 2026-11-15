package polyester

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	// Every generated package of the public contract, so the walk below sees
	// every service.
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/ai/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/analytics/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/deposit/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/checkpoints/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/claims/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/fees/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/notifications/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/api"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/api/validation/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/ratelimit/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/rewards/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/swap/quickswap/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/swap/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/tradews/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/vip/v1"
)

// replyIDFields walks the reply message of every RPC in the public contract,
// recursively, and returns every non-repeated scalar field named "id" or
// ending in "_id".
func replyIDFields() map[string]bool {
	out := map[string]bool{}
	seen := map[protoreflect.FullName]bool{}
	var walk func(md protoreflect.MessageDescriptor)
	walk = func(md protoreflect.MessageDescriptor) {
		if seen[md.FullName()] || strings.HasPrefix(string(md.FullName()), "google.") {
			return
		}
		seen[md.FullName()] = true
		fields := md.Fields()
		for i := 0; i < fields.Len(); i++ {
			f := fields.Get(i)
			name := string(f.Name())
			if f.Kind() == protoreflect.MessageKind || f.Kind() == protoreflect.GroupKind {
				walk(f.Message())
				continue
			}
			if !f.IsList() && !f.IsMap() && (name == "id" || strings.HasSuffix(name, "_id")) {
				out[string(md.FullName())+"."+name] = true
			}
		}
	}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		services := fd.Services()
		for i := 0; i < services.Len(); i++ {
			methods := services.Get(i).Methods()
			for j := 0; j < methods.Len(); j++ {
				walk(methods.Get(j).Output())
			}
		}
		return true
	})
	return out
}

// Every ID field of every reply in the contract is classified in
// testdata/reply_id_inventory.tsv, which the Rust and Python SDKs share: a new
// ID field fails this test until it is listed as required (and its decoder
// refuses 0) or not_required with a reason.
func TestReplyIDInventoryListsEveryReplyIDField(t *testing.T) {
	file, err := os.Open("testdata/reply_id_inventory.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	listed := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 || (parts[1] != "required" && parts[1] != "not_required") || (parts[1] == "not_required" && parts[2] == "") {
			t.Fatalf("malformed inventory line %q", line)
		}
		if listed[parts[0]] {
			t.Fatalf("%s is listed twice", parts[0])
		}
		listed[parts[0]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	walked := replyIDFields()
	var missing, gone []string
	for field := range walked {
		if !listed[field] {
			missing = append(missing, field)
		}
	}
	for field := range listed {
		if !walked[field] {
			gone = append(gone, field)
		}
	}
	sort.Strings(missing)
	sort.Strings(gone)
	if len(missing) > 0 || len(gone) > 0 {
		t.Fatalf("reply ID inventory out of date:\nunlisted: %v\nlisted but not in any reply: %v", missing, gone)
	}
}
