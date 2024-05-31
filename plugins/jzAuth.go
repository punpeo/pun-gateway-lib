package plugins

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/punpeo/pun-gateway-lib/access/control/controlClient"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	gateway "github.com/punpeo/pun-gateway-lib"

	"context"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/punpeo/punpeo-lib/rest/result"
	"github.com/punpeo/punpeo-lib/rest/xerr"
	"github.com/punpeo/punpeo-lib/utils/jzcrypto"
	"github.com/spf13/cast"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/zrpc"

	"google.golang.org/grpc/metadata"
)

// c端：公共参数 https://jz-tech.yuque.com/jz-tech/lg6nsn/pql09s
var appCommHeader = []string{
	"program_type", //应用类型
	"channel_id",   //渠道id
	"appversion",   //app版本
	"appcode",      //只有安卓有，app代码逻辑用来判断实际的版本
	"app_type",     //安卓 ios区分
	"game_version", //游戏主包版本号
	"device",       //手机设备类型（如：A73 OPPO A73）
	"os",           //手机操作系统版本（如：Android 7.1.1）
	"brand",        //设备的品牌中文（苹果、华为、oppo ...）
	"security_key", //用户登录秘钥
}

// PluginJzAuth 简知校验插件
type PluginJzAuth struct {
	gateway.BasicRpcHandler

	gw               *gateway.Server
	config           *gateway.GatewayConf
	accessControlRpc controlClient.Control
}

const mdKey = "moreMd"

func NewPluginJzAuth(c *gateway.GatewayConf) *PluginJzAuth {
	return &PluginJzAuth{
		config:           c,
		accessControlRpc: controlClient.NewControl(zrpc.MustNewClient(c.AccessControlRpc)),
	}
}

func (p *PluginJzAuth) Name() string {
	return "jzAuth"
}

func (p *PluginJzAuth) Middleware() rest.Middleware {
	hdl := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			moreMd, err, code := HeaderProcess(p.config, p.accessControlRpc, r)
			if err != nil {
				httpx.WriteJson(w, http.StatusOK, &result.ResponseSuccessBean{Code: code, Msg: err.Error(), Data: nil})
				return
			}

			ctx = context.WithValue(ctx, mdKey, moreMd)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}

	return rest.ToMiddleware(hdl)
}

func (p *PluginJzAuth) OnSendHeaders(r *http.Request, md metadata.MD) metadata.MD {
	//提取uid加载到md
	ctx := r.Context()
	switch ctx.Value(mdKey).(type) {
	case []string:
		if uidData, ok := ctx.Value(mdKey).([]string); ok {
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

func (p *PluginJzAuth) OnReceiveResponse(respJson string, md metadata.MD, _ http.ResponseWriter) string {
	respCode, respMsg := 1000, "成功"

	xStatusCodeArr := md.Get("X-Status-Code")
	if len(xStatusCodeArr) > 0 {
		codeInt64, _ := strconv.ParseInt(xStatusCodeArr[0], 10, 64)
		respCode = int(codeInt64)
	}

	xErrorMessage := md.Get("X-Error-Message")
	if len(xErrorMessage) > 0 {
		respMsg = xErrorMessage[0]
	}

	xData := md.Get("X-Data")
	if len(xData) > 0 {
		respJson = xData[0]
	}

	return fmt.Sprintf("{\"code\":%d, \"msg\": \"%s\", \"data\":%s}", respCode, respMsg, respJson)
}

// HeaderProcess http header处理校验和提取uid
func HeaderProcess(config *gateway.GatewayConf, accessControlRpc controlClient.Control, req *http.Request) (moreMd []string, err error, code uint32) {
	sk, sign, auth, sysType, bodyData := getCheckInfo(req)
	//校验配置文件
	if _, ok := config.AuthCheckMapping[strings.ToLower(req.Method)]; !ok {
		err = fmt.Errorf(fmt.Sprintf("route mapping http request method empty：%s | %s", req.RequestURI, req.Method))
		return
	}
	methodMatch := config.AuthCheckMapping[strings.ToLower(req.Method)]
	verifyFuncControlMatch := config.VerifyFuncControlMapping[strings.ToLower(req.Method)]
	//默认检验Authorization / security_key / sign
	uri := strings.Split(req.RequestURI, "?")[0]
	uri = strings.Replace(uri, "//", "/", 1)
	var uid string
	if AuthCheck, ok := methodMatch[strings.ToLower(uri)]; ok {
		if AuthCheck {
			if len(sign) > 0 { //php请求较多，优先判断
				if !VerifySign(bodyData, config.SignKey) {
					err = fmt.Errorf("sign校验失败")
					code = xerr.SERVER_COMMON_ERROR
					return
				}
			} else if len(sk) > 0 { //用户端
				uid, err = DecodeSecurityKey(sk, config.Safe.Key, config.Safe.Iv)
				//app公共头部提取
				ret := GetAppCommonHeader(req)
				ret = append(ret, "uid:"+uid)
				return ret, err, code
			} else if len(auth) > 0 { //管理后台js
				//校验 Authorization => uid
				resp, rpcErr := accessControlRpc.ParseAuthToken(req.Context(), &controlClient.ParseAuthTokenReq{Token: auth})
				if rpcErr == nil && resp != nil && resp.AdminId != 0 {
					uid = strconv.FormatInt(resp.AdminId, 10)
				} else {
					err = fmt.Errorf("Authorization 校验失败：%+v", rpcErr)
					code = xerr.LOGIN_EXPIRE_ERROR
					return
				}

				if VerifyFuncControl, ok := verifyFuncControlMatch[strings.ToLower(uri)]; ok && VerifyFuncControl {
					sysType, _ := strconv.Atoi(sysType)
					// 校验 功能权限
					resp, rpcErr := accessControlRpc.VerifyFuncControl(req.Context(), &controlClient.VerifyFuncControlReq{
						AdminId: int32(resp.AdminId),
						Url:     uri,
						Method:  strings.ToLower(req.Method),
						SysType: int32(sysType),
					})
					if rpcErr != nil || resp == nil || !resp.Result {
						err = fmt.Errorf("功能权限 校验失败：err：%+v；data：%+v", rpcErr, resp)
						code = xerr.MISSED_FUNC_PERMISSIONS_ERROR
						return
					}
				}

				return []string{"uid:" + uid}, err, code
			} else {
				err = fmt.Errorf("签名检验失败，请先登录或授权")
				code = xerr.LOGIN_EXPIRE_ERROR
				return
			}
		} else {
			if len(sk) > 0 {
				//家长端首页不强制登录，但是如果有传递security_key，也需要获取用户id
				uid, _ = DecodeSecurityKey(sk, config.Safe.Key, config.Safe.Iv)
			}
			ret := GetAppCommonHeader(req)
			ret = append(ret, "uid:"+uid)
			return ret, err, code
		}
	} else {
		ret := GetAppCommonHeader(req)
		ret = append(ret, "uid:"+uid)
		return ret, err, code
	}
	return
}

// 默认拦截 Authorization / security_key / sign
func getCheckInfo(req *http.Request) (sk, sign, auth, sysType string, signData map[string]any) {
	authArr := req.Header.Values("Authorization")
	headerKey := req.Header.Values("Security_key")
	signKey := req.Header.Values("sign")
	sysTypeArr := req.Header.Values("SysType")
	//auth
	if len(authArr) > 0 {
		auth = authArr[0]
	}
	//sysType
	if len(sysTypeArr) > 0 {
		sysType = sysTypeArr[0]
	}
	//sk
	if len(headerKey) > 0 {
		sk = headerKey[0]
	} else {
		sk = req.FormValue("security_key")
	}
	sk = strings.Trim(sk, "\n")
	sk = strings.TrimSpace(sk)
	//sign
	if len(signKey) > 0 {
		sign = signKey[0]
	} else {
		sign = req.FormValue("sign")
		if len(sign) != 0 {
			signForm := make(map[string]any)
			for k, v := range req.Form {
				if len(v) > 0 {
					signForm[k] = cast.ToString(v[0])
				} else {
					signForm[k] = ""
				}
			}
			signData = signForm
		} else {
			if body, ok := getBody(req); ok {
				m := make(map[string]any)
				if err := json.NewDecoder(body).Decode(&m); err == nil {
					if bodySign, okMap := m["sign"]; okMap {
						sign = cast.ToString(bodySign)
					}
					signData = m
					formatJson, _ := json.Marshal(m)
					req.Body = io.NopCloser(bytes.NewReader(formatJson))
				}
			}
		}
	}
	return sk, sign, auth, sysType, signData
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

// VerifySign 校验sign
func VerifySign(data map[string]any, key string) bool {
	var keys []string
	//判断是否有sign字段
	_, ok := data["sign"]
	if !ok {
		return false
	}
	sign := cast.ToString(data["sign"])
	delete(data, "sign")
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sortData map[string]any
	sortData = make(map[string]any)
	for _, k := range keys {
		sortData[k] = data[k]
	}
	urlParams := ToUrlParams(sortData, keys)
	h := md5.New()
	str := urlParams + "&key=" + key
	h.Write([]byte(str))
	makeSign := hex.EncodeToString(h.Sum(nil))
	return makeSign == sign
}

func ToUrlParams(data map[string]interface{}, keys ...[]string) string {
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

// DecodeSecurityKey 简知security_key解析
func DecodeSecurityKey(sk, key, iv string) (string, error) {
	if sk != "" {
		if strings.Contains(sk, " ") {
			sk = url.QueryEscape(sk)
		}
		decodeToken, _ := url.PathUnescape(sk)
		decodeToken2, _ := url.PathUnescape(decodeToken)
		dataStr, err := jzcrypto.TripleDesDecrypt(decodeToken2, key, iv) //线上大部分都是这种urlencode2次的。
		if err != nil {
			dataStr, err = jzcrypto.TripleDesDecrypt(decodeToken, key, iv)
			if err != nil {
				dataStr, err = jzcrypto.TripleDesDecrypt(sk, key, iv)
				if err != nil {
					return "", err
				}
			}
		}
		data, _ := url.ParseQuery(dataStr)
		if data.Has("user_id") && data.Has("expire_time") {
			return data["user_id"][0], err
		}
	}
	return "", fmt.Errorf("info为空")
}

// JwtParseToken 解析管理后台 jwt
func JwtParseToken(securetKey string, tokenString string) (*LoginAccount, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JwtClaims{}, func(tokenString *jwt.Token) (i interface{}, e error) {
		return []byte(securetKey), nil
	})

	if err != nil {
		if ve, ok := err.(*jwt.ValidationError); ok {
			if ve.Errors&jwt.ValidationErrorMalformed != 0 { // Token不正确
				return nil, errors.New("token不正确，请重新登录")
			} else if ve.Errors&jwt.ValidationErrorExpired != 0 {
				// Token is expired
				return nil, errors.New("token已过期，请重新登录") // Token已过期
			} else if ve.Errors&jwt.ValidationErrorNotValidYet != 0 {
				return nil, errors.New("token无效，请重新登录") // Token无效
			} else {
				return nil, errors.New("这不是一个token，请重新登录") // 这不是一个token
			}
		}
	}

	// 检查令牌是否有效
	if claims, ok := token.Claims.(*JwtClaims); ok && token.Valid {
		return &claims.LoginAccount, nil
	}

	return nil, errors.New("这不是一个token，请重新登录")
}

type JwtClaims struct {
	jwt.RegisteredClaims
	LoginAccount LoginAccount `json:"data"`
}

type LoginAccount struct {
	AdminId    int64  `json:"admin_id"`
	Username   string `json:"user_name"`
	RealName   string `json:"real_name"`
	ExpireTime int64  `json:"expire_time"`
}

func GetAppCommonHeader(req *http.Request) []string {
	var commonHeader []string
	for _, s := range appCommHeader {
		headerKey := req.Header.Values(strings.ToUpper(s))
		if len(headerKey) > 0 {
			if s == "brand" {
				commonHeader = append(commonHeader, s+":"+url.QueryEscape(strings.Trim(headerKey[0], "\n")))
			} else {
				commonHeader = append(commonHeader, s+":"+strings.Trim(headerKey[0], "\n"))
			}
		} else {
			if s == "brand" {
				commonHeader = append(commonHeader, s+":"+url.QueryEscape(strings.Trim(req.FormValue(s), "\n")))
			} else {
				commonHeader = append(commonHeader, s+":"+strings.Trim(req.FormValue(s), "\n"))
			}
		}
	}
	return commonHeader
}
