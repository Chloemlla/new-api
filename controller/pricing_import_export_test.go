package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type importExportResponse struct {
	Success bool                 `json:"success"`
	Message string               `json:"message"`
	Data    *pricingConfigExport `json:"data"`
}

// withPricingStateSnapshot captures the current ratio maps and tiered billing
// settings and restores them when the test finishes, so a test that imports a
// document cannot leak its values into other tests in this package.
func withPricingStateSnapshot(t *testing.T) {
	t.Helper()

	orig := map[string]map[string]float64{
		"model_ratio":            ratio_setting.GetModelRatioCopy(),
		"completion_ratio":       ratio_setting.GetCompletionRatioCopy(),
		"cache_ratio":            ratio_setting.GetCacheRatioCopy(),
		"create_cache_ratio":     ratio_setting.GetCreateCacheRatioCopy(),
		"image_ratio":            ratio_setting.GetImageRatioCopy(),
		"audio_ratio":            ratio_setting.GetAudioRatioCopy(),
		"audio_completion_ratio": ratio_setting.GetAudioCompletionRatioCopy(),
		"model_price":            ratio_setting.GetModelPriceCopy(),
	}
	origBillingMode := billing_setting.GetBillingModeCopy()
	origBillingExpr := billing_setting.GetBillingExprCopy()

	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(mustMarshalString(t, orig["model_ratio"])))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(mustMarshalString(t, orig["completion_ratio"])))
		require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(mustMarshalString(t, orig["cache_ratio"])))
		require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString(mustMarshalString(t, orig["create_cache_ratio"])))
		require.NoError(t, ratio_setting.UpdateImageRatioByJSONString(mustMarshalString(t, orig["image_ratio"])))
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(mustMarshalString(t, orig["audio_ratio"])))
		require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(mustMarshalString(t, orig["audio_completion_ratio"])))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(mustMarshalString(t, orig["model_price"])))
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"billing_setting." + billing_setting.BillingModeField: mustMarshalString(t, origBillingMode),
			"billing_setting." + billing_setting.BillingExprField: mustMarshalString(t, origBillingExpr),
		}))
		model.InvalidatePricingCache()
	})
}

// setupPricingImportExportTest prepares an in-memory DB with the tables the
// import handler touches and makes sure the option map is writable.
func setupPricingImportExportTest(t *testing.T) *gorm.DB {
	t.Helper()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Log{}))

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()

	return db
}

func mustMarshalString(t *testing.T, v any) string {
	t.Helper()
	bytes, err := common.Marshal(v)
	require.NoError(t, err)
	return string(bytes)
}

func postImportPricing(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/option/import_pricing", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ImportPricingConfig(ctx)
	return recorder
}

func decodeImportExportResponse(t *testing.T, recorder *httptest.ResponseRecorder) importExportResponse {
	t.Helper()

	var resp importExportResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	return resp
}

func TestExtractPricingFields(t *testing.T) {
	tests := []struct {
		name   string
		raw    map[string]json.RawMessage
		expect []string
	}{
		{
			name:   "bare field map",
			raw:    map[string]json.RawMessage{"model_ratio": json.RawMessage(`{"gpt-4o": 1}`)},
			expect: []string{"model_ratio"},
		},
		{
			name: "versioned export document",
			raw: map[string]json.RawMessage{
				"version":      json.RawMessage(`1`),
				"exported_at":  json.RawMessage(`123`),
				"model_price":  json.RawMessage(`{"dall-e-3": 0.04}`),
				"billing_expr": json.RawMessage(`{}`),
			},
			expect: []string{"model_price", "billing_expr"},
		},
		{
			name: "nested pricing envelope",
			raw: map[string]json.RawMessage{
				"pricing": json.RawMessage(`{"completion_ratio": {"gpt-4o": 3}}`),
			},
			expect: []string{"completion_ratio"},
		},
		{
			name: "wrapped API response data",
			raw: map[string]json.RawMessage{
				"success": json.RawMessage(`true`),
				"data":    json.RawMessage(`{"cache_ratio": {"gpt-4o": 0.5}}`),
			},
			expect: []string{"cache_ratio"},
		},
		{
			name:   "no pricing fields",
			raw:    map[string]json.RawMessage{"foo": json.RawMessage(`1`)},
			expect: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := extractPricingFields(tc.raw)
			if tc.expect == nil {
				assert.Nil(t, fields)
				return
			}
			require.NotNil(t, fields)
			for _, field := range tc.expect {
				_, ok := fields[field]
				assert.True(t, ok, "expected field %q to be present", field)
			}
		})
	}
}

func TestExportPricingConfig(t *testing.T) {
	withPricingStateSnapshot(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"zz-export-model": 2.5}`))

	setupPricingImportExportTest(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/option/export_pricing", nil)

	ExportPricingConfig(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeImportExportResponse(t, recorder)
	require.True(t, resp.Success)
	require.NotNil(t, resp.Data)
	assert.Equal(t, pricingConfigVersion, resp.Data.Version)
	require.NotEmpty(t, resp.Data.ExportedAt)
	assert.Equal(t, 2.5, resp.Data.ModelRatio["zz-export-model"])
	assert.NotNil(t, resp.Data.CompletionRatio)
	assert.NotNil(t, resp.Data.ModelPrice)
}

func TestImportPricingConfigAppliesDocument(t *testing.T) {
	withPricingStateSnapshot(t)
	db := setupPricingImportExportTest(t)

	doc := `{
		"version": 1,
		"model_ratio": {"zz-import-test": 1.5},
		"completion_ratio": {"zz-import-test": 2},
		"cache_ratio": {"zz-import-test": 0.5},
		"model_price": {"zz-fixed-import": 0.05},
		"billing_mode": {"zz-tiered-import": "tiered_expr"},
		"billing_expr": {"zz-tiered-import": "tier(\"base\", p * 1 + c * 2)"}
	}`

	recorder := postImportPricing(t, doc)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeImportExportResponse(t, recorder)
	require.True(t, resp.Success)

	// In-memory ratio maps are updated.
	ratio, ok, _ := ratio_setting.GetModelRatio("zz-import-test")
	require.True(t, ok)
	assert.Equal(t, 1.5, ratio)
	assert.Equal(t, 2.0, ratio_setting.GetCompletionRatio("zz-import-test"))
	cacheRatio, ok := ratio_setting.GetCacheRatio("zz-import-test")
	require.True(t, ok)
	assert.Equal(t, 0.5, cacheRatio)

	// Fixed price applied.
	price, ok := ratio_setting.GetModelPrice("zz-fixed-import", false)
	require.True(t, ok)
	assert.Equal(t, 0.05, price)

	// Tiered billing applied.
	assert.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("zz-tiered-import"))
	expr, ok := billing_setting.GetBillingExpr("zz-tiered-import")
	require.True(t, ok)
	assert.Contains(t, expr, "base")

	// Options persisted to the DB.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	var ratioValue, exprValue string
	require.NoError(t, sqlDB.QueryRow("SELECT value FROM options WHERE key = ?", "ModelRatio").Scan(&ratioValue))
	assert.Contains(t, ratioValue, "zz-import-test")
	require.NoError(t, sqlDB.QueryRow("SELECT value FROM options WHERE key = ?", "billing_setting.billing_expr").Scan(&exprValue))
	assert.Contains(t, exprValue, "zz-tiered-import")
}

func TestImportPricingConfigRoundTripsExport(t *testing.T) {
	withPricingStateSnapshot(t)
	setupPricingImportExportTest(t)

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"zz-roundtrip": 1.25}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting." + billing_setting.BillingModeField: mustMarshalString(t, map[string]string{"zz-roundtrip": "tiered_expr"}),
		"billing_setting." + billing_setting.BillingExprField: mustMarshalString(t, map[string]string{"zz-roundtrip": `tier("base", p + c)`}),
	}))

	exportRecorder := httptest.NewRecorder()
	exportCtx, _ := gin.CreateTestContext(exportRecorder)
	exportCtx.Request = httptest.NewRequest(http.MethodGet, "/api/option/export_pricing", nil)
	ExportPricingConfig(exportCtx)
	exported := decodeImportExportResponse(t, exportRecorder)
	require.True(t, exported.Success)
	require.NotNil(t, exported.Data)

	exportedBytes, err := common.Marshal(exported.Data)
	require.NoError(t, err)

	importRecorder := postImportPricing(t, string(exportedBytes))
	require.Equal(t, http.StatusOK, importRecorder.Code)
	resp := decodeImportExportResponse(t, importRecorder)
	require.True(t, resp.Success)

	ratio, ok, _ := ratio_setting.GetModelRatio("zz-roundtrip")
	require.True(t, ok)
	assert.Equal(t, 1.25, ratio)
	assert.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("zz-roundtrip"))
}

func TestImportPricingConfigRejectsMalformedField(t *testing.T) {
	withPricingStateSnapshot(t)
	setupPricingImportExportTest(t)

	recorder := postImportPricing(t, `{"model_ratio": {"zz-bad": "not-a-number"}}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := decodeImportExportResponse(t, recorder)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Message, "model_ratio")

	// Nothing was applied.
	_, ok, _ := ratio_setting.GetModelRatio("zz-bad")
	assert.False(t, ok)
}

func TestImportPricingConfigRejectsNegativeBillingExpr(t *testing.T) {
	withPricingStateSnapshot(t)
	setupPricingImportExportTest(t)

	// The expression is syntactically valid but yields a negative result for
	// the smoke-test vectors, so it must be rejected before persisting.
	recorder := postImportPricing(t, `{
		"billing_mode": {"zz-neg-expr": "tiered_expr"},
		"billing_expr": {"zz-neg-expr": "p - 2 * c"}
	}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := decodeImportExportResponse(t, recorder)
	assert.False(t, resp.Success)
	assert.Contains(t, strings.ToLower(resp.Message), "billing expression")

	// Nothing was applied.
	assert.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("zz-neg-expr"))
}

func TestImportPricingConfigRejectsNoPricingFields(t *testing.T) {
	withPricingStateSnapshot(t)
	setupPricingImportExportTest(t)

	recorder := postImportPricing(t, `{"foo": 1, "bar": "baz"}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := decodeImportExportResponse(t, recorder)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Message, "No pricing fields")
}

func TestImportPricingConfigRejectsInvalidJSON(t *testing.T) {
	withPricingStateSnapshot(t)
	setupPricingImportExportTest(t)

	recorder := postImportPricing(t, `{"model_ratio": `)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	resp := decodeImportExportResponse(t, recorder)
	assert.False(t, resp.Success)
}
