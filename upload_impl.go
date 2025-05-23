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
	"errors"
	"io"
	"net/http"
	"os"

	gu "gitee.com/ivfzhou/goroutine-util"
)

type uploadImpl struct {
	*baseImpl
	MultiUploader
}

// Upload 上传文件。
func (c *uploadImpl) Upload(ctx context.Context, fileId string, reqBody []byte) error {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 是否启用分片模式上传。
	size := int64(len(reqBody))
	if useMultipart(size) {
		return c.multiUploadFromReaderWithSize(ctx, fileId, size, bytes.NewReader(reqBody))
	}

	return c.upload(ctx, fileId, reqBody)
}

// UploadFromReader 上传文件。
func (c *uploadImpl) UploadFromReader(ctx context.Context, fileId string, r io.Reader) error {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 初始化上传。
	uploadId, err := c.InitMultiUpload(ctx, fileId)
	if err != nil {
		return err
	}

	// 并发上传。
	type data struct {
		buf []byte
		num int64
	}
	run, wait := gu.NewRunner(ctx, NumRoutines, func(ctx context.Context, t *data) error {
		defer rollbackBytes(t.buf)
		return c.UploadPart(ctx, fileId, uploadId, t.num, t.buf)
	})

	for i, next, n := 1, true, 0; next; i++ {
		buf := makeBytes()
		n, err = io.ReadFull(r, buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				next = false
			} else {
				return err
			}
		}
		if err = run(&data{buf[:n], int64(i)}, false); err != nil {
			return err
		}
	}

	if err = wait(true); err != nil {
		noCancelCtx := context.WithoutCancel(ctx)
		go func() { // 出错就丢弃已上传的分片。
			_ = wait(false)
			printError(c.AbortMultiUpload(noCancelCtx, fileId, uploadId))
		}()
		return err
	}

	// 合并分片，结束上传。
	if err = c.CompleteMultiUpload(ctx, fileId, uploadId); err != nil {
		noCancelCtx := context.WithoutCancel(ctx)
		go func() { // 出错就丢弃已上传的分片。
			printError(c.AbortMultiUpload(noCancelCtx, fileId, uploadId))
		}()
	}

	return err
}

// UploadFromReaderWithSize 上传文件。
func (c *uploadImpl) UploadFromReaderWithSize(ctx context.Context, fileId string, contentLength int64,
	r io.Reader) error {

	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 是否启用分片模式上传。
	if useMultipart(contentLength) {
		return c.multiUploadFromReaderWithSize(ctx, fileId, contentLength, r)
	}

	return c.uploadFromReaderWithSize(ctx, fileId, contentLength, io.NopCloser(r))
}

// UploadFromDisk 上传文件。
func (c *uploadImpl) UploadFromDisk(ctx context.Context, fileId, filePath string) error {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 获取文件信息。
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	// 打开文件流。
	fileObj, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer closeIO(fileObj)
	size := fileInfo.Size()

	// 是否启用分片模式上传。
	if useMultipart(size) {
		return c.multiUploadFromReaderWithSize(ctx, fileId, size, fileObj)
	}

	return c.uploadFromReaderWithSize(ctx, fileId, size, fileObj)
}

// 上传文件。
func (c *uploadImpl) upload(ctx context.Context, fileId string, content []byte) error {
	req := c.genReq(http.MethodPut, fileId, nil, nil, content)
	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return err
	}
	closeRsp(rsp)
	return nil
}

// 上传文件。
func (c *uploadImpl) uploadFromReaderWithSize(ctx context.Context, fileId string, contentLength int64,
	rc io.ReadCloser) error {

	req := c.genReqForReader(http.MethodPut, fileId, nil, nil, contentLength, rc)
	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return err
	}
	closeRsp(rsp)
	return nil
}

// 从读取流中读取上传文件。
func (c *uploadImpl) multiUploadFromReaderWithSize(ctx context.Context, fileId string, contentLength int64,
	r io.Reader) error {

	// 初始化分片上传。
	uploadId, err := c.InitMultiUpload(ctx, fileId)
	if err != nil {
		return err
	}

	type data struct {
		buf []byte
		num int64
	}
	run, wait := gu.NewRunner(ctx, NumRoutines, func(ctx context.Context, t *data) error {
		defer rollbackBytes(t.buf)
		return c.UploadPart(ctx, fileId, uploadId, t.num, t.buf)
	})

	// 并发上传分片。
	partSize := getPartSize()
	for i, totalRead, n := 1, int64(0), int64(0); totalRead < contentLength; i, totalRead = i+1, totalRead+partSize {
		n = partSize
		var buf []byte
		if totalRead+partSize > contentLength {
			n = contentLength - totalRead
			buf = make([]byte, n)
		} else {
			buf = makeBytes()
		}
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return err
		}
		if err = run(&data{buf, int64(i)}, false); err != nil {
			return err
		}
	}

	if err = wait(true); err != nil {
		noCancelCtx := context.WithoutCancel(ctx)
		go func() { // 出错就丢弃已上传的分片。
			_ = wait(false) // 等待所有协程退出。
			printError(c.AbortMultiUpload(noCancelCtx, fileId, uploadId))
		}()
		return err
	}

	// 合并分片。
	if err = c.CompleteMultiUpload(ctx, fileId, uploadId); err != nil {
		noCancelCtx := context.WithoutCancel(ctx)
		go func() { // 出错就丢弃已上传的分片。
			printError(c.AbortMultiUpload(noCancelCtx, fileId, uploadId))
		}()
	}

	return err
}
