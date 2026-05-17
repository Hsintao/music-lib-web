# music-lib-web

`music-lib-web` 是一个本地运行的网易云公开歌单下载工具，基于 [`github.com/guohuiyuan/music-lib`](https://github.com/guohuiyuan/music-lib) 封装了一个轻量 Web UI。

它适合在本机使用：输入公开网易云歌单链接或歌单 ID，预览曲目，然后下载到本地目录。项目不提供登录页、不做扫码登录，也不适合作为公开 Web 服务部署。

## 功能

- 支持网易云公开歌单链接和纯歌单 ID。
- 默认监听 `127.0.0.1:51873`，避免占用常见开发端口。
- 默认下载目录为 `./Downloads`，页面中可为单次任务自定义下载根目录。
- 每个歌单保存到下载根目录下的同名文件夹，例如 `Downloads/<歌单名>/`。
- 歌曲文件名为 `歌曲名 - 歌手.ext`，不会在文件名前追加序号。
- 支持 MP3 和无损格式选择。
- 可填写网易云 Cookie 获取账号可用的更高音质资源。
- Cookie 会保存到本地文件，后续任务和服务重启后自动复用。
- 下载失败时会自动搜索后端可用音源并换源下载。
- 任务结果会显示实际使用的音源，例如 `netease`、`qq`、`kuwo`、`migu`。
- 下载完成后会把封面和歌词写入音频文件元数据。
- 支持停止下载、失败项重试。
- 支持后台下载和多个任务并行。
- 页面刷新后会自动恢复当前服务内的任务列表。
- 支持白天主题和暗黑主题切换。

## 环境要求

- Go 1.22 或更新版本。
- 能访问 `github.com/guohuiyuan/music-lib` 以及相关音乐源接口的网络环境。

## 快速开始

```bash
go run ./cmd/music-lib-web
```

启动后访问：

```text
http://127.0.0.1:51873
```

构建二进制：

```bash
go build -o music-lib-web ./cmd/music-lib-web
./music-lib-web
```

## Docker 部署

项目已发布 Docker Hub 镜像：

```text
xintao0/music-lib-web:latest
```

镜像支持：

```text
linux/amd64
linux/arm64
```

### Docker Compose

如果是在本仓库目录中运行，可以直接使用自带的 `docker-compose.yml`：

```bash
docker compose up -d --build
```

启动后访问：

```text
http://127.0.0.1:51873
```

Compose 默认会把容器内 `/data` 挂载到项目目录下的 `./docker-data`：

```text
docker-data/
  Downloads/                 下载文件
  .music-lib-web-cookie      持久化 Cookie
```

停止服务：

```bash
docker compose down
```

查看日志：

```bash
docker compose logs -f
```

如果不想本地构建，而是直接使用 Docker Hub 镜像，可以新建一个 `compose.yml`：

默认下载路径为容器里的`/data/Downloads`
```yaml
services:
  music-lib-web:
    image: xintao0/music-lib-web:latest
    container_name: music-lib-web
    restart: unless-stopped
    ports:
      - "51873:51873"
    volumes:
      - ./docker-data:/data
      # -./你的路径:/data/Downloads
```

然后启动：

```bash
docker compose up -d
```

### Docker Run

不使用 Compose 时，可以直接运行 Docker Hub 镜像：

```bash
docker run -d \
  --name music-lib-web \
  --restart unless-stopped \
  -p 51873:51873 \
  -v "$PWD/docker-data:/data" \
  xintao0/music-lib-web:latest
```

停止并删除容器：

```bash
docker stop music-lib-web
docker rm music-lib-web
```

也可以本地构建镜像后运行：

```bash
docker build -t music-lib-web:local .
docker run --rm \
  -p 51873:51873 \
  -v "$PWD/docker-data:/data" \
  music-lib-web:local
```

容器内默认参数：

```text
--addr 0.0.0.0:51873
--download-dir /data/Downloads
--cookie-file /data/.music-lib-web-cookie
--concurrency 3
```

## 启动参数

```bash
go run ./cmd/music-lib-web \
  --addr 127.0.0.1:51873 \
  --download-dir ./Downloads \
  --concurrency 3 \
  --cookie-file ./.music-lib-web-cookie
```

参数说明：

- `--addr`：HTTP 监听地址，默认 `127.0.0.1:51873`。
- `--download-dir`：默认下载根目录，默认 `./Downloads`。
- `--concurrency`：下载并发数，默认 `3`。
- `--cookie-file`：网易云 Cookie 持久化文件，默认 `./.music-lib-web-cookie`。

## 使用方式

1. 启动服务并打开页面。
2. 输入网易云公开歌单链接，或直接输入歌单 ID。
3. 可选：填写下载路径、Cookie，并选择 MP3 或无损。
4. 点击“解析歌单”查看歌单名称、封面和曲目。
5. 点击“开始下载”创建任务。
6. 在任务区域查看进度、当前歌曲、成功项、失败项和实际使用音源。
7. 如有失败歌曲，可点击“重试失败项”。

下载任务创建后会在后端继续运行。可以继续解析并创建新的歌单任务；多个任务会并行执行。刷新页面后，前端会重新从服务端读取任务列表。

## 下载规则

- 歌单目录名会做文件系统安全清洗，只替换非法字符。
- 如果同名歌单目录已经存在，会复用该目录。
- 如果同名音频文件已经存在，会跳过重复下载。
- MP3 会请求标准音质。
- 无损会请求 FLAC 等无损资源，通常需要 Cookie 对应账号具备权限。
- 网易云主源获取 URL 失败时，会自动尝试其他可用音源。
- 网易云主源拿到 URL 但实际传输失败时，也会尝试换源重试。
- 全部音源都失败时，该歌曲会进入失败列表，不会中断整个任务。

当前自动换源会尝试的后端源包括：

```text
qq, kugou, kuwo, migu, qianqian, soda, jamendo, joox, bilibili, fivesing
```

## Cookie

Cookie 是可选的。公开歌单解析和部分普通音质下载通常可以不填 Cookie。

填写 Cookie 后：

- 后端会优先使用该 Cookie 解析歌单和获取下载 URL。
- Cookie 会写入 `--cookie-file` 指定的本地文件。
- 后续任务即使页面不再填写 Cookie，也会复用已保存的 Cookie。
- 服务重启后会重新读取该 Cookie 文件。
- API 响应不会返回 Cookie 内容。

Cookie 文件会以 `0600` 权限写入。仍建议不要把 Cookie 文件提交到 Git 仓库。

## 元数据写入

下载完成后，服务会尝试抓取并写入：

- 歌曲封面
- 歌词
- 标题
- 歌手
- 专辑
- 曲目序号

MP3 使用 ID3 标签；FLAC 使用 Vorbis/Picture 元数据。元数据写入失败不会让下载任务失败。

## API

本项目主要面向页面使用，也提供本地 REST API：

- `GET /api/config`：读取本地配置、下载目录、并发数和 Cookie 状态。
- `POST /api/playlists/parse`：解析歌单。
- `POST /api/jobs`：创建下载任务。
- `GET /api/jobs`：查询当前服务内的所有任务。
- `GET /api/jobs/{id}`：查询任务进度。
- `POST /api/jobs/{id}/retry`：重试失败歌曲。
- `POST /api/jobs/{id}/cancel`：停止下载任务。

示例：

```bash
curl -X POST http://127.0.0.1:51873/api/jobs \
  -H 'Content-Type: application/json' \
  -d '{"playlist_link":"https://music.163.com/#/playlist?id=123456","quality":"mp3"}'
```

任务状态：

```text
queued, running, completed, completed_with_errors, failed, canceled
```

单曲状态：

```text
queued, running, success, failed, skipped
```

## 本地长期运行

普通开发运行用 `go run ./cmd/music-lib-web` 即可。

如果希望在 macOS 当前用户会话中托管运行，可以使用：

```bash
go build -o /tmp/music-lib-web-server ./cmd/music-lib-web
launchctl submit -l local.music-lib-web -- /bin/zsh -lc 'cd /path/to/music-lib-web && exec /tmp/music-lib-web-server --addr 127.0.0.1:51873 >> /tmp/music-lib-web.log 2>&1'
```

停止：

```bash
launchctl remove local.music-lib-web
```

如果浏览器访问本地地址失败，请检查代理软件是否拦截了 `127.0.0.1` 或 `localhost`，并把它们设置为直连。

## 开发

运行测试：

```bash
go test ./...
```

构建检查：

```bash
go build ./cmd/music-lib-web
```

项目结构：

```text
cmd/music-lib-web/      启动入口
internal/config/        启动参数和默认配置
internal/server/        HTTP API
internal/jobs/          下载任务状态和并发控制
internal/netease/       歌单解析、下载、换源和元数据抓取
internal/metadata/      MP3/FLAC 元数据写入
web/                    HTML/CSS/JS 前端
Downloads/              默认下载目录
```

## 许可证与声明

本项目依赖的上游核心库 `github.com/guohuiyuan/music-lib` 使用 AGPL-3.0 许可证。请遵守上游项目许可证要求。

本工具仅用于学习和技术研究。请遵守法律法规，不要商用；下载的资源请按上游项目提示及时删除。使用 Cookie 时请妥善保管账号凭据，不要把 Cookie 分享给他人或提交到公开仓库。
