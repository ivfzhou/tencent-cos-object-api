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
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type queryImpl struct {
	*baseImpl
}

// Info 获取文件信息。
func (c *queryImpl) Info(ctx context.Context, fileId string) (*FileInfo, error) {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return nil, errors.New("fileId is invalid")
	}

	// 发送 HTTP 请求。
	rsp, err := c.head(ctx, fileId)
	if err != nil {
		return nil, err
	}

	// 解析响应。
	size, err := strconv.ParseInt(rsp.Header.Get("Content-Length"), 10, 64)
	printError(err)
	info := &FileInfo{
		Size:      size,
		EntityTag: rsp.Header.Get("Etag"),
		Crc64:     rsp.Header.Get("x-cos-hash-crc64ecma"),
	}
	info.UploadTime, _ = time.ParseInLocation(time.RFC1123, rsp.Header.Get("Last-Modified"), time.Local)
	info.ExpireTime, _ = time.ParseInLocation(time.RFC1123, rsp.Header.Get("Expires"), time.Local)

	return info, nil
}

// Exist 文件是否存在。
func (c *queryImpl) Exist(ctx context.Context, fileId string) (bool, error) {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return false, errors.New("fileId is invalid")
	}

	// 发送 HTTP 请求。
	_, err := c.head(ctx, fileId)
	if err != nil && !errors.Is(err, ErrNotExists) {
		return false, err
	}
	if err == nil {
		return true, nil
	}

	return false, nil
}

// ListFiles 获取文件列表信息列表。
func (c *queryImpl) ListFiles(ctx context.Context, dir, fileNamePrefix, offset string, limit int64) (
	files []*File, nextOffset string, err error) {

	// 生成文件路径。
	dir = filepath.Clean(dir)
	fileNamePrefix = filepath.Clean(fileNamePrefix)
	if strings.Contains(fileNamePrefix, "/") {
		index := strings.LastIndex(fileNamePrefix, "/")
		dir = filepath.Join(dir, fileNamePrefix[:index])
		fileNamePrefix = fileNamePrefix[index:]
	}
	fileId := filepath.Join(dir, fileNamePrefix)

	// 创建请求体。
	query := url.Values{}
	query.Set("prefix", fileId)
	query.Set("delimiter", "/")
	if limit > 0 {
		query.Set("max-keys", strconv.FormatInt(limit, 10))
	}
	if len(offset) > 0 {
		query.Set("marker", offset)
	}
	req := c.genReq(http.MethodGet, "", query, nil, nil)

	// 发送 HTTP 请求。
	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return
	}
	rspBody, err := io.ReadAll(rsp.Body)
	closeRsp(rsp)
	if err != nil {
		return
	}

	// 解析响应体。
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
	var res ListBucketResult
	if err = xml.Unmarshal(rspBody, &res); err != nil {
		return
	}

	// 组装文件信息。
	nextOffset = res.NextMarker
	files = make([]*File, len(res.Contents))
	for i, v := range res.Contents {
		mt, _ := time.Parse(time.RFC3339, v.LastModified)
		files[i] = &File{
			ID:         v.Key,
			Size:       v.Size,
			EntityTag:  v.ETag,
			UploadTime: mt,
		}
	}

	return
}
