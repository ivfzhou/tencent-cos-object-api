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
	"io"
)

// FilePartInfo 文件分片信息。
type FilePartInfo struct {
	// PartNumber 序号。
	PartNumber int
	// EntityTag 对象被创建时标识对象内容的信息标签。
	EntityTag string
	// Size 分片大小。
	Size int64
}

type Uploader interface {
	// Upload 上传文件。
	Upload(ctx context.Context, fileId string, content []byte) error

	// UploadFromReader 上传文件。
	UploadFromReader(ctx context.Context, fileId string, r io.Reader) error

	// UploadFromReaderWithSize 上传文件。
	UploadFromReaderWithSize(ctx context.Context, fileId string, contentLength int64, r io.Reader) error

	// UploadFromDisk 上传文件。
	UploadFromDisk(ctx context.Context, fileId, filePath string) error

	MultiUploader
}
