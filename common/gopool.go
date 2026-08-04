package common

import (
	"context"
	"fmt"

	"github.com/bytedance/gopkg/util/gopool"
)

// MaxRelayGoroutines is the maximum number of goroutines in the relay pool.
// Set to 20000 which is generous for any realistic workload while preventing
// unbounded goroutine creation that could lead to OOM under extreme load.
const MaxRelayGoroutines = 20000

var relayGoPool gopool.Pool

func init() {
	relayGoPool = gopool.NewPool("gopool.RelayPool", MaxRelayGoroutines, gopool.NewConfig())
	relayGoPool.SetPanicHandler(func(ctx context.Context, i interface{}) {
		if stopChan, ok := ctx.Value("stop_chan").(chan bool); ok {
			SafeSendBool(stopChan, true)
		}
		SysError(fmt.Sprintf("panic in gopool.RelayPool: %v", i))
	})
}

func RelayCtxGo(ctx context.Context, f func()) {
	relayGoPool.CtxGo(ctx, f)
}