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

type MultiUploader interface {
	// InitMultiUpload 初始化分片上传区域。
	InitMultiUpload(ctx context.Context, fileId string) (uploadId string, err error)

	// UploadPart 上传分片。
	UploadPart(ctx context.Context, fileId, uploadId string, partNumber int64, reqBody []byte) error

	// UploadPartByReader 上传分片。
	UploadPartByReader(ctx context.Context, fileId, uploadId string, partNumber, contentLength int64, r io.Reader) error

	// ListFileParts 获取已上传的分片信息。
	ListFileParts(ctx context.Context, fileId, uploadId string) ([]*FilePartInfo, error)

	// CompleteMultiUpload 结束分片上传。
	CompleteMultiUpload(ctx context.Context, fileId, uploadId string) error

	// AbortMultiUpload 丢弃上传的分片。
	AbortMultiUpload(ctx context.Context, fileId, uploadId string) error
}
