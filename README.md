# music-lib-web

本地网易云公开歌单下载 UI，基于 `github.com/guohuiyuan/music-lib`。

## 运行

```bash
go run ./cmd/music-lib-web
```

默认访问地址：

```text
http://127.0.0.1:51873
```

可选参数：

```bash
go run ./cmd/music-lib-web --addr 127.0.0.1:51991 --download-dir ./Downloads --concurrency 3 --cookie-file ./.music-lib-web-cookie
```

## 说明

- 第一版只支持公开网易云歌单链接或歌单 ID。
- 下载文件默认会保存到 `Downloads/<歌单名>/`，也可以在页面里为单次任务自定义下载根目录。
- 文件名使用 `歌名 - 歌手.ext`，前面不加序号。
- 音质格式可在页面选择 `MP3` 或 `无损`。MP3 会请求标准音质；无损会请求网易云无损资源，通常需要 Cookie 对应账号具备权限。
- 网易云 Cookie 是可选项。填写后会尝试使用账号可用的更高音质下载；Cookie 会保存到本地 `./.music-lib-web-cookie`，后续任务和服务重启后都会复用，不会出现在 API 响应里。
- 下载完成后会抓取歌曲封面和歌词，并写入音频文件元数据中：MP3 使用 ID3 标签，FLAC 使用 Vorbis/Picture 元数据。
- 下载过程中可以点击“停止下载”，后端会取消任务并尽量中断正在进行的下载请求。
- 同名目录会复用，已存在的同名音频文件不会重复下载。
- VIP 或版权受限歌曲可能无法获取下载地址，会记录为失败项。
- 本工具仅用于学习和技术研究。请遵守法律法规，不要商用。
