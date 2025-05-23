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
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type deleteImpl struct {
	*baseImpl
}

// Delete 删除文件。
func (c *deleteImpl) Delete(ctx context.Context, fileId string) error {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	req := c.genReq(http.MethodDelete, fileId, nil, nil, nil)
	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return err
	}
	closeRsp(rsp)

	return nil
}

// Deletes 删除多个文件。
func (c *deleteImpl) Deletes(ctx context.Context, fileIds ...string) (undeleted map[string]error) {
	// 处理文件 ID。
	undeleted = make(map[string]error, len(fileIds))
	if len(fileIds) <= 0 {
		return
	}
	cleanedFileIds := make([]string, 0, len(fileIds))
	for _, v := range fileIds {
		v = suitFileId(v)
		if len(v) <= 0 {
			continue
		}
		cleanedFileIds = append(cleanedFileIds, v)
	}
	if len(cleanedFileIds) <= 0 {
		return
	}

	type Object struct {
		Key string `xml:"Key"`
	}
	type Delete struct {
		Quiet  bool      `xml:"Quiet"`
		Object []*Object `xml:",any"`
	}
	type Error struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
		Key     string `xml:"Key"`
	}
	type Deleted struct {
		Key string `xml:"Key"`
	}
	type DeleteResult struct {
		Error   []*Error   `xml:"Error"`
		Deleted []*Deleted `xml:"Deleted"`
	}

	// 循环删除文件。
	for len(cleanedFileIds) > 0 {
		// 每次最多删除一千个。
		n := min(1000, len(cleanedFileIds))
		ids := cleanedFileIds[:n]
		cleanedFileIds = cleanedFileIds[n:]

		// 组装请求体。
		query := url.Values{}
		query.Set("delete", "")
		reqObj := &Delete{Quiet: true, Object: make([]*Object, len(ids))}
		for i, v := range ids {
			reqObj.Object[i] = &Object{Key: v}
		}
		reqBody, _ := xml.Marshal(reqObj)
		header := http.Header{}
		reqBodySum := md5.Sum(reqBody)
		header.Set("Content-MD5", base64.StdEncoding.EncodeToString(reqBodySum[:]))
		req := c.genReq(http.MethodPost, "", query, header, reqBody)

		// 发送 HTTP 请求。
		rsp, err := c.sendHttp(ctx, req)
		if err != nil {
			for _, v := range ids {
				undeleted[v] = err
			}
			continue
		}
		rspBody, err := io.ReadAll(rsp.Body)
		closeRsp(rsp)
		if err != nil {
			e := fmt.Errorf("%v %v", err, string(rspBody))
			for _, v := range ids {
				undeleted[v] = e
			}
			continue
		}

		// 解析响应体。
		rspObj := &DeleteResult{}
		err = xml.Unmarshal(rspBody, rspObj)
		if err != nil {
			e := fmt.Errorf("%v %v", err, string(rspBody))
			for _, v := range ids {
				undeleted[v] = e
			}
			continue
		}

		// 保存删除失败的文件。
		for _, v := range rspObj.Error {
			if v.Key != "" {
				undeleted[v.Key] = fmt.Errorf("code is %s, message is %s", v.Code, v.Message)
			}
		}
	}

	return
}
