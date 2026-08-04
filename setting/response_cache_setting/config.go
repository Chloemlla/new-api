package response_cache_setting

import "github.com/QuantumNous/new-api/setting/config"

// ResponseCacheSetting 请求/响应缓存配置
//
// 该功能缓存完全相同的 API 请求（方法、路径、查询参数、请求体、调用身份均一致）
// 的响应，命中缓存时直接返回，不再经过上游渠道，也不产生调用扣费。
type ResponseCacheSetting struct {
	// Enabled 是否启用请求/响应缓存
	Enabled bool `json:"enabled"`
	// TTLSeconds 缓存有效期（秒）
	TTLSeconds int `json:"ttl_seconds"`
	// MaxResponseSizeKB 单条缓存响应的最大大小（KB），超出则不被缓存
	MaxResponseSizeKB int `json:"max_response_size_kb"`
	// OnlyDeterministic 仅缓存确定性请求
	// 为 true 时只缓存 stream=false、temperature=0（或未设置）、top_p=1（或未设置）、n=1（或未设置）的请求，
	// 避免把一次随机采样冻结后反复返回。
	OnlyDeterministic bool `json:"only_deterministic"`
}

// 默认配置
var responseCacheSetting = ResponseCacheSetting{
	Enabled:           false,
	TTLSeconds:        60,
	MaxResponseSizeKB: 1024,
	OnlyDeterministic: true,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("response_cache_setting", &responseCacheSetting)
}

// GetResponseCacheSetting 获取请求/响应缓存配置
func GetResponseCacheSetting() *ResponseCacheSetting {
	return &responseCacheSetting
}
