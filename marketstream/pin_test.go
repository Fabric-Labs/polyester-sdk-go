package marketstream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// The generated decoders, the vendored schema and its lock agree: the lock
// pins the schema file's digest, and the schema's id and version are the
// decoders'. scripts/generate_marketstream_sbe.sh --check regenerates them.
func TestDecodersArePinnedToTheVendoredSchema(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "sbe", "marketstream.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		SchemaFile    string `json:"schemaFile"`
		SchemaID      int    `json:"schemaId"`
		SchemaVersion int    `json:"schemaVersion"`
		SHA256        string `json:"sha256"`
		Source        string `json:"source"`
		Generator     struct {
			Tool    string `json:"tool"`
			Version string `json:"version"`
		} `json:"generator"`
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join("..", lock.SchemaFile))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(schema)
	if got := hex.EncodeToString(digest[:]); got != lock.SHA256 {
		t.Fatalf("%s sha256 %s, lock pins %s", lock.SchemaFile, got, lock.SHA256)
	}
	var root struct {
		ID      int    `xml:"id,attr"`
		Version int    `xml:"version,attr"`
		Package string `xml:"package,attr"`
	}
	if err := xml.Unmarshal(schema, &root); err != nil {
		t.Fatal(err)
	}
	if root.ID != 20 || root.Package != "marketstream" || root.ID != lock.SchemaID || root.Version != lock.SchemaVersion {
		t.Fatalf("schema %+v, lock %d version %d", root, lock.SchemaID, lock.SchemaVersion)
	}
	if int(SchemaID) != root.ID || int(SchemaVersion) != root.Version {
		t.Fatalf("decoders are schema %d version %d, the vendored schema is %d version %d", SchemaID, SchemaVersion, root.ID, root.Version)
	}
	if filepath.Base(lock.SchemaFile) != "marketstream-v"+strconv.Itoa(root.Version)+".xml" {
		t.Fatalf("schema file %s does not name version %d", lock.SchemaFile, root.Version)
	}
	if lock.Generator.Tool != "sbe-all" || lock.Generator.Version == "" {
		t.Fatalf("generator %+v", lock.Generator)
	}
	if lock.Source != "polyester-proto" && lock.Source != "unpublished" {
		t.Fatalf("source %q", lock.Source)
	}
	if os.Getenv("POLYESTER_RELEASE") != "" && lock.Source != "polyester-proto" {
		t.Fatal("a release must generate the decoders from the schema published in polyester-proto")
	}
}
