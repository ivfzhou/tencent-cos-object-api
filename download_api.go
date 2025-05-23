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
	"time"
)

type Downloader interface {
	// Download 下载文件。
	//
	// 注意：调用方负责关闭 rc。
	Download(ctx context.Context, fileId string) (rc io.ReadCloser, fileSize int64, err error)

	// DownloadToWriter 下载文件。
	DownloadToWriter(ctx context.Context, fileId string, w io.Writer) error

	// DownloadToWriterWithSize 下载文件。
	DownloadToWriterWithSize(ctx context.Context, fileId string, contentLength int64, w io.Writer) error

	// DownloadToDisk 下载文件。
	DownloadToDisk(ctx context.Context, fileId, filePath string) error

	// DownloadToWriterAt 下载文件。
	DownloadToWriterAt(ctx context.Context, fileId string, wa io.WriterAt) error

	// GetDownloadUrl 获取文件下载链接。
	GetDownloadUrl(fileId string, expiration time.Duration) string
}
