package main

import (
	"encoding/json"
	"fmt"
	"github.com/atotto/clipboard"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	hook "github.com/robotn/gohook"
	"github.com/spf13/viper"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	*AppConfig    `mapstructure:"app"`
	*ApiConfig    `mapstructure:"api"`
	*HotKeyConfig `mapstructure:"hot_key"`
}
type AppConfig struct {
	Name     string `mapstructure:"name"`
	FontSize int    `mapstructure:"font_size"`
}
type ApiConfig struct {
	Url      string `mapstructure:"url"`
	QueryKey string `mapstructure:"query_key"`
}
type HotKeyConfig struct {
	KeyCode     []uint16 `mapstructure:"key_code"`
	EffectSpan  int      `mapstructure:"effect_span"`
	RequestSpan int      `mapstructure:"request_span"`
}

var (
	window = MainWindow{}
	config = new(Config)
	op     = time.Now()
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
}
func translateText(text string) (string, bool) {

	requestSpan := config.HotKeyConfig.RequestSpan
	if time.Since(op) < time.Duration(requestSpan)*time.Millisecond {
		return "请求过于频繁，请稍后再试", false
	}
	xurl := config.ApiConfig.Url
	queryKey := config.ApiConfig.QueryKey
	baseUrl, err := url.Parse(xurl)
	if err != nil {
		return "解析url出错了：" + err.Error(), false
	}
	params := baseUrl.Query()
	params.Add(queryKey, strings.Replace(text, "\r\n", "", -1))
	baseUrl.RawQuery = params.Encode()
	api := baseUrl.String()
	fmt.Println("api:", api)
	resp, err := http.Get(api)
	if err != nil {
		return "请求出错了：" + err.Error(), false
	}
	m := make(map[string]interface{})
	buffer, err := io.ReadAll(resp.Body)
	if err != nil {
		return "获取请求结果出错了：" + err.Error(), false
	}
	s := string(buffer)
	println(s)
	err = json.Unmarshal(buffer, &m)
	if err != nil {
		return "解析请求结果出错了：" + err.Error(), false

	}
	op = time.Now()
	return (m["text"]).(string), true
}

func main() {
	var inputLine, outputLine *walk.TextEdit
	defer func() {
		if err := recover(); err != nil {
			handleError(err.(error))
		}
	}()
	Init()
	window = MainWindow{
		Title:  config.AppConfig.Name,
		Size:   Size{Width: 600, Height: 300},
		Layout: VBox{},
		Children: []Widget{
			Label{Text: "输入要翻译的文本:"},
			TextEdit{AssignTo: &inputLine, OnKeyDown: func(key walk.Key) {
				if walk.ModifiersDown() == walk.ModControl && key == walk.KeyW {
					translatedText, _ := translateText(inputLine.Text())
					outputLine.SetText(outputLine.Text() + "\r\n" + translatedText)
				}
			},
				MaxSize: Size{Width: 600},
				VScroll: true,
				Font:    Font{PointSize: config.AppConfig.FontSize}},
			TextEdit{AssignTo: &outputLine, ReadOnly: true, Font: Font{PointSize: config.AppConfig.FontSize}, MaxSize: Size{Width: 600}, VScroll: true},
		},
	}
	go func() {
		hooks := hook.Start()
		defer hook.End()
		keyCode := config.HotKeyConfig.KeyCode
		var re uint8 = 0
		for i, _ := range keyCode {
			re += 1 << uint8(i)
		}
		ori := re
		effectSpan := config.HotKeyConfig.EffectSpan
		last := time.Now()
		for ev := range hooks {
			fmt.Println("keyCode", ev.Keycode)
			for i, code := range keyCode {
				if ev.Keycode == code && re>>uint8(i)&1 == 1 && ev.Mask == 2 {
					if time.Since(last) > time.Duration(effectSpan)*time.Millisecond {
						re = ori
						last = time.Now()
						break
					}
					showBinary(re)
					re ^= 1 << uint8(i)
					last = time.Now()
				}
				if re == 0 {
					text, _ := clipboard.ReadAll()
					translatedText, _ := translateText(text)
					inputLine.SetText(text)
					outputLine.SetText(outputLine.Text() + "\r\n" + translatedText)
					op = time.Now()
					re = ori
				}
			}
		}
	}()

	window.Run()
}
func handleError(err error) {
	fmt.Println("服务发生错误，Error:", err)
	var s string
	fmt.Scanln(&s)
	os.Exit(-1)
}

// 展示一个数的二进制
func showBinary(num uint8) {
	var s string
	for i := 0; i < 8; i++ {
		if num&1 == 1 {
			s = "1" + s
		} else {
			s = "0" + s
		}
		num >>= 1
	}
	fmt.Println(s)
}
