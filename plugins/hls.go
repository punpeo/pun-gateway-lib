package plugins

import (
	gateway "github.com/punpeo/pun-gateway-lib"
	"github.com/zeromicro/go-zero/rest"
	"google.golang.org/grpc/metadata"
	"net/http"
	"net/url"
)

// PluginEmpty 空插件，为屏蔽某一路由的插件使用
type PluginHls struct {
	gateway.BasicRpcHandler
}

func NewPluginHls() *PluginHls {
	return &PluginHls{}
}

func (p *PluginHls) Name() string {
	return "hls"
}

func (p *PluginHls) Middleware() rest.Middleware {
	hdl := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//设置响应头
			r.Header.Set("Content-Type", "application/x-mpegurl; charset=utf-8")
			next.ServeHTTP(w, r)
		})
	}

	return rest.ToMiddleware(hdl)
}

func (p *PluginHls) OnReceiveResponse(respJson string, md metadata.MD, w http.ResponseWriter) string {
	//设置响应头部
	//获取metadata
	//w.(http.ResponseWriter).Header().Set("Access-Control-Allow-Origin", "*")
	w.(http.ResponseWriter).Header().Set("Content-Type", "application/x-mpegurl; charset=utf-8")
	fileData := md.Get("X-Data")
	if len(fileData) > 0 {
		respJson = fileData[0]
		respJson, _ = url.QueryUnescape(respJson)
	}
	return respJson
}
