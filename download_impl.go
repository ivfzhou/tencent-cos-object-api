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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	gu "gitee.com/ivfzhou/goroutine-util"
	iu "gitee.com/ivfzhou/io-util"
)

type downloadImpl struct {
	*baseImpl
	MultiUploader
}

// Download 下载文件。
//
// 注意：调用方负责关闭 rc。
func (c *downloadImpl) Download(ctx context.Context, fileId string) (rc io.ReadCloser, size int64, err error) {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return nil, 0, errors.New("fileId is invalid")
	}

	// 获取文件信息。
	size, err = c.getFileSize(ctx, fileId)
	if err != nil {
		return nil, 0, err
	}

	// 是否使用分片模式下载。
	if useMultipart(size) {
		rc, err = c.multiDownloadToReader(ctx, fileId, size)
		return
	}

	rc, err = c.download(ctx, fileId)
	return
}

// DownloadToWriter 下载文件。
func (c *downloadImpl) DownloadToWriter(ctx context.Context, fileId string, w io.Writer) error {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 获取文件信息。
	fileSize, err := c.getFileSize(ctx, fileId)
	if err != nil {
		return err
	}

	// 下载。
	var rc io.ReadCloser
	if useMultipart(fileSize) {
		if rc, err = c.multiDownloadToReader(ctx, fileId, fileSize); err != nil {
			return err
		}
	} else {
		if rc, err = c.download(ctx, fileId); err != nil {
			return err
		}
	}
	defer closeIO(rc)

	_, err = io.Copy(w, rc)
	return err
}

// DownloadToWriterWithSize 下载文件。
func (c *downloadImpl) DownloadToWriterWithSize(ctx context.Context, fileId string, contentLength int64,
	w io.Writer) (err error) {

	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 下载。
	var rc io.ReadCloser
	if useMultipart(contentLength) {
		rc, err = c.multiDownloadToReader(ctx, fileId, contentLength)
		if err != nil {
			return err
		}
	} else {
		rc, err = c.download(ctx, fileId)
		if err != nil {
			return err
		}
	}
	defer closeIO(rc)

	_, err = io.Copy(w, rc)
	return err
}

// DownloadToDisk 下载文件。
func (c *downloadImpl) DownloadToDisk(ctx context.Context, fileId, filePath string) (err error) {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 获取文件信息。
	fileSize, err := c.getFileSize(ctx, fileId)
	if err != nil {
		return err
	}

	// 开启文件流。
	if err = os.MkdirAll(filepath.Dir(filePath), os.ModeDir); err != nil {
		return err
	}
	fileObj, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	defer func() {
		closeIO(fileObj)
		if err != nil {
			printError(os.Remove(filePath))
		}
	}()

	// 是否使用分片模式下载。
	if useMultipart(fileSize) {
		return c.downloadToWriterAt(ctx, fileId, fileSize, fileObj)
	}

	rc, err := c.download(ctx, fileId)
	if err != nil {
		return err
	}
	defer closeIO(rc)
	_, err = io.Copy(fileObj, rc)
	return err
}

// DownloadToWriterAt 下载文件。
func (c *downloadImpl) DownloadToWriterAt(ctx context.Context, fileId string, wa io.WriterAt) error {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 获取文件信息。
	fileSize, err := c.getFileSize(ctx, fileId)
	if err != nil {
		return err
	}

	// 是否使用分片模式下载。
	if useMultipart(fileSize) {
		return c.downloadToWriterAt(ctx, fileId, fileSize, wa)
	}

	// 下载。
	rc, err := c.download(ctx, fileId)
	if err != nil {
		return err
	}
	defer closeIO(rc)
	_, err = iu.CopyReaderToWriterAt(rc, wa, 0, false)
	return err
}

// GetDownloadUrl 获取文件下载链接。
func (c *downloadImpl) GetDownloadUrl(fileId string, expiration time.Duration) string {
	fileId = suitFileId(fileId)
	signString := c.GenerateAuthorization(fileId, http.MethodGet, nil, nil, expiration)
	schema := "http"
	if c.tls {
		schema = "https"
	}
	return fmt.Sprintf("%s://%s/%s?sign=%s", schema, c.host, fileId, url.QueryEscape(signString))
}

// 下载文件，并从读取流中读出。
func (c *downloadImpl) download(ctx context.Context, fileId string) (io.ReadCloser, error) {
	req := c.genReq(http.MethodGet, fileId, nil, nil, nil)
	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return nil, err
	}
	return rsp.Body, nil
}

// 下载文件到写入流。
func (c *downloadImpl) downloadToWriterAt(ctx context.Context, fileId string, fileSize int64,
	wa io.WriterAt) (err error) {

	type data struct {
		offset, end int64
	}
	run, wait := gu.NewRunner(ctx, NumRoutines, func(ctx context.Context, t *data) error {
		return c.downloadPartToWriterAt(ctx, fileId, t.offset, t.end, wa, false)
	})

	// 并发下载。
	partSize := getPartSize()
	for offset, end, next := int64(0), partSize-1, true; next; {
		if end >= fileSize-1 {
			end = fileSize - 1
			next = false
		}
		if err = run(&data{offset, end}, false); err != nil {
			return err
		}
		offset += partSize
		end = offset + partSize - 1
	}

	return wait(true)
}

// 下载文件，并从读取流中读出。
func (c *downloadImpl) multiDownloadToReader(ctx context.Context, fileId string, fileSize int64) (
	io.ReadCloser, error) {

	var (
		wc iu.WriteAtCloser
		rc io.ReadCloser
	)
	if c.nonUseDisk {
		var rc2 iu.ReadCloser
		wc, rc2 = iu.NewWriteAtToReader2()
		rc = iu.ToReader(rc2)
	} else {
		wc, rc = iu.NewWriteAtToReader()
	}

	type data struct {
		offset, end int64
	}
	run, wait := gu.NewRunner(ctx, NumRoutines, func(ctx context.Context, t *data) error {
		return c.downloadPartToWriterAt(ctx, fileId, t.offset, t.end, wc, c.nonUseDisk)
	})

	// 并发下载数据。
	go func() {
		partSize := getPartSize()
		for offset, end, next := int64(0), partSize-1, true; next; {
			if end > fileSize-1 {
				end = fileSize - 1
				next = false
			}

			if err := run(&data{offset, end}, false); err != nil {
				printError(wc.CloseByError(err))
				return
			}
			offset += partSize
			end = offset + partSize - 1
		}
		printError(wc.CloseByError(wait(true)))
	}()

	return rc, nil
}

// 下载分片字节数据到写入流。
func (c *downloadImpl) downloadPartToWriterAt(ctx context.Context, fileId string, offset, end int64,
	wa io.WriterAt, nonBuffer bool) error {

	header := http.Header{}
	header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))
	req := c.genReq(http.MethodGet, fileId, nil, header, nil)

	rsp, err := c.sendHttp(ctx, req)
	defer closeRsp(rsp)
	if err != nil {
		return err
	}
	n, err := iu.CopyReaderToWriterAt(rsp.Body, wa, offset, nonBuffer)
	if err != nil {
		return err
	}
	if n != end-offset+1 {
		return fmt.Errorf("part size not match, actual is %v, expected is %v, offset is %v, end is %v",
			n, end-offset+1, offset, end)
	}

	return nil
}
