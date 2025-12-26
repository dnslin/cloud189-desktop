package crypto

import (
	"net/url"
	"sort"
	"strings"
)

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

// EncodeURLValues 以 key 排序后拼接 url.Values，保持原始内容不转义。
func EncodeURLValues(vals url.Values) string {
	if len(vals) == 0 {
		return ""
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	for _, key := range keys {
		for _, v := range vals[key] {
			if buf.Len() > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(key)
			buf.WriteByte('=')
			buf.WriteString(v)
		}
	}
	return buf.String()
}
