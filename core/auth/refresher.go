package auth

import "context"

// Refresher 定义刷新凭证的能力，供重试策略回调。
type Refresher interface {
	Refresh(ctx context.Context) error
	NeedsRefresh() bool
}
