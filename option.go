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

import "net/http"

type options struct {
	client     *http.Client
	tls        bool
	nonUseDisk bool
}

type option func(*options)

// WithHttpClient 使用自定义 HTTP 客户端实现。默认使用 http.DefaultClient。
func WithHttpClient(client *http.Client) option {
	return func(o *options) {
		o.client = client
	}
}

// WithHttps 使用 https 协议。
func WithHttps() option {
	return func(o *options) {
		o.tls = true
	}
}

// WithNonUseDisk 临时文件数据不放置到外存，而是放置在内存。
func WithNonUseDisk() option {
	return func(o *options) {
		o.nonUseDisk = true
	}
}
