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
	"time"
)

type File struct {
	// 文件ID。
	ID string
	// Size 文件大小。
	Size int64
	// EntityTag 对象被创建时标识对象内容的信息标签。
	EntityTag string
	// UploadTime 对象的最近一次上传的时间。
	UploadTime time.Time
}

// FileInfo 文件信息。
type FileInfo struct {
	// Size 文件大小。
	Size int64
	// EntityTag 对象被创建时标识对象内容的信息标签。
	EntityTag string
	// 对象的 CRC64 值。
	Crc64 string
	// UploadTime 对象的最近一次上传的时间。
	UploadTime time.Time
	// ExpireTime 过期时间。
	ExpireTime time.Time
}

type Querier interface {
	// Info 获取文件信息。
	Info(ctx context.Context, fileId string) (*FileInfo, error)

	// Exist 文件是否存在。
	Exist(ctx context.Context, fileId string) (bool, error)

	// ListFiles 获取文件列表信息列表。
	ListFiles(ctx context.Context, dir, fileNamePrefix, offset string, limit int64) (
		files []*File, nextOffset string, err error)
}
