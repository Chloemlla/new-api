package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx         *gin.Context
	TokenGroup  string
	ModelName   string
	RequestPath string
	Retry       *int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled, Retry remains the global
// retry counter. The context stores the retry at which the current group started,
// so each group's priority retry is derived without resetting the outer budget.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
		if len(autoGroups) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}

		startGroupIndex := 0
		startRetryIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
		if value, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := value.(int); ok {
				startGroupIndex = idx
			}
		}
		if value, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex); exists {
			if idx, ok := value.(int); ok {
				startRetryIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			priorityRetry := param.GetRetry()
			if crossGroupRetry {
				priorityRetry -= startRetryIndex
				if priorityRetry < 0 {
					priorityRetry = 0
				}
			} else if i > startGroupIndex {
				priorityRetry = 0
			}

			if crossGroupRetry && priorityRetry > 0 {
				priorityCount, countErr := model.GetSatisfiedChannelPriorityCount(autoGroup, param.ModelName, param.RequestPath)
				if countErr != nil {
					return nil, autoGroup, countErr
				}
				if priorityRetry >= priorityCount {
					startRetryIndex = param.GetRetry()
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, startRetryIndex)
					continue
				}
			}

			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)
			channel, err = model.GetRandomSatisfiedChannelWithLoadAware(autoGroup, param.ModelName, priorityRetry, param.RequestPath)
			if err != nil {
				return nil, autoGroup, err
			}
			if channel == nil {
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				startRetryIndex = param.GetRetry()
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, startRetryIndex)
				continue
			}

			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, startRetryIndex)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s, priorityRetry: %d", autoGroup, priorityRetry)
			break
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannelWithLoadAware(param.TokenGroup, param.ModelName, param.GetRetry(), param.RequestPath)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}
