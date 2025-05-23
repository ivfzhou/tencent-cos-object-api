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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

func TestDownload(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			data, much := MakeBytes()
			fileId := "/ivfzhou_test_file"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					switch req.Method {
					case http.MethodGet:
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					}
				} else {
					switch req.Method {
					case http.MethodGet:
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, nil),
						}, nil
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					}
				}
				return nil, nil
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			rc, size, err := cos.NewClient(host, appKey, appSecret, nonUseDisk, cos.WithHttpClient(MockHttpClient(fn))).
				Download(context.Background(), fileId)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if size != int64(len(data)) {
				t.Errorf("unexpected size: want %v, got %v", len(data), size)
			}
			bs, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(data, bs) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("下载失败", func(t *testing.T) {
		for i := 0; i < 30; i++ {
			data, much := MakeBytes()
			fileId := "/ivfzhou_test_file"
			expectedErr := "expected error"
			occurErrStep := rand.Intn(2)
			occurErrRange := rand.Intn(len(data)/cos.PartSize + 1)
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					switch req.Method {
					case http.MethodGet:
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						if occurErrStep == 1 && uint64(occurErrRange) == begin/uint64(cos.PartSize) {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					case http.MethodHead:
						if occurErrStep == 0 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					}
				} else {
					switch req.Method {
					case http.MethodGet:
						if occurErrStep == 1 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, nil),
						}, nil
					case http.MethodHead:
						if occurErrStep == 0 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					}
				}
				return nil, nil
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			rc, size, err := cos.NewClient(host, appKey, appSecret, nonUseDisk, cos.WithHttpClient(MockHttpClient(fn))).
				Download(context.Background(), fileId)
			if err != nil && !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if rc != nil {
				bs, err := io.ReadAll(rc)
				if err == nil || !strings.Contains(err.Error(), expectedErr) {
					t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
				}
				if err = rc.Close(); err != nil {
					t.Errorf("unexpected error: want %v, got %v", nil, err)
				}
				if !bytes.HasPrefix(data, bs) {
					t.Errorf("unexpected response: want %v, got %v", len(data), len(bs))
				}
			}
			if size != 0 && size != int64(len(data)) {
				t.Errorf("unexpected length: want %v, got %v", len(data), size)
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			data, much := MakeBytes()
			fileId := "/ivfzhou_test_file"
			expectedErr := errors.New("expected error")
			ctx, cancel := NewCtxCancelWithError()
			occurCancelStep := rand.Intn(2)
			occurCancelRange := rand.Intn(len(data)/cos.PartSize + 1)
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					switch req.Method {
					case http.MethodGet:
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						if occurCancelStep == 1 && uint64(occurCancelRange) == begin/uint64(cos.PartSize) {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					case http.MethodHead:
						if occurCancelStep == 0 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					}
				} else {
					switch req.Method {
					case http.MethodGet:
						if occurCancelStep == 1 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, nil),
						}, nil
					case http.MethodHead:
						if occurCancelStep == 0 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					}
				}
				return nil, nil
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			rc, size, err := cos.NewClient(host, appKey, appSecret, nonUseDisk, cos.WithHttpClient(MockHttpClient(fn))).
				Download(ctx, fileId)
			if err != nil && !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if rc != nil {
				bs, err := io.ReadAll(rc)
				if err != nil && !errors.Is(err, expectedErr) {
					t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
				}
				if err = rc.Close(); err != nil {
					t.Errorf("unexpected error: want %v, got %v", nil, err)
				}
				if !bytes.HasPrefix(data, bs) {
					t.Errorf("unexpected response: want %v, got %v", len(data), len(bs))
				}
			}
			if size != 0 && size != int64(len(data)) {
				t.Errorf("unexpected length: want %v, got %v", len(data), size)
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("响应无数据", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodGet:
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodHead:
					return &http.Response{
						StatusCode: http.StatusOK,
					}, nil
				}
				return nil, nil
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			rc, size, err := cos.NewClient(host, appKey, appSecret, nonUseDisk, cos.WithHttpClient(MockHttpClient(fn))).
				Download(context.Background(), fileId)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if size != 0 {
				t.Errorf("unexpected size: want 0, got %v", size)
			}
			bs, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if len(bs) > 0 {
				t.Errorf("unexpected result: want 0, got %v", len(bs))
			}
			if err = rc.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})
}

func TestDownloadToWriter(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					if req.Method == http.MethodHead {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					}
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				} else {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data, nil, nil, nil),
					}, nil
				}
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			writer := bytes.Buffer{}
			err := client.DownloadToWriter(context.Background(), fileId, &writer)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(data, writer.Bytes()) {
				t.Errorf("unexpected response: want %v, got %v", len(data), writer.Len())
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("写入失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			atomic.StoreInt32(&CloseCount, -1)
			expectedErr := errors.New("expected error")
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					if req.Method == http.MethodHead {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					}
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				} else {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data, nil, nil, nil),
					}, nil
				}
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			var result []byte
			occurErrIndex := rand.Intn(len(data))
			writer := NewWriter(func(p []byte) (int, error) {
				result = append(result, p...)
				if len(result) > occurErrIndex {
					return len(p), expectedErr
				}
				return len(p), nil
			})
			err := client.DownloadToWriter(context.Background(), fileId, writer)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, result) {
				t.Errorf("unexpected response: want %v, got %v", len(data), len(result))
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("下载失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			expectedErr := errors.New("expected error")
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if req.Method == http.MethodHead {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
					}, nil
				}
				if much {
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
						return nil, expectedErr
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				}
				return nil, expectedErr
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			buf := &bytes.Buffer{}
			err := client.DownloadToWriter(context.Background(), fileId, buf)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, buf.Bytes()) {
				t.Errorf("unexpected response: want %v, got %v", len(data), buf.Len())
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			expectedErr := errors.New("expected error")
			atomic.StoreInt32(&CloseCount, 0)
			ctx, cancel := NewCtxCancelWithError()
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if req.Method == http.MethodHead {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
					}, nil
				}
				if much {
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
						cancel(expectedErr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(data, nil, nil, nil),
				}, nil
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			buf := &bytes.Buffer{}
			err := client.DownloadToWriter(ctx, fileId, buf)
			if err != nil && !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, buf.Bytes()) {
				t.Errorf("unexpected response: want %v, got %v", len(data), buf.Len())
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			expectedErr := errors.New("expected error")
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if req.Method == http.MethodHead {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
					}, nil
				}
				if much {
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, expectedErr),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(data, nil, nil, expectedErr),
				}, nil
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			buf := &bytes.Buffer{}
			err := client.DownloadToWriter(context.Background(), fileId, buf)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, buf.Bytes()) {
				t.Errorf("unexpected response: want %v, got %v", len(data), buf.Len())
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})
}

func TestDownloadToWriterWithSize(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					if req.Method != http.MethodGet {
						t.Errorf("unexpected method: want %v, got %v", http.MethodGet, req.Method)
					}
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				} else {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data, nil, nil, nil),
					}, nil
				}
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			writer := bytes.Buffer{}
			err := client.DownloadToWriterWithSize(context.Background(), fileId, int64(len(data)), &writer)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(data, writer.Bytes()) {
				t.Errorf("unexpected response: want %v, got %v", len(data), writer.Len())
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("写入失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			atomic.StoreInt32(&CloseCount, -1)
			expectedErr := errors.New("expected error")
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					if req.Method != http.MethodGet {
						t.Errorf("unexpected method: want %v, got %v", http.MethodGet, req.Method)
					}
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				} else {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data, nil, nil, nil),
					}, nil
				}
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			var result []byte
			occurErrIndex := rand.Intn(len(data))
			writer := NewWriter(func(p []byte) (int, error) {
				result = append(result, p...)
				if len(result) > occurErrIndex {
					return len(p), expectedErr
				}
				return len(p), nil
			})
			err := client.DownloadToWriterWithSize(context.Background(), fileId, int64(len(data)), writer)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, result) {
				t.Errorf("unexpected response: want %v, got %v", len(data), len(result))
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("下载失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			expectedErr := errors.New("expected error")
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if req.Method != http.MethodGet {
					t.Errorf("unexpected method: want %v, got %v", http.MethodGet, req.Method)
				}
				if much {
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
						return nil, expectedErr
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				}
				return nil, expectedErr
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			buf := &bytes.Buffer{}
			err := client.DownloadToWriterWithSize(context.Background(), fileId, int64(len(data)), buf)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, buf.Bytes()) {
				t.Errorf("unexpected response: want %v, got %v", len(data), buf.Len())
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			expectedErr := errors.New("expected error")
			atomic.StoreInt32(&CloseCount, 0)
			ctx, cancel := NewCtxCancelWithError()
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if req.Method != http.MethodGet {
					t.Errorf("unexpected method: want %v, got %v", http.MethodGet, req.Method)
				}
				if much {
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
						cancel(expectedErr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(data, nil, nil, nil),
				}, nil
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			buf := &bytes.Buffer{}
			err := client.DownloadToWriterWithSize(ctx, fileId, int64(len(data)), buf)
			if err != nil && !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, buf.Bytes()) {
				t.Errorf("unexpected response: want %v, got %v", len(data), buf.Len())
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			expectedErr := errors.New("expected error")
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if req.Method != http.MethodGet {
					t.Errorf("unexpected method: want %v, got %v", http.MethodGet, req.Method)
				}
				if much {
					rangeStr := req.Header.Get("Range")
					if len(rangeStr) <= 0 {
						t.Errorf("unexpected range: want >0, got %v", rangeStr)
					}
					rangeStr = rangeStr[len("bytes="):]
					pair := strings.Split(rangeStr, "-")
					if len(pair) != 2 {
						t.Errorf("unexpected range: want =2, got %v", pair)
					}
					begin, err := strconv.ParseUint(pair[0], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					end, err := strconv.ParseUint(pair[1], 10, 64)
					if err != nil {
						t.Errorf("unexpected range: want nil, got %v", err)
					}
					if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, expectedErr),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(data[begin:end+1], nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(data, nil, nil, expectedErr),
				}, nil
			}
			nonUseDisk := cos.WithNonUseDisk()
			if rand.Intn(2) == 0 {
				nonUseDisk = nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)), nonUseDisk)
			buf := &bytes.Buffer{}
			err := client.DownloadToWriterWithSize(context.Background(), fileId, int64(len(data)), buf)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if !bytes.HasPrefix(data, buf.Bytes()) {
				t.Errorf("unexpected response: want %v, got %v", len(data), buf.Len())
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})
}

func TestDownloadToDisk(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			filePath := filepath.Join(os.TempDir(), fileId)
			data, much := MakeBytes()
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodHead:
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
					}, nil
				case http.MethodGet:
					if much {
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					} else {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, nil),
						}, nil
					}
				default:
					return nil, nil
				}
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				DownloadToDisk(context.Background(), fileId, filePath)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			fileDate, err := os.ReadFile(filePath)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			os.RemoveAll(filePath)
			if !bytes.Equal(fileDate, data) {
				t.Errorf("unexpected response: want %v, got %v", len(data), len(fileDate))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("下载失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			filePath := filepath.Join(os.TempDir(), fileId)
			data, much := MakeBytes()
			expectedErr := errors.New("expected error")
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					switch req.Method {
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					case http.MethodGet:
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
							return nil, expectedErr
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					}
				} else {
					switch req.Method {
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					case http.MethodGet:
						return nil, expectedErr

					}
				}
				return nil, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				DownloadToDisk(context.Background(), fileId, filePath)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if _, err = os.Stat(filePath); err == nil {
				t.Errorf("unexpected file: want no nil, got %v", err)
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			filePath := filepath.Join(os.TempDir(), fileId)
			data, much := MakeBytes()
			expectedErr := errors.New("expected error")
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					switch req.Method {
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					case http.MethodGet:
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       NewReader(data[begin:end+1], nil, nil, expectedErr),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					}
				} else {
					switch req.Method {
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					case http.MethodGet:
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, expectedErr),
						}, nil
					}
				}
				return nil, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				DownloadToDisk(context.Background(), fileId, filePath)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if _, err = os.Stat(filePath); err == nil {
				t.Errorf("unexpected file: want no nil, got %v", err)
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			fileId := "/ivfzhou_test_file"
			filePath := filepath.Join(os.TempDir(), fileId)
			data, much := MakeBytes()
			expectedErr := errors.New("expected error")
			ctx, cancel := NewCtxCancelWithError()
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					switch req.Method {
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					case http.MethodGet:
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					}
				} else {
					switch req.Method {
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					case http.MethodGet:
						cancel(expectedErr)
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, nil),
						}, nil
					}
				}
				return nil, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				DownloadToDisk(ctx, fileId, filePath)
			if err != nil && !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if err == nil {
				os.Remove(filePath)
			} else if _, err = os.Stat(filePath); err == nil {
				t.Errorf("unexpected file: want no nil, got %v", err)
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})
}

func TestDownloadToWriterAt(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if much {
					switch req.Method {
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					case http.MethodGet:
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					}
				} else {
					switch req.Method {
					case http.MethodHead:
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						}, nil
					case http.MethodGet:
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, nil),
						}, nil
					}
				}
				return nil, nil
			}
			result := sync.Map{}
			wa := NewWriterAt(func(bs []byte, of int64) (int, error) {
				index := rand.Intn(len(bs) + 1)
				p := make([]byte, index)
				copy(p, bs[:index])
				result.Store(of, p)
				return len(p), nil
			})
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				DownloadToWriterAt(context.Background(), fileId, wa)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			keys := make([]int64, 0, 10)
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int64))
				return true
			})
			slices.SortFunc(keys, func(i, j int64) int { return int(i - j) })
			var resultData []byte
			for _, v := range keys {
				value, _ := result.Load(v)
				resultData = append(resultData, value.([]byte)...)
			}
			if !bytes.Equal(data, resultData) {
				t.Errorf("unexpected data: want %v, got %v", len(data), len(resultData))
			}
		}
	})

	t.Run("下载失败", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			expectedErr := errors.New("expected error")
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			result := sync.Map{}
			wa := NewWriterAt(func(bs []byte, of int64) (int, error) {
				index := rand.Intn(len(bs) + 1)
				p := make([]byte, index)
				copy(p, bs[:index])
				result.Store(of, p)
				return len(p), nil
			})
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodHead:
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodGet:
					if much {
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
							return nil, expectedErr
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					} else {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, expectedErr),
						}, nil
					}
				}
				return nil, nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
			err := client.DownloadToWriterAt(context.Background(), fileId, wa)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			keys := make([]int64, 0, 10)
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int64))
				return true
			})
			slices.SortFunc(keys, func(i, j int64) int { return int(i - j) })
			var resultData []byte
			prevOffset := int64(0)
			for _, v := range keys {
				if prevOffset != v {
					break
				}
				value, _ := result.Load(v)
				resultData = append(resultData, value.([]byte)...)
				prevOffset += int64(len(value.([]byte)))
			}
			if !bytes.HasPrefix(data, resultData) {
				t.Errorf("unexpected data: want %v, got %v", len(data), len(resultData))
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			fileId := "/ivfzhou_test_file"
			data, much := MakeBytes()
			expectedErr := errors.New("expected error")
			ctx, cancel := NewCtxCancelWithError()
			occurErrIndex := rand.Intn(len(data)/(cos.MultiThreshold*cos.PartSize) + 1)
			result := sync.Map{}
			wa := NewWriterAt(func(bs []byte, of int64) (int, error) {
				index := rand.Intn(len(bs) + 1)
				p := make([]byte, index)
				copy(p, bs[:index])
				result.Store(of, p)
				return len(p), nil
			})
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodHead:
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Length": []string{fmt.Sprintf("%d", len(data))}},
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodGet:
					if much {
						rangeStr := req.Header.Get("Range")
						if len(rangeStr) <= 0 {
							t.Errorf("unexpected range: want >0, got %v", rangeStr)
						}
						rangeStr = rangeStr[len("bytes="):]
						pair := strings.Split(rangeStr, "-")
						if len(pair) != 2 {
							t.Errorf("unexpected range: want =2, got %v", pair)
						}
						begin, err := strconv.ParseUint(pair[0], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						end, err := strconv.ParseUint(pair[1], 10, 64)
						if err != nil {
							t.Errorf("unexpected range: want nil, got %v", err)
						}
						if begin/uint64(cos.PartSize*cos.MultiThreshold) == uint64(occurErrIndex) {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data[begin:end+1], nil, nil, nil),
						}, nil
					} else {
						cancel(expectedErr)
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(data, nil, nil, expectedErr),
						}, nil
					}
				}
				return nil, nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
			err := client.DownloadToWriterAt(ctx, fileId, wa)
			if err != nil && !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			keys := make([]int64, 0, 10)
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int64))
				return true
			})
			slices.SortFunc(keys, func(i, j int64) int { return int(i - j) })
			var resultData []byte
			prevOffset := int64(0)
			for _, v := range keys {
				if prevOffset != v {
					break
				}
				value, _ := result.Load(v)
				resultData = append(resultData, value.([]byte)...)
				prevOffset += int64(len(value.([]byte)))
			}
			if !bytes.HasPrefix(data, resultData) {
				t.Errorf("unexpected data: want %v, got %v", len(data), len(resultData))
			}
		}
	})
}
