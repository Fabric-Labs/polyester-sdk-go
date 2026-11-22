package chain

// SendCalls keeps the UserOperation hash once the send may have been accepted.
//
// A local mock plays the chain RPC, the bundler and the paymaster on
// 127.0.0.1 and stores every UserOperation it accepts. It computes each
// operation's hash itself, from the JSON the SDK sent, so the hash the SDK
// reports is checked against what the bundler holds. No chain is contacted;
// the owner key is a throwaway test value.

import (
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const sendCallsOwnerKey = "4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f4f" // throwaway

var sendCallsWithdraw = ChainCall{
	To:    "0x1111111111111111111111111111111111111111",
	Data:  []byte{0xde, 0xad, 0xbe, 0xef},
	Value: big.NewInt(0),
}

func mockHexBytes(t *testing.T, value any) []byte {
	t.Helper()
	text, _ := value.(string)
	out, err := hex.DecodeString(strings.TrimPrefix(text, "0x"))
	if err != nil {
		t.Errorf("mock: bad hex %q", text)
	}
	return out
}

func mockHexInt(t *testing.T, value any) *big.Int {
	t.Helper()
	text, _ := value.(string)
	n, ok := new(big.Int).SetString(strings.TrimPrefix(text, "0x"), 16)
	if !ok {
		t.Errorf("mock: bad quantity %q", text)
		return big.NewInt(0)
	}
	return n
}

func mockWord(hi, lo *big.Int) [32]byte {
	var word [32]byte
	hi.FillBytes(word[:16])
	lo.FillBytes(word[16:])
	return word
}

// mockUserOperationHash is EntryPoint v0.7 getUserOpHash of a JSON
// UserOperation, as a bundler computes it.
func mockUserOperationHash(t *testing.T, op map[string]any, entryPoint string, chainID int64) string {
	t.Helper()
	var initCode []byte
	if factory, ok := op["factory"]; ok {
		initCode = append(mockHexBytes(t, factory), mockHexBytes(t, op["factoryData"])...)
	}
	var paymasterAndData []byte
	if paymaster, ok := op["paymaster"]; ok {
		paymasterAndData = mockHexBytes(t, paymaster)
		paymasterAndData = append(paymasterAndData, common.LeftPadBytes(mockHexInt(t, op["paymasterVerificationGasLimit"]).Bytes(), 16)...)
		paymasterAndData = append(paymasterAndData, common.LeftPadBytes(mockHexInt(t, op["paymasterPostOpGasLimit"]).Bytes(), 16)...)
		paymasterAndData = append(paymasterAndData, mockHexBytes(t, op["paymasterData"])...)
	}
	bytes32 := mustABIType("bytes32")
	packed, err := abi.Arguments{
		{Type: typeAddress}, {Type: typeUint256}, {Type: bytes32}, {Type: bytes32},
		{Type: bytes32}, {Type: typeUint256}, {Type: bytes32}, {Type: bytes32},
	}.Pack(
		common.HexToAddress(op["sender"].(string)),
		mockHexInt(t, op["nonce"]),
		[32]byte(crypto.Keccak256(initCode)),
		[32]byte(crypto.Keccak256(mockHexBytes(t, op["callData"]))),
		mockWord(mockHexInt(t, op["verificationGasLimit"]), mockHexInt(t, op["callGasLimit"])),
		mockHexInt(t, op["preVerificationGas"]),
		mockWord(mockHexInt(t, op["maxPriorityFeePerGas"]), mockHexInt(t, op["maxFeePerGas"])),
		[32]byte(crypto.Keccak256(paymasterAndData)),
	)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := abi.Arguments{{Type: bytes32}, {Type: typeAddress}, {Type: typeUint256}}.Pack(
		[32]byte(crypto.Keccak256(packed)), common.HexToAddress(entryPoint), big.NewInt(chainID),
	)
	if err != nil {
		t.Fatal(err)
	}
	return "0x" + hex.EncodeToString(crypto.Keccak256(outer))
}

// mockBundler is the mock chain RPC + bundler + paymaster and its failure switches.
type mockBundler struct {
	t       *testing.T
	chainID int64
	url     string

	mu         sync.Mutex
	operations []string // hashes of accepted operations, in arrival order
	receipt503 int      // (A) receipt polls answer HTTP 503 this many times
	sendDelay  time.Duration
	sendStatus int // (C) the send is stored, then answered with this HTTP status
	sendError  map[string]any

	storedSendError map[string]any // the send is stored, then answered with this JSON-RPC error
	sendReplySet    bool           // the send is stored, then answered with sendReply
	sendReply       any
	receiptMissing  bool // receipt reads answer null
	receiptDelay    time.Duration
	statusDelay     time.Duration
}

func newMockBundler(t *testing.T) *mockBundler {
	m := &mockBundler{t: t, chainID: PolyesterDevnetEnvironment.ChainID}
	srv := hardening.NewTestServer(http.HandlerFunc(m.serve))
	t.Cleanup(srv.Close)
	m.url = srv.URL
	return m
}

func (m *mockBundler) held() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.operations...)
}

func (m *mockBundler) set(f func(*mockBundler)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f(m)
}

func (m *mockBundler) serve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params []any           `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status, answer, delay := m.handle(req.Method, req.Params)
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	if answer == nil {
		w.WriteHeader(status)
		_, _ = w.Write([]byte("upstream unavailable"))
		return
	}
	answer["jsonrpc"] = "2.0"
	answer["id"] = req.ID
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(answer)
}

func (m *mockBundler) handle(method string, params []any) (int, map[string]any, time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch method {
	case "eth_getCode":
		return 200, map[string]any{"result": "0x"}, 0
	case "eth_call":
		// getNonce(sender, key): sequence 0 under the requested key.
		data := mockHexBytes(m.t, params[0].(map[string]any)["data"])
		key := new(big.Int).SetBytes(data[36:68])
		word := common.LeftPadBytes(new(big.Int).Lsh(key, 64).Bytes(), 32)
		return 200, map[string]any{"result": "0x" + hex.EncodeToString(word)}, 0
	case "pimlico_getUserOperationGasPrice":
		tier := map[string]any{"maxFeePerGas": "0x3b9aca00", "maxPriorityFeePerGas": "0x3b9aca00"}
		return 200, map[string]any{"result": map[string]any{"slow": tier, "standard": tier, "fast": tier}}, 0
	case "pm_sponsorUserOperation":
		return 200, map[string]any{"result": map[string]any{
			"callGasLimit":                  "0x186a0",
			"verificationGasLimit":          "0x30d40",
			"preVerificationGas":            "0xc350",
			"paymasterVerificationGasLimit": "0xea60",
			"paymasterPostOpGasLimit":       "0x1",
			"paymaster":                     "0x3333333333333333333333333333333333333333",
			"paymasterData":                 "0x1234",
		}}, 0
	case "eth_sendUserOperation":
		if m.sendError != nil {
			return 200, map[string]any{"error": m.sendError}, 0
		}
		hash := mockUserOperationHash(m.t, params[0].(map[string]any), params[1].(string), m.chainID)
		m.operations = append(m.operations, hash)
		if m.sendStatus != 0 {
			return m.sendStatus, nil, 0
		}
		if m.storedSendError != nil {
			return 200, map[string]any{"error": m.storedSendError}, 0
		}
		if m.sendReplySet {
			return 200, map[string]any{"result": m.sendReply}, 0
		}
		return 200, map[string]any{"result": hash}, m.sendDelay
	case "eth_getUserOperationReceipt":
		if m.receipt503 > 0 {
			m.receipt503--
			return 503, nil, 0
		}
		for _, hash := range m.operations {
			if hash == params[0] && !m.receiptMissing {
				return 200, map[string]any{"result": map[string]any{
					"userOpHash": hash,
					"success":    true,
					"receipt":    map[string]any{"transactionHash": "0x" + strings.Repeat("ab", 32), "status": "0x1"},
				}}, m.receiptDelay
			}
		}
		return 200, map[string]any{"result": nil}, m.receiptDelay
	case "pimlico_getUserOperationStatus":
		return 200, map[string]any{"result": map[string]any{"status": "not_found"}}, m.statusDelay
	}
	return 200, map[string]any{"error": map[string]any{"code": -32601, "message": "method " + method + " not found"}}, 0
}

func (m *mockBundler) account(t *testing.T, timeout time.Duration) *SmartAccount {
	t.Helper()
	env := PolyesterDevnetEnvironment
	env.RPCURL = m.url
	env.AccountAbstraction.BundlerURL = m.url
	env.AccountAbstraction.PaymasterURL = m.url
	account, err := NewSmartAccount(sendCallsOwnerKey, &env, nil)
	if err != nil {
		t.Fatal(err)
	}
	account.rpc = NewJSONRPCClient(m.url, timeout)
	account.bundler = NewJSONRPCClient(m.url, timeout)
	account.paymaster = NewJSONRPCClient(m.url, timeout)
	return account
}

// assertUnknownWithHash checks that err names the one stored operation's hash
// and reads as outcome unknown, and returns that hash.
func assertUnknownWithHash(t *testing.T, err error, m *mockBundler) string {
	t.Helper()
	held := m.held()
	if len(held) != 1 {
		t.Fatalf("bundler holds %d operations, want 1", len(held))
	}
	if err == nil {
		t.Fatal("SendCalls returned no error")
	}
	if !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("MutationOutcomeUnknown(%v) = false", err)
	}
	if sdkerrors.IsRetryable(err) {
		t.Fatalf("IsRetryable(%v) = true", err)
	}
	var unknown *UserOperationOutcomeUnknownError
	if !stderrors.As(err, &unknown) {
		t.Fatalf("error %T is not a *UserOperationOutcomeUnknownError: %v", err, err)
	}
	if unknown.UserOperationHash != held[0] {
		t.Fatalf("error names hash %s, bundler holds %s", unknown.UserOperationHash, held[0])
	}
	if !strings.Contains(err.Error(), held[0]) {
		t.Fatalf("error text %q does not name the hash", err)
	}
	return held[0]
}

// recoverByWaiting is the documented recovery: wait on the hash, never sign
// a second operation.
func recoverByWaiting(t *testing.T, account *SmartAccount, m *mockBundler, hash string) {
	t.Helper()
	receipt, err := account.WaitForReceipt(hash, 3*time.Second, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForReceipt: %v", err)
	}
	if receipt.UserOperationHash != hash || !receipt.Success {
		t.Fatalf("receipt = %+v", receipt)
	}
	if held := m.held(); len(held) != 1 || held[0] != hash {
		t.Fatalf("bundler holds %v, want only %s", held, hash)
	}
}

func TestSendCallsModeAReceiptPoll503IsRetried(t *testing.T) {
	m := newMockBundler(t)
	m.set(func(m *mockBundler) { m.receipt503 = 1 })
	receipt, err := m.account(t, 2*time.Second).SendCalls([]ChainCall{sendCallsWithdraw}, true, 3*time.Second)
	if err != nil {
		t.Fatalf("SendCalls: %v", err)
	}
	held := m.held()
	if len(held) != 1 || receipt.UserOperationHash != held[0] || !receipt.Success {
		t.Fatalf("receipt %+v, bundler holds %v", receipt, held)
	}
}

func TestSendCallsModeAReceiptPollsFailingPastTheDeadlineKeepTheHash(t *testing.T) {
	m := newMockBundler(t)
	m.set(func(m *mockBundler) { m.receipt503 = 1 << 30 })
	account := m.account(t, 2*time.Second)
	started := time.Now()
	_, err := account.SendCalls([]ChainCall{sendCallsWithdraw}, true, 1500*time.Millisecond)
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("SendCalls took %v", elapsed)
	}
	hash := assertUnknownWithHash(t, err, m)
	m.set(func(m *mockBundler) { m.receipt503 = 0 })
	recoverByWaiting(t, account, m, hash)
}

func TestSendCallsModeBSendAnswerAfterClientTimeoutKeepsTheHash(t *testing.T) {
	m := newMockBundler(t)
	m.set(func(m *mockBundler) { m.sendDelay = 1500 * time.Millisecond })
	account := m.account(t, 500*time.Millisecond)
	_, err := account.SendCalls([]ChainCall{sendCallsWithdraw}, true, 3*time.Second)
	hash := assertUnknownWithHash(t, err, m)
	m.set(func(m *mockBundler) { m.sendDelay = 0 })
	recoverByWaiting(t, account, m, hash)
}

func TestSendCallsModeCSendAnswered504KeepsTheHash(t *testing.T) {
	m := newMockBundler(t)
	m.set(func(m *mockBundler) { m.sendStatus = http.StatusGatewayTimeout })
	account := m.account(t, 2*time.Second)
	_, err := account.SendCalls([]ChainCall{sendCallsWithdraw}, true, 3*time.Second)
	hash := assertUnknownWithHash(t, err, m)
	if status := sdkerrors.AnswerStatus(err); status != http.StatusGatewayTimeout {
		t.Fatalf("AnswerStatus = %d, want 504", status)
	}
	recoverByWaiting(t, account, m, hash)
}

func TestSendCallsWithoutWaitReturnsTheHashTheBundlerHolds(t *testing.T) {
	m := newMockBundler(t)
	receipt, err := m.account(t, 2*time.Second).SendCalls([]ChainCall{sendCallsWithdraw}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if held := m.held(); len(held) != 1 || held[0] != receipt.UserOperationHash {
		t.Fatalf("hash %s, bundler holds %v", receipt.UserOperationHash, held)
	}
}

func TestSendCallsDefiniteRefusalIsNotOutcomeUnknown(t *testing.T) {
	for _, code := range []int{-32602, -32500, -32507, -32601} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			m := newMockBundler(t)
			m.set(func(m *mockBundler) { m.sendError = map[string]any{"code": code, "message": "invalid UserOperation"} })
			_, err := m.account(t, 2*time.Second).SendCalls([]ChainCall{sendCallsWithdraw}, true, 3*time.Second)
			var rpcErr *JSONRPCError
			if !stderrors.As(err, &rpcErr) || rpcErr.Code == nil || *rpcErr.Code != code {
				t.Fatalf("error = %v, want the JSON-RPC refusal %d", err, code)
			}
			if sdkerrors.MutationOutcomeUnknown(err) {
				t.Fatal("a refusal reads as outcome unknown")
			}
			var unknown *UserOperationOutcomeUnknownError
			if stderrors.As(err, &unknown) {
				t.Fatal("a refusal carries an outcome-unknown hash")
			}
			if held := m.held(); len(held) != 0 {
				t.Fatalf("bundler holds %v", held)
			}
		})
	}
}

func TestSendCallsInternalErrorAnswerKeepsTheHash(t *testing.T) {
	m := newMockBundler(t)
	m.set(func(m *mockBundler) { m.sendError = map[string]any{"code": -32603, "message": "internal error"} })
	_, err := m.account(t, 2*time.Second).SendCalls([]ChainCall{sendCallsWithdraw}, true, 3*time.Second)
	if !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("MutationOutcomeUnknown(%v) = false", err)
	}
	var unknown *UserOperationOutcomeUnknownError
	var rpcErr *JSONRPCError
	if !stderrors.As(err, &unknown) || !strings.HasPrefix(unknown.UserOperationHash, "0x") || !stderrors.As(err, &rpcErr) {
		t.Fatalf("error = %v, want the hash and the JSON-RPC error", err)
	}
}

func TestUserOperationHashMatchesEntryPointV07(t *testing.T) {
	// Reference values: viem getUserOperationHash, entryPointVersion "0.7".
	env := PolyesterDevnetEnvironment
	env.ChainID = 31337
	env.AccountAbstraction.EntryPoint.Address = "0x0000000071727De22E5E9d8BAf0edAc6f37da032"
	nonce := new(big.Int).Or(new(big.Int).Lsh(big.NewInt(123456789), 64), big.NewInt(7))
	for _, tc := range []struct {
		name             string
		initCode         []byte
		paymasterAndData []byte
		want             string
	}{
		{
			name:             "factory and paymaster",
			initCode:         append(common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes(), 0xde, 0xad, 0xbe, 0xef),
			paymasterAndData: PackPaymasterAndData("0x3333333333333333333333333333333333333333", big.NewInt(60_000), big.NewInt(100_000), []byte{0x12, 0x34}),
			want:             "0xec0c13a94cb1a6c84a85cf970fe0ce14164dbb8c24db022e46315ed4631aa9cd",
		},
		{name: "bare", want: "0x3a04aade54da78403e340bc7112c76296af9658ec85525ceb7ad9f18021e6c85"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := userOperationHash(SignSafeUserOperationParams{
				Environment:          env,
				Sender:               "0x1111111111111111111111111111111111111111",
				Nonce:                nonce,
				InitCode:             tc.initCode,
				CallData:             []byte{0xab, 0xcd, 0xef, 0x01},
				CallGasLimit:         big.NewInt(100_000),
				VerificationGasLimit: big.NewInt(200_000),
				PreVerificationGas:   big.NewInt(50_000),
				MaxFeePerGas:         big.NewInt(3_000_000_000),
				MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
				PaymasterAndData:     tc.paymasterAndData,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("hash = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSendCallsUnrecognizedErrorCodeAfterStoringKeepsTheHash(t *testing.T) {
	// An HTTP 200 JSON-RPC error whose code is not a defined refusal proves
	// nothing: here the bundler stored the operation before answering it.
	for _, code := range []int{504, 1, -32000, -32099, -32506, -32603} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			m := newMockBundler(t)
			m.set(func(m *mockBundler) { m.storedSendError = map[string]any{"code": code, "message": "gateway timeout"} })
			account := m.account(t, 2*time.Second)
			_, err := account.SendCalls([]ChainCall{sendCallsWithdraw}, true, 3*time.Second)
			hash := assertUnknownWithHash(t, err, m)
			var rpcErr *JSONRPCError
			if !stderrors.As(err, &rpcErr) || rpcErr.Code == nil || *rpcErr.Code != code {
				t.Fatalf("error %v does not keep the JSON-RPC error %d", err, code)
			}
			recoverByWaiting(t, account, m, hash)
		})
	}
}

func TestSendCallsAnswerNamingAnotherHashKeepsTheComputedHash(t *testing.T) {
	for _, reply := range []any{"0x" + strings.Repeat("cd", 32), "", 7, nil} {
		for _, wait := range []bool{true, false} {
			t.Run(fmt.Sprintf("%v/wait=%v", reply, wait), func(t *testing.T) {
				m := newMockBundler(t)
				m.set(func(m *mockBundler) { m.sendReplySet, m.sendReply = true, reply })
				account := m.account(t, 2*time.Second)
				_, err := account.SendCalls([]ChainCall{sendCallsWithdraw}, wait, 3*time.Second)
				hash := assertUnknownWithHash(t, err, m)
				if reply == hash {
					t.Fatal("the error names the answer's hash")
				}
				recoverByWaiting(t, account, m, hash)
			})
		}
	}
}

func TestSendCallsReceiptAnsweredAfterTheDeadlineIsNotReturned(t *testing.T) {
	m := newMockBundler(t)
	m.set(func(m *mockBundler) { m.receiptDelay = 1500 * time.Millisecond })
	account := m.account(t, 5*time.Second)
	started := time.Now()
	_, err := account.SendCalls([]ChainCall{sendCallsWithdraw}, true, 400*time.Millisecond)
	if elapsed := time.Since(started); elapsed > 1200*time.Millisecond {
		t.Fatalf("SendCalls took %v on a 400ms receipt budget", elapsed)
	}
	hash := assertUnknownWithHash(t, err, m)
	m.set(func(m *mockBundler) { m.receiptDelay = 0 })
	recoverByWaiting(t, account, m, hash)
}

func TestWaitForReceiptStalledStatusReadEndsWithTheWait(t *testing.T) {
	m := newMockBundler(t)
	sent, err := m.account(t, 2*time.Second).SendCalls([]ChainCall{sendCallsWithdraw}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.set(func(m *mockBundler) { m.receiptMissing, m.statusDelay = true, 3*time.Second })
	account := m.account(t, 5*time.Second)
	started := time.Now()
	if _, err := account.WaitForReceipt(sent.UserOperationHash, 400*time.Millisecond, 50*time.Millisecond); err == nil {
		t.Fatal("WaitForReceipt returned a receipt")
	}
	if elapsed := time.Since(started); elapsed > 1200*time.Millisecond {
		t.Fatalf("WaitForReceipt took %v on a 400ms budget", elapsed)
	}
	m.set(func(m *mockBundler) { m.receiptMissing, m.statusDelay = false, 0 })
	recoverByWaiting(t, account, m, sent.UserOperationHash)
}
