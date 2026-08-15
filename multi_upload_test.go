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
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

const (
	fileId   = "/ivfzhou_test_file"
	uploadId = "expected_upload_id"
)

func TestInitMultiUpload(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		atomic.StoreInt32(&CloseCount, 0)
		fn := func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Errorf("unexpected method: want %v, got %v", http.MethodPost, req.Method)
			}
			if req.URL.Path != fileId {
				t.Errorf("unexpected path: want %v, got %v", fileId, req.URL.Path)
			}
			if !req.URL.Query().Has("uploads") {
				t.Errorf("unexpected query: want uploads, got %v", req.URL.Query())
			}
			auth := req.Header.Get("Authorization")
			if !CheckAuthorization(auth, req.URL.Path, req.Method, req.Header, req.URL.Query()) {
				t.Errorf("unexpected auth: got %v", auth)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
					uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
			}, nil
		}
		client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
		got, err := client.InitMultiUpload(context.Background(), fileId)
		if err != nil {
			t.Fatalf("unexpected error: want nil, got %v", err)
		}
		if got != uploadId {
			t.Errorf("unexpected uploadId: want %v, got %v", uploadId, got)
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})

	t.Run("fileId无效", func(t *testing.T) {
		client := cos.NewClient(host, appKey, appSecret)
		_, err := client.InitMultiUpload(context.Background(), ".")
		if err == nil || !strings.Contains(err.Error(), "fileId is invalid") {
			t.Errorf("unexpected error: want fileId is invalid, got %v", err)
		}
	})
}

func TestUploadPart(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		data := MakeBytesWithSize(1024)
		partNumber := int64(3)
		atomic.StoreInt32(&CloseCount, 0)
		fn := func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPut {
				t.Errorf("unexpected method: want %v, got %v", http.MethodPut, req.Method)
			}
			if req.URL.Path != fileId {
				t.Errorf("unexpected path: want %v, got %v", fileId, req.URL.Path)
			}
			if req.URL.Query().Get("uploadId") != uploadId {
				t.Errorf("unexpected uploadId: want %v, got %v", uploadId, req.URL.Query().Get("uploadId"))
			}
			if req.URL.Query().Get("partNumber") != strconv.FormatInt(partNumber, 10) {
				t.Errorf("unexpected partNumber: want %v, got %v", partNumber, req.URL.Query().Get("partNumber"))
			}
			auth := req.Header.Get("Authorization")
			if !CheckAuthorization(auth, req.URL.Path, req.Method, req.Header, req.URL.Query()) {
				t.Errorf("unexpected auth: got %v", auth)
			}
			bs, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(bs, data) {
				t.Errorf("unexpected body: want %v, got %v", len(data), len(bs))
			}
			return &http.Response{StatusCode: http.StatusOK, Body: NewReader(nil, nil, nil, nil)}, nil
		}
		client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
		if err := client.UploadPart(context.Background(), fileId, uploadId, partNumber, data); err != nil {
			t.Fatalf("unexpected error: want nil, got %v", err)
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})
}

func TestUploadPartByReader(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		data := MakeBytesWithSize(2048)
		partNumber := int64(5)
		atomic.StoreInt32(&CloseCount, 0)
		fn := func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPut {
				t.Errorf("unexpected method: want %v, got %v", http.MethodPut, req.Method)
			}
			if req.URL.Path != fileId {
				t.Errorf("unexpected path: want %v, got %v", fileId, req.URL.Path)
			}
			if req.URL.Query().Get("uploadId") != uploadId {
				t.Errorf("unexpected uploadId: want %v, got %v", uploadId, req.URL.Query().Get("uploadId"))
			}
			if req.URL.Query().Get("partNumber") != strconv.FormatInt(partNumber, 10) {
				t.Errorf("unexpected partNumber: want %v, got %v", partNumber, req.URL.Query().Get("partNumber"))
			}
			if req.ContentLength != int64(len(data)) {
				t.Errorf("unexpected content length: want %v, got %v", len(data), req.ContentLength)
			}
			auth := req.Header.Get("Authorization")
			if !CheckAuthorization(auth, req.URL.Path, req.Method, req.Header, req.URL.Query()) {
				t.Errorf("unexpected auth: got %v", auth)
			}
			bs, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(bs, data) {
				t.Errorf("unexpected body: want %v, got %v", len(data), len(bs))
			}
			return &http.Response{StatusCode: http.StatusOK, Body: NewReader(nil, nil, nil, nil)}, nil
		}
		client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
		err := client.UploadPartByReader(context.Background(), fileId, uploadId, partNumber,
			int64(len(data)), bytes.NewReader(data))
		if err != nil {
			t.Fatalf("unexpected error: want nil, got %v", err)
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})
}

func TestListFileParts(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		atomic.StoreInt32(&CloseCount, 0)
		parts := []struct {
			num  int
			etag string
			size int64
		}{
			{3, "etag3", 30},
			{1, "etag1", 10},
			{2, "etag2", 20},
		}
		fn := func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Errorf("unexpected method: want %v, got %v", http.MethodGet, req.Method)
			}
			if req.URL.Path != fileId {
				t.Errorf("unexpected path: want %v, got %v", fileId, req.URL.Path)
			}
			if req.URL.Query().Get("uploadId") != uploadId {
				t.Errorf("unexpected uploadId: want %v, got %v", uploadId, req.URL.Query().Get("uploadId"))
			}
			auth := req.Header.Get("Authorization")
			if !CheckAuthorization(auth, req.URL.Path, req.Method, req.Header, req.URL.Query()) {
				t.Errorf("unexpected auth: got %v", auth)
			}
			var sb strings.Builder
			sb.WriteString("<ListPartsResult>")
			for _, v := range parts {
				sb.WriteString("<Part>")
				sb.WriteString("<PartNumber>")
				sb.WriteString(strconv.Itoa(v.num))
				sb.WriteString("</PartNumber>")
				sb.WriteString("<ETag>")
				sb.WriteString(v.etag)
				sb.WriteString("</ETag>")
				sb.WriteString("<Size>")
				sb.WriteString(strconv.FormatInt(v.size, 10))
				sb.WriteString("</Size>")
				sb.WriteString("</Part>")
			}
			sb.WriteString("</ListPartsResult>")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       NewReader([]byte(sb.String()), nil, nil, nil),
			}, nil
		}
		client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
		got, err := client.ListFileParts(context.Background(), fileId, uploadId)
		if err != nil {
			t.Fatalf("unexpected error: want nil, got %v", err)
		}
		if len(got) != len(parts) {
			t.Fatalf("unexpected number of parts: want %v, got %v", len(parts), len(got))
		}
		// 建立序号到期望值的映射。
		wantByNum := make(map[int]struct {
			etag string
			size int64
		}, len(parts))
		for _, v := range parts {
			wantByNum[v.num] = struct {
				etag string
				size int64
			}{v.etag, v.size}
		}
		// 应按照分片序号升序返回。
		for i, v := range got {
			if v.PartNumber != i+1 {
				t.Errorf("unexpected part number: want %v, got %v", i+1, v.PartNumber)
			}
			want := wantByNum[v.PartNumber]
			if v.EntityTag != want.etag {
				t.Errorf("unexpected etag: want %v, got %v", want.etag, v.EntityTag)
			}
			if v.Size != want.size {
				t.Errorf("unexpected size: want %v, got %v", want.size, v.Size)
			}
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})
}

func TestAbortMultiUpload(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		atomic.StoreInt32(&CloseCount, 0)
		fn := func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Errorf("unexpected method: want %v, got %v", http.MethodDelete, req.Method)
			}
			if req.URL.Path != fileId {
				t.Errorf("unexpected path: want %v, got %v", fileId, req.URL.Path)
			}
			if req.URL.Query().Get("uploadId") != uploadId {
				t.Errorf("unexpected uploadId: want %v, got %v", uploadId, req.URL.Query().Get("uploadId"))
			}
			auth := req.Header.Get("Authorization")
			if !CheckAuthorization(auth, req.URL.Path, req.Method, req.Header, req.URL.Query()) {
				t.Errorf("unexpected auth: got %v", auth)
			}
			return &http.Response{StatusCode: http.StatusNoContent, Body: NewReader(nil, nil, nil, nil)}, nil
		}
		client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
		if err := client.AbortMultiUpload(context.Background(), fileId, uploadId); err != nil {
			t.Fatalf("unexpected error: want nil, got %v", err)
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})
}

func TestCompleteMultiUpload(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		atomic.StoreInt32(&CloseCount, 0)
		fn := func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != fileId {
				t.Errorf("unexpected path: want %v, got %v", fileId, req.URL.Path)
			}
			if req.URL.Query().Get("uploadId") != uploadId {
				t.Errorf("unexpected uploadId: want %v, got %v", uploadId, req.URL.Query().Get("uploadId"))
			}
			auth := req.Header.Get("Authorization")
			if !CheckAuthorization(auth, req.URL.Path, req.Method, req.Header, req.URL.Query()) {
				t.Errorf("unexpected auth: got %v", auth)
			}
			switch req.Method {
			case http.MethodGet: // ListFileParts
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: NewReader([]byte("<ListPartsResult>"+
						"<Part><PartNumber>1</PartNumber><ETag>etag1</ETag><Size>10</Size></Part>"+
						"<Part><PartNumber>2</PartNumber><ETag>etag2</ETag><Size>20</Size></Part>"+
						"</ListPartsResult>"), nil, nil, nil),
				}, nil
			case http.MethodPost: // Complete
				bs, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				type PartInfo struct {
					PartNumber string
					ETag       string
				}
				type CompleteMultipartUpload struct {
					Parts []*PartInfo `xml:"Part"`
				}
				var reqObj CompleteMultipartUpload
				if err = xml.Unmarshal(bs, &reqObj); err != nil {
					t.Errorf("unexpected unmarshal: want nil, got %v", err)
				}
				if len(reqObj.Parts) != 2 {
					t.Errorf("unexpected number of parts: want 2, got %v", len(reqObj.Parts))
				}
				return &http.Response{StatusCode: http.StatusOK, Body: NewReader(nil, nil, nil, nil)}, nil
			}
			return nil, fmt.Errorf("unexpected method: %v", req.Method)
		}
		client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
		if err := client.CompleteMultiUpload(context.Background(), fileId, uploadId); err != nil {
			t.Fatalf("unexpected error: want nil, got %v", err)
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})

	t.Run("响应失败", func(t *testing.T) {
		atomic.StoreInt32(&CloseCount, 0)
		expectedErr := errors.New("expected error")
		fn := func(req *http.Request) (*http.Response, error) {
			return nil, expectedErr
		}
		client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
		if err := client.CompleteMultiUpload(context.Background(), fileId, uploadId); !errors.Is(err, expectedErr) {
			t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
		}
		if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
			t.Errorf("unexpected close count: want 0, got %v", closeCount)
		}
	})
}
