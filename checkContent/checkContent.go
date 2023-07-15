package main

import (
	"fmt"
	"github.com/spf13/viper"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	config    = new(Config)
	fileList  []string
	mutex     sync.RWMutex
	totalSize int64
	loopCount int
	loopMutex sync.Mutex
	ch        = make(chan int)
)

type Config struct {
	PassList      []string `mapstructure:"pass_list"`
	EachWorkCount int      `mapstructure:"each_work_count"`
}

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
	fmt.Println(config)
}
func main() {
	if len(os.Args) <= 1 {
		handleError(fmt.Errorf("请传入文件路径"))
	}

	defer func() {
		if err := recover(); err != nil {
			handleError(fmt.Errorf("%v", err))
		}
	}()
	Init()
	start := time.Now()
	getFileList(os.Args[1:])

	p := operateFileList()
	duration := time.Since(start)

	var s string
	fmt.Println("检查了 ", loopCount, "/", len(fileList), " 个文件")
	fmt.Println("耗时：", duration, "存在", p, "个问题")
	fmt.Println("输入回车键退出")
	fmt.Scanln(&s)
}

func getFileList(args []string) {
	for i := range args {
		filePath := args[i]
		stat, err := os.Stat(filePath)
		if err != nil {
			fmt.Println(filePath + " 文件不存在")
			continue
		}
		fileName := filePath[strings.LastIndex(filePath, string(os.PathSeparator))+1:]
		index := strings.LastIndex(fileName, ".")
		// 目录
		if index == -1 {
			ReadAllDir(filePath)
			continue
		}
		// 文件
		fileType := fileName[index:]
		// 目前只支持md
		if fileType == ".md" {
			mutex.Lock()
			fileList = append(fileList, filePath)
			totalSize += stat.Size()
			mutex.Unlock()
		}
	}
	fmt.Println("文件总数：", len(fileList))
	fmt.Println("文件总大小：", totalSize/1024, "KB")
}

func operateFileList() int {
	eachWorkCount := config.EachWorkCount
	var p int
	totalLen := len(fileList)
	count := totalLen / eachWorkCount
	least := totalLen % eachWorkCount
	var wg sync.WaitGroup
	if count >= 1 {
		for i := 1; i <= count; i++ {
			wg.Add(1)
			go func(i int) {
				var p int
				// 10个文件一组
				for j := i * eachWorkCount; j < (i+1)*eachWorkCount && j < totalLen; j++ {
					err, s := operateFile(fileList[j], checkInvalidLink, checkLocalLink)
					if err != nil {
						fmt.Println("打开文件错误，请检查：", fileList[j])
						continue
					}
					p += s
				}
				ch <- p
				wg.Done()
			}(i - 1)
		}
		for i := count * eachWorkCount; i < totalLen; i++ {
			err, s := operateFile(fileList[i], checkInvalidLink, checkLocalLink)
			if err != nil {
				fmt.Println("打开文件错误，请检查：", fileList[i])
				continue
			}
			s += p
		}
		for i := 1; i <= count; i++ {
			p += <-ch
		}
	} else {
		for i := 0; i < least; i++ {
			err, s := operateFile(fileList[i], checkInvalidLink, checkLocalLink)
			if err != nil {
				fmt.Println("打开文件错误，请检查：", fileList[i])
				continue
			}
			s += p
		}
	}
	wg.Wait()
	return p
}
func operateFile(filePath string, fs ...func(content, filePath string) int) (error, int) {
	//fmt.Println("准备检查：", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return err, 0
	}

	stat, _ := file.Stat()
	buf := make([]byte, stat.Size())
	_, _ = file.Read(buf)
	defer file.Close()
	content := string(buf)
	var p int
	for _, f := range fs {
		p += f(content, filePath)
	}
	loopMutex.Lock()
	loopCount++
	loopMutex.Unlock()
	return nil, p
}
func checkLocalLink(content, filePath string) int {
	regx := `[A-Z]:\\`
	strs := getTargetContent(content, regx)
	if len(strs) > 0 {
		fmt.Println("本地链接问题----->", "文件："+filePath+"存在本地链接", len(strs), "个")
	}
	return len(strs)
}
func checkInvalidLink(content, filePath string) int {

	regx := "(https?|ftp|file)://[-A-Za-z0-9一-龥+&@#/%?=~_|!:,.;]+[-A-Z一-龥a-z0-9+&@#/%=~_|]"
	strs := getTargetContent(content, regx)
	unInvalidLinkCount := 0
	for i := range strs {
		xurl, err := url.Parse(strs[i][0])
		if err != nil || xurl == nil {
			fmt.Println("url问题----->", "文件："+filePath+" url解析失败 ", strs[i][0])
			unInvalidLinkCount++
		}
		//fmt.Println(xurl.String())
		client := http.DefaultClient
		client.Timeout = time.Second * 3
		resp, err := client.Get(xurl.String())
		//fmt.Println("resp:", resp, "err:", err)
		if resp == nil {
			fmt.Println("url问题----->", "文件："+filePath+" url请求失败", strs[i][0]+" err:", err)
			unInvalidLinkCount++
			continue
		}
		if err != nil || (resp != nil && resp.StatusCode != http.StatusOK) {
			//fmt.Println(xurl.Host)
			unInvalidLinkCount++
			fmt.Println("url问题----->", "文件："+filePath+" url请求失败", strs[i][0]+" err:", err, "respCode:", resp.StatusCode)
			continue
		}
	}
	return unInvalidLinkCount
}
func getTargetContent(s string, regx string) [][]string {
	// 1. 解析规则
	reg := regexp.MustCompile(regx)
	if reg == nil { // 解释失败
		fmt.Println("MustCompile err")
		return nil
	}
	// 2. 根据规则提取关键信息
	res := reg.FindAllStringSubmatch(s, -1)
	return res
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
			if contains(fileName, config.PassList) {
				continue
			}
			wg.Add(1)
			//fmt.Println("目录名", fullPath)
			go func(fileName string) {
				ReadAllDir(fullPath)
				defer wg.Done()
			}(fileName)
		} else {
			index := strings.LastIndex(fileName, ".")
			if index <= 0 || index >= len(fileName)-1 {
				continue
			}
			if fileName[index:] == ".md" {
				mutex.Lock()
				fileList = append(fileList, fullPath)
				totalSize += fileInfo.Size()
				mutex.Unlock()
			}
			//fmt.Println("文件名", fullPath)
		}
	}
	wg.Wait()
}
func contains(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
func handleError(err error) {
	fmt.Println("服务发生错误，Error:", err)
	var s string
	fmt.Scanln(&s)
	os.Exit(-1)
}
