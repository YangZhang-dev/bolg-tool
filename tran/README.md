翻译软件

打开软件后，使用快捷键即可调用软件的翻译程序，在一个小窗口内显示翻译内容



## 编译
1. go  get github.com/lxn/walk
2. go get github.com/akavel/rsrc
3. 在github.com/akavel/rsrc下编译生成exe放到目录下
4. 写一个manifest文件
5. /rsrc -manifest .\tran.exe.manifest -o tran.syso
6. 直接运行会有dos窗口，使用 go build -ldflags="-H windowsgui" -o train.exe

# 使用 
- 粘贴后使用Shift+Space
- 全局热键：Ctrl+Q