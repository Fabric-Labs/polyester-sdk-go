//go:build integration

package integration_test

import "github.com/Fabric-Labs/polyester-sdk-go/models"

func pricePtr(p models.PriceInput) *models.PriceInput { return &p }
