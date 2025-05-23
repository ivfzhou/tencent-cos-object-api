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
	"errors"
	"time"
)

var (
	// ErrNotExists 文件不存在。
	ErrNotExists = errors.New("file not found")
	// PartSize 分片上传下载时，每个分片的大小。不可在文件上传下载期间修改值。
	PartSize = 10 * 1024 * 1024
	// MultiThreshold 文件大小超过多少个分片大小后，启用分片模式传输。
	MultiThreshold = 10
	// NumRoutines 分片上传下载时，并发运行协程的数量。
	NumRoutines = 5
	// AuthExpirationTime 每一个 HTTP 请求的凭证时效。
	AuthExpirationTime = 10 * time.Minute
)

type Api interface {
	Baser
	Uploader
	Downloader
	Deleter
	Querier
}

// NewClient 创建 COS Object 操作客户端。
func NewClient(host, appKey, secretKey string, opts ...option) Api {
	c := &baseImpl{
		appKey:    appKey,
		secretKey: secretKey,
		host:      host,
	}

	// 设置参数。
	for _, v := range opts {
		if v == nil {
			continue
		}
		v(&c.options)
	}

	multiUploader := &multiUploadImpl{c}
	uploader := &uploadImpl{c, multiUploader}
	downloader := &downloadImpl{c, multiUploader}
	querier := &queryImpl{c}
	deleter := &deleteImpl{c}

	return &impl{c, uploader, downloader, deleter, querier}
}
