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
go run ./cmd/music-lib-web --addr 127.0.0.1:51991 --download-dir ./download --concurrency 3
```

## 说明

- 第一版只支持公开网易云歌单链接或歌单 ID。
- 下载文件默认会保存到 `download/<歌单名>/`，也可以在页面里为单次任务自定义下载根目录。
- 文件名使用 `歌名 - 歌手.ext`，前面不加序号。
- 网易云 Cookie 是可选项。填写后会尝试使用账号可用的更高音质下载；Cookie 只在本地内存中用于当前解析和下载任务，不会保存到文件或 API 响应。
- 同名目录会复用，已存在的同名音频文件不会重复下载。
- VIP 或版权受限歌曲可能无法获取下载地址，会记录为失败项。
- 本工具仅用于学习和技术研究。请遵守法律法规，不要商用。
