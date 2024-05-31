package plugins

import (
	"fmt"
	"github.com/punpeo/punpeo-lib/rest/result"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
	gateway "github/punpeo/pun-gateway-lib"
	"github/punpeo/pun-gateway-lib/plugins/uridispatch"
	"net/http"
	"strings"
)

type PluginUriDispatch struct {
	gateway.BasicRpcHandler

	gw     *gateway.Server
	config *gateway.GatewayConf
}

func NewPluginUriDispatch(config *gateway.GatewayConf) *PluginUriDispatch {
	return &PluginUriDispatch{
		config: config,
	}
}

func (p *PluginUriDispatch) Name() string {
	return "uriDispatch"
}

func (p *PluginUriDispatch) Middleware() rest.Middleware {
	hdl := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uri := strings.Split(r.RequestURI, "?")[0]
			uri = strings.Replace(uri, "//", "/", 1)
			routeConfigMap, ok := p.config.UpstreamsRouteMap[strings.ToLower(r.Method)]
			if !ok {
				err := fmt.Errorf(fmt.Sprintf("route mapping http request method empty：%s | %s", r.RequestURI, r.Method))
				httpx.WriteJson(w, http.StatusOK, &result.ResponseSuccessBean{Msg: err.Error(), Data: nil})
				return
			}
			routeConfig, ok2 := routeConfigMap[strings.ToLower(uri)]
			if !ok2 {
				err := fmt.Errorf(fmt.Sprintf("route mapping http request method empty：%s | %s", r.RequestURI, r.Method))
				httpx.WriteJson(w, http.StatusOK, &result.ResponseSuccessBean{Msg: err.Error(), Data: nil})
				return
			}
			uridispatch.Mode = p.config.Mode
			uridispatch.NewUriDispatch(routeConfig).Handler(w, r, next)
		})
	}

	return rest.ToMiddleware(hdl)
}
