# wanx-compatible
## 项目背景
阿里通义万象文生图模型wanx在官方文档中只允许通过阿里云DashScope接口进行异步调用，这对接入部分公开的大模型应用项目造成了困难。本项目的目的是通过中转代理，允许使用OpenAI兼容的接口格式，通过聊天(Chat接口)的方式进行调用。同时进行了伪同步处理，方便不方便异步调用的使用场景。

## 使用方法
### 直接使用
1. 下载Releases中的程序
2. 给予运行权限 chmod +x ./wanx-compatible
3. 带变量运行程序
   
   `./wanx-compatible -port=8080 -openai-key=set_your_key -dashscope-key=your_dashscope_key`
   
   port:运行端口
   
   openai-key:请求本接口的密钥，例如:sk-67ta7gx9ta9dxgagx87a0gsb，设定一个复杂点的避免被爆破
   
   dashscope-key:调用通义万象接口的密钥，阿里云百炼服务生成一个
5. 可以将程序注册成系统服务后台运行，使用systemctl管理，方法自行搜索
### 编译使用
略

## 接口详情
### 简易模式
简易模式下可直接通过常规聊天的方式调用
```
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your_openai_key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "wanx2.1-t2i-turbo",
    "messages": [
      {"role": "user", "content": "雪地教堂"}
    ]
  }'
```
### 高级模式
高级模式下支持自定义参数调用，允许自定义反向提示词、生成的图像分辨率和生成图片数量。
调用方法依旧通过聊天的方式调用，但需遵循以下格式：

`[提示词=一个红衣服长发女孩][反向提示词=金色头发][图像分辨率=1024* 1024][图片数量=4]`

同时需要在预设中设置system角色的提示词内容为“通义万象高级模式”。
```
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your_openai_key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "wanx2.1-t2i-turbo",
    "messages": [
      {"role": "system", "content": "通义万象高级模式"},
      {"role": "user", "content": "[提示词=一个红衣服长发女孩][反向提示词=金色头发][图像分辨率=1024* 1024][图片数量=4]"}
    ]
  }'
```

## To DO
- [x] OpenAI Chat接口调用
- [ ] OpenAI Image接口调用

## 开发与贡献
* 欢迎提出改进和建议，有问题请提交Issue

## 许可
MIT License.
