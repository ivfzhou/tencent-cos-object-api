# 一、书名

高性能腾讯云 COS（云对象存储）文件上传、下载、删除、查询 Go 客户端库。

[![codecov](https://codecov.io/gh/ivfzhou/tencent-cos-object-api/graph/badge.svg?token=N949TSNU2T)](https://codecov.io/gh/ivfzhou/tencent-cos-object-api)
[![Go Reference](https://pkg.go.dev/badge/gitee.com/ivfzhou/tencent-cos-object-api.svg)](https://pkg.go.dev/gitee.com/ivfzhou/tencent-cos-object-api)

# 二、特性

- **多种上传方式** — 支持字节数组、`io.Reader`、本地文件上传，自动根据文件大小切换分片模式
- **多种下载方式** — 支持下载到 `io.ReadCloser`、`io.Writer`、`io.WriterAt`、本地磁盘，支持生成带签名的下载链接
- **分片上传** — 完整的分片上传生命周期管理（初始化、上传分片、查询分片、完成/取消）
- **并发优化** — 分片传输使用多协程并发执行，默认 5 个并发协程
- **内存池复用** — 使用 `sync.Pool` 复用 HTTP 请求对象和字节缓冲区，降低 GC 压力
- **批量操作** — 支持批量删除文件
- **文件查询** — 获取文件信息、判断存在性、分页列举目录下文件
- **灵活配置** — 支持 HTTPS、自定义 HTTP Client、内存模式等选项

# 三、安装

```shell
go get gitee.com/ivfzhou/tencent-cos-object-api@latest
```

# 四、快速开始

```golang
import cos "gitee.com/ivfzhou/tencent-cos-object-api"

// 创建客户端
client := cos.NewClient("your_host", "app_key", "app_secret")

// 测试连接
err := client.Ping(ctx)
```

## 创建客户端（可选配置）

```golang
import (
    "net/http"
    "time"
    cos "gitee.com/ivfzhou/tencent-cos-object-api"
)

// 基础用法
client := cos.NewClient("your_host", "app_key", "app_secret")

// 使用 HTTPS 协议
client := cos.NewClient("your_host", "app_key", "app_secret", cos.WithHttps())

// 使用自定义 HTTP Client（可设置超时等）
httpClient := &http.Client{Timeout: 30 * time.Second}
client := cos.NewClient("your_host", "app_key", "app_secret", cos.WithHttpClient(httpClient))

// 临时文件使用内存而非磁盘
client := cos.NewClient("your_host", "app_key", "app_secret", cos.WithNonUseDisk())
```

# 五、API 文档

### 上传文件

| 方法 | 说明 |
|------|------|
| `Upload(ctx, fileId, []byte)` | 通过字节数组上传 |
| `UploadFromReader(ctx, fileId, io.Reader)` | 从 Reader 流式上传 |
| `UploadFromReaderWithSize(ctx, fileId, contentLength, io.Reader)` | 指定大小的 Reader 上传 |
| `UploadFromDisk(ctx, fileId, filePath)` | 从本地上传文件 |

> 当文件大小超过阈值（默认 `PartSize * MultiThreshold` = 100MB）或大于 5GiB 时，会自动使用分片模式上传。

```golang
// 字节数组上传
err := client.Upload(ctx, "dir/file.txt", bytes)

// Reader 上传
file, _ := os.Open("local/file.txt")
defer file.Close()
err := client.UploadFromReader(ctx, "dir/file.txt", file)

// 本地文件上传
err := client.UploadFromDisk(ctx, "dir/file.txt", "/path/to/local/file.txt")
```

### 分片上传

适用于大文件或需要控制上传进度的场景。

| 方法 | 说明 |
|------|------|
| `InitMultiUpload(ctx, fileId)` | 初始化分片上传任务，返回 uploadId |
| `UploadPart(ctx, fileId, uploadId, partNumber, []byte)` | 上传单个分片（字节） |
| `UploadPartByReader(ctx, fileId, uploadId, partNumber, contentLength, io.Reader)` | 上传单个分片（流式） |
| `ListFileParts(ctx, fileId, uploadId)` | 查询已上传的分片列表 |
| `CompleteMultiUpload(ctx, fileId, uploadId)` | 完成分片上传，合并所有分片 |
| `AbortMultiUpload(ctx, fileId, uploadId)` | 取消分片上传，丢弃已上传的分片 |

```golang
uploadId, err := client.InitMultiUpload(ctx, "large/file.bin")
if err != nil {
    // handle error
}

// 逐个上传分片
partNumber := int64(1)
for ... {
    err := client.UploadPartByReader(ctx, "large/file.bin", uploadId, partNumber, size, reader)
    if err != nil {
        // 失败时可以丢弃已上传分片
        _ = client.AbortMultiUpload(ctx, "large/file.bin", uploadId)
        return
    }
    partNumber++
}

// 完成合并
err := client.CompleteMultiUpload(ctx, "large/file.bin", uploadId)
```

### 下载文件

| 方法 | 说明 |
|------|------|
| `Download(ctx, fileId)` | 下载文件，返回 `io.ReadCloser` 和文件大小（**调用方需负责关闭**） |
| `DownloadToWriter(ctx, fileId, io.Writer)` | 下载到 Writer |
| `DownloadToWriterWithSize(ctx, fileId, contentLength, io.Writer)` | 指定大小下载到 Writer |
| `DownloadToDisk(ctx, fileId, filePath)` | 下载到本地文件 |
| `DownloadToWriterAt(ctx, fileId, io.WriterAt)` | 下载到 WriterAt（支持随机写入） |
| `GetDownloadUrl(fileId, expiration)` | 生成带签名的下载链接 |

```golang
// 读取为流
reader, fileSize, err := client.Download(ctx, "dir/file.txt")
if err == nil {
    defer reader.Close()
    // 使用 reader...
}

// 保存到磁盘
err := client.DownloadToDisk(ctx, "dir/file.txt", "/tmp/save/file.txt")

// 写入自定义 Writer
var buf bytes.Buffer
err := client.DownloadToWriter(ctx, "dir/file.txt", &buf)

// 生成带时效的下载链接（7天有效）
url := client.GetDownloadUrl("dir/file.txt", 7*24*time.Hour)
```

### 删除文件

| 方法 | 说明 |
|------|------|
| `Delete(ctx, fileId)` | 删除单个文件 |
| `Deletes(ctx, fileIds...)` | 批量删除多个文件，返回未成功删除的文件及错误 |

```golang
// 删除单个文件
err := client.Delete(ctx, "dir/file.txt")

// 批量删除
undeleted := client.Deletes(ctx, "file1.txt", "file2.txt", "file3.txt")
for fid, e := range undeleted {
    fmt.Printf("删除失败 %s: %v\n", fid, e)
}
```

### 文件查询

| 方法 | 说明 |
|------|------|
| `Info(ctx, fileId)` | 获取文件详细信息（大小、ETag、CRC64、上传时间、过期时间） |
| `Exist(ctx, fileId)` | 判断文件是否存在 |
| `ListFiles(ctx, dir, prefix, offset, limit)` | 分页列举目录下的文件 |

```golang
// 获取文件信息
info, err := client.Info(ctx, "dir/file.txt")
if err == nil {
    fmt.Printf("大小: %d, ETag: %s\n", info.Size, info.EntityTag)
}

// 判断文件是否存在
exists, err := client.Exist(ctx, "dir/file.txt")

// 分页列出目录下文件
files, nextOffset, err := client.ListFiles(ctx, "/my-bucket/", "log-", "", 100)
if err == nil {
    for _, f := range files {
        fmt.Printf("%s  %d  %s\n", f.ID, f.Size, f.UploadTime)
    }
    // nextOffset 为空表示没有更多数据
    if nextOffset != "" {
        // 继续翻页...
    }
}
```

### 工具方法

| 方法 | 说明 |
|------|------|
| `Ping(ctx)` | 测试与服务端的连通性 |
| `GenerateAuthorization(fileId, method, query, header, expiration)` | 生成 HTTP 请求签名字符串 |

# 六、全局配置项

可在初始化客户端之前修改以下全局变量来自定义行为：

| 变量 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `cos.PartSize` | `int64` | `10 * 1024 * 1024` (10MB) | 单个分片的大小，范围 1MiB ~ 5GiB |
| `cos.MultiThreshold` | `int` | `10` | 超过此数量的分片后启用分片模式（即 10 * PartSize = 100MB） |
| `cos.NumRoutines` | `int` | `5` | 分片上传/下载时的并发协程数 |
| `cos.AuthExpirationTime` | `time.Duration` | `10 * time.Minute` | 每个 HTTP 请求凭证的有效期 |

# 七、数据类型

### File（文件列表项）

| 字段 | 类型 | 说明 |
|------|------|------|
| ID | `string` | 文件 ID / 路径 |
| Size | `int64` | 文件大小（字节） |
| EntityTag | `string` | ETag 标签 |
| UploadTime | `time.Time` | 最近一次上传时间 |

### FileInfo（文件详情）

| 字段 | 类型 | 说明 |
|------|------|------|
| Size | `int64` | 文件大小（字节） |
| EntityTag | `string` | ETag 标签 |
| Crc64 | `string` | CRC64 校验值 |
| UploadTime | `time.Time` | 上传时间 |
| ExpireTime | `time.Time` | 过期时间 |

### FilePartInfo（分片信息）

| 字段 | 类型 | 说明 |
|------|------|------|
| PartNumber | `int` | 分片序号 |
| EntityTag | `string` | 该分片的 ETag |
| Size | `int64` | 分片大小（字节） |

### 错误

- `cos.ErrNotExists` — 文件不存在错误
