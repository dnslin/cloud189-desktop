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
	if c == nil {
		return errors.New("cloud189: Client 未初始化")
	}
	if c.appSigner == nil {
		return errors.New("cloud189: App 签名器未设置")
	}
	if c.session == nil {
		return errors.New("cloud189: SessionProvider 未设置")
	}
	secret := c.session.GetSessionSecret()
	if len(secret) < 16 {
		return errors.New("cloud189: 会话密钥不足 16 位")
	}
	if params == nil {
		params = url.Values{}
	}
	encoded := encodeValues(params)
	cipher, err := crypto.EncryptECB([]byte(secret[:16]), []byte(encoded))
	if err != nil {
		return err
	}
	hexParams := hex.EncodeToString(cipher)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(c.uploadBase, path), nil)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	q.Set("params", hexParams)
	req.URL.RawQuery = q.Encode()

	// 与参考实现一致的上传头。
	req.Header.Set("decodefields", "familyId,parentFolderId,fileName,fileMd5,fileSize,sliceMd5,sliceSize,albumId,extend,lazyCheck,isLog")
	req.Header.Set("accept", "application/json;charset=UTF-8")
	req.Header.Set("cache-control", "no-cache")

	return c.useMiddlewares(req, out, c.appSigner.Middleware())
}

// WebUpload 使用 Web 上传签名。
func (c *Client) WebUpload(ctx context.Context, path string, params url.Values, rsaKey *WebRSA, out any) error {
	if c == nil {
		return errors.New("cloud189: Client 未初始化")
	}
	if c.webSigner == nil {
		return errors.New("cloud189: Web 签名器未设置")
	}
	if params == nil {
		params = url.Values{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(c.uploadBase, path), nil)
	if err != nil {
		return err
	}
	if err := c.webSigner.Sign(req, params, rsaKey); err != nil {
		return err
	}
	return c.useMiddlewares(req, out)
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
	if c.session == nil {
		return "", errors.New("cloud189: SessionProvider 未设置")
	}
	if key := c.session.GetSessionKey(); key != "" {
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
		return "", errors.New("cloud189: Web 会话缺少 sessionKey")
	}
	if s, ok := c.session.(interface{ SetSessionKey(string) }); ok {
		s.SetSessionKey(info.SessionKey)
	}
	return info.SessionKey, nil
}
