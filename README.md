# 文章工具

关于文章可能使用到的工具

## 使用
clone本项目，在对应目录下
1. 直接运行：

```go
go run xx.go D:\test\ D:\blog\test.png
```

2. 编译为可执行文件使用：

```go
go build xx.go
```
将需要处理的文件使用exe打开即可

## checkContent

- 测试url链接是否有效的功能
- 是否包含本地链接
- 可批量检查，支持文件夹
- 同目录下的config.yaml可进行配置
  - 文件夹白名单：不检查的文件夹（node_modules,.git,.idea等）
  - 每个协程处理的文件数：默认为10

## upload2oss

- 上传图片文件到oss，返回oss链接。
- 通过计算md5值，防止重复上传
- 可批量上传，支持文件夹。
- 同目录下的config.yaml可进行配置
  - oss信息
  - 分片配置
