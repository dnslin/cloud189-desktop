package cloud189

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"

	"github.com/gowsp/cloud189-desktop/core/crypto"
)

// AppUpload 使用 App 签名器与 AES 参数加密执行上传相关接口。
func (c *Client) AppUpload(ctx context.Context, path string, params url.Values, out any) error {
	session, err := c.prepareSessionProvider(ctx)
	if err != nil {
		return err
	}
	secret := session.GetSessionSecret()
	if len(secret) < 16 {
		return WrapCloudError(ErrCodeInvalidToken, "会话密钥不足", errors.New("cloud189: 会话密钥不足 16 位"))
	}
	if params == nil {
		params = url.Values{}
	}
	encoded := encodeValues(params)
	cipher, err := crypto.EncryptECB([]byte(secret[:16]), []byte(encoded))
	if err != nil {
		return ensureCloudError(ErrCodeUnknown, "参数加密失败", err)
	}
	hexParams := hex.EncodeToString(cipher)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(c.uploadBase, path), nil)
	if err != nil {
		return WrapCloudError(ErrCodeInvalidRequest, "构建上传请求失败", err)
	}
	q := req.URL.Query()
	q.Set("params", hexParams)
	req.URL.RawQuery = q.Encode()

	// 与参考实现一致的上传头。
	req.Header.Set("decodefields", "familyId,parentFolderId,fileName,fileMd5,fileSize,sliceMd5,sliceSize,albumId,extend,lazyCheck,isLog")
	req.Header.Set("accept", "application/json;charset=UTF-8")
	req.Header.Set("cache-control", "no-cache")

	signer, err := c.prepareAppSigner(ctx)
	if err != nil {
		return err
	}
	return ensureCloudError(ErrCodeUnknown, "上传请求失败", toCloudError(c.useMiddlewares(req, out, signer.Middleware())))
}

// WebUpload 使用 Web 上传签名。
func (c *Client) WebUpload(ctx context.Context, path string, params url.Values, rsaKey *WebRSA, out any) error {
	if params == nil {
		params = url.Values{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(c.uploadBase, path), nil)
	if err != nil {
		return WrapCloudError(ErrCodeInvalidRequest, "构建上传请求失败", err)
	}
	signer, err := c.prepareWebSigner(ctx)
	if err != nil {
		return err
	}
	if err := signer.Sign(req, params, rsaKey); err != nil {
		return ensureCloudError(ErrCodeInvalidRequest, "签名上传请求失败", err)
	}
	return ensureCloudError(ErrCodeUnknown, "上传请求失败", toCloudError(c.useMiddlewares(req, out)))
}

// FetchWebRSA 获取 Web 上传所需 RSA 公钥配置。
func (c *Client) FetchWebRSA(ctx context.Context) (*WebRSA, error) {
	if c == nil {
		return nil, errors.New("cloud189: Client 未初始化")
	}
	var rsp WebRSA
	if err := c.WebGet(ctx, "/security/generateRsaKey.action", nil, &rsp); err != nil {
		return nil, err
	}
	return &rsp, nil
}

// WebSession 提供 Web 上传所需 SessionKey（依赖 Web 端会话）。
func (c *Client) WebSession(ctx context.Context) (string, error) {
	if c == nil {
		return "", errors.New("cloud189: Client 未初始化")
	}
	session, err := c.prepareSessionProvider(ctx)
	if err != nil {
		return "", err
	}
	if key := session.GetSessionKey(); key != "" {
		return key, nil
	}
	// 通过 Web API 获取简要信息以填充 SessionKey。
	var info struct {
		SessionKey string `json:"sessionKey,omitempty"`
		CodeResponse
	}
	if err := c.WebGet(ctx, "/portal/v2/getUserBriefInfo.action", nil, &info); err != nil {
		return "", err
	}
	if info.SessionKey == "" {
		return "", WrapCloudError(ErrCodeInvalidToken, "Web 会话缺少 sessionKey", errors.New("cloud189: Web 会话缺少 sessionKey"))
	}
	if s, ok := session.(interface{ SetSessionKey(string) }); ok {
		s.SetSessionKey(info.SessionKey)
	}
	return info.SessionKey, nil
}
