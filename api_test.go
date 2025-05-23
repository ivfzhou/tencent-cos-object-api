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
	crand "crypto/rand"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

const (
	host       = "bucket-appId.cos.region.myqcloud.com"
	appKey     = "app_key"
	appSecret  = "app_secret"
	actualTest = false
)

func TestActual(t *testing.T) {
	if !actualTest {
		return
	}
	client := cos.NewClient(host, appKey, appSecret)
	ctx := context.Background()
	err := client.Ping(ctx)
	if err != nil {
		t.Errorf("unexpected error: want nil, got %v", err)
	}
	threshold := int64(cos.MultiThreshold * cos.PartSize)
	size := int64(cos.PartSize * cos.NumRoutines * 2)
	if size <= threshold {
		size = threshold + 1
	}
	size += int64(rand.Intn(cos.PartSize/2) + 1)
	datas := [][]byte{MakeBytesWithSize(int(threshold/2 + rand.Int63n(threshold/2) + 1)), MakeBytesWithSize(int(size))}
	fileId := "ivfzhou_test_file"
	for _, data := range datas {
		if err = client.Upload(ctx, fileId, data); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		var exist bool
		if exist, err = client.Exist(ctx, fileId); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if !exist {
			t.Errorf("unexpected exist: want true, got %v", exist)
		}
		var fileInfo *cos.FileInfo
		if fileInfo, err = client.Info(ctx, fileId); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if fileInfo.Size != int64(len(data)) {
			t.Errorf("unexpected size: want %d, got %d", len(data), fileInfo.Size)
		}
		var rc io.ReadCloser
		if rc, size, err = client.Download(ctx, fileId); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if size != int64(len(data)) {
			t.Errorf("unexpected size: want %d, got %d", len(data), size)
		}
		var bs []byte
		if bs, err = io.ReadAll(rc); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if err = rc.Close(); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if !bytes.Equal(bs, data) {
			t.Errorf("unexpected data: want %v, got %v", len(data), len(bs))
		}
		if err = client.Delete(ctx, fileId); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if err = client.UploadFromReader(ctx, fileId, bytes.NewReader(data)); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		filePath := filepath.Join(os.TempDir(), fileId)
		if err = client.DownloadToDisk(ctx, fileId, filePath); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if bs, err = os.ReadFile(filePath); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if !bytes.Equal(bs, data) {
			t.Errorf("unexpected data: want %v, got %v", len(data), len(bs))
		}
		fileId2 := fileId + "_2"
		if err = client.UploadFromReaderWithSize(ctx, fileId2, int64(len(data)), bytes.NewReader(data)); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		buf := &bytes.Buffer{}
		if err = client.DownloadToWriter(ctx, fileId2, buf); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if buf.Len() != len(data) {
			t.Errorf("unexpected size: want %d, got %d", len(data), buf.Len())
		}
		if !bytes.Equal(buf.Bytes(), data) {
			t.Errorf("unexpected data: want %v, got %v", len(data), len(buf.Bytes()))
		}
		var fileObj *os.File
		if fileObj, err = os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if err = client.DownloadToWriterAt(ctx, fileId2, fileObj); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if _, err = fileObj.Seek(0, 0); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if bs, err = io.ReadAll(fileObj); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if err = fileObj.Close(); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if !bytes.Equal(bs, data) {
			t.Errorf("unexpected data: want %v, got %v", len(data), len(bs))
		}
		downloadUrl := client.GetDownloadUrl(fileId2, 10*time.Minute)
		var rsp *http.Response
		if rsp, err = http.Get(downloadUrl); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if bs, err = io.ReadAll(rsp.Body); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if err = rsp.Body.Close(); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if !bytes.Equal(bs, data) {
			t.Errorf("unexpected data: want %v, got %v", len(data), len(bs))
		}
		if undeleted := client.Deletes(ctx, fileId, fileId2); len(undeleted) > 0 {
			t.Errorf("unexpected error: want nil, got %v", undeleted)
		}
		if err = os.Remove(filePath); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
	}
}

func TestPerformance(t *testing.T) {
	if !actualTest {
		return
	}
	var uploadFiles, downloadFiles [10]string
	fileIds := make([]string, 0, 20)
	for i := 0; i < len(uploadFiles); i++ {
		uploadFiles[i] = filepath.Join(os.TempDir(), `ivfzhou_test_file_`+strconv.Itoa(i))
		fileIds = append(fileIds, filepath.Base(uploadFiles[i]))
	}
	for i := 0; i < len(downloadFiles); i++ {
		downloadFiles[i] = filepath.Join(os.TempDir(), `ivfzhou_test_file_`+strconv.Itoa(len(uploadFiles)+i))
		fileIds = append(fileIds, downloadFiles[i])
	}
	size := int64(cos.PartSize * cos.NumRoutines * 2)
	if size <= int64(cos.MultiThreshold*cos.PartSize) {
		size = int64(cos.MultiThreshold*cos.PartSize + 1)
	}
	size += 13
	ctx := context.Background()
	client := cos.NewClient(host, appKey, appSecret)
	if undeleted := client.Deletes(ctx, fileIds...); len(undeleted) > 0 {
		t.Errorf("unexpected error: want nil, got %v", undeleted)
	}
	client2 := cos.NewClient(host, appKey, appSecret, cos.WithNonUseDisk())
	for _, cli := range []cos.Api{client, client2} {
		for _, v := range uploadFiles {
			fileInfo, err := os.Stat(v)
			if fileInfo != nil && fileInfo.IsDir() {
				if e := os.RemoveAll(v); e != nil {
					t.Errorf("unexpected error: want nil, got %v", e)
				}
			} else if err == nil && fileInfo.Size() != size {
				if e := os.RemoveAll(v); e != nil {
					t.Errorf("unexpected error: want nil, got %v", e)
				}
			} else if err == nil {
				continue
			}
			if err = os.MkdirAll(filepath.Dir(v), 0700); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			file, err := os.OpenFile(v, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			data := make([]byte, size)
			n, err := crand.Read(data)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if n != len(data) {
				t.Errorf("unexpected size: want %d, got %d", len(data), n)
			}
			if _, err = io.Copy(file, bytes.NewReader(data)); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if err = file.Close(); err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
		}
		for i := range downloadFiles {
			fileId := filepath.Base(uploadFiles[i])
			fileInfo, err := cli.Info(ctx, fileId)
			if errors.Is(err, cos.ErrNotExists) || fileInfo.Size != size {
				if err = cli.UploadFromDisk(ctx, fileId, uploadFiles[i]); err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
			}
		}
		wg := sync.WaitGroup{}
		wg.Add(len(downloadFiles) + len(uploadFiles))
		now := time.Now()
		for i, v := range uploadFiles {
			go func(i int, fp string) {
				defer wg.Done()
				if err := cli.UploadFromDisk(ctx, filepath.Base(downloadFiles[i]), fp); err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
			}(i, v)
		}
		for i, v := range downloadFiles {
			go func(i int, fp string) {
				defer wg.Done()
				file, err := os.OpenFile(v, os.O_RDONLY|os.O_CREATE|os.O_TRUNC, 0600)
				if err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				if err = cli.DownloadToWriter(ctx, filepath.Base(uploadFiles[i]), file); err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
				if err = file.Close(); err != nil {
					t.Errorf("unexpected error: want nil, got %v", err)
				}
			}(i, v)
		}
		wg.Wait()
		cost := time.Since(now)
		if cli == client {
			t.Log("use disk cost:", cost)
		} else {
			t.Log("no use disk cost:", cost)
		}
		for i, v := range uploadFiles {
			original, err := os.ReadFile(v)
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			destination, err := os.ReadFile(downloadFiles[i])
			if err != nil {
				t.Errorf("unexpected error: want nil, got %v", err)
			}
			if !bytes.Equal(original, destination) {
				t.Errorf("unexpected result: want %v, got %v", len(original), len(destination))
			}
		}
	}
	for i, v := range downloadFiles {
		if err := os.Remove(v); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
		if err := os.Remove(uploadFiles[i]); err != nil {
			t.Errorf("unexpected error: want nil, got %v", err)
		}
	}
	if undeleted := client.Deletes(ctx, fileIds...); len(undeleted) > 0 {
		t.Errorf("unexpected error: want nil, got %v", undeleted)
	}
}
