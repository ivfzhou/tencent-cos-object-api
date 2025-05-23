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

package cos_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

func TestPing(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != "/ping" {
					t.Errorf("unexpected req path: want /ping, got %v", path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				if req.Method != http.MethodHead {
					t.Errorf("unexpected method: want %v, got %v", http.MethodHead, req.Method)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Ping(context.Background())
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("响应失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			expectedErr := "expected error"
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != "/ping" {
					t.Errorf("unexpected req path: want /ping, got %v", path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				if req.Method != http.MethodHead {
					t.Errorf("unexpected method: want %v, got %v", http.MethodHead, req.Method)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       NewReader([]byte(expectedErr), nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Ping(context.Background())
			if err == nil || !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			expectedErr := errors.New("expected error")
			ctx, cancel := NewCtxCancelWithError()
			cancel(expectedErr)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != "/ping" {
					t.Errorf("unexpected req path: want /ping, got %v", path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				if req.Method != http.MethodHead {
					t.Errorf("unexpected method: want %v, got %v", http.MethodHead, req.Method)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				select {
				case <-req.Context().Done():
					return nil, req.Context().Err()
				default:
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).Ping(ctx)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})
}

func TestGenerateAuthorization(t *testing.T) {
	for i := 0; i < 100; i++ {
		client := cos.NewClient(host, appKey, appSecret)
		fileId := strconv.Itoa(rand.Intn(999999))
		method := http.MethodGet
		switch rand.Intn(5) {
		case 0:
			method = http.MethodHead
		case 1:
			method = http.MethodPut
		case 2:
			method = http.MethodDelete
		case 3:
			method = http.MethodPost
		}
		query := url.Values{}
		query.Set(strconv.Itoa(rand.Intn(999999)), strconv.Itoa(rand.Intn(999999)))
		query.Set(strconv.Itoa(rand.Intn(999999)), strconv.Itoa(rand.Intn(999999)))
		header := http.Header{}
		header.Set(strconv.Itoa(rand.Intn(999999)), strconv.Itoa(rand.Intn(999999)))
		header.Set(strconv.Itoa(rand.Intn(999999)), strconv.Itoa(rand.Intn(999999)))
		auth := client.GenerateAuthorization(fileId, method, query, header, cos.AuthExpirationTime)
		if !CheckAuthorization(auth, fileId, method, header, query) {
			t.Errorf("unexpected auth: got %v", auth)
		}
	}
}

func CheckAuthorization(auth, path, method string, wantHeader http.Header, wantQuery url.Values) bool {
	elems := strings.Split(auth, "&")
	beginTime, endTime, sign := "", "", ""
	var header, query []string
	shouldElemNumbers := 7
	for _, v := range elems {
		pair := strings.Split(v, "=")
		if len(pair) != 2 {
			return false
		}
		switch pair[0] {
		case "q-sign-algorithm":
			if pair[1] != "sha1" {
				return false
			}
			shouldElemNumbers--
		case "q-ak":
			if pair[1] != appKey {
				return false
			}
			shouldElemNumbers--
		case "q-sign-time":
			arr := strings.Split(pair[1], ";")
			if len(arr) != 2 {
				return false
			}
			if len(beginTime) > 0 && beginTime != arr[0] {
				return false
			}
			if len(endTime) > 0 && endTime != arr[1] {
				return false
			}
			if len(beginTime) <= 0 && len(endTime) <= 0 {
				beginTime = arr[0]
				endTime = arr[1]
				bt, err := strconv.ParseInt(beginTime, 10, 64)
				if err != nil {
					return false
				}
				et, err := strconv.ParseInt(endTime, 10, 64)
				if err != nil {
					return false
				}
				if bt > et {
					return false
				}
				if bt+int64(cos.AuthExpirationTime.Seconds()) != et {
					return false
				}
			}
			if !(len(beginTime) > 0 && len(endTime) > 0) {
				return false
			}
			shouldElemNumbers--
		case "q-key-time":
			arr := strings.Split(pair[1], ";")
			if len(arr) != 2 {
				return false
			}
			if len(beginTime) > 0 && beginTime != arr[0] {
				return false
			}
			if len(endTime) > 0 && endTime != arr[1] {
				return false
			}
			if len(beginTime) <= 0 && len(endTime) <= 0 {
				beginTime = arr[0]
				endTime = arr[1]
				bt, err := strconv.ParseInt(beginTime, 10, 64)
				if err != nil {
					return false
				}
				et, err := strconv.ParseInt(endTime, 10, 64)
				if err != nil {
					return false
				}
				if bt > et {
					return false
				}
				if bt+int64(cos.AuthExpirationTime.Seconds()) != et {
					return false
				}
			}
			if !(len(beginTime) > 0 && len(endTime) > 0) {
				return false
			}
			shouldElemNumbers--
		case "q-header-list":
			if len(pair[1]) > 0 {
				header = strings.Split(pair[1], ";")
			}
			shouldElemNumbers--
		case "q-url-param-list":
			if len(pair[1]) > 0 {
				query = strings.Split(pair[1], ";")
			}
			shouldElemNumbers--
		case "q-signature":
			sign = pair[1]
			shouldElemNumbers--
		default:
			return false
		}
	}
	if shouldElemNumbers != 0 {
		return false
	}
	headerMap := make(map[string]struct{}, len(header))
	for _, v := range header {
		_, ok := wantHeader[http.CanonicalHeaderKey(v)]
		if !ok {
			return false
		}
		headerMap[v] = struct{}{}
	}
	queryMap := make(map[string]struct{}, len(query))
	for _, v := range query {
		has := false
		for k := range wantQuery {
			if strings.ToLower(k) != v {
				continue
			}
			has = true
			break
		}
		if !has {
			return false
		}
		queryMap[v] = struct{}{}
	}
	path = strings.Trim(path, "/")
	keyTime := fmt.Sprintf("%s;%s", beginTime, endTime)
	var httpParameters string
	{
		keyList := make([]string, 0, len(query))
		paramList := make([]string, 0, len(query))
		tmp := make(map[string][]string, len(query))
		for k, v := range wantQuery {
			_, ok := queryMap[strings.ToLower(k)]
			if !ok {
				continue
			}
			n := strings.ToLower(UrlEncode(k))
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
				paramList = append(paramList, fmt.Sprintf("%s=%s", v, UrlEncode(m)))
			}
		}
		httpParameters = strings.Join(paramList, "&")
	}
	var httpHeaders string
	{
		keyList := make([]string, 0, len(header))
		paramList := make([]string, 0, len(header))
		tmp := make(map[string][]string, len(header))
		for k, v := range wantHeader {
			_, ok := headerMap[strings.ToLower(k)]
			if !ok {
				continue
			}
			n := strings.ToLower(UrlEncode(k))
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
				paramList = append(paramList, fmt.Sprintf("%s=%s", v, UrlEncode(n)))
			}
		}
		httpHeaders = strings.Join(paramList, "&")
	}
	var signKey string
	{
		hash := hmac.New(sha1.New, []byte(appSecret))
		hash.Write([]byte(keyTime))
		signKey = fmt.Sprintf("%x", hash.Sum(nil))
	}
	var httpString string
	{
		httpString = fmt.Sprintf("%s\n/%s\n%s\n%s\n",
			strings.ToLower(method), path, httpParameters, httpHeaders)
	}
	var stringToSign string
	{
		hash := sha1.New()
		hash.Write([]byte(httpString))
		stringToSign = fmt.Sprintf("sha1\n%s\n%s\n", keyTime, fmt.Sprintf("%x", hash.Sum(nil)))
	}
	var signature string
	{
		hash := hmac.New(sha1.New, []byte(signKey))
		hash.Write([]byte(stringToSign))
		signature = fmt.Sprintf("%x", hash.Sum(nil))
	}
	return signature == sign
}
