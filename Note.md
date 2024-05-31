附：官方文档 : https://go-zero.dev/docs/tutorials/gateway/grpc
概述
随着微服务架构的流行，gRPC 作为一种高性能、跨语言的远程过程调用（RPC）框架被广泛应用。但是，gRPC 并不适用于所有应用场景。例如，当客户端不支持 gRPC 协议时，或者需要将 gRPC 服务暴露给 Web 应用程序时，需要一种将 RESTful API 转换为 gRPC 的方式。因此，gRPC 网关应运而生。
gRPC 网关在 go-zero 中的实现
go-zero 中的 gRPC 网关是一个 HTTP 服务器，它将 RESTful API 转换为 gRPC 请求，然后将 gRPC 响应转换为 RESTful API。大致流程如下：
1. 从 proto 文件中解析出 gRPC 服务的定义。
2. 从 配置文件中解析出 gRPC 服务的 HTTP 映射规则。
3. 根据 gRPC 服务的定义和 HTTP 映射规则，生成 gRPC 服务的 HTTP 处理器。
4. 启动 HTTP 服务器，处理 HTTP 请求。
5. 将 HTTP 请求转换为 gRPC 请求。
6. 将 gRPC 响应转换为 HTTP 响应。
7. 返回 HTTP 响应。
二、 业务目标
解决php、js、app跨语言项目且暂不支持grpc协议，需要直接调度到rpc的问题。
1、进入项目根目录
2、启动你的RPC服务：如study的rpc服务：
go run rpc/study/study.go -f etc/test/study-rpc.yaml
3、生成pd文件(注：生成的pb文件放zero-gateway项目在etc目录下)
pb文件方式已弃用，调整为所有环境rpc服务开启反射 ：
//if c.Mode == service.DevMode || c.Mode == service.TestMode {
//	reflection.Register(grpcServer)
//}
reflection.Register(grpcServer)
文件：etc/test/study/study-rpc-gateway.yaml
rpcPath字段配置说明：{proto.PackageName}/{proto.ServiceName}/{proto.Service.rpcName}
Name: study-rpc-gateway
Host: 0.0.0.0
Port: 8080
Timeout: 180000
CpuThreshold: 0

AccessControlRpc:
Etcd:
Hosts:
- 172.16.32.16:2379
Key: access-control-rpc
Timeout: 180000

Upstreams:
- Grpc:
Etcd:
Hosts:
- 172.16.32.16:2379
Key: study-rpc
# ProtoSets:
#   - etc/test/study/study.pb
Mappings:
- Method: post
Path: /campUnlock/batchQueryUserUnlockChapterInfoByDay
RpcPath: study.study/batchQueryUserUnlockChapterInfoByDay
#默认是true 校验security_key / sign / Authorization
AuthCheck : true
- Method: post
Path: /campUnlock/batchQueryUserUnlockChapterNumByDay
RpcPath: study.study/batchQueryUserUnlockChapterNumByDay
AuthCheck: true
- Method: get
Path: /open/test/test
RpcPath: study.study/test
AuthCheck: true
DshAdminConf:
DshAdminUrl: "https://adm-dsh-test.jianzhiweike.net/"
ApiKey: "*****"

Prometheus:
Host: 0.0.0.0
Port: 9090
Path: /metrics

#解析security_key
Safe:
Key: "******"
Iv: "******"
#解析php sign
SignKey: "******"
#解析后台 jwt
AuthKey:  "******"
