package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/fullstorydev/grpcurl"
	"github.com/golang/protobuf/jsonpb"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
	"io"
	"net/http"
)

//c端：公共参数 https://jz-tech.yuque.com/jz-tech/lg6nsn/pql09s
var unSetKeys = []string{
	"security_key", //用户密钥
	"timestamp",    //当前时间戳（秒）
	"sign",         //内部php调用签名
	"program_type", //应用类型
	"channel_id",   //渠道id
	"appversion",   //app版本
	"appcode",      //只有安卓有，app代码逻辑用来判断实际的版本
	"app_type",     //安卓 ios区分
	"game_version", //游戏主包版本号
	"device",       //手机设备类型（如：A73 OPPO A73）
	"os",           //手机操作系统版本（如：Android 7.1.1）
	"brand",        //设备的品牌中文（苹果、华为、oppo ...）
}

// NewRequestParser creates a new request parser from the given http.Request and resolver.
func NewRequestParser(r *http.Request, resolver jsonpb.AnyResolver) (grpcurl.RequestParser, error) {
	vars := pathvar.Vars(r)
	params, err := httpx.GetFormValues(r)
	if err != nil {
		return nil, err
	}
	for k, v := range vars {
		params[k] = v
	}
	//X-Forwarded-For 数据处理
	unsetCheckVal(params)

	body, ok := getBody(r)
	if !ok {
		return buildJsonRequestParser(params, resolver)
	}

	m := make(map[string]any)
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		fmt.Println(fmt.Sprintf("%s：%+v", "body请求参数解析错误", err))
		m = make(map[string]any)
	}
	//body 数据处理
	unsetCheckVal(m)

	if len(params) == 0 {
		formatJson, _ := json.Marshal(m)
		formatReader := bytes.NewReader(formatJson)
		return grpcurl.NewJSONRequestParserWithUnmarshaler(formatReader, jsonpb.Unmarshaler{
			AllowUnknownFields: true,
			AnyResolver:        resolver,
		}), nil
	}
	for k, v := range params {
		m[k] = v
	}

	return buildJsonRequestParser(m, resolver)
}

func buildJsonRequestParser(m map[string]any, resolver jsonpb.AnyResolver) (
	grpcurl.RequestParser, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(m); err != nil {
		return nil, err
	}

	return grpcurl.NewJSONRequestParserWithUnmarshaler(&buf, jsonpb.Unmarshaler{AnyResolver: resolver, AllowUnknownFields: true}), nil
}

func getBody(r *http.Request) (io.Reader, bool) {
	if r.Body == nil {
		return nil, false
	}

	if r.ContentLength == 0 {
		return nil, false
	}

	if r.ContentLength > 0 {
		return r.Body, true
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r.Body); err != nil {
		return nil, false
	}

	if buf.Len() > 0 {
		return &buf, true
	}

	return nil, false
}

func unsetCheckVal(m map[string]any) {
	for _, key := range unSetKeys {
		delete(m, key)
	}
}
