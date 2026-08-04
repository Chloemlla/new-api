package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

const (
	// pricingConfigVersion identifies the export document layout so a future
	// incompatible change can be detected when re-importing old files.
	pricingConfigVersion = 1
	// maxPricingImportBytes caps the accepted import body size (10MB), matching
	// the limit used for upstream ratio config fetches.
	maxPricingImportBytes = 10 << 20
)

// pricingConfigExport is the self-describing JSON document produced by
// ExportPricingConfig and consumed by ImportPricingConfig. The pricing fields
// live at the top level alongside lightweight metadata.
type pricingConfigExport struct {
	Version              int                `json:"version"`
	ExportedAt           int64              `json:"exported_at"`
	ModelRatio           map[string]float64 `json:"model_ratio"`
	CompletionRatio      map[string]float64 `json:"completion_ratio"`
	CacheRatio           map[string]float64 `json:"cache_ratio"`
	CreateCacheRatio     map[string]float64 `json:"create_cache_ratio"`
	ImageRatio           map[string]float64 `json:"image_ratio"`
	AudioRatio           map[string]float64 `json:"audio_ratio"`
	AudioCompletionRatio map[string]float64 `json:"audio_completion_ratio"`
	ModelPrice           map[string]float64 `json:"model_price"`
	BillingMode          map[string]string  `json:"billing_mode"`
	BillingExpr          map[string]string  `json:"billing_expr"`
}

// pricingFieldOptionKeys maps each exported pricing field name to the option
// key that persists it. The slice keeps a stable order for deterministic
// iteration; numeric ratio maps and string billing maps are validated
// differently on import (see isBillingField).
var pricingFieldOptionKeys = []struct{ field, optionKey string }{
	{"model_ratio", "ModelRatio"},
	{"completion_ratio", "CompletionRatio"},
	{"cache_ratio", "CacheRatio"},
	{"create_cache_ratio", "CreateCacheRatio"},
	{"image_ratio", "ImageRatio"},
	{"audio_ratio", "AudioRatio"},
	{"audio_completion_ratio", "AudioCompletionRatio"},
	{"model_price", "ModelPrice"},
	{billing_setting.BillingModeField, "billing_setting.billing_mode"},
	{billing_setting.BillingExprField, "billing_setting.billing_expr"},
}

func isBillingField(field string) bool {
	return field == billing_setting.BillingModeField || field == billing_setting.BillingExprField
}

// buildPricingConfigDocument snapshots the currently configured model pricing
// (ratios, fixed prices, and tiered billing expressions) into an exportable
// document. Values are read from the live in-memory maps rather than the
// 30s exposed-data cache so an export always reflects the latest changes.
func buildPricingConfigDocument() *pricingConfigExport {
	return &pricingConfigExport{
		Version:              pricingConfigVersion,
		ExportedAt:           time.Now().Unix(),
		ModelRatio:           ratio_setting.GetModelRatioCopy(),
		CompletionRatio:      ratio_setting.GetCompletionRatioCopy(),
		CacheRatio:           ratio_setting.GetCacheRatioCopy(),
		CreateCacheRatio:     ratio_setting.GetCreateCacheRatioCopy(),
		ImageRatio:           ratio_setting.GetImageRatioCopy(),
		AudioRatio:           ratio_setting.GetAudioRatioCopy(),
		AudioCompletionRatio: ratio_setting.GetAudioCompletionRatioCopy(),
		ModelPrice:           ratio_setting.GetModelPriceCopy(),
		BillingMode:          billing_setting.GetBillingModeCopy(),
		BillingExpr:          billing_setting.GetBillingExprCopy(),
	}
}

// ExportPricingConfig returns the full model pricing configuration as a JSON
// document that can be re-imported on this or another instance.
func ExportPricingConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildPricingConfigDocument(),
	})
}

// containsPricingField reports whether the given object carries at least one
// known pricing field, i.e. whether it can be treated as pricing payload.
func containsPricingField(m map[string]json.RawMessage) bool {
	for _, field := range pricingFieldOptionKeys {
		if _, ok := m[field.field]; ok {
			return true
		}
	}
	return false
}

// extractPricingFields unwraps the pricing payload from an imported document.
// It accepts three layouts: the top-level fields of the exported document
// (with version/exported_at metadata), a bare pricing field map, and nested
// envelopes such as {"pricing": {...}} or a wrapped API response {"data": {...}}.
func extractPricingFields(raw map[string]json.RawMessage) map[string]json.RawMessage {
	for _, key := range []string{"pricing", "data"} {
		nested, ok := raw[key]
		if !ok {
			continue
		}
		var nestedMap map[string]json.RawMessage
		if err := common.Unmarshal(nested, &nestedMap); err != nil {
			continue
		}
		if containsPricingField(nestedMap) {
			return nestedMap
		}
	}
	if containsPricingField(raw) {
		return raw
	}
	return nil
}

// validateImportedBillingExprs smoke-tests every tiered billing expression
// bundled in the import before anything is persisted, so a broken expression
// cannot be applied silently.
func validateImportedBillingExprs(pricingFields map[string]json.RawMessage) error {
	billingMode := make(map[string]string)
	if rawMode, ok := pricingFields[billing_setting.BillingModeField]; ok {
		if err := common.Unmarshal(rawMode, &billingMode); err != nil {
			return err
		}
	}
	rawExpr, ok := pricingFields[billing_setting.BillingExprField]
	if !ok {
		return nil
	}
	billingExpr := make(map[string]string)
	if err := common.Unmarshal(rawExpr, &billingExpr); err != nil {
		return err
	}
	for model, expr := range billingExpr {
		if strings.TrimSpace(expr) == "" {
			continue
		}
		if billingMode[model] != billing_setting.BillingModeTieredExpr {
			continue
		}
		if err := billing_setting.SmokeTestExpr(expr); err != nil {
			return fmt.Errorf("invalid billing expression for %q: %w", model, err)
		}
	}
	return nil
}

// ImportPricingConfig replaces the current model pricing configuration with
// the values carried by the uploaded JSON document. Every field is validated
// and the resulting options are persisted atomically in a single transaction.
func ImportPricingConfig(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxPricingImportBytes+1))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(body) > maxPricingImportBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"success": false,
			"message": "Pricing config JSON is too large",
		})
		return
	}

	var raw map[string]json.RawMessage
	if err := common.Unmarshal(body, &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid JSON: " + err.Error(),
		})
		return
	}

	pricingFields := extractPricingFields(raw)
	if len(pricingFields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "No pricing fields found in the imported JSON",
		})
		return
	}

	// Validate every field's shape and build the option values to persist.
	values := make(map[string]string, len(pricingFields))
	for _, field := range pricingFieldOptionKeys {
		rawField, ok := pricingFields[field.field]
		if !ok {
			continue
		}
		var encoded []byte
		if isBillingField(field.field) {
			m := make(map[string]string)
			if err := common.Unmarshal(rawField, &m); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("Invalid %s field: %v", field.field, err),
				})
				return
			}
			encoded, err = common.Marshal(m)
		} else {
			m := make(map[string]float64)
			if err := common.Unmarshal(rawField, &m); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("Invalid %s field: %v", field.field, err),
				})
				return
			}
			encoded, err = common.Marshal(m)
		}
		if err != nil {
			common.ApiError(c, err)
			return
		}
		values[field.optionKey] = string(encoded)
	}

	if err := validateImportedBillingExprs(pricingFields); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidatePricingCache()
	// 出于安全考虑只记录操作名称，不记录定价内容。
	recordManageAudit(c, "option.update", map[string]interface{}{
		"key": "pricing_config_import",
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
