# RPC 网关 Gateway

## 插件系统

一个插件对象为 `Plugin`，需要实现 `Name()` 返回名称，`Middleware()` 返回 HTTP 路由的中间件以及实现 `RpcHandler`。

```go
type Plugin interface {
    // Name 插件名，全局唯一
    Name() string

    // Middleware http 请求中间件
    Middleware() rest.Middleware

    // RpcHandler 实现 RpcHandler 的接口，如果不需要请导入 gateway.BasicRpcHandler
    RpcHandler
}
```

一个网关的请求到来时，首先处理其 HTTP 请求，然后根据路由请求对应的 gRPC，最后根据 gRPC 的响应构造 HTTP 响应。

`Plugin` 对象的 `Middleware` 部分处理到来的 HTTP 请求，`RpcHandler` 部分处理调用 gRPC 前的数据，gRPC 响应的数据以及构造 HTTP 响应。

一个 HTTP 请求进入网关后流程如下，gRPC 的 metadata 类似于 Headers。

【HTTP 请求】 -> 网关 -> 【gRPC 请求】 -> RPC 服务 -> 【gRPC 响应】 -> 网关 -> 【HTTP 响应】

## 配置文件

在配置文件中指定**插件名称**来使用插件，这里的插件名称是上面结构体里的 `Name()` 方法返回的字符串，应当**全局唯一**。

在 `Upstreams` - `Grpc` 同级配置该上游的插件链，根据配置的**插件顺序**调用插件。

``` yaml
Upstreams:
  - Grpc:
      Etcd:
        Hosts:
          - 172.16.32.16:2379
        Key: goods-rpc
    
    # 配置该上游的全局插件
    # 根据插件顺序进行调用
    Plugins:
        - jzAuth
        - plugin01
        - plugin02

    Mappings:
    # 以下省略

```

如果没有指定插件，则**默认全局开启** `jz-Auth` 插件。

如指定了该上游的全局插件，而要在某单一路由中不使用插件，则使用 `empty`。

以 goods-rpc 为例：

``` yaml
Name: goods-rpc-gateway
Host: 0.0.0.0
Port: 8080
Upstreams:
  - Grpc:
      Etcd:
      # 此处省略

    # 配置该上游的全局插件
    # 根据插件顺序进行调用
    Plugins:
        - jzAuth
        - plugin01
        - plugin02

    # reflection mode, no ProtoSet settings
    Mappings:
      - Method: get
        Path: /SearchParentVipProduct
        RpcPath: goods.Goods/SearchParentVipProduct
        Plugins: # 将覆盖全局插件
            - plugin03

      - Method: get
        Path: /GetParentVipProduct
        RpcPath: goods.Goods/GetParentVipProduct
        Plugins: # 覆盖掉该上游的全局插件，不使用插件
            - empty

      # 未配置插件，使用该上游的全局插件
      - Method: post
        Path: /AddParentVipProduct
        RpcPath: goods.Goods/AddParentVipProduct

      - Method: post
        Path: /DisableParentVipProductDefault
        RpcPath: goods.Goods/DisableParentVipProductDefault
        Plugins: # 只使用 plugin02
            - plugin02

    # 以下省略
```

## 插件开发

实现了 `gateway.Plugin` 接口即可完成插件开发。

一般情况下可以嵌入 `gateway.BasicRpcHandler` 实现 `gateway.RpcHandler` 接口，如需要实现其中某部分接口直接在插件重写即可。
`gateway.RpcHandler` 接口的定义是为了处理 `grpcurl.InvocationEventHandler` 接口的链式调用，`gateway.GrpcChainHandler` 实现了 `grpcurl.InvocationEventHandler` 接口，并把 `gateway.RpcHandler` 串成链式调用。

指定了多个插件的，将根据指定插件的顺序，以此调用插件的 `RpcHandler` 中的接口。

``` go
// RpcHandler Rpc处理
type RpcHandler interface {

    // OnReceiveResponse is called for each response message received.
    // gRPC 响应时调用，其中 string 是 HTTP 响应体，如需处理则在此处理并返回
    OnReceiveResponse(string, metadata.MD) string

    // OnReceiveTrailers is called when response trailers and final RPC status have been received.
    // gRPC 所有附加字段和最终状态接收时调用
    OnReceiveTrailers(*status.Status, metadata.MD) metadata.MD

    // OnResolveMethod is called with a descriptor of the method that is being invoked.
    OnResolveMethod(*desc.MethodDescriptor)

    // OnSendHeaders is called with the request metadata that is being sent. 
    // gRPC 发送请求时调用，将传入本次 HTTP 连接的 request
    OnSendHeaders(*http.Request, metadata.MD) metadata.MD

    // OnReceiveHeaders is called when response headers have been received.
    // gRPC 响应时调用，将保存 metadata 给 OnReceiveResponse 调用 
    OnReceiveHeaders(metadata.MD) metadata.MD
}
```

需要注意的是，插件对象是**全局唯一的，无状态的**；而**不应该在插件实现的内部保存状态变量**，否则将污染连接。

``` go
// Plugin02 插件02
type Plugin02 struct {
    // 如不全部实现 gateway.RpcHandler
    // 可内嵌 gateway.BasicRpcHandler 实现接口
    gateway.BasicRpcHandler

    // HttpStatusCode int // 不应该保存某一请求的状态，会污染请求
}

func NewPlugin02() *Plugin02 {
    return &Plugin02{}
}

// Name 插件名称，全局唯一
func (p *Plugin02) Name() string {
    return "plugin02"
}

// Middleware 实现的 HTTP 中间件
func (p *Plugin02) Middleware() rest.Middleware {
    // 不涉及 HTTP 中间件则直接返回 nil
    return nil
}

// OnSendHeaders 重写方法
func (p *Plugin02) OnSendHeaders(r *http.Request, md metadata.MD) metadata.MD {
    p.DealWith(md.Get("metadata-value"))
}
```

实现了 `gateway.Plugin` 后，需要在网关启动时注册该插件。

``` go
func main() {
    flag.Parse()
    var c gateway.GatewayConf
    conf.MustLoad(*configFile, &c)
    plugins.LoadRouteMap(&c)
    gw := gateway.MustNewServer(&c)
    
    // loadPlugins 加载插件
    loadPlugins(gw)
    defer gw.Stop()
    gw.Start()

}

// loadPlugins 加载插件
func loadPlugins(gw *gateway.Server) {
    gw.Register(plugins.NewPluginJzAuth(gw.Config))
    gw.Register(plugins.NewPluginEmpty())
}
```

## 文档
+ [go-zero 网关技术方案](https://jz-tech.yuque.com/jz-tech/ehcfio/ag2zr8hfel4ex45e)