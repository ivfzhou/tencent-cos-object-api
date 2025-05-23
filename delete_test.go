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
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

func TestDelete(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			fileId := "/ivfzhou_test_file"
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want /ping, got %v", path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				if req.Method != http.MethodDelete {
					t.Errorf("unexpected method: want %v, got %v", http.MethodDelete, req.Method)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       NewReader(nil, nil, nil, nil),
				}, nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
			err := client.Delete(context.Background(), fileId)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected closeCount: want 0, got %v", closeCount)
			}
		}
	})

	t.Run("删除失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			fileId := "/ivfzhou_test_file"
			expectedErr := "expected error"
			atomic.StoreInt32(&CloseCount, 0)
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != fileId {
					t.Errorf("unexpected req path: want /ping, got %v", path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				if req.Method != http.MethodDelete {
					t.Errorf("unexpected method: want %v, got %v", http.MethodDelete, req.Method)
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       NewReader([]byte(expectedErr), nil, nil, nil),
				}, nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
			err := client.Delete(context.Background(), fileId)
			if err == nil || !strings.Contains(err.Error(), expectedErr) {
				t.Errorf("unexpected error: want %v, got %v", expectedErr, err)
			}
			if closeCount := atomic.LoadInt32(&CloseCount); closeCount != 0 {
				t.Errorf("unexpected closeCount: want 0, got %v", closeCount)
			}
		}
	})
}

func TestDeletes(t *testing.T) {
	t.Run("正常运行", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			count := rand.Intn(2000) + 1
			files := make([]string, 0, count)
			for i := 0; i < count; i++ {
				files = append(files, "file/ivfzhou_test_file"+strconv.Itoa(i))
			}
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != "/" {
					t.Errorf("unexpected req path: want /ping, got %v", path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				if req.Method != http.MethodPost {
					t.Errorf("unexpected method: want %v, got %v", http.MethodPost, req.Method)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if !req.URL.Query().Has("delete") {
					t.Errorf("unexpected query: want true, got false")
				}
				reqBody, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				type Object struct {
					Key string `xml:"Key"`
				}
				type Delete struct {
					Quiet  bool      `xml:"Quiet"`
					Object []*Object `xml:",any"`
				}
				var reqObj Delete
				if err = xml.Unmarshal(reqBody, &reqObj); err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				for _, v := range reqObj.Object {
					if v.Key != files[0] {
						t.Errorf("unexpected object key: want %v, got %v", files[0], v.Key)
					}
					files = files[1:]
				}
				type Error struct {
					Code    string `xml:"Code"`
					Message string `xml:"Message"`
					Key     string `xml:"Key"`
				}
				type Deleted struct {
					Key string `xml:"Key"`
				}
				type DeleteResult struct {
					Error   []*Error   `xml:"Error"`
					Deleted []*Deleted `xml:"Deleted"`
				}
				var rspObj DeleteResult
				rspBody, err := xml.Marshal(&rspObj)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: NewReader(rspBody, nil, nil, nil)}, nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
			undeleted := client.Deletes(context.Background(), files...)
			if len(undeleted) > 0 {
				t.Errorf("undeleted files: want nil, got %v", undeleted)
			}
			if atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("删除失败", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			atomic.StoreInt32(&CloseCount, 0)
			count := rand.Intn(2000) + 1
			files := make([]string, 0, count)
			occurErrIndex := rand.Intn(count)
			expectedErr := "expected error"
			for i := 0; i < count; i++ {
				files = append(files, "file/ivfzhou_test_file"+strconv.Itoa(i))
			}
			expectedFileId := files[occurErrIndex]
			once := true
			fn := func(req *http.Request) (*http.Response, error) {
				path := req.URL.Path
				if path != "/" {
					t.Errorf("unexpected req path: want /ping, got %v", path)
				}
				if req.Host != host {
					t.Errorf("unexpected host: want %v, got %v", host, req.Host)
				}
				if req.Method != http.MethodPost {
					t.Errorf("unexpected method: want %v, got %v", http.MethodPost, req.Method)
				}
				auth := req.Header.Get("Authorization")
				if !CheckAuthorization(auth, path, req.Method, req.Header, req.URL.Query()) {
					t.Errorf("unexpected auth: got %v", auth)
				}
				if !req.URL.Query().Has("delete") {
					t.Errorf("unexpected query: want true, got false")
				}
				reqBody, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				type Object struct {
					Key string `xml:"Key"`
				}
				type Delete struct {
					Quiet  bool      `xml:"Quiet"`
					Object []*Object `xml:",any"`
				}
				var reqObj Delete
				if err = xml.Unmarshal(reqBody, &reqObj); err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				type Error struct {
					Code    string `xml:"Code"`
					Message string `xml:"Message"`
					Key     string `xml:"Key"`
				}
				type Deleted struct {
					Key string `xml:"Key"`
				}
				type DeleteResult struct {
					Error   []*Error   `xml:"Error"`
					Deleted []*Deleted `xml:"Deleted"`
				}
				var rspObj DeleteResult
				for _, v := range reqObj.Object {
					if v.Key != files[0] {
						t.Errorf("unexpected object key: want %v, got %v", files[0], v.Key)
					}
					files = files[1:]
					occurErrIndex--
					if once && occurErrIndex < 0 {
						once = false
						rspObj.Error = append(rspObj.Error, &Error{
							Code:    "1",
							Message: expectedErr,
							Key:     v.Key,
						})
					}
				}
				rspBody, err := xml.Marshal(&rspObj)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: NewReader(rspBody, nil, nil, nil)}, nil
			}
			client := cos.NewClient(host, appKey, appSecret, cos.WithHttpClient(MockHttpClient(fn)))
			undeleted := client.Deletes(context.Background(), files...)
			if len(undeleted) != 1 {
				t.Errorf("undeleted files: want =1, got %v", undeleted)
			}
			for k, v := range undeleted {
				if k != expectedFileId {
					t.Errorf("unexpected object key: want %v, got %v", expectedFileId, k)
				}
				if !strings.Contains(v.Error(), expectedErr) {
					t.Errorf("unexpected error message: want %v, got %v", expectedErr, v.Error())
				}
				break
			}
			for atomic.LoadInt32(&CloseCount) != 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})
}
