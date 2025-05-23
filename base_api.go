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
	"net/http"
	"net/url"
	"time"
)

type Baser interface {
	// Ping 测试连接。
	Ping(ctx context.Context) error

	// GenerateAuthorization 生成 HTTP 请求的签名字符串。
	GenerateAuthorization(fileId, method string, query url.Values, header http.Header, expiration time.Duration) string
}
