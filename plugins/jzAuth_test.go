package plugins

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cast"
	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
	gateway "github/punpeo/pun-gateway-lib"
)

const (
	id      = 1
	addr    = "127.0.0.1:58085"
	signKey = "71srx3YW31fqC8Oa"

	// token 和 securityKey 手            动填入
	securityKey = "U1%252FOIwAfq3ibmfgN%252FjxNVaJnLfidXWt%252FqENT7oOjZl2LfDjIVj1zXRGPhP8xq0MFm9CBmQkMfH5ZyggN8TYNrdRuV3y4%252FcWNXT%252BRIT%252FT26A%253D"
	token       = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJkYXRhIjp7ImFkbWluX2lkIjo1OTUsImV4cGlyZV90aW1lIjoxNjk4MjAwMTYwfSwiaXNzIjoianpraiIsImV4cCI6MTY5ODIwMDE2MH0.DyWtVqsvV2EIDe1w45cIr4ljCE1P_kAnHk_kb6c2Ta0"

	mobileUA = "Go Test"
	pcUA     = "Go Test"
)

var wg *sync.WaitGroup

var c = gateway.GatewayConf{
	RestConf: rest.RestConf{
		Host: "127.0.0.1",
		Port: 58085,
	},
	Upstreams: []gateway.Upstream{
		{
			Name: "goods-rpc",
			Grpc: zrpc.RpcClientConf{
				Etcd: discov.EtcdConf{
					Hosts: []string{"172.16.32.16:2379"},
					Key:   "goods-rpc",
				},
				Endpoints:     nil,
				Target:        "",
				App:           "",
				Token:         "",
				NonBlock:      false,
				Timeout:       0,
				KeepaliveTime: 0,
				Middlewares:   zrpc.ClientMiddlewaresConf{},
			},
			ProtoSets: nil,
			Mappings: []gateway.RouteMapping{
				{
					Method:  "get",
					Path:    "/GetParentVipProduct",
					RpcPath: "goods.Goods/GetParentVipProduct",
				},
			},
			Plugins: []string{"jzAuth"},
		},
	},
	AccessControlRpc: zrpc.RpcClientConf{
		Etcd: discov.EtcdConf{
			Hosts: []string{"172.16.32.16:2379"},
			Key:   "access-control-rpc",
		},
		Timeout: 1800000,
	},
	AuthCheckMapping: nil,
	Safe: gateway.Safe{
		Key: "5qHIk7yMbGu29SFA6",
		Iv:  "Xoji5qa9",
	},
	SignKey: signKey,
}

func TestSecurityKey(t *testing.T) {
	if nil != wg {
		defer wg.Done()
	}
	client := &http.Client{}
	reqUrl := fmt.Sprintf("http://%s/GetParentVipProduct?id=%d&security_key=%s", addr, id, securityKey)
	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("User-Agent", mobileUA)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s\n", bodyText)

	if jsoniter.Get(bodyText, "code").ToInt() != 1000 {
		t.FailNow()
	}
}

func TestToken(t *testing.T) {
	if nil != wg {
		defer wg.Done()
	}
	client := &http.Client{}
	reqUrl := fmt.Sprintf("http://%s/GetParentVipProduct?id=%d", addr, id)
	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("User-Agent", pcUA)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s\n", bodyText)

	if jsoniter.Get(bodyText, "code").ToInt() != 1000 {
		t.FailNow()
	}
}

func TestSign(t *testing.T) {
	if nil != wg {
		defer wg.Done()
	}
	var (
		keys []string
		data = map[string]any{
			"timestamp": time.Now().Unix(),
			"id":        id,
		}
	)

	// 构造 sign
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sortData map[string]any
	sortData = make(map[string]any)
	for _, k := range keys {
		sortData[k] = data[k]
	}

	urlParams := toUrlParams(sortData, keys)
	urlParams = urlParams + "&key=" + signKey
	h := md5.New()
	h.Write([]byte(urlParams))
	sign := hex.EncodeToString(h.Sum(nil))

	data["sign"] = sign

	// 发送http请求
	client := &http.Client{}
	var jsonData = bytes.NewBuffer(nil)
	err := json.NewEncoder(jsonData).Encode(data)
	if nil != err {
		t.Fatal(err)
	}

	reqUrl := fmt.Sprintf("http://%s/GetParentVipProduct", addr)
	req, err := http.NewRequest("GET", reqUrl, jsonData)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", pcUA)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("%s", bodyText))

	if jsoniter.Get(bodyText, "code").ToInt() != 1000 {
		t.FailNow()
	}
}

func TestJzAuth(t *testing.T) {
	wg = new(sync.WaitGroup)
	wg.Add(3)
	go TestSecurityKey(t)
	go TestToken(t)
	go TestSign(t)
	wg.Wait()
}

func TestServer_Start(t *testing.T) {
	gw := gateway.MustNewServer(&c)
	gw.Register(NewPluginJzAuth(gw.Config))
	gw.Register(NewPluginEmpty())
	defer gw.Stop()
	gw.Start()

}

func toUrlParams(data map[string]interface{}, keys ...[]string) string {
	var params []string
	if len(keys) > 0 {
		for _, k := range keys[0] {
			if "sign" == k {
				continue
			}
			v := data[k]
			switch v.(type) {
			case string, int, int64, int32, int16, int8, float64, float32:
				params = append(params, k+"="+cast.ToString(v))
			}
		}
	} else {
		for k, v := range data {
			if "sign" == k {
				continue
			}
			switch v.(type) {
			case string, int, int64, int16, int8, float64, float32:
				params = append(params, k+"="+cast.ToString(v))
			}
		}
	}
	return strings.Join(params, "&")
}
