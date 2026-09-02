package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*kitdto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex

// inFlightRequests tracks the number of in-flight requests per channel.
// Used for load-aware channel selection.
var inFlightRequests sync.Map // map[int]*int64

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		InvalidatePricingCache()
		rebuildTaskAliasView()
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*kitdto.AdvancedCustomConfig)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	channelSyncLock.Unlock()
	// Lock ordering: InvalidatePricingCache acquires updatePricingLock, and
	// GetPricing (holding updatePricingLock) nests channelSyncLock.RLock via
	// loadPricingAdvancedCustomConfigs. channelSyncLock MUST be released before
	// invalidating the pricing cache, otherwise the reversed order deadlocks.
	InvalidatePricingCache()
	rebuildTaskAliasView()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry, filters)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	channels, _ := filterCandidateIDs(group2model2channels[group][model], model, filters)

	channels = filterChannelsByCircuitBreaker(channels)
	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels, _ = filterCandidateIDs(group2model2channels[group][normalizedModel], model, filters)
		channels = filterChannelsByCircuitBreaker(channels)
	}

	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	priorityIndex := -1
	targetPriority := int64(0)
	lastPriority := int64(0)
	previousPriority := int64(0)
	for _, channelId := range channels {
		channel, ok := channelsIDM[channelId]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
		priority := channel.GetPriority()
		lastPriority = priority
		if priorityIndex == -1 || priority != previousPriority {
			priorityIndex++
			if priorityIndex == retry {
				targetPriority = priority
			}
			previousPriority = priority
		}
	}
	if priorityIndex == -1 {
		return nil, nil
	}
	if retry > priorityIndex {
		targetPriority = lastPriority
	}

	// get the priority for the given retry number
	var sumWeight = 0
	targetChannelCount := 0
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannelCount++
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if targetChannelCount == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = targetChannelCount * 100
		smoothingAdjustment = 100
	} else if sumWeight/targetChannelCount < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channelId := range channels {
		channel := channelsIDM[channelId]
		if channel.GetPriority() != targetPriority {
			continue
		}
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

// GetSatisfiedChannelPriorityCount returns the number of distinct priorities
// available for the group and model after request-path filtering.
func GetSatisfiedChannelPriorityCount(group string, model string, requestPath string) (int, error) {
	if !common.MemoryCacheEnabled {
		modelNames := []string{model}
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		if normalizedModel != model {
			modelNames = append(modelNames, normalizedModel)
		}
		for _, modelName := range modelNames {
			var abilities []Ability
			if err := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, modelName, true).Find(&abilities).Error; err != nil {
				return 0, err
			}
			if requestPath != "" {
				abilities = filterAbilitiesByConstraints(abilities, model, []dto.ChannelFilter{
					{Kind: dto.FilterRequestPath, RequestPath: requestPath},
				})
			}
			priorities := make(map[int64]struct{}, len(abilities))
			for _, ability := range abilities {
				if ability.Priority != nil {
					priorities[*ability.Priority] = struct{}{}
				}
			}
			if len(priorities) > 0 {
				return len(priorities), nil
			}
		}
		return 0, nil
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	modelNames := []string{model}
	normalizedModel := ratio_setting.FormatMatchingModelName(model)
	if normalizedModel != model {
		modelNames = append(modelNames, normalizedModel)
	}
	for _, modelName := range modelNames {
		channels := filterChannelsByRequestPathAndModel(group2model2channels[group][modelName], requestPath, model)
		priorities := make(map[int64]struct{}, len(channels))
		for _, channelID := range channels {
			channel, ok := channelsIDM[channelID]
			if !ok {
				return 0, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
			}
			priorities[channel.GetPriority()] = struct{}{}
		}
		if len(priorities) > 0 {
			return len(priorities), nil
		}
	}
	return 0, nil
}

// filterChannelsByRequestPathAndModel restricts candidates by request path and
// model. Only Advanced Custom (type 58) channels are path-checked: they are kept
// only when one of their configured routes matches requestPath and model. All
// other channel types always pass. When requestPath is empty, filtering is skipped.
// Caller must hold channelSyncLock (read lock). The cached slice is never mutated.
func filterChannelsByRequestPathAndModel(channels []int, requestPath string, model string) []int {
	if requestPath == "" || len(channels) == 0 {
		return channels
	}
	var filtered []int
	for i, channelId := range channels {
		channel, ok := channelsIDM[channelId]
		if !ok {
			// keep it so the downstream consistency error is raised as before
			if filtered != nil {
				filtered = append(filtered, channelId)
			}
			continue
		}
		if channel.Type != constant.ChannelTypeAdvancedCustom {
			if filtered != nil {
				filtered = append(filtered, channelId)
			}
			continue
		}
		if filtered == nil {
			filtered = make([]int, 0, len(channels))
			filtered = append(filtered, channels[:i]...)
		}
		if config := channel2advancedCustomConfig[channelId]; config != nil && config.SupportsPathForModel(requestPath, model) {
			filtered = append(filtered, channelId)
		}
	}
	if filtered == nil {
		return channels
	}
	return filtered
}

// filterChannelsByCircuitBreaker drops channels whose circuit breaker is open so
// a tripped channel is never selected for routing. It is a no-op (returns the
// input slice) when the breaker is disabled. Caller must hold channelSyncLock
// (read lock). The cached slice is never mutated.
func filterChannelsByCircuitBreaker(channelIds []int) []int {
	if !common.CircuitBreakerEnabled {
		return channelIds
	}
	filtered := make([]int, 0, len(channelIds))
	for _, channelId := range channelIds {
		if !common.ChannelBreaker.IsBlocked(channelId) {
			filtered = append(filtered, channelId)
		}
	}
	return filtered
}
func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				for i, channelId := range channels {
					if channelId == id {
						// remove the channel from the slice
						group2model2channels[group][model] = append(channels[:i], channels[i+1:]...)
						break
					}
				}
			}
		}
		return
	}

	channel, ok := channelsIDM[id]
	if !ok {
		return
	}
	for _, group := range strings.Split(channel.Group, ",") {
		model2channels, ok := group2model2channels[group]
		if !ok {
			model2channels = make(map[string][]int)
			group2model2channels[group] = model2channels
		}
		for _, modelName := range strings.Split(channel.Models, ",") {
			channels := model2channels[modelName]
			present := false
			for _, channelId := range channels {
				if channelId == id {
					present = true
					break
				}
			}
			if present {
				continue
			}
			channels = append(channels, id)
			sort.SliceStable(channels, func(i, j int) bool {
				return channelsIDM[channels[i]].GetPriority() > channelsIDM[channels[j]].GetPriority()
			})
			model2channels[modelName] = channels
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	if channel == nil {
		channelSyncLock.Unlock()
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
	}
	channelsIDM[channel.Id] = channel
	if channel2advancedCustomConfig == nil {
		channel2advancedCustomConfig = make(map[int]*kitdto.AdvancedCustomConfig)
	}
	delete(channel2advancedCustomConfig, channel.Id)
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
			channel2advancedCustomConfig[channel.Id] = config
		}
	}
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
	// Lock ordering: do NOT hold channelSyncLock while calling
	// InvalidatePricingCache. GetPricing acquires updatePricingLock first and then
	// channelSyncLock.RLock (via loadPricingAdvancedCustomConfigs); acquiring
	// updatePricingLock while holding channelSyncLock would be an AB-BA deadlock.
	channelSyncLock.Unlock()
	InvalidatePricingCache()
}

// ---------------------------------------------------------------------------
// In-flight request tracking for load-aware channel selection
// ---------------------------------------------------------------------------

// IncrementInFlightRequests increments the in-flight counter for a channel.
func IncrementInFlightRequests(channelID int) {
	v, _ := inFlightRequests.LoadOrStore(channelID, new(int64))
	atomic.AddInt64(v.(*int64), 1)
}

// DecrementInFlightRequests decrements the in-flight counter for a channel.
func DecrementInFlightRequests(channelID int) {
	if v, ok := inFlightRequests.Load(channelID); ok {
		atomic.AddInt64(v.(*int64), -1)
	}
}

// GetInFlightRequests returns the current in-flight count for a channel.
func GetInFlightRequests(channelID int) int64 {
	if v, ok := inFlightRequests.Load(channelID); ok {
		return atomic.LoadInt64(v.(*int64))
	}
	return 0
}

// GetAllInFlightRequests returns a map of channel ID to in-flight count.
func GetAllInFlightRequests() map[int]int64 {
	result := make(map[int]int64)
	inFlightRequests.Range(func(key, value interface{}) bool {
		result[key.(int)] = atomic.LoadInt64(value.(*int64))
		return true
	})
	return result
}
// ChannelInFlight returns the number of in-flight requests for the given channel.
func ChannelInFlight(channelID int) int {
	return int(GetInFlightRequests(channelID))
}

// HoldChannelLoad records an in-flight load slot for the selected channel. It
// releases any previous slot held by this request (from a retry across channels)
// before holding the new one, so retries never double count in-flight load.
func HoldChannelLoad(c interface {
	GetInt(key string) int
	Set(key string, value interface{})
}, channelID int) {
	held := c.GetInt(string(constant.ContextKeyHeldChannelLoad))
	if held > 0 && held != channelID {
		DecrementInFlightRequests(held)
	}
	if held != channelID {
		IncrementInFlightRequests(channelID)
		c.Set(string(constant.ContextKeyHeldChannelLoad), channelID)
	}
}

// ReleaseHeldChannelLoad releases the in-flight slot held by the current request.
func ReleaseHeldChannelLoad(c interface {
	GetInt(key string) int
	Set(key string, value interface{})
}) {
	held := c.GetInt(string(constant.ContextKeyHeldChannelLoad))
	if held > 0 {
		DecrementInFlightRequests(held)
		c.Set(string(constant.ContextKeyHeldChannelLoad), 0)
	}
}
// GetRandomSatisfiedChannelWithLoadAware selects a channel with load awareness.
// Among channels at the target priority, it prefers channels with fewer in-flight
// requests, weighted by a combination of weight and inverse load factor.
func GetRandomSatisfiedChannelWithLoadAware(group string, model string, retry int, filters []dto.ChannelFilter) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry, filters)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	channels, _ := filterCandidateIDs(group2model2channels[group][model], model, filters)
	channels = filterChannelsByCircuitBreaker(channels)
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels, _ = filterCandidateIDs(group2model2channels[group][normalizedModel], model, filters)
		channels = filterChannelsByCircuitBreaker(channels)
	}

	if len(channels) == 0 {
		return nil, nil
	}
	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	// Find target priority based on retry index.
	priorityIndex := -1
	targetPriority := int64(0)
	lastPriority := int64(0)
	previousPriority := int64(0)
	for _, channelId := range channels {
		channel, ok := channelsIDM[channelId]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
		priority := channel.GetPriority()
		lastPriority = priority
		if priorityIndex == -1 || priority != previousPriority {
			priorityIndex++
			if priorityIndex == retry {
				targetPriority = priority
			}
			previousPriority = priority
		}
	}
	if priorityIndex == -1 {
		return nil, nil
	}
	if retry > priorityIndex {
		targetPriority = lastPriority
	}

	// Build candidate list with effective weights adjusted by load.
	type candidate struct {
		channel *Channel
		weight  int
		load    int64
	}
	var candidates []candidate
	totalWeight := 0

	for _, channelId := range channels {
		ch, ok := channelsIDM[channelId]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
		if ch.GetPriority() != targetPriority {
			continue
		}
		load := GetInFlightRequests(channelId)
		baseWeight := int(ch.GetWeight())
		if baseWeight <= 0 {
			baseWeight = 100
		}

		// Adjust weight based on load: high load reduces effective weight.
		loadFactor := 100
		if load > 10 {
			loadFactor = max(10, 100-int(load*5))
		}
		effectiveWeight := baseWeight * loadFactor / 100
		if effectiveWeight <= 0 {
			effectiveWeight = 1
		}

		candidates = append(candidates, candidate{channel: ch, weight: effectiveWeight, load: load})
		totalWeight += effectiveWeight
	}

	if len(candidates) == 0 {
		return nil, errors.New("no channel found")
	}

	// Weighted random selection.
	randomWeight := rand.Intn(totalWeight)
	for _, c := range candidates {
		randomWeight -= c.weight
		if randomWeight < 0 {
			return c.channel, nil
		}
	}

	return candidates[len(candidates)-1].channel, nil
}
