package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateModelRequestRateLimitGroupPreservesConfigOnInvalidJSON(t *testing.T) {
	previous := ModelRequestRateLimitGroup
	t.Cleanup(func() { ModelRequestRateLimitGroup = previous })

	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(`{"default":[10,5]}`))
	assert.Error(t, UpdateModelRequestRateLimitGroupByJSONString(`{"default":[10,`))

	total, success, found := GetGroupRateLimit("default")
	require.True(t, found)
	assert.Equal(t, 10, total)
	assert.Equal(t, 5, success)
}
