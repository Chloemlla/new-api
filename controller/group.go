package controller

import (
	"net/http"
	"sort"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

// GetGroupModels returns the models enabled for each user group. The result is
// derived from the enabled channel abilities (a model is usable by a group when
// at least one enabled channel serves that group and model), so it is read-only
// and reflects the current channel configuration.
func GetGroupModels(c *gin.Context) {
	abilities := model.GetAllEnableAbilities()

	groups := make(map[string]struct{})
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groups[groupName] = struct{}{}
	}
	modelsByGroup := make(map[string]map[string]struct{})
	for _, ability := range abilities {
		if !ability.Enabled {
			continue
		}
		if _, ok := groups[ability.Group]; !ok {
			groups[ability.Group] = struct{}{}
		}
		if modelsByGroup[ability.Group] == nil {
			modelsByGroup[ability.Group] = make(map[string]struct{})
		}
		modelsByGroup[ability.Group][ability.Model] = struct{}{}
	}

	result := make(map[string][]string, len(groups))
	for groupName := range groups {
		modelSet := modelsByGroup[groupName]
		modelNames := make([]string, 0, len(modelSet))
		for modelName := range modelSet {
			modelNames = append(modelNames, modelName)
		}
		sort.Strings(modelNames)
		result[groupName] = modelNames
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			usableGroups[groupName] = map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, groupName),
				"desc":  desc,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
