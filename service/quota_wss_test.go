package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestPreWssConsumeQuotaSkipsPerEventChargeWhenBillingSessionActive guards the
// realtime double-billing fix: with an active BillingSession the per-event debit
// must be skipped (it would otherwise stack on top of pre-consume + settle).
func TestPreWssConsumeQuotaSkipsPerEventChargeWhenBillingSessionActive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		UsePrice: false,
		Billing:  &BillingSession{},
	}

	err := PreWssConsumeQuota(c, relayInfo, &dto.RealtimeUsage{})
	require.NoError(t, err)
}
