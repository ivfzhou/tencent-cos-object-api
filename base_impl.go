/*
 * Copyright (c) 2025 ivfzhou
 * tencent-cos-object-api is licensed under Mulan PSL v2.
 * You can use this software according to the terms and conditions of the Mulan PSL v2.
 * You may obtain a copy of Mulan PSL v2 at:
 *          http://license.coscl.org.cn/MulanPSL2
 * THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND,
 * EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT,
 * MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
 * See the Mulan PSL v2 for more details.
 */

package cos

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type baseImpl struct {
	host, appKey, secretKey string
	options
}

// Ping 测试连接。
func (c *baseImpl) Ping(ctx context.Context) error {
	_, err := c.head(ctx, "ping")
	if errors.Is(err, ErrNotExists) {
		err = nil
	}
	return err
}

// GenerateAuthorization 生成 HTTP 请求的签名字符串。
func (c *baseImpl) GenerateAuthorization(fileId, method string, query url.Values, header http.Header,
	expiration time.Duration) string {

	fileId = suitFileId(fileId)
	if query == nil {
		query = url.Values{}
	}
	if header == nil {
		header = http.Header{}
	}

	// 生成签名有效时间 KeyTime。
	var keyTime string
	{
		now := time.Now()
		keyTime = fmt.Sprintf("%d;%d", now.Unix(), now.Add(expiration).Unix())
	}

	// 生成 UrlParamList 和 HttpParameters。
	var httpParameters string
	var urlParamList string
	{
		keyList := make([]string, 0, len(query))
		paramList := make([]string, 0, len(query))
		tmp := make(map[string][]string, len(query))
		for k, v := range query {
			n := strings.ToLower(urlEncode(k))
			tmp[n] = v
			for range v {
				keyList = append(keyList, n)
			}
		}
		sort.Strings(keyList)
		pre := ""
		for _, v := range keyList {
			if pre == v {
				continue
			}
			pre = v
			for _, m := range tmp[v] {
				paramList = append(paramList, fmt.Sprintf("%s=%s", v, urlEncode(m)))
			}
		}
		urlParamList = strings.Join(keyList, ";")
		httpParameters = strings.Join(paramList, "&")
	}

	// 生成 HeaderList 和 HttpHeaders。
	var headerList string
	var httpHeaders string
	{
		keyList := make([]string, 0, len(header))
		paramList := make([]string, 0, len(header))
		tmp := make(map[string][]string, len(header))
		for k, v := range header {
			n := strings.ToLower(urlEncode(k))
			tmp[n] = v
			for range v {
				keyList = append(keyList, n)
			}
		}
		sort.Strings(keyList)
		pre := ""
		for _, v := range keyList {
			if pre == v {
				continue
			}
			pre = v
			for _, n := range tmp[v] {
				paramList = append(paramList, fmt.Sprintf("%s=%s", v, urlEncode(n)))
			}
		}
		headerList = strings.Join(keyList, ";")
		httpHeaders = strings.Join(paramList, "&")
	}

	// 生成 API 密钥 SignKey。
	var signKey string
	{
		hash := hmac.New(sha1.New, []byte(c.secretKey))
		hash.Write([]byte(keyTime))
		signKey = fmt.Sprintf("%x", hash.Sum(nil))
	}

	// 生成过程参数 HttpString。
	var httpString string
	{
		httpString = fmt.Sprintf("%s\n/%s\n%s\n%s\n",
			strings.ToLower(method), fileId, httpParameters, httpHeaders)
	}

	// 生成过程参数 StringToSign。
	var stringToSign string
	{
		hash := sha1.New()
		hash.Write([]byte(httpString))
		stringToSign = fmt.Sprintf("sha1\n%s\n%s\n", keyTime, fmt.Sprintf("%x", hash.Sum(nil)))
	}

	// 生成过程参数 Signature。
	var signature string
	{
		hash := hmac.New(sha1.New, []byte(signKey))
		hash.Write([]byte(stringToSign))
		signature = fmt.Sprintf("%x", hash.Sum(nil))
	}

	// 生成签名。
	return fmt.Sprintf(
		"q-sign-algorithm=sha1&q-ak=%s&q-sign-time=%s&q-key-time=%s&q-header-list=%s&q-url-param-list=%s&q-signature=%s",
		c.appKey, keyTime, keyTime, headerList, urlParamList, signature)

}

// 发送 HTTP 请求。
func (c *baseImpl) sendHttp(ctx context.Context, req *http.Request) (rsp *http.Response, err error) {
	defer rollbackRequest(req) // 回收请求体。
	req = req.WithContext(ctx)
	if c.client == nil {
		rsp, err = http.DefaultClient.Do(req)
	} else {
		rsp, err = c.client.Do(req)
	}
	if err != nil {
		return nil, err
	}
	if rsp == nil {
		return nil, errors.New("http response object is nil")
	}

	// 非成功的响应码就返回错误。
	if !(rsp.StatusCode >= 200 && rsp.StatusCode < 300) {
		if rsp.StatusCode == http.StatusNotFound {
			closeRsp(rsp)
			return nil, ErrNotExists
		}
		return nil, fmt.Errorf("status codeis %d, method is %v, reqPath is %v, rspBody is %s",
			rsp.StatusCode, req.Method, req.URL.Path, string(readAndClose(rsp)))
	}

	return rsp, nil
}

// 生成 HTTP 请求体。
func (c *baseImpl) genReq(method, fileId string, query url.Values, header http.Header, content []byte) *http.Request {
	// 生成请求头。
	if query == nil {
		query = url.Values{}
	}
	if header == nil {
		header = http.Header{}
	}
	header.Set("Host", c.host)
	if len(content) > 0 {
		header.Set("Content-Length", strconv.Itoa(len(content)))
	}
	header.Set("Authorization", c.GenerateAuthorization(fileId, method, query, header, AuthExpirationTime))

	// 生成 URL。
	schema := "http"
	if c.tls {
		schema = "https"
	}
	u, _ := url.Parse(fmt.Sprintf("%s://%s/%s?%s", schema, c.host, strings.TrimLeft(fileId, "/"), query.Encode()))

	// 获取响应体，并赋值。
	req := getRequest()
	req.Method = method
	req.URL = u
	req.Header = header
	req.Body = io.NopCloser(bytes.NewReader(content))
	req.ContentLength = int64(len(content))
	req.Host = c.host

	return req
}

// 生成 HTTP 请求体。
func (c *baseImpl) genReqForReader(method, fileId string, query url.Values, header http.Header,
	contentLength int64, content io.Reader) *http.Request {

	// 生成请求头。
	if query == nil {
		query = url.Values{}
	}
	if header == nil {
		header = http.Header{}
	}
	header.Set("Host", c.host)
	if contentLength > 0 {
		header.Set("Content-Length", strconv.FormatInt(contentLength, 10))
	}
	header.Set("Authorization", c.GenerateAuthorization(fileId, method, query, header, AuthExpirationTime))

	// 生成 URL。
	schema := "http"
	if c.tls {
		schema = "https"
	}
	u, _ := url.Parse(fmt.Sprintf("%s://%s/%s?%s", schema, c.host, strings.TrimLeft(fileId, "/"), query.Encode()))

	// 获取请求体，并赋值。
	req := getRequest()
	req.Method = method
	req.URL = u
	req.Header = header
	req.Body = io.NopCloser(content)
	req.ContentLength = contentLength
	req.Host = c.host

	return req
}

// 发送 HTTP/HEAD 请求。
func (c *baseImpl) head(ctx context.Context, fileId string) (*http.Response, error) {
	req := c.genReq(http.MethodHead, fileId, nil, nil, nil)
	rsp, err := c.sendHttp(ctx, req)
	closeRsp(rsp)
	return rsp, err
}

// 获取文件大小。
func (c *baseImpl) getFileSize(ctx context.Context, fileId string) (int64, error) {
	rsp, err := c.head(ctx, fileId)
	if err != nil {
		return 0, err
	}
	length := rsp.ContentLength
	if length <= 0 {
		lengthStr := rsp.Header.Get("Content-Length")
		length, _ = strconv.ParseInt(lengthStr, 10, 64)
	}
	return length, nil
}
