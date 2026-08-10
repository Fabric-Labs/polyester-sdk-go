//go:build ignore

// Read-only live smoke: concurrent auth + public :proto trades subscription.
//
//	set -a && source .env && set +a
//	go run ./scripts/live_smoke_qa.go
package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go"
)

func main() {
	if os.Getenv("POLYESTER_API_KEY_ID") == "" || os.Getenv("POLYESTER_API_PRIVATE_KEY") == "" {
		fmt.Println("FAIL: missing API key env")
		os.Exit(2)
	}
	client, err := polyester.FromEnv()
	if err != nil {
		fmt.Println("FAIL: client:", err)
		os.Exit(1)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := client.WaitForCatalogs(ctx); err != nil {
		fmt.Println("FAIL: WaitForCatalogs:", err)
		os.Exit(1)
	}

	limit := 1
	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false, nil)
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	var n int
	var sample error
	for err := range errCh {
		n++
		if sample == nil {
			sample = err
		}
	}
	if n > 0 {
		fmt.Printf("FAIL: concurrent ListOpen: %d/32 errors; sample: %v\n", n, sample)
		os.Exit(1)
	}
	fmt.Println("OK: concurrent ListOpen 32/32")

	symbol := os.Getenv("POLYESTER_TEST_TRADE_SYMBOL")
	if symbol == "" {
		symbol = "BTC-USDT"
	}
	fmt.Printf("trade_symbol=%s\n", symbol)
	sub, err := client.MarketData.SubscribeTrades(ctx, &symbol, nil)
	if err != nil {
		fmt.Println("FAIL: SubscribeTrades:", err)
		os.Exit(1)
	}
	defer sub.Close()
	deadline := time.Now().Add(25 * time.Second)
	got := 0
	for time.Now().Before(deadline) && got < 1 {
		select {
		case _, ok := <-sub.Messages():
			if !ok {
				fmt.Println("FAIL: trades subscription closed without publications")
				os.Exit(1)
			}
			got++
		case <-time.After(3 * time.Second):
		}
	}
	if got < 1 {
		fmt.Printf("FAIL: no public trades publications on %s within 25s\n", symbol)
		os.Exit(1)
	}
	fmt.Printf("OK: public trades :proto received %d publication(s) on %s\n", got, symbol)
}
