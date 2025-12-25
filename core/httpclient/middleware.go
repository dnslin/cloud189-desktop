package httpclient

import "net/http"

// Middleware 是请求预处理钩子，用于注入签名、UA、Content-Type 等。
type Middleware func(req *http.Request) error

// PrepareChain 代表按顺序执行的中间件集合。
type PrepareChain []Middleware

// Apply 依次执行链路中的中间件，遇到错误立即返回。
func (c PrepareChain) Apply(req *http.Request) error {
	for _, mw := range c {
		if mw == nil {
			continue
		}
		if err := mw(req); err != nil {
			return err
		}
	}
	return nil
}

// WithHeader 设置请求头。
func WithHeader(key, value string) Middleware {
	return func(req *http.Request) error {
		req.Header.Set(key, value)
		return nil
	}
}

// WithUserAgent 设置 User-Agent。
func WithUserAgent(ua string) Middleware {
	return WithHeader("User-Agent", ua)
}

// WithContentType 设置 Content-Type。
func WithContentType(ct string) Middleware {
	return WithHeader("Content-Type", ct)
}
