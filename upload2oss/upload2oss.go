package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	*OssConfig   `mapstructure:"oss"`
	*ChunkConfig `mapstructure:"chunk"`
}
type OssConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	AccessKeyId     string `mapstructure:"access_key_id"`
	AccessKeySecret string `mapstructure:"access_key_secret"`
	BucketName      string `mapstructure:"bucket_name"`
	Path            string `mapstructure:"path"`
}
type ChunkConfig struct {
	unit          string `mapstructure:"unit"`
	OpenChunkSize int64  `mapstructure:"open_chunk_size"`
	ChunkSize     int64  `mapstructure:"chunk_size"`
}

var (
	fileList    []string
	total       = 0
	mutex       sync.RWMutex
	uploadMutex sync.RWMutex
	config      = new(Config)
)

func Init() {

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	dir, _ := os.Getwd()
	//dir = dir[:strings.LastIndex(dir, string(os.PathSeparator))+1]
	viper.AddConfigPath(dir)

	err := viper.ReadInConfig()
	if err != nil {
		handleError(err)
	}

	if err := viper.Unmarshal(config); err != nil {
		handleError(err)
	}
	u := config.ChunkConfig.unit
	if u == "KB" {
		config.ChunkConfig.OpenChunkSize = config.ChunkConfig.OpenChunkSize * 1024
		config.ChunkConfig.ChunkSize = config.ChunkConfig.ChunkSize * 1024
	} else if u == "MB" {
		config.ChunkConfig.OpenChunkSize = config.ChunkConfig.OpenChunkSize * 1024 * 1024
		config.ChunkConfig.ChunkSize = config.ChunkConfig.ChunkSize * 1024 * 1024
	} else if u == "GB" {
		config.ChunkConfig.OpenChunkSize = config.ChunkConfig.OpenChunkSize * 1024 * 1024 * 1024
		config.ChunkConfig.ChunkSize = config.ChunkConfig.ChunkSize * 1024 * 1024 * 1024
	}
}

func main() {
	if len(os.Args) <= 1 {
		handleError(errors.New("请传入文件路径"))
	}
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("main 当前上传发生错误，Error:", err)
			var s string
			fmt.Println()
			fmt.Scanln(&s)
		}
	}()

	Init()
	endpoint := config.OssConfig.Endpoint
	path := config.OssConfig.Path
	fmt.Println("是否保存本地副本？(y/n)")
	var isSaveLocalFile string
	fmt.Scanln(&isSaveLocalFile)
	fmt.Println("是否开启大文件分片上传？(y/n)")
	var isOpenChunk string
	fmt.Scanln(&isOpenChunk)

	args := os.Args[1:]

	xoss := getOSS()

	start := time.Now()
	for i := range args {
		filePath := args[i]
		_, err := os.Stat(filePath)
		if err != nil {
			fmt.Println(filePath + " 文件不存在")
			continue
		}
		fileName := filePath[strings.LastIndex(filePath, string(os.PathSeparator))+1:]
		index := strings.LastIndex(fileName, ".")
		// 传目录
		if index == -1 {
			ReadAllDir(filePath)
		} else {
			fileType := fileName[index:]
			if fileType == ".png" || fileType == "jpg" || fileType == "jpeg" || fileType == "webp" || fileType == "gif" {
				uploadMutex.Lock()
				fileList = append(fileList, filePath)
				uploadMutex.Unlock()
			} else {
				fmt.Println(filePath + " 文件类型不支持")
			}

		}
	}
	var wg sync.WaitGroup
	for i := range fileList {
		filePath := fileList[i]
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
			fileName := filePath[strings.LastIndex(filePath, string(os.PathSeparator))+1:]
			fileType := fileName[strings.LastIndex(fileName, "."):]
			ReFileName, err := getMd5(filePath)
			if err != nil {
				fmt.Println("getMd5 当前上传发生错误，Error:", err)
				return
			}
			ReFileName = ReFileName + fileType
			isExist, err := xoss.IsObjectExist(path + ReFileName)
			if err != nil {
				fmt.Println("isExists 当前上传发生错误，Error:", err)
				return
			}
			if isExist {
				fmt.Println("源文件名为：" + fileName + " 重命名为 " + ReFileName + "的文件已存在，跳过上传" + " 访问路径为：https://" + endpoint + "/" + path + ReFileName)
				mutex.Lock()
				total++
				mutex.Unlock()
				return
			}
			// 获取当前文件的大小
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				fmt.Println("getSize 当前上传发生错误，Error:", err)
				return
			}
			size := fileInfo.Size()
			//start := time.Now()
			if size > config.ChunkConfig.OpenChunkSize && isOpenChunk == "y" {
				xchunk(*xoss, path+ReFileName, filePath)
			} else {
				uploadFile(*xoss, path+ReFileName, filePath)
			}
			//cost := time.Since(start)
			//fmt.Printf("cost=[%s]", cost)
			if isSaveLocalFile == "n" {
				err = os.Remove(filePath)
				if err != nil {
					fmt.Println("remove 当前上传发生错误，Error:", err)
					return
				}
			}
			fmt.Println("上传成功，源文件名为：" + fileName + " 重命名为 " + ReFileName + " 访问路径为：https://" + endpoint + "/" + path + ReFileName + "\n")
			mutex.Lock()
			total++
			mutex.Unlock()
		}(filePath)
	}
	wg.Wait()
	var s string
	cost := time.Since(start)
	fmt.Println("成功上传 ", total, "/", len(fileList), " 耗时", cost.String(), " 按回车键退出")
	fmt.Scanln(&s)
}

func ReadAllDir(path string) {
	FileInfo, err := ioutil.ReadDir(path)
	if err != nil {
		handleError(err)
	}
	var wg sync.WaitGroup
	for _, fileInfo := range FileInfo {
		fileName := fileInfo.Name()
		fullPath := path + string(os.PathSeparator) + fileInfo.Name()
		if fileInfo.IsDir() {
			wg.Add(1)
			//fmt.Println("目录名", fullPath)
			go func(fileName string) {
				ReadAllDir(fullPath)
				defer wg.Done()
			}(fileName)
		} else {
			fileType := fileName[strings.LastIndex(fileName, "."):]
			if fileType == ".png" || fileType == "jpg" || fileType == "jpeg" || fileType == "webp" || fileType == "gif" {
				uploadMutex.Lock()
				fileList = append(fileList, fullPath)
				uploadMutex.Unlock()
			} else {
				fmt.Println(fullPath + " 文件类型不支持")
			}
			//fmt.Println("文件名", fullPath)
		}
	}
	wg.Wait()
}

func getMd5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	hash := md5.New()
	_, _ = io.Copy(hash, file)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func getOSS() *oss.Bucket {
	client, err := oss.New(config.OssConfig.Endpoint, config.OssConfig.AccessKeyId, config.OssConfig.AccessKeySecret)
	if err != nil {
		handleError(err)
	}
	bucket, err := client.Bucket(config.OssConfig.BucketName)
	if err != nil {
		handleError(err)
	}
	return bucket
}
func uploadFile(bucket oss.Bucket, objectName string, localFile string) {
	err := bucket.PutObjectFromFile(objectName, localFile)
	if err != nil {
		fmt.Println("upload 当前上传发生错误，Error:", err)
		return
	}
}
func xchunk(bucket oss.Bucket, objectName string, localFilename string) {
	fd, err := os.Open(localFilename)
	if err != nil {
		fmt.Println("chunk 当前上传发生错误，Error:", err)
		return
	}
	defer func() {
		err := fd.Close()
		if err != nil {
			fmt.Println("chunk 当前上传发生错误，Error:", err)
		}
		if err := recover(); err != nil {
			fmt.Println("chunk 当前上传发生错误，Error:", err)
		}
	}()
	// 将本地文件分片
	chunks, err := oss.SplitFileByPartSize(localFilename, config.ChunkConfig.ChunkSize)
	if err != nil {
		fmt.Println("chunk 当前上传发生错误，Error:", err)
		return
	}
	//fmt.Printf("chunks:%v\n", chunks)
	// 步骤1：初始化一个分片上传事件
	imur, err := bucket.InitiateMultipartUpload(objectName, nil)
	if err != nil {
		fmt.Println("chunk 当前上传发生错误，Error:", err)
		return
	}
	// 步骤2：上传分片
	var parts []oss.UploadPart
	waitGroup := sync.WaitGroup{}

	for _, chunk := range chunks {
		waitGroup.Add(1)
		go func(chunk oss.FileChunk) {
			defer waitGroup.Done()
			partData := io.NewSectionReader(fd, chunk.Offset, chunk.Size)
			// 调用UploadPart方法上传每个分片。
			//start := time.Now()
			//fmt.Printf("准备发送分片[%d]\n", chunk.Number)
			part, err := bucket.UploadPart(imur, partData, chunk.Size, chunk.Number)
			//cost := time.Since(start)
			//fmt.Printf("[%v]cost=[%s] partSize:[%d]\n", chunk, cost, partData.Size())
			if err != nil {
				fmt.Println("chunk 当前上传发生错误，Error:", err)
				panic(err)
			}
			parts = append(parts, part)
		}(chunk)
	}
	waitGroup.Wait()
	// 步骤3：完成分片上传
	_, err = bucket.CompleteMultipartUpload(imur, parts)
	if err != nil {
		fmt.Println("chunk 当前上传发生错误，Error:", err)
		return
	}
}

func handleError(err error) {
	fmt.Println("服务发生错误，Error:", err)
	var s string
	fmt.Scanln(&s)
	os.Exit(-1)
}
