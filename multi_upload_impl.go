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
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

type multiUploadImpl struct {
	*baseImpl
}

// InitMultiUpload 初始化分片上传区域。
func (c *multiUploadImpl) InitMultiUpload(ctx context.Context, fileId string) (string, error) {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return "", errors.New("fileId is invalid")
	}

	// 生成请求体。
	query := url.Values{}
	query.Set("uploads", "")
	header := http.Header{}
	header.Set("Content-Length", "0")
	req := c.genReq(http.MethodPost, fileId, query, header, nil)

	// 发送 HTTP 请求。
	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return "", err
	}

	// 读取出响应体。
	rspBody, err := io.ReadAll(rsp.Body)
	closeRsp(rsp)
	if err != nil {
		return "", err
	}

	// 解析响应体。
	var rspData struct {
		XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
		UploadId string
	}
	if err = xml.Unmarshal(rspBody, &rspData); err != nil {
		return "", err
	}

	return rspData.UploadId, nil
}

// UploadPart 上传分片。
func (c *multiUploadImpl) UploadPart(ctx context.Context, fileId, uploadId string, partNumber int64,
	reqBody []byte) error {

	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 生成请求体。
	query := url.Values{}
	query.Set("partNumber", strconv.FormatInt(partNumber, 10))
	query.Set("uploadId", uploadId)
	req := c.genReq(http.MethodPut, fileId, query, nil, reqBody)

	// 发送 HTTP 请求。
	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return err
	}
	closeRsp(rsp)

	return nil
}

// UploadPartByReader 上传分片。
func (c *multiUploadImpl) UploadPartByReader(ctx context.Context, fileId, uploadId string, partNumber,
	contentLength int64, r io.Reader) error {

	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 生成请求体。
	query := url.Values{}
	query.Set("uploadId", uploadId)
	query.Set("partNumber", strconv.FormatInt(partNumber, 10))
	req := c.genReqForReader(http.MethodPut, fileId, query, nil, contentLength, r)

	// 发送 HTTP 请求。
	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return err
	}
	closeRsp(rsp)

	return nil
}

// ListFileParts 获取已上传的分片信息。
func (c *multiUploadImpl) ListFileParts(ctx context.Context, fileId, uploadId string) ([]*FilePartInfo, error) {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return nil, errors.New("fileId is invalid")
	}

	parts := make([]*FilePartInfo, 0, MultiThreshold)
	next := ""
	for {
		// 生成请求体。
		query := url.Values{}
		query.Set("uploadId", uploadId)
		if len(next) > 0 {
			query.Set("part-number-marker", next)
		}
		req := c.genReq(http.MethodGet, fileId, query, nil, nil)

		// 发送请求。
		rsp, err := c.sendHttp(ctx, req)
		if err != nil {
			return nil, err
		}

		// 读取出响应体。
		rspBody, err := io.ReadAll(rsp.Body)
		closeRsp(rsp)
		if err != nil {
			return nil, err
		}

		// 解析响应体。
		var rspData struct {
			ListPartResultParts []struct {
				PartNumber string
				ETag       string
				Size       string
			} `xml:"Part"`
			NextPartNumberMarker string
		}
		if err = xml.Unmarshal(rspBody, &rspData); err != nil {
			return nil, err
		}

		// 组装分片信息。
		next = rspData.NextPartNumberMarker
		for _, v := range rspData.ListPartResultParts {
			partNum, err := strconv.Atoi(v.PartNumber)
			if err != nil {
				return nil, err
			}
			size, err := strconv.ParseInt(v.Size, 10, 64)
			if err != nil {
				return nil, err
			}
			parts = append(parts, &FilePartInfo{
				PartNumber: partNum,
				EntityTag:  v.ETag,
				Size:       size,
			})
		}

		// 没有更多分片了就跳出循环。
		if len(next) <= 0 {
			break
		}
	}

	// 分片信息排序。
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })

	return parts, nil
}

// CompleteMultiUpload 结束分片上传。
func (c *multiUploadImpl) CompleteMultiUpload(ctx context.Context, fileId, uploadId string) error {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 获取所有以上传的分片。
	parts, err := c.ListFileParts(ctx, fileId, uploadId)
	if err != nil {
		return err
	}

	// 生成请求体。
	type PartInfo struct {
		PartNumber string
		ETag       string
	}
	type CompleteMultipartUpload struct {
		Parts []*PartInfo `xml:"Part"`
	}
	var req CompleteMultipartUpload
	req.Parts = make([]*PartInfo, len(parts))
	for i, v := range parts {
		req.Parts[i] = &PartInfo{
			PartNumber: strconv.Itoa(v.PartNumber),
			ETag:       v.EntityTag,
		}
	}
	reqBody, _ := xml.Marshal(req)

	// 发送 HTTP 请求。
	query := url.Values{}
	query.Set("uploadId", uploadId)
	header := http.Header{}
	rsp, err := c.sendHttp(ctx, c.genReq(http.MethodPost, fileId, query, header, reqBody))
	if err != nil {
		return err
	}
	closeRsp(rsp)

	return nil
}

// AbortMultiUpload 丢弃上传的分片。
func (c *multiUploadImpl) AbortMultiUpload(ctx context.Context, fileId, uploadId string) error {
	fileId = suitFileId(fileId)
	if len(fileId) <= 0 {
		return errors.New("fileId is invalid")
	}

	// 发送 HTTP 请求。
	query := url.Values{}
	query.Set("uploadId", uploadId)
	req := c.genReq(http.MethodDelete, fileId, query, nil, nil)

	rsp, err := c.sendHttp(ctx, req)
	if err != nil {
		return err
	}
	closeRsp(rsp)

	return nil
}
