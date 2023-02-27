package dxauth

import (
	"bytes"
	"crypto/rsa"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/dgrijalva/jwt-go"
	config "github.com/fulltimelink/gateway/api/gateway/config/v1"
	"github.com/fulltimelink/gateway/middleware"
	"github.com/go-kratos/kratos/v2/log"
)

var _nopBody = io.NopCloser(&bytes.Buffer{})
var pubkeys *rsa.PublicKey

func init() {

	// --  @# 读取公钥
	keyData, err := os.ReadFile("pubkey/pubkey")
	if err != nil {
		log.Fatalf("Occur error when read pubkey:%+v\n", err)
	}
	// --  @# 校验公钥
	pubkeys, err = jwt.ParseRSAPublicKeyFromPEM(keyData)
	if err != nil {
		log.Fatalf("Occur error when parse pubkey:%+v\n", err)
	}

	middleware.Register("dxauth", Middleware)
}

// Middleware is a jwt RSA256 auth middleware.
func Middleware(c *config.Middleware) (middleware.Middleware, error) {
	return func(next http.RoundTripper) http.RoundTripper {
		return middleware.RoundTripperFunc(func(req *http.Request) (reply *http.Response, err error) {
			token := strings.TrimSpace(req.Header.Get("Authorization"))
			prefix := "Bearer"
			reqToken := strings.TrimSpace(strings.TrimPrefix(token, prefix))
			if "" == reqToken {
				log.Warnf("Token parse empty: %s \n", token)
				return &http.Response{
					Status:     http.StatusText(http.StatusForbidden),
					StatusCode: http.StatusForbidden,
					Body:       _nopBody,
				}, nil
			}

			// --  @# 检验jwt格式
			parts := strings.Split(reqToken, ".")
			if 3 != len(parts) {
				log.Warnf("Not a jwt rs256 Token: %s \n", reqToken)
				return &http.Response{
					Status:     http.StatusText(http.StatusForbidden),
					StatusCode: http.StatusForbidden,
					Body:       _nopBody,
				}, nil
			}
			// --  @# jwt RSA256 校验
			err = jwt.SigningMethodRS256.Verify(strings.Join(parts[0:2], "."), parts[2], pubkeys)
			if err != nil {
				log.Warnf("Occur error when verify Jwt rsa256 Token: %s \n", reqToken)
				return &http.Response{
					Status:     http.StatusText(http.StatusForbidden),
					StatusCode: http.StatusForbidden,
					Body:       _nopBody,
				}, nil
			}

			reply, err = next.RoundTrip(req)
			return reply, err
		})
	}, nil
}
