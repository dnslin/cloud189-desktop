package crypto

import (
	"net/url"
	"sort"
	"strings"
)

// EncodeParams 保留原始键顺序进行 URL 编码。
func EncodeParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	var buf strings.Builder
	first := true
	for k, v := range params {
		if !first {
			buf.WriteByte('&')
		}
		first = false
		buf.WriteString(url.QueryEscape(k))
		buf.WriteByte('=')
		buf.WriteString(url.QueryEscape(v))
	}
	return buf.String()
}

// EncodeParamsSorted 以 key 排序后进行 URL 编码。
func EncodeParamsSorted(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(url.QueryEscape(k))
		buf.WriteByte('=')
		buf.WriteString(url.QueryEscape(params[k]))
	}
	return buf.String()
}
