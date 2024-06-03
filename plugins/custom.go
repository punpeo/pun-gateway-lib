package plugins

import (
	"context"
	gateway "github.com/punpeo/pun-gateway-lib"
	"github.com/zeromicro/go-zero/rest"
	"google.golang.org/grpc/metadata"
	"net/http"
	"strconv"
	"strings"
)

const customMdKey = "customMd"

var customHeaders = []string{
	"wechatpay-signature-type",
	"wechatpay-signature",
	"wechatpay-serial",
	"wechatpay-timestamp",
	"wechatpay-nonce",
}

// PluginCustom 自定义相应格式插件
type PluginCustom struct {
	gateway.BasicRpcHandler
}

func NewPluginCustom() *PluginCustom {
	return &PluginCustom{}
}

func (p *PluginCustom) Name() string {
	return "custom"
}

func (p *PluginCustom) Middleware() rest.Middleware {
	hdl := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			moreMd := GetCustomHeader(r)

			ctx = context.WithValue(ctx, customMdKey, moreMd)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}

	return rest.ToMiddleware(hdl)
}

func (p *PluginCustom) OnSendHeaders(r *http.Request, md metadata.MD) metadata.MD {
	//提取uid加载到md
	ctx := r.Context()
	switch ctx.Value(customMdKey).(type) {
	case []string:
		if uidData, ok := ctx.Value(customMdKey).([]string); ok {
			for _, uid := range uidData {
				sep := strings.Split(uid, ":")
				if len(sep) != 2 {
					continue
				}
				md.Append(sep[0], sep[1])
			}
		}
		return md
	}
	return md
}

func (p *PluginCustom) OnReceiveResponse(respJson string, md metadata.MD, w http.ResponseWriter) string {
	//获取metadata
	httpStatus := md.Get("X-Http-Status")
	if len(httpStatus) > 0 {
		statusCode, _ := strconv.Atoi(httpStatus[0])
		if http.StatusText(statusCode) != "" {
			w.WriteHeader(statusCode)
		}
	}
	return respJson
}

func GetCustomHeader(req *http.Request) []string {
	var commonHeader []string
	for _, s := range customHeaders {
		headerKey := req.Header.Values(s)
		if len(headerKey) > 0 {
			commonHeader = append(commonHeader, s+":"+strings.Trim(headerKey[0], "\n"))
		} else {
			commonHeader = append(commonHeader, s+":"+strings.Trim(req.FormValue(s), "\n"))
		}
	}
	return commonHeader
}
