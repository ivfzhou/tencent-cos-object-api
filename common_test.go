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

package cos_test

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	gu "gitee.com/ivfzhou/goroutine-util"
	cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

var CloseCount int32

type mockTransport struct {
	fn func(*http.Request) (*http.Response, error)
}

type ctxCancelWithError struct {
	context.Context
	err gu.AtomicError
}

type writeCloser struct {
	closeFlag int64
	w         func([]byte) (int, error)
}

type writerAt struct {
	f func([]byte, int64) (int, error)
}

type readCloser struct {
	closeErr    error
	readErr     error
	closeFlag   int32
	data        []byte
	readCount   int
	total       int
	interceptor func()
}

func NewReader(data []byte, interceptor func(), closeErr, readErr error) io.ReadCloser {
	atomic.AddInt32(&CloseCount, 1)
	return &readCloser{
		closeErr:    closeErr,
		readErr:     readErr,
		data:        data,
		total:       len(data),
		interceptor: interceptor,
	}
}

func NewWriter(f func([]byte) (int, error)) io.WriteCloser {
	atomic.AddInt32(&CloseCount, 1)
	return &writeCloser{w: f}
}

func NewWriterAt(f func([]byte, int64) (int, error)) io.WriterAt {
	return &writerAt{f: f}
}

func WriteAll(w io.Writer, data []byte) (written int64, err error) {
	n := 0
	for len(data) > 0 {
		n, err = w.Write(data)
		written += int64(n)
		if err != nil {
			return written, err
		}
		data = data[min(n, len(data)):]
	}
	return
}

func NewCtxCancelWithError() (context.Context, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &ctxCancelWithError{Context: ctx}
	return c, func(cause error) {
		c.err.Set(cause)
		cancel()
	}
}

func MakeBytes() ([]byte, bool) {
	var size int
	much := rand.Intn(2) == 1
	if much {
		size = cos.PartSize*cos.MultiThreshold*(rand.Intn(5)+1) + rand.Intn(14) + 1
	} else {
		size = rand.Intn(cos.PartSize * cos.MultiThreshold)
	}
	data := make([]byte, size)
	n, err := crand.Read(data)
	if err != nil || n != len(data) {
		panic("rand.Read fail")
	}
	return data, much
}

func UrlEncode(s string) string {
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

func MakeBytesWithSize(n int) []byte {
	data := make([]byte, n)
	n, err := crand.Read(data)
	if err != nil || n != len(data) {
		panic("rand.Read fail")
	}
	return data
}

func MockHttpClient(fn func(*http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{
		Transport: &mockTransport{
			fn: fn,
		},
	}
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.fn(req)
}

func (w *writerAt) WriteAt(p []byte, of int64) (int, error) {
	return w.f(p, of)
}

func (rc *readCloser) Read(p []byte) (int, error) {
	if rc.interceptor != nil {
		rc.interceptor()
	}
	if len(rc.data) <= 0 {
		if rc.readErr != nil {
			rc.data = nil
			return 0, rc.readErr
		}
		return 0, io.EOF
	}
	if rc.readErr != nil {
		if rc.readCount >= rc.total/2 {
			rc.data = nil
			return 0, rc.readErr
		}
	}
	n := copy(p, rc.data)
	rc.data = rc.data[n:]
	rc.readCount += n
	if len(rc.data) <= 0 {
		if rc.readErr != nil {
			p[len(p)-1]++
			p = p[:len(p)-1]
			rc.data = nil
			return n - 1, rc.readErr
		}
		return n, io.EOF
	}
	return n, nil
}

func (rc *readCloser) Close() error {
	if atomic.CompareAndSwapInt32(&rc.closeFlag, 0, 1) {
		atomic.AddInt32(&CloseCount, -1)
		return rc.closeErr
	}
	return fmt.Errorf("reader already closed")
}

func (c *ctxCancelWithError) Deadline() (deadline time.Time, ok bool) {
	return c.Context.Deadline()
}

func (c *ctxCancelWithError) Done() <-chan struct{} {
	return c.Context.Done()
}

func (c *ctxCancelWithError) Err() error {
	return c.err.Get()
}

func (c *ctxCancelWithError) Value(key any) any {
	return c.Context.Value(key)
}

func (w *writeCloser) Write(p []byte) (n int, err error) {
	return w.w(p)
}

func (w *writeCloser) Close() error {
	if atomic.CompareAndSwapInt64(&w.closeFlag, 0, 1) {
		atomic.AddInt32(&CloseCount, -1)
		return nil
	}
	return fmt.Errorf("writer already closed")
}
