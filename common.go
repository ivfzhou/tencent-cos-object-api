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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"
)

var (
	requestPool = sync.Pool{New: func() any {
		return &http.Request{
			ProtoMajor: 1,
			ProtoMinor: 1,
		}
	}}
	bytesPool = sync.Pool{New: func() any { return make([]byte, getPartSize()) }}
)

// 获取请求体。
func getRequest() *http.Request {
	return requestPool.Get().(*http.Request)
}

// 回收请求体。
func rollbackRequest(req *http.Request) {
	if req != nil {
		req.Method = ""
		req.URL = nil
		req.Proto = ""
		req.Header = nil
		req.Body = nil
		req.GetBody = nil
		req.TransferEncoding = nil
		req.Close = false
		req.Form = nil
		req.PostForm = nil
		req.MultipartForm = nil
		req.Trailer = nil
		req.RemoteAddr = ""
		req.RequestURI = ""
		req.TLS = nil
		req.Cancel = nil
		req.Response = nil
		req.Pattern = ""
		requestPool.Put(req)
	}
}

// 获取字节数组。
func makeBytes() []byte {
	return bytesPool.Get().([]byte)
}

// 回收字节数组。
func rollbackBytes(data []byte) {
	if int64(cap(data)) != getPartSize() {
		data = nil
		return
	}
	if cap(data) > len(data) {
		data = unsafe.Slice(&data[0], cap(data))
	}
	bytesPool.Put(data)
}

// 获取分片大小。
func getPartSize() int64 {
	partSize := PartSize
	if partSize < 1024*1024 {
		return 1024 * 1024 * 8 // 一个分片最小 1MiB。
	} else if partSize > 5*1024*1024*1024 {
		return 5 * 1024 * 1024 * 1024 // 一个分片最大 5GiB。
	}
	return int64(partSize)
}

// 读取响应体并关闭。
func readAndClose(rsp *http.Response) []byte {
	if rsp != nil && rsp.Body != nil {
		bs, err := io.ReadAll(rsp.Body)
		printError(err)
		closeRsp(rsp)
		return bs
	}
	return nil
}

// 关闭流。
func closeIO(closer io.Closer) {
	if closer != nil {
		printError(closer.Close())
	}
}

// 关闭 HTTP 响应对象的响应体。
func closeRsp(r *http.Response) {
	if r != nil && r.Body != nil {
		printError(r.Body.Close())
	}
}

// URL 编码。
func urlEncode(s string) string {
	var b bytes.Buffer
	written := 0
	for i, n := 0, len(s); i < n; i++ {
		ch := s[i]
		switch ch {
		case '-', '_', '.', '!', '~', '*', '\'', '(', ')':
			continue
		default:
			if 'a' <= ch && ch <= 'z' {
				continue
			}
			if 'A' <= ch && ch <= 'Z' {
				continue
			}
			if '0' <= ch && ch <= '9' {
				continue
			}
		}
		b.WriteString(s[written:i])
		_, _ = fmt.Fprintf(&b, "%%%02X", ch)
		written = i + 1
	}

	if written == 0 {
		return s
	} else {
		b.WriteString(s[written:])
		s = b.String()
	}

	s = strings.ReplaceAll(s, "!", "%21")
	s = strings.ReplaceAll(s, "'", "%27")
	s = strings.ReplaceAll(s, "(", "%28")
	s = strings.ReplaceAll(s, ")", "%29")
	s = strings.ReplaceAll(s, "*", "%2A")

	return s
}

// 向标准错误输出流打印错误信息。
func printError(err error) {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "tencent-cos-object-api: %v\n", err)
	}
}

// 纠正文件 ID。
func suitFileId(fileId string) string {
	return strings.TrimLeft(strings.TrimLeft(filepath.Clean(strings.Trim(fileId, "/")), "."), "/")
}

// 判断文件大小是否用分片模式传输。
func useMultipart(size int64) bool {
	partSize := getPartSize()
	return size > int64(MultiThreshold)*partSize || size > 5*1024*1024*1024 // 大于 5GiB，必须要用分片上传。
}
