# 说明

高性能腾讯云 COS 文件上传下载删除查看客户端

[![codecov](https://codecov.io/gh/ivfzhou/tencent-cos-object-api/graph/badge.svg?token=N949TSNU2T)](https://codecov.io/gh/ivfzhou/tencent-cos-object-api)

# 使用

```shell
go get gitee.com/ivfzhou/tencent-cos-object-api@latest
```

```golang
import cos "gitee.com/ivfzhou/tencent-cos-object-api"

client := cos.NewClient("your_host", "app_key", "app_secret")
err := client.Ping()

// 上传文件
err := client.Upload(ctx, fileId, bytes)
err := client.UploadFromReader(ctx, fileId, reader)
err := client.UploadFromDisk(ctx, fileId, filePath)

// 分片上传
uploadId, err := client.InitMultiUpload(ctx, fileId)
err := client.UploadPartByReader(ctx, fileId, uploadId, fileSize, rc)
err := client.CompleteMultiUpload(ctx, fileId, uploadId)

// 下载文件
reader, fileSize, err := client.Download(ctx, fileId)
err := client.DownloadToDisk(ctx, fileId, filePath)
err := client.DownloadToWriterAt(ctx, fileId, writer)
err := client.DownloadToWriter(ctx, fileId, writer)

// 删除文件
err := client.Delete(ctx, fileId)
err := client.Deletes(ctx, fileId, fileId2, ....)

// 查看文件
files, nextOffset, err := client.ListFiles(ctx, "/dir/path/to/", "fileNamePrefix", "offset", 100)
```

# 联系作者

电邮：ivfzhou@126.com  
微信：h899123
