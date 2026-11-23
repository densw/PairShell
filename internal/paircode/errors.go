package paircode

import "errors"

// RequestError retains the stage/status without recording codes, tokens or bodies.
type RequestError struct {
	Operation string
	Status    int
}

func (e *RequestError) Error() string { return "pairing service request failed: " + e.Operation }
func (e *RequestError) Unwrap() error { return ErrCode }

func Explain(err error) string {
	if errors.Is(err, ErrFormat) {
		return "仅支持完整的 12 位配对码，请重新复制"
	}
	var request *RequestError
	if errors.As(err, &request) {
		switch request.Status {
		case 0:
			return "无法连接配对服务，请检查两端服务器地址、网络或证书后重试"
		case 429:
			return "配对请求过于频繁，请稍候重试"
		case 403:
			return "配对码不可用，请确认两端使用同一服务器，再获取新配对码"
		case 404, 405:
			return "服务器不支持当前配对接口，请升级自建服务器或改用官方服务"
		default:
			return "配对服务响应异常，请检查服务器后重试"
		}
	}
	return "配对验证未通过，请确认两端使用同一服务器，并重新复制有效的 12 位配对码"
}
