package cloud189

import (
	"context"
	"errors"
)

// GetUserInfo 获取用户信息。
func (c *Client) GetUserInfo(ctx context.Context) (*UserInfo, error) {
	if c == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	var rsp UserInfo
	if err := c.AppGet(ctx, "/getUserInfo.action", nil, &rsp); err != nil {
		return nil, err
	}
	return &rsp, nil
}

// GetCapacity 获取容量信息。
func (c *Client) GetCapacity(ctx context.Context) (*CapacityInfo, error) {
	if c == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	var rsp CapacityInfo
	if err := c.AppGet(ctx, "/getUserInfo.action", nil, &rsp); err != nil {
		return nil, err
	}
	return &rsp, nil
}

// SignIn 执行签到任务。
func (c *Client) SignIn(ctx context.Context) (*SignInResult, error) {
	if c == nil {
		return nil, WrapCloudError(ErrCodeInvalidRequest, "客户端未初始化", errors.New("cloud189: Client 未初始化"))
	}
	var rsp SignInResult
	if err := c.AppGet(ctx, "/mkt/userSign.action", nil, &rsp); err != nil {
		return nil, err
	}
	return &rsp, nil
}
