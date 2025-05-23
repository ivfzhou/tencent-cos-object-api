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
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

func TestUpload(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			data, much := MakeBytes()
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
					case http.MethodPut:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						partNumberStr := req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(partNumberStr)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: partNumberStr,
								ETag:       uploadIdStr + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
					case http.MethodPost:
						if req.URL.Query().Has("uploads") {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
									uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							}, nil
						}
						v := req.URL.Query().Get("uploadId")
						if v != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
						}
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
						if len(reqObj.Parts) != len(parts) {
							t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
						}
						sort.Slice(parts, func(i, j int) bool {
							x, err := strconv.Atoi(parts[i].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							y, err := strconv.Atoi(parts[j].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							return x < y
						})
						prevNum := 1
						for i, v := range reqObj.Parts {
							if v.PartNumber != strconv.Itoa(prevNum) {
								t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
							}
							prevNum++
							if parts[i].PartNumber != v.PartNumber {
								t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
							}
							if parts[i].ETag != v.ETag {
								t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
							}
						}
					case http.MethodGet:
						v := req.URL.Query().Get("uploadId")
						if v != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
						}
						v = req.URL.Query().Get("part-number-marker")
						if v != listPartsNext {
							t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, v)
						}
						wg.Wait()
						index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
						ps := parts[listPartsBegin:index]
						listPartsBegin = index
						var rspData struct {
							XMLName              xml.Name   `xml:"ListPartsResult"`
							ListPartResultParts  []PartInfo `xml:"Part"`
							NextPartNumberMarker string
						}
						rspData.ListPartResultParts = ps
						if len(parts[listPartsBegin:]) > 0 {
							rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(999999999))
							listPartsNext = rspData.NextPartNumberMarker
						}
						bs, err := xml.Marshal(&rspData)
						if err != nil {
							t.Errorf("unexpected upload id: want nil, got %v", err)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(bs, nil, nil, nil),
						}, nil
					}
				} else {
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if !bytes.Equal(bs, data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
					}
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Upload(context.Background(), fileId, data)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if much {
				var keys []int
				result.Range(func(key, _ any) bool {
					keys = append(keys, key.(int))
					return true
				})
				sort.Ints(keys)
				var receivedData []byte
				for _, v := range keys {
					value, _ := result.Load(v)
					receivedData = append(receivedData, value.([]byte)...)
				}
				if !bytes.Equal(receivedData, data) {
					t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
				}
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上传失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			uploadId := "expected upload id"
			data, much := MakeBytes()
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			occurErrStep := rand.Intn(4)
			occurErrPartNum := rand.Intn(len(data)/cos.PartSize+1) + 1
			expectedErr := "expected error"
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
					case http.MethodDelete:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPut:
						v := req.URL.Query().Get("uploadId")
						if v != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
						}
						v = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(v)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: v,
								ETag:       v + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
						if occurErrStep == 1 && num == occurErrPartNum {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPost:
						if req.URL.Query().Has("uploads") {
							if occurErrStep == 0 {
								return &http.Response{
									StatusCode: http.StatusInternalServerError,
									Body:       NewReader([]byte(expectedErr), nil, nil, nil),
								}, nil
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
									uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							}, nil
						}
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
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
						if len(reqObj.Parts) != len(parts) {
							t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
						}
						sort.Slice(parts, func(i, j int) bool {
							x, err := strconv.Atoi(parts[i].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							y, err := strconv.Atoi(parts[j].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							return x < y
						})
						prevNum := 1
						for i, v := range reqObj.Parts {
							if v.PartNumber != strconv.Itoa(prevNum) {
								t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
							}
							prevNum++
							if parts[i].PartNumber != v.PartNumber {
								t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
							}
							if parts[i].ETag != v.ETag {
								t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
							}
						}
						if occurErrStep == 3 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodGet:
						v := req.URL.Query().Get("uploadId")
						if v != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
						}
						v = req.URL.Query().Get("part-number-marker")
						if v != listPartsNext {
							t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, v)
						}
						wg.Wait()
						index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
						ps := parts[listPartsBegin:index]
						listPartsBegin = index
						var rspData struct {
							XMLName              xml.Name   `xml:"ListPartsResult"`
							ListPartResultParts  []PartInfo `xml:"Part"`
							NextPartNumberMarker string
						}
						rspData.ListPartResultParts = ps
						if len(parts[listPartsBegin:]) > 0 {
							rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(999999999))
							listPartsNext = rspData.NextPartNumberMarker
						}
						bs, err := xml.Marshal(&rspData)
						if err != nil {
							t.Errorf("unexpected upload id: want nil, got %v", err)
						}
						if occurErrStep == 2 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(bs, nil, nil, nil),
						}, nil
					}
				} else {
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if !bytes.Equal(bs, data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
					}
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       NewReader([]byte(expectedErr), nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Upload(context.Background(), fileId, data)
			if err == nil || !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if much {
				var keys []int
				result.Range(func(key, _ any) bool {
					keys = append(keys, key.(int))
					return true
				})
				sort.Ints(keys)
				var receivedData []byte
				prevNum := 1
				for _, v := range keys {
					if v != prevNum {
						break
					}
					value, _ := result.Load(v)
					receivedData = append(receivedData, value.([]byte)...)
					prevNum = v
				}
				if !bytes.Contains(data, receivedData) {
					t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
				}
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("没有内容上传", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if req.Method != http.MethodPut {
					t.Errorf("unexpected method: want %v, got %v", http.MethodPut, req.Method)
				}
				bs, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				if len(bs) > 0 {
					t.Errorf("unexpected result: want 0, got %v", len(bs))
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Upload(context.Background(), fileId, nil)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			uploadId := "expected upload id"
			data, much := MakeBytes()
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			occurCancelStep := rand.Intn(4)
			occurCancelPartNum := rand.Intn(len(data)/cos.PartSize+1) + 1
			expectedErr := errors.New("expected error")
			wg := sync.WaitGroup{}
			ctx, cancel := NewCtxCancelWithError()
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
					case http.MethodDelete:
						v := req.URL.Query().Get("uploadId")
						if v != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPut:
						v := req.URL.Query().Get("uploadId")
						if v != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
						}
						v = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(v)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: v,
								ETag:       v + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
						if occurCancelStep == 1 && num == occurCancelPartNum {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPost:
						if req.URL.Query().Has("uploads") {
							if occurCancelStep == 0 {
								cancel(expectedErr)
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
									uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							}, nil
						}
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
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
						if len(reqObj.Parts) != len(parts) {
							t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
						}
						sort.Slice(parts, func(i, j int) bool {
							x, err := strconv.Atoi(parts[i].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							y, err := strconv.Atoi(parts[j].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							return x < y
						})
						prevNum := 1
						for i, v := range reqObj.Parts {
							if v.PartNumber != strconv.Itoa(prevNum) {
								t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
							}
							prevNum++
							if parts[i].PartNumber != v.PartNumber {
								t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
							}
							if parts[i].ETag != v.ETag {
								t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
							}
						}
						if occurCancelStep == 3 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodGet:
						v := req.URL.Query().Get("uploadId")
						if v != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
						}
						v = req.URL.Query().Get("part-number-marker")
						if v != listPartsNext {
							t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, v)
						}
						wg.Wait()
						index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
						ps := parts[listPartsBegin:index]
						listPartsBegin = index
						var rspData struct {
							XMLName              xml.Name   `xml:"ListPartsResult"`
							ListPartResultParts  []PartInfo `xml:"Part"`
							NextPartNumberMarker string
						}
						rspData.ListPartResultParts = ps
						if len(parts[listPartsBegin:]) > 0 {
							rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(999999999))
							listPartsNext = rspData.NextPartNumberMarker
						}
						bs, err := xml.Marshal(&rspData)
						if err != nil {
							t.Errorf("unexpected upload id: want nil, got %v", err)
						}
						if occurCancelStep == 2 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(bs, nil, nil, nil),
						}, nil
					}
				} else {
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if !bytes.Equal(bs, data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
					}
				}
				cancel(expectedErr)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				Upload(ctx, fileId, data)
			if err != nil && !strings.Contains(err.Error(), expectedErr.Error()) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if much {
				var keys []int
				result.Range(func(key, _ any) bool {
					keys = append(keys, key.(int))
					return true
				})
				sort.Ints(keys)
				var receivedData []byte
				prevNum := 1
				for _, v := range keys {
					if v != prevNum {
						break
					}
					value, _ := result.Load(v)
					receivedData = append(receivedData, value.([]byte)...)
					prevNum = v
				}
				if !bytes.Contains(data, receivedData) {
					t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
				}
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})
}

func TestUploadFromReader(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			data, _ := MakeBytes()
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodPut:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					uploadIdStr = req.URL.Query().Get("partNumber")
					num, err := strconv.Atoi(uploadIdStr)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected io: want nil, got %v", err)
					}
					if int64(len(bs)) != req.ContentLength {
						t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
					}
					result.Store(num, bs)
					wg.Add(1)
					go func() {
						defer wg.Done()
						lock.Lock()
						defer lock.Unlock()
						parts = append(parts, PartInfo{
							PartNumber: uploadIdStr,
							ETag:       uploadIdStr + "_etag",
							Size:       strconv.Itoa(len(bs)),
						})
					}()
				case http.MethodPost:
					if req.URL.Query().Has("uploads") {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
								uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
						}, nil
					}
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
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
					if len(reqObj.Parts) != len(parts) {
						t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
					}
					sort.Slice(parts, func(i, j int) bool {
						x, err := strconv.Atoi(parts[i].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						y, err := strconv.Atoi(parts[j].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						return x < y
					})
					prevNum := 1
					for i, v := range reqObj.Parts {
						if v.PartNumber != strconv.Itoa(prevNum) {
							t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
						}
						prevNum++
						if parts[i].PartNumber != v.PartNumber {
							t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
						}
						if parts[i].ETag != v.ETag {
							t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
						}
					}
				case http.MethodGet:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					uploadIdStr = req.URL.Query().Get("part-number-marker")
					if uploadIdStr != listPartsNext {
						t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
					}
					wg.Wait()
					index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
					ps := parts[listPartsBegin:index]
					listPartsBegin = index
					var rspData struct {
						XMLName              xml.Name   `xml:"ListPartsResult"`
						ListPartResultParts  []PartInfo `xml:"Part"`
						NextPartNumberMarker string
					}
					rspData.ListPartResultParts = ps
					if len(parts[listPartsBegin:]) > 0 {
						rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
						listPartsNext = rspData.NextPartNumberMarker
					}
					bs, err := xml.Marshal(&rspData)
					if err != nil {
						t.Errorf("unexpected upload id: want nil, got %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(bs, nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReader(context.Background(), fileId, bytes.NewReader(data))
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			var keys []int
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int))
				return true
			})
			sort.Ints(keys)
			var receivedData []byte
			for _, v := range keys {
				value, _ := result.Load(v)
				receivedData = append(receivedData, value.([]byte)...)
			}
			if !bytes.Equal(receivedData, data) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上传失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			data, _ := MakeBytes()
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			occurErrStep := rand.Intn(4)
			occurErrPartNum := rand.Intn(len(data)/cos.PartSize+1) + 1
			expectedErr := "expected error"
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodDelete:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPut:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					uploadIdStr = req.URL.Query().Get("partNumber")
					num, err := strconv.Atoi(uploadIdStr)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected io: want nil, got %v", err)
					}
					if int64(len(bs)) != req.ContentLength {
						t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
					}
					result.Store(num, bs)
					wg.Add(1)
					go func() {
						defer wg.Done()
						lock.Lock()
						defer lock.Unlock()
						parts = append(parts, PartInfo{
							PartNumber: uploadIdStr,
							ETag:       uploadIdStr + "_etag",
							Size:       strconv.Itoa(len(bs)),
						})
					}()
					if occurErrStep == 1 && num == occurErrPartNum {
						return &http.Response{
							StatusCode: http.StatusInternalServerError,
							Body:       NewReader([]byte(expectedErr), nil, nil, nil),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPost:
					if req.URL.Query().Has("uploads") {
						if occurErrStep == 0 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
								Header:     http.Header{"Content-Length": []string{strconv.Itoa(len(expectedErr))}},
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
								uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							Header: http.Header{
								"Content-Length": []string{strconv.Itoa(len(uploadId))},
							},
						}, nil
					}
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
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
					if len(reqObj.Parts) != len(parts) {
						t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
					}
					sort.Slice(parts, func(i, j int) bool {
						x, err := strconv.Atoi(parts[i].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						y, err := strconv.Atoi(parts[j].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						return x < y
					})
					prevNum := 1
					for i, v := range reqObj.Parts {
						if v.PartNumber != strconv.Itoa(prevNum) {
							t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
						}
						prevNum++
						if parts[i].PartNumber != v.PartNumber {
							t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
						}
						if parts[i].ETag != v.ETag {
							t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
						}
					}
					if occurErrStep == 3 {
						return &http.Response{
							StatusCode: http.StatusInternalServerError,
							Body:       NewReader([]byte(expectedErr), nil, nil, nil),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodGet:
					v := req.URL.Query().Get("uploadId")
					if v != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
					}
					v = req.URL.Query().Get("part-number-marker")
					if v != listPartsNext {
						t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, v)
					}
					wg.Wait()
					index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
					ps := parts[listPartsBegin:index]
					listPartsBegin = index
					var rspData struct {
						XMLName              xml.Name   `xml:"ListPartsResult"`
						ListPartResultParts  []PartInfo `xml:"Part"`
						NextPartNumberMarker string
					}
					rspData.ListPartResultParts = ps
					if len(parts[listPartsBegin:]) > 0 {
						rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
						listPartsNext = rspData.NextPartNumberMarker
					}
					bs, err := xml.Marshal(&rspData)
					if err != nil {
						t.Errorf("unexpected upload id: want nil, got %v", err)
					}
					if occurErrStep == 2 {
						return &http.Response{
							StatusCode: http.StatusInternalServerError,
							Body:       NewReader([]byte(expectedErr), nil, nil, nil),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(bs, nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       NewReader([]byte(expectedErr), nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReader(context.Background(), fileId, bytes.NewReader(data))
			if err == nil || !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			var keys []int
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int))
				return true
			})
			sort.Ints(keys)
			var receivedData []byte
			prevNum := 1
			for _, v := range keys {
				if v != prevNum {
					break
				}
				value, _ := result.Load(v)
				receivedData = append(receivedData, value.([]byte)...)
				prevNum = v
			}
			if !bytes.HasPrefix(data, receivedData) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("Reader 读取失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			reqBody, _ := MakeBytes()
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			expectedErr := errors.New("expected error")
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, -1)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodDelete:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPut:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					uploadIdStr = req.URL.Query().Get("partNumber")
					num, err := strconv.Atoi(uploadIdStr)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected io: want nil, got %v", err)
					}
					if int64(len(bs)) != req.ContentLength {
						t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
					}
					result.Store(num, bs)
					wg.Add(1)
					go func() {
						defer wg.Done()
						lock.Lock()
						defer lock.Unlock()
						parts = append(parts, PartInfo{
							PartNumber: uploadIdStr,
							ETag:       uploadIdStr + "_etag",
							Size:       strconv.Itoa(len(bs)),
						})
					}()
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPost:
					if req.URL.Query().Has("uploads") {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
								uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							Header: http.Header{
								"Content-Length": []string{strconv.Itoa(len(uploadId))},
							},
						}, nil
					}
					v := req.URL.Query().Get("uploadId")
					if v != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
					}
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
					if len(reqObj.Parts) != len(parts) {
						t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
					}
					sort.Slice(parts, func(i, j int) bool {
						x, err := strconv.Atoi(parts[i].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						y, err := strconv.Atoi(parts[j].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						return x < y
					})
					prevNum := 1
					for i, v := range reqObj.Parts {
						if v.PartNumber != strconv.Itoa(prevNum) {
							t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
						}
						prevNum++
						if parts[i].PartNumber != v.PartNumber {
							t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
						}
						if parts[i].ETag != v.ETag {
							t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
						}
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodGet:
					v := req.URL.Query().Get("uploadId")
					if v != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
					}
					v = req.URL.Query().Get("part-number-marker")
					if v != listPartsNext {
						t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, v)
					}
					wg.Wait()
					index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
					ps := parts[listPartsBegin:index]
					listPartsBegin = index
					var rspData struct {
						XMLName              xml.Name   `xml:"ListPartsResult"`
						ListPartResultParts  []PartInfo `xml:"Part"`
						NextPartNumberMarker string
					}
					rspData.ListPartResultParts = ps
					if len(parts[listPartsBegin:]) > 0 {
						rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
						listPartsNext = rspData.NextPartNumberMarker
					}
					bs, err := xml.Marshal(&rspData)
					if err != nil {
						t.Errorf("unexpected upload id: want nil, got %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(bs, nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       NewReader([]byte(expectedErr.Error()), nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReader(context.Background(), fileId, NewReader(reqBody, nil, nil, expectedErr))
			if err == nil || !strings.Contains(err.Error(), expectedErr.Error()) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			var keys []int
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int))
				return true
			})
			sort.Ints(keys)
			var receivedData []byte
			prevNum := 1
			for _, v := range keys {
				if v != prevNum {
					break
				}
				value, _ := result.Load(v)
				receivedData = append(receivedData, value.([]byte)...)
				prevNum = v
			}
			if !bytes.HasPrefix(reqBody, receivedData) {
				t.Errorf("unexpected result: want %v, got %v", len(reqBody), len(receivedData))
			}
		}
	})

	t.Run("没有内容上传", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				atomic.StoreInt32(&CloseCount, 0)
				switch req.Method {
				case http.MethodPut:
					v := req.URL.Query().Get("uploadId")
					if v != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
					}
					v = req.URL.Query().Get("partNumber")
					num, err := strconv.Atoi(v)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected io: want nil, got %v", err)
					}
					if int64(len(bs)) != req.ContentLength {
						t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
					}
					result.Store(num, bs)
					wg.Add(1)
					go func() {
						defer wg.Done()
						lock.Lock()
						defer lock.Unlock()
						parts = append(parts, PartInfo{
							PartNumber: v,
							ETag:       v + "_etag",
							Size:       strconv.Itoa(len(bs)),
						})
					}()
				case http.MethodPost:
					if req.URL.Query().Has("uploads") {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
								uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
						}, nil
					}
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
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
					if len(reqObj.Parts) != len(parts) {
						t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
					}
					sort.Slice(parts, func(i, j int) bool {
						x, err := strconv.Atoi(parts[i].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						y, err := strconv.Atoi(parts[j].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						return x < y
					})
					prevNum := 1
					for i, v := range reqObj.Parts {
						if v.PartNumber != strconv.Itoa(prevNum) {
							t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
						}
						prevNum++
						if parts[i].PartNumber != v.PartNumber {
							t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
						}
						if parts[i].ETag != v.ETag {
							t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
						}
					}
				case http.MethodGet:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					uploadIdStr = req.URL.Query().Get("part-number-marker")
					if uploadIdStr != listPartsNext {
						t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
					}
					wg.Wait()
					index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
					ps := parts[listPartsBegin:index]
					listPartsBegin = index
					var rspData struct {
						XMLName              xml.Name   `xml:"ListPartsResult"`
						ListPartResultParts  []PartInfo `xml:"Part"`
						NextPartNumberMarker string
					}
					rspData.ListPartResultParts = ps
					if len(parts[listPartsBegin:]) > 0 {
						rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
						listPartsNext = rspData.NextPartNumberMarker
					}
					bs, err := xml.Marshal(&rspData)
					if err != nil {
						t.Errorf("unexpected upload id: want nil, got %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(bs, nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReader(context.Background(), fileId, bytes.NewReader(nil))
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			var keys []int
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int))
				return true
			})
			sort.Ints(keys)
			var receivedData []byte
			for _, v := range keys {
				value, _ := result.Load(v)
				receivedData = append(receivedData, value.([]byte)...)
			}
			if len(receivedData) > 0 {
				t.Errorf("unexpected result: want nil, got %v", len(receivedData))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 30; i++ {
			data, _ := MakeBytes()
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			occurCancelStep := rand.Intn(4)
			occurCancelPartNum := rand.Intn(len(data)/cos.PartSize+1) + 1
			ctx, cancel := NewCtxCancelWithError()
			expectedErr := errors.New("expected error")
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodDelete:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPut:
					v := req.URL.Query().Get("uploadId")
					if v != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
					}
					v = req.URL.Query().Get("partNumber")
					num, err := strconv.Atoi(v)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected io: want nil, got %v", err)
					}
					if int64(len(bs)) != req.ContentLength {
						t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
					}
					result.Store(num, bs)
					wg.Add(1)
					go func() {
						defer wg.Done()
						lock.Lock()
						defer lock.Unlock()
						parts = append(parts, PartInfo{
							PartNumber: v,
							ETag:       v + "_etag",
							Size:       strconv.Itoa(len(bs)),
						})
					}()
					if occurCancelStep == 1 && num == occurCancelPartNum {
						cancel(expectedErr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPost:
					if req.URL.Query().Has("uploads") {
						if occurCancelStep == 0 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
								uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							Header: http.Header{
								"Content-Length": []string{strconv.Itoa(len(uploadId))},
							},
						}, nil
					}
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
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
					if len(reqObj.Parts) != len(parts) {
						t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
					}
					sort.Slice(parts, func(i, j int) bool {
						x, err := strconv.Atoi(parts[i].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						y, err := strconv.Atoi(parts[j].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						return x < y
					})
					prevNum := 1
					for i, v := range reqObj.Parts {
						if v.PartNumber != strconv.Itoa(prevNum) {
							t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
						}
						prevNum++
						if parts[i].PartNumber != v.PartNumber {
							t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
						}
						if parts[i].ETag != v.ETag {
							t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
						}
					}
					if occurCancelStep == 3 {
						cancel(expectedErr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodGet:
					v := req.URL.Query().Get("uploadId")
					if v != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, v)
					}
					v = req.URL.Query().Get("part-number-marker")
					if v != listPartsNext {
						t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, v)
					}
					wg.Wait()
					index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
					ps := parts[listPartsBegin:index]
					listPartsBegin = index
					var rspData struct {
						XMLName              xml.Name   `xml:"ListPartsResult"`
						ListPartResultParts  []PartInfo `xml:"Part"`
						NextPartNumberMarker string
					}
					rspData.ListPartResultParts = ps
					if len(parts[listPartsBegin:]) > 0 {
						rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
						listPartsNext = rspData.NextPartNumberMarker
					}
					bs, err := xml.Marshal(&rspData)
					if err != nil {
						t.Errorf("unexpected upload id: want nil, got %v", err)
					}
					if occurCancelStep == 2 {
						cancel(expectedErr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(bs, nil, nil, nil),
					}, nil
				default:
					t.Errorf("unexpected method: got %v", req.Method)
				}
				cancel(expectedErr)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReader(ctx, fileId, bytes.NewReader(data))
			if err != nil && !strings.Contains(err.Error(), expectedErr.Error()) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			var keys []int
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int))
				return true
			})
			sort.Ints(keys)
			var receivedData []byte
			prevNum := 1
			for _, v := range keys {
				if v != prevNum {
					break
				}
				value, _ := result.Load(v)
				receivedData = append(receivedData, value.([]byte)...)
				prevNum = v
			}
			if !bytes.HasPrefix(data, receivedData) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})
}

func TestUploadFromReaderWithSize(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			data, much := MakeBytes()
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
					case http.MethodPut:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(uploadIdStr)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: uploadIdStr,
								ETag:       uploadIdStr + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
					case http.MethodPost:
						if req.URL.Query().Has("uploads") {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
									uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							}, nil
						}
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
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
						if len(reqObj.Parts) != len(parts) {
							t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
						}
						sort.Slice(parts, func(i, j int) bool {
							x, err := strconv.Atoi(parts[i].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							y, err := strconv.Atoi(parts[j].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							return x < y
						})
						prevNum := 1
						for i, v := range reqObj.Parts {
							if v.PartNumber != strconv.Itoa(prevNum) {
								t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
							}
							prevNum++
							if parts[i].PartNumber != v.PartNumber {
								t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
							}
							if parts[i].ETag != v.ETag {
								t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
							}
						}
					case http.MethodGet:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("part-number-marker")
						if uploadIdStr != listPartsNext {
							t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
						}
						wg.Wait()
						index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
						ps := parts[listPartsBegin:index]
						listPartsBegin = index
						var rspData struct {
							XMLName              xml.Name   `xml:"ListPartsResult"`
							ListPartResultParts  []PartInfo `xml:"Part"`
							NextPartNumberMarker string
						}
						rspData.ListPartResultParts = ps
						if len(parts[listPartsBegin:]) > 0 {
							rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
							listPartsNext = rspData.NextPartNumberMarker
						}
						bs, err := xml.Marshal(&rspData)
						if err != nil {
							t.Errorf("unexpected upload id: want nil, got %v", err)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(bs, nil, nil, nil),
						}, nil
					}
				} else {
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if !bytes.Equal(bs, data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
					}
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReaderWithSize(context.Background(), fileId, int64(len(data)), bytes.NewReader(data))
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if much {
				var keys []int
				result.Range(func(key, _ any) bool {
					keys = append(keys, key.(int))
					return true
				})
				sort.Ints(keys)
				var receivedData []byte
				for _, v := range keys {
					value, _ := result.Load(v)
					receivedData = append(receivedData, value.([]byte)...)
				}
				if !bytes.Equal(receivedData, data) {
					t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
				}
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上传失败", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			fileId := "/ivfzhou_test_file"
			uploadId := "expected upload id"
			data, much := MakeBytes()
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			occurErrStep := rand.Intn(4)
			occurErrPartNum := rand.Intn(len(data)/cos.PartSize+1) + 1
			expectedErr := "expected error"
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
					case http.MethodDelete:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPut:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(uploadIdStr)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: uploadIdStr,
								ETag:       uploadIdStr + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
						if occurErrStep == 1 && num == occurErrPartNum {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPost:
						if req.URL.Query().Has("uploads") {
							if occurErrStep == 0 {
								return &http.Response{
									StatusCode: http.StatusInternalServerError,
									Body:       NewReader([]byte(expectedErr), nil, nil, nil),
								}, nil
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
									uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							}, nil
						}
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
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
						if len(reqObj.Parts) != len(parts) {
							t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
						}
						sort.Slice(parts, func(i, j int) bool {
							x, err := strconv.Atoi(parts[i].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							y, err := strconv.Atoi(parts[j].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							return x < y
						})
						prevNum := 1
						for i, v := range reqObj.Parts {
							if v.PartNumber != strconv.Itoa(prevNum) {
								t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
							}
							prevNum++
							if parts[i].PartNumber != v.PartNumber {
								t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
							}
							if parts[i].ETag != v.ETag {
								t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
							}
						}
						if occurErrStep == 3 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodGet:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("part-number-marker")
						if uploadIdStr != listPartsNext {
							t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
						}
						wg.Wait()
						index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
						ps := parts[listPartsBegin:index]
						listPartsBegin = index
						var rspData struct {
							XMLName              xml.Name   `xml:"ListPartsResult"`
							ListPartResultParts  []PartInfo `xml:"Part"`
							NextPartNumberMarker string
						}
						rspData.ListPartResultParts = ps
						if len(parts[listPartsBegin:]) > 0 {
							rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
							listPartsNext = rspData.NextPartNumberMarker
						}
						bs, err := xml.Marshal(&rspData)
						if err != nil {
							t.Errorf("unexpected upload id: want nil, got %v", err)
						}
						if occurErrStep == 2 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(bs, nil, nil, nil),
						}, nil
					}
				} else {
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if !bytes.Equal(bs, data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
					}
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       NewReader([]byte(expectedErr), nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReaderWithSize(context.Background(), fileId, int64(len(data)), bytes.NewReader(data))
			if err == nil || !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if much {
				var keys []int
				result.Range(func(key, _ any) bool {
					keys = append(keys, key.(int))
					return true
				})
				sort.Ints(keys)
				var receivedData []byte
				prevNum := 1
				for _, v := range keys {
					if v != prevNum {
						break
					}
					value, _ := result.Load(v)
					receivedData = append(receivedData, value.([]byte)...)
					prevNum = v
				}
				if !bytes.HasPrefix(data, receivedData) {
					t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
				}
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("Reader 读取失败", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			data, much := MakeBytes()
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			expectedErr := errors.New("expected error")
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, -1)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodDelete:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPut:
					if !much {
						bs, err := io.ReadAll(req.Body)
						if !errors.Is(err, expectedErr) {
							t.Errorf("unexpected io: want %v, got %v", expectedErr, err)
						}
						if err != nil {
							return nil, err
						}
						result.Store(1, bs)
					} else {
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(uploadIdStr)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: uploadIdStr,
								ETag:       uploadIdStr + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPost:
					if req.URL.Query().Has("uploads") {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
								uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
						}, nil
					}
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
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
					if len(reqObj.Parts) != len(parts) {
						t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
					}
					sort.Slice(parts, func(i, j int) bool {
						x, err := strconv.Atoi(parts[i].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						y, err := strconv.Atoi(parts[j].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						return x < y
					})
					prevNum := 1
					for i, v := range reqObj.Parts {
						if v.PartNumber != strconv.Itoa(prevNum) {
							t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
						}
						prevNum++
						if parts[i].PartNumber != v.PartNumber {
							t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
						}
						if parts[i].ETag != v.ETag {
							t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
						}
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodGet:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					uploadIdStr = req.URL.Query().Get("part-number-marker")
					if uploadIdStr != listPartsNext {
						t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
					}
					wg.Wait()
					index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
					ps := parts[listPartsBegin:index]
					listPartsBegin = index
					var rspData struct {
						XMLName              xml.Name   `xml:"ListPartsResult"`
						ListPartResultParts  []PartInfo `xml:"Part"`
						NextPartNumberMarker string
					}
					rspData.ListPartResultParts = ps
					if len(parts[listPartsBegin:]) > 0 {
						rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
						listPartsNext = rspData.NextPartNumberMarker
					}
					bs, err := xml.Marshal(&rspData)
					if err != nil {
						t.Errorf("unexpected upload id: want nil, got %v", err)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(bs, nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReaderWithSize(context.Background(), fileId, int64(len(data)),
					NewReader(data, nil, nil, expectedErr))
			if err == nil || !strings.Contains(err.Error(), expectedErr.Error()) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			var keys []int
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int))
				return true
			})
			sort.Ints(keys)
			var receivedData []byte
			prevNum := 1
			for _, v := range keys {
				if v != prevNum {
					break
				}
				value, _ := result.Load(v)
				receivedData = append(receivedData, value.([]byte)...)
				prevNum = v
			}
			if !bytes.HasPrefix(data, receivedData) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("没有内容上传", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if req.Method != http.MethodPut {
					t.Errorf("unexpected method: want %v, got %v", http.MethodPut, req.Method)
				}
				bs, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("unexpected io: want nil, got %v", err)
				}
				if int64(len(bs)) != req.ContentLength {
					t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
				}
				if len(bs) != 0 {
					t.Errorf("unexpected content: want 0, got %v", len(bs))
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReaderWithSize(context.Background(), fileId, 0, bytes.NewReader(nil))
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			data, much := MakeBytes()
			uploadId := "expected upload id"
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			occurCancelStep := rand.Intn(4)
			occurCancelPartNum := rand.Intn(len(data)/cos.PartSize+1) + 1
			ctx, cancel := NewCtxCancelWithError()
			expectedErr := errors.New("expected error")
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				switch req.Method {
				case http.MethodDelete:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPut:
					if !much {
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(1, bs)
						if occurCancelStep == 1 {
							cancel(expectedErr)
						}
					} else {
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(uploadIdStr)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: uploadIdStr,
								ETag:       uploadIdStr + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
						if occurCancelStep == 1 && num == occurCancelPartNum {
							cancel(expectedErr)
						}
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodPost:
					if req.URL.Query().Has("uploads") {
						if occurCancelStep == 0 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
								uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
						}, nil
					}
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
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
					if len(reqObj.Parts) != len(parts) {
						t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
					}
					sort.Slice(parts, func(i, j int) bool {
						x, err := strconv.Atoi(parts[i].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						y, err := strconv.Atoi(parts[j].PartNumber)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						return x < y
					})
					prevNum := 1
					for i, v := range reqObj.Parts {
						if v.PartNumber != strconv.Itoa(prevNum) {
							t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
						}
						prevNum++
						if parts[i].PartNumber != v.PartNumber {
							t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
						}
						if parts[i].ETag != v.ETag {
							t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
						}
					}
					if occurCancelStep == 3 {
						cancel(expectedErr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(nil, nil, nil, nil),
					}, nil
				case http.MethodGet:
					uploadIdStr := req.URL.Query().Get("uploadId")
					if uploadIdStr != uploadId {
						t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
					}
					uploadIdStr = req.URL.Query().Get("part-number-marker")
					if uploadIdStr != listPartsNext {
						t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
					}
					wg.Wait()
					index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
					ps := parts[listPartsBegin:index]
					listPartsBegin = index
					var rspData struct {
						XMLName              xml.Name   `xml:"ListPartsResult"`
						ListPartResultParts  []PartInfo `xml:"Part"`
						NextPartNumberMarker string
					}
					rspData.ListPartResultParts = ps
					if len(parts[listPartsBegin:]) > 0 {
						rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
						listPartsNext = rspData.NextPartNumberMarker
					}
					bs, err := xml.Marshal(&rspData)
					if err != nil {
						t.Errorf("unexpected upload id: want nil, got %v", err)
					}
					if occurCancelStep == 2 {
						cancel(expectedErr)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       NewReader(bs, nil, nil, nil),
					}, nil
				}
				cancel(expectedErr)
				return &http.Response{StatusCode: http.StatusOK}, nil
			}
			err := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromReaderWithSize(ctx, fileId, int64(len(data)), bytes.NewReader(data))
			if err != nil && !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			var keys []int
			result.Range(func(key, _ any) bool {
				keys = append(keys, key.(int))
				return true
			})
			sort.Ints(keys)
			var receivedData []byte
			prevNum := 1
			for _, v := range keys {
				if v != prevNum {
					break
				}
				value, _ := result.Load(v)
				receivedData = append(receivedData, value.([]byte)...)
				prevNum = v
			}
			if !bytes.HasPrefix(data, receivedData) {
				t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})
}

func TestUploadFromDisk(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			data, much := MakeBytes()
			uploadId := "expected upload id"
			fileObj, err := os.CreateTemp("", "")
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if _, err = WriteAll(fileObj, data); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = fileObj.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
					case http.MethodPut:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(uploadIdStr)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: uploadIdStr,
								ETag:       uploadIdStr + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
					case http.MethodPost:
						if req.URL.Query().Has("uploads") {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
									uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							}, nil
						}
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
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
						if len(reqObj.Parts) != len(parts) {
							t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
						}
						sort.Slice(parts, func(i, j int) bool {
							x, err := strconv.Atoi(parts[i].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							y, err := strconv.Atoi(parts[j].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							return x < y
						})
						prevNum := 1
						for i, v := range reqObj.Parts {
							if v.PartNumber != strconv.Itoa(prevNum) {
								t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
							}
							prevNum++
							if parts[i].PartNumber != v.PartNumber {
								t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
							}
							if parts[i].ETag != v.ETag {
								t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
							}
						}
					case http.MethodGet:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("part-number-marker")
						if uploadIdStr != listPartsNext {
							t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
						}
						wg.Wait()
						index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
						ps := parts[listPartsBegin:index]
						listPartsBegin = index
						var rspData struct {
							XMLName              xml.Name   `xml:"ListPartsResult"`
							ListPartResultParts  []PartInfo `xml:"Part"`
							NextPartNumberMarker string
						}
						rspData.ListPartResultParts = ps
						if len(parts[listPartsBegin:]) > 0 {
							rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
							listPartsNext = rspData.NextPartNumberMarker
						}
						bs, err := xml.Marshal(&rspData)
						if err != nil {
							t.Errorf("unexpected upload id: want nil, got %v", err)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(bs, nil, nil, nil),
						}, nil
					}
				} else {
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if !bytes.Equal(bs, data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
					}
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err = cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromDisk(context.Background(), fileId, fileObj.Name())
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = os.Remove(fileObj.Name()); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if much {
				var keys []int
				result.Range(func(key, _ any) bool {
					keys = append(keys, key.(int))
					return true
				})
				sort.Ints(keys)
				var receivedData []byte
				for _, v := range keys {
					value, _ := result.Load(v)
					receivedData = append(receivedData, value.([]byte)...)
				}
				if !bytes.Equal(receivedData, data) {
					t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
				}
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上传失败", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			uploadId := "expected upload id"
			data, much := MakeBytes()
			fileObj, err := os.CreateTemp("", "")
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if _, err = WriteAll(fileObj, data); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = fileObj.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			occurErrStep := rand.Intn(4)
			occurErrPartNum := rand.Intn(len(data)/cos.PartSize+1) + 1
			expectedErr := "expected error"
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
					case http.MethodDelete:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPut:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(uploadIdStr)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: uploadIdStr,
								ETag:       uploadIdStr + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
						if occurErrStep == 1 && num == occurErrPartNum {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPost:
						if req.URL.Query().Has("uploads") {
							if occurErrStep == 0 {
								return &http.Response{
									StatusCode: http.StatusInternalServerError,
									Body:       NewReader([]byte(expectedErr), nil, nil, nil),
								}, nil
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
									uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							}, nil
						}
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
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
						if len(reqObj.Parts) != len(parts) {
							t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
						}
						sort.Slice(parts, func(i, j int) bool {
							x, err := strconv.Atoi(parts[i].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							y, err := strconv.Atoi(parts[j].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							return x < y
						})
						prevNum := 1
						for i, v := range reqObj.Parts {
							if v.PartNumber != strconv.Itoa(prevNum) {
								t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
							}
							prevNum++
							if parts[i].PartNumber != v.PartNumber {
								t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
							}
							if parts[i].ETag != v.ETag {
								t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
							}
						}
						if occurErrStep == 3 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodGet:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("part-number-marker")
						if uploadIdStr != listPartsNext {
							t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
						}
						wg.Wait()
						index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
						ps := parts[listPartsBegin:index]
						listPartsBegin = index
						var rspData struct {
							XMLName              xml.Name   `xml:"ListPartsResult"`
							ListPartResultParts  []PartInfo `xml:"Part"`
							NextPartNumberMarker string
						}
						rspData.ListPartResultParts = ps
						if len(parts[listPartsBegin:]) > 0 {
							rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
							listPartsNext = rspData.NextPartNumberMarker
						}
						bs, err := xml.Marshal(&rspData)
						if err != nil {
							t.Errorf("unexpected upload id: want nil, got %v", err)
						}
						if occurErrStep == 2 {
							return &http.Response{
								StatusCode: http.StatusInternalServerError,
								Body:       NewReader([]byte(expectedErr), nil, nil, nil),
							}, nil
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(bs, nil, nil, nil),
						}, nil
					}
				} else {
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if !bytes.Equal(bs, data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
					}
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       NewReader([]byte(expectedErr), nil, nil, nil),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       NewReader([]byte(expectedErr), nil, nil, nil),
				}, nil
			}
			err = cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromDisk(context.Background(), fileId, fileObj.Name())
			if err == nil || !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if err = os.Remove(fileObj.Name()); err != nil {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if much {
				var keys []int
				result.Range(func(key, _ any) bool {
					keys = append(keys, key.(int))
					return true
				})
				sort.Ints(keys)
				var receivedData []byte
				prevNum := 1
				for _, v := range keys {
					if v != prevNum {
						break
					}
					value, _ := result.Load(v)
					receivedData = append(receivedData, value.([]byte)...)
					prevNum = v
				}
				if !bytes.HasPrefix(data, receivedData) {
					t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
				}
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("上下文终止", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			uploadId := "expected upload id"
			data, much := MakeBytes()
			fileObj, err := os.CreateTemp("", "")
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if _, err = WriteAll(fileObj, data); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = fileObj.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			fileId := "/ivfzhou_test_file"
			result := sync.Map{}
			type PartInfo struct {
				PartNumber string
				ETag       string
				Size       string
			}
			var parts []PartInfo
			lock := sync.Mutex{}
			listPartsNext := ""
			listPartsBegin := 0
			occurErrStep := rand.Intn(4)
			occurErrPartNum := rand.Intn(len(data)/cos.PartSize+1) + 1
			expectedErr := errors.New("expected error")
			ctx, cancel := NewCtxCancelWithError()
			wg := sync.WaitGroup{}
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
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
					case http.MethodDelete:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPut:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("partNumber")
						num, err := strconv.Atoi(uploadIdStr)
						if err != nil {
							t.Errorf("unexpected error: want nil, got %v", err)
						}
						bs, err := io.ReadAll(req.Body)
						if err != nil {
							t.Errorf("unexpected io: want nil, got %v", err)
						}
						if int64(len(bs)) != req.ContentLength {
							t.Errorf("unexpected content length: want %v, got %v", req.ContentLength, len(bs))
						}
						result.Store(num, bs)
						wg.Add(1)
						go func() {
							defer wg.Done()
							lock.Lock()
							defer lock.Unlock()
							parts = append(parts, PartInfo{
								PartNumber: uploadIdStr,
								ETag:       uploadIdStr + "_etag",
								Size:       strconv.Itoa(len(bs)),
							})
						}()
						if occurErrStep == 1 && num == occurErrPartNum {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodPost:
						if req.URL.Query().Has("uploads") {
							if occurErrStep == 0 {
								cancel(expectedErr)
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Body: NewReader([]byte("<InitiateMultipartUploadResult><UploadId>"+
									uploadId+"</UploadId></InitiateMultipartUploadResult>"), nil, nil, nil),
							}, nil
						}
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
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
						if len(reqObj.Parts) != len(parts) {
							t.Errorf("unexpected number of parts: want %v, got %v", len(parts), len(reqObj.Parts))
						}
						sort.Slice(parts, func(i, j int) bool {
							x, err := strconv.Atoi(parts[i].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							y, err := strconv.Atoi(parts[j].PartNumber)
							if err != nil {
								t.Errorf("unexpected error: want nil, got %v", err)
							}
							return x < y
						})
						prevNum := 1
						for i, v := range reqObj.Parts {
							if v.PartNumber != strconv.Itoa(prevNum) {
								t.Errorf("unexpected part: want %v, got %v", prevNum, v.PartNumber)
							}
							prevNum++
							if parts[i].PartNumber != v.PartNumber {
								t.Errorf("unexpected part number: want %v, got %v", parts[i].PartNumber, v.PartNumber)
							}
							if parts[i].ETag != v.ETag {
								t.Errorf("unexpected etag: want %v, got %v", parts[i].ETag, v.ETag)
							}
						}
						if occurErrStep == 3 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(nil, nil, nil, nil),
						}, nil
					case http.MethodGet:
						uploadIdStr := req.URL.Query().Get("uploadId")
						if uploadIdStr != uploadId {
							t.Errorf("unexpected upload id: want %v, got %v", uploadId, uploadIdStr)
						}
						uploadIdStr = req.URL.Query().Get("part-number-marker")
						if uploadIdStr != listPartsNext {
							t.Errorf("unexpected upload id: want %v, got %v", listPartsNext, uploadIdStr)
						}
						wg.Wait()
						index := listPartsBegin + rand.Intn(len(parts[listPartsBegin:])+1)
						ps := parts[listPartsBegin:index]
						listPartsBegin = index
						var rspData struct {
							XMLName              xml.Name   `xml:"ListPartsResult"`
							ListPartResultParts  []PartInfo `xml:"Part"`
							NextPartNumberMarker string
						}
						rspData.ListPartResultParts = ps
						if len(parts[listPartsBegin:]) > 0 {
							rspData.NextPartNumberMarker = strconv.Itoa(rand.Intn(1000000))
							listPartsNext = rspData.NextPartNumberMarker
						}
						bs, err := xml.Marshal(&rspData)
						if err != nil {
							t.Errorf("unexpected upload id: want nil, got %v", err)
						}
						if occurErrStep == 2 {
							cancel(expectedErr)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       NewReader(bs, nil, nil, nil),
						}, nil
					}
				} else {
					bs, err := io.ReadAll(req.Body)
					if err != nil {
						t.Errorf("unexpected error: want nil, got %v", err)
					}
					if !bytes.Equal(bs, data) {
						t.Errorf("unexpected result: want %v, got %v", len(data), len(bs))
					}
					cancel(expectedErr)
				}
				cancel(expectedErr)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err = cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromDisk(ctx, fileId, fileObj.Name())
			if err != nil && !errors.Is(err, expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if err = os.Remove(fileObj.Name()); err != nil {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if much {
				var keys []int
				result.Range(func(key, _ any) bool {
					keys = append(keys, key.(int))
					return true
				})
				sort.Ints(keys)
				var receivedData []byte
				prevNum := 1
				for _, v := range keys {
					if v != prevNum {
						break
					}
					value, _ := result.Load(v)
					receivedData = append(receivedData, value.([]byte)...)
					prevNum = v
				}
				if !bytes.HasPrefix(data, receivedData) {
					t.Errorf("unexpected result: want %v, got %v", len(data), len(receivedData))
				}
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("没有数据", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileObj, err := os.CreateTemp("", "")
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = fileObj.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			fileId := "/ivfzhou_test_file"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want %v, got %v", fileId, path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				bs, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				if len(bs) > 0 {
					t.Errorf("unexpected result: want 0, got %v", len(bs))
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			err = cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn))).
				UploadFromDisk(context.Background(), fileId, fileObj.Name())
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = os.Remove(fileObj.Name()); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected close count: want 0, got %v", closeCount)
			}
		}
	})
}
