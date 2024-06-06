package gateway

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/jsonpb"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"net/http"
	"strings"
)

// pluginDefault 完全没设置插件的时候默认empty空白插件
//var pluginDefault = []string{"jzAuth"}
var pluginDefault = []string{"empty"}

// Plugin 插件，可以拦截和添加中间件
// 默认需要实现以下接口，若个别部分不需要则可返回nil
type Plugin interface {
	// Name 插件名
	Name() string

	// Middleware http 请求中间件
	Middleware() rest.Middleware

	// RpcHandler 实现 RpcHandler 的接口，如果不需要请导入 internal.BasicRpcHandler
	RpcHandler
}

// PluginManager 插件管理，在网关启动时接入插件
type PluginManager struct {
	// plugins 插件的名称和对应插件对象
	plugins map[string]Plugin

	// pluginRoutes 路由到插件名称的映射
	pluginRoutes map[string][]Plugin
}

func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins:      make(map[string]Plugin),
		pluginRoutes: map[string][]Plugin{},
	}
}

// MustGetPlugin 获取一个插件，否则 panic
func (pm *PluginManager) MustGetPlugin(name string) Plugin {
	pl, has := pm.plugins[name]
	if !has {
		logx.Must(fmt.Errorf("找不到插件：%s", name))
		return nil
	}
	return pl
}

// Register 注册插件，不支持并发
func (pm *PluginManager) Register(p Plugin) {
	if nil == p {
		logx.Must(errors.New("插件对象为空"))
	} else if len(p.Name()) == 0 {
		logx.Must(errors.New("插件名称为空"))
	} else if _, has := pm.plugins[p.Name()]; has {
		logx.Must(errors.New("已存在同名插件"))
	}

	pm.plugins[p.Name()] = p
}

// RouteKey 唯一确定一个路由
// 因为一个网关里不会出现两条 Method 和路径都相同的 http
func (pm *PluginManager) RouteKey(method, httpPath string) string {
	httpPath = strings.ReplaceAll(httpPath, "//", "/")
	return strings.ToUpper(method) + httpPath
}

// LoadRouteMapping 加载路由并记录插件
func (pm *PluginManager) LoadRouteMapping(up *Upstream, rm *RouteMapping) {
	// 如果设置了插件，则用插件
	plugins := rm.Plugins

	// 如果未设置插件，则用默认插件
	if len(rm.Plugins) == 0 && len(up.Plugins) == 0 {
		//默认插件 PluginJzAuth
		plugins = pluginDefault
	} else if len(rm.Plugins) == 0 {
		plugins = up.Plugins
	}

	k := pm.RouteKey(rm.Method, rm.Path)
	if strings.Contains(rm.Path, ":") {
		logx.Must(errors.New(fmt.Sprintf("路由配置有误，不允许在uri配置变量参数 ： %s ", rm.Path)))
	}
	for _, name := range plugins {
		pl := pm.MustGetPlugin(name)
		pm.pluginRoutes[k] = append(pm.pluginRoutes[k], pl)
	}
}

// WrapMiddleware 注入中间件
func (pm *PluginManager) WrapMiddleware(r *rest.Route) rest.Route {
	var (
		plgs = pm.pluginRoutes[pm.RouteKey(r.Method, r.Path)]
		mws  = make([]rest.Middleware, 0, len(plgs))
	)

	for _, plg := range plgs {
		mw := plg.Middleware()
		if nil == mw {
			continue
		}

		mws = append(mws, mw)
	}

	rs := rest.WithMiddlewares(mws, *r)
	if len(rs) == 0 {
		return *r
	}
	return rs[0]
}

// GetRpcHandler 设置 RPC 处理插件
func (pm *PluginManager) GetRpcHandler(w http.ResponseWriter, r *http.Request, resolver jsonpb.AnyResolver, origName bool) *GrpcChainHandler {
	plgs := pm.pluginRoutes[pm.RouteKey(r.Method, r.URL.Path)]
	handlers := make([]RpcHandler, len(plgs))

	for i, pl := range plgs {
		handlers[i] = pl
	}

	return &GrpcChainHandler{
		writer:  w,
		request: r,
		marshaler: jsonpb.Marshaler{
			OrigName:     origName,
			EmitDefaults: true,
			AnyResolver:  resolver,
		},
		chains: handlers,
	}
}
