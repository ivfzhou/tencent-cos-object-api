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
	"encoding/xml"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

func TestInfo(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			fileSize := rand.Int63n(999999)
			etag := strconv.FormatInt(rand.Int63n(999999), 10)
			crc64 := strconv.FormatInt(rand.Int63n(999999), 10)
			mt := time.Now().Add(time.Second * time.Duration(rand.Intn(100))).Format(time.RFC1123)
			et := time.Now().Add(time.Second * time.Duration(rand.Intn(100))).Format(time.RFC1123)
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Method != http.MethodHead {
					t.Errorf("unexpected method: want %v, got %v", http.MethodHead, req.Method)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				header := http.Header{
					"Content-Length": []string{strconv.FormatInt(fileSize, 10)},
					"Etag":           []string{etag},
					"Last-Modified":  []string{mt},
					"Expires":        []string{et},
				}
				header.Set("x-cos-hash-crc64ecma", crc64)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     header,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			info, err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Info(context.Background(), fileId)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if info.Size != fileSize {
				t.Errorf("unexpected size: want %v, got %v", fileSize, info.Size)
			}
			if info.UploadTime.Format(time.RFC1123) != mt {
				t.Errorf("unexpected upload time: want %v, got %v", mt, info.UploadTime.Format(time.RFC1123))
			}
			if info.ExpireTime.Format(time.RFC1123) != et {
				t.Errorf("unexpected expire time: want %v, got %v", et, info.ExpireTime.Format(time.RFC1123))
			}
			if info.Crc64 != crc64 {
				t.Errorf("unexpected crc64: want %v, got %v", crc64, info.Crc64)
			}
			if info.EntityTag != etag {
				t.Errorf("unexpected entity tag: want %v, got %v", etag, info.EntityTag)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("响应失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			expectedErr := "expected error"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
			_, err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Info(context.Background(), fileId)
			if err == nil || !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("文件不存在", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			fileId := "/ivfzhou_test_file"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Method != http.MethodHead {
					t.Errorf("unexpected method: want %v, got %v", http.MethodHead, req.Method)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			_, err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Info(context.Background(), fileId)
			if !errors.Is(err, cos.ErrNotExists) {
				t.Errorf("unexpected error: want %v, got %v", cos.ErrNotExists, err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			expectedErr := errors.New("expected error")
			atomic.StoreInt32(&CloseCount, 0)
			ctx, cancel := NewCtxCancelWithError()
			cancel(expectedErr)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
			_, err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Info(ctx, fileId)
			if !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})
}

func TestExist(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			no := rand.Intn(2) == 1
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Method != http.MethodHead {
					t.Errorf("unexpected method: want %v, got %v", http.MethodHead, req.Method)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				sc := http.StatusNotFound
				if no {
					sc = http.StatusOK
				}
				return &http.Response{
					StatusCode: sc,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			b, err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Exist(context.Background(), fileId)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if b != no {
				t.Errorf("unexpected result: want %v, got %v", no, b)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("响应失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			expectedErr := "expected error"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
			_, err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Exist(context.Background(), fileId)
			if err == nil || !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		fileId := "/ivfzhou_test_file"
		expectedErr := errors.New("expected error")
		atomic.StoreInt32(&CloseCount, 0)
		ctx, cancel := NewCtxCancelWithError()
		cancel(expectedErr)
		fn := func(req *http.Request) (*http.Response, error) {
			path := req.URL.Path
			if path != fileId {
				t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
		_, err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).Exist(ctx, fileId)
		if !errors.Is(err, expectedErr) {
			t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("expected close count: want 0, got %v", closeCount)
		}
	})
}

func TestListFiles(t *testing.T) {
	for i := 0; i < 100; i++ {
		atomic.StoreInt32(&CloseCount, 0)
		type Contents struct {
			Key          string
			LastModified string
			ETag         string
			Size         int64
		}
		type ListBucketResult struct {
			NextMarker string
			Contents   []Contents
		}
		data := []Contents{
			{
				Key:          strconv.Itoa(rand.Intn(99999)),
				LastModified: time.Now().Add(time.Second * time.Duration(rand.Intn(9999))).Format(time.RFC3339),
				ETag:         strconv.Itoa(rand.Intn(99999)),
				Size:         rand.Int63n(999),
			},
			{
				Key:          strconv.Itoa(rand.Intn(99999)),
				LastModified: time.Now().Add(time.Second * time.Duration(rand.Intn(9999))).Format(time.RFC3339),
				ETag:         strconv.Itoa(rand.Intn(99999)),
				Size:         rand.Int63n(999),
			},
			{
				Key:          strconv.Itoa(rand.Intn(99999)),
				LastModified: time.Now().Add(time.Second * time.Duration(rand.Intn(9999))).Format(time.RFC3339),
				ETag:         strconv.Itoa(rand.Intn(99999)),
				Size:         rand.Int63n(999),
			},
			{
				Key:          strconv.Itoa(rand.Intn(99999)),
				LastModified: time.Now().Add(time.Second * time.Duration(rand.Intn(9999))).Format(time.RFC3339),
				ETag:         strconv.Itoa(rand.Intn(99999)),
				Size:         rand.Int63n(999),
			},
		}
		next := ""
		var data1 []Contents
		fn := func(req *http.Request) (*http.Response, error) {
			path := req.URL.Path
			if path != "/" {
				t.Errorf("unexpected req path: want /, got %v", path)
			}
			if req.Method != http.MethodGet {
				t.Errorf("unexpected method: want %v, got %v", http.MethodGet, req.Method)
			}
			auth := req.Header.Get("Authorization")
			if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
				t.Errorf("unexpected auth: got %v", auth)
			}
			fileId := req.URL.Query().Get("prefix")
			if fileId != "file/ivfzhou" {
				t.Errorf("unexpected fileId: want file/ivfzhou, got %v", fileId)
			}
			limit, _ := strconv.Atoi(req.URL.Query().Get("max-keys"))
			index := min(limit, len(data))
			rsp := &ListBucketResult{
				Contents: data[:index],
			}
			data1 = data[:index]
			data = data[index:]
			if len(data) > 0 {
				next = strconv.Itoa(rand.Intn(9999))
			} else {
				next = ""
			}
			rsp.NextMarker = next
			rspBody, _ := xml.Marshal(rsp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       NewReader(rspBody, nil, nil, nil),
			}, nil
		}
		client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
		for {
			files, offset, err := client.ListFiles(context.Background(), "file", "ivfzhou", "", 2)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if offset != next {
				t.Errorf("unexpected offset: want %v, got %v", next, offset)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("expected close count: want 0, got %v", closeCount)
			}
			for _, v := range files {
				if data1[0].Key != v.ID {
					t.Errorf("unexpected ID: want %v, got %v", data1[0].Key, v.ID)
				}
				if data1[0].ETag != v.EntityTag {
					t.Errorf("unexpected EntityTag: want %v, got %v", data1[0].ETag, v.EntityTag)
				}
				if data1[0].LastModified != v.UploadTime.Format(time.RFC3339) {
					t.Errorf("unexpected UploadTime: want %v, got %v", data1[0].LastModified, v.UploadTime)
				}
				if data1[0].Size != v.Size {
					t.Errorf("unexpected Size: want %v, got %v", data1[0].Size, v.Size)
				}
				data1 = data1[1:]
			}
			if len(offset) <= 0 {
				break
			}
		}
	}
}
