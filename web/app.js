const form = document.querySelector("#playlistForm");
const input = document.querySelector("#playlistInput");
const downloadDirInput = document.querySelector("#downloadDirInput");
const cookieInput = document.querySelector("#cookieInput");
const notice = document.querySelector("#notice");
const configText = document.querySelector("#configText");
const disclaimer = document.querySelector("#disclaimer");
const playlistPanel = document.querySelector("#playlistPanel");
const playlistCover = document.querySelector("#playlistCover");
const playlistName = document.querySelector("#playlistName");
const playlistMeta = document.querySelector("#playlistMeta");
const songRows = document.querySelector("#songRows");
const downloadButton = document.querySelector("#downloadButton");
const jobPanel = document.querySelector("#jobPanel");
const jobTitle = document.querySelector("#jobTitle");
const jobStatus = document.querySelector("#jobStatus");
const jobCounts = document.querySelector("#jobCounts");
const currentSong = document.querySelector("#currentSong");
const progressBar = document.querySelector("#progressBar");
const resultList = document.querySelector("#resultList");
const retryButton = document.querySelector("#retryButton");

let activePlaylistLink = "";
let activeJobID = "";
let pollTimer = 0;

async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || `请求失败：${response.status}`);
  }
  return data;
}

function showNotice(message, isError = false) {
  notice.textContent = message;
  notice.classList.toggle("error", isError);
}

function escapeText(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
  })[char]);
}

async function loadConfig() {
  try {
    const cfg = await api("/api/config");
    configText.textContent = `${cfg.addr} · ${cfg.download_dir} · 并发 ${cfg.concurrency}`;
    downloadDirInput.value = cfg.download_dir || "./download";
    disclaimer.textContent = cfg.disclaimer;
  } catch (error) {
    configText.textContent = "配置读取失败";
  }
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const link = input.value.trim();
  if (!link) {
    showNotice("请输入网易云歌单链接或 ID。", true);
    return;
  }
  activePlaylistLink = link;
  downloadButton.disabled = true;
  showNotice("正在解析歌单...");
  try {
    const data = await api("/api/playlists/parse", {
      method: "POST",
      body: JSON.stringify({ link, cookie: cookieInput.value.trim() }),
    });
    renderPlaylist(data.playlist, data.songs || []);
    showNotice("解析完成。");
    downloadButton.disabled = false;
  } catch (error) {
    playlistPanel.classList.add("hidden");
    showNotice(error.message, true);
  }
});

downloadButton.addEventListener("click", async () => {
  if (!activePlaylistLink) return;
  downloadButton.disabled = true;
  showNotice("正在创建下载任务...");
  try {
    const job = await api("/api/jobs", {
      method: "POST",
      body: JSON.stringify({
        playlist_link: activePlaylistLink,
        download_dir: downloadDirInput.value.trim(),
        cookie: cookieInput.value.trim(),
      }),
    });
    activeJobID = job.id;
    jobPanel.classList.remove("hidden");
    renderJob(job);
    startPolling();
  } catch (error) {
    showNotice(error.message, true);
    downloadButton.disabled = false;
  }
});

retryButton.addEventListener("click", async () => {
  if (!activeJobID) return;
  retryButton.disabled = true;
  showNotice("正在重试失败项...");
  try {
    const job = await api(`/api/jobs/${activeJobID}/retry`, { method: "POST" });
    renderJob(job);
    startPolling();
  } catch (error) {
    showNotice(error.message, true);
  }
});

function renderPlaylist(playlist, songs) {
  playlistPanel.classList.remove("hidden");
  playlistName.textContent = playlist?.name || "未命名歌单";
  playlistMeta.textContent = `${playlist?.creator || "未知创建者"} · ${songs.length} 首 · 播放 ${playlist?.play_count || 0}`;
  playlistCover.src = playlist?.cover || "";
  playlistCover.alt = playlist?.name || "歌单封面";
  songRows.innerHTML = songs.map((song, index) => `
    <tr>
      <td>${index + 1}</td>
      <td>${escapeText(song.name)}</td>
      <td>${escapeText(song.artist)}</td>
      <td>${escapeText(song.album)}</td>
    </tr>
  `).join("");
}

function startPolling() {
  window.clearInterval(pollTimer);
  pollTimer = window.setInterval(fetchJob, 1200);
  fetchJob();
}

async function fetchJob() {
  if (!activeJobID) return;
  try {
    const job = await api(`/api/jobs/${activeJobID}`);
    renderJob(job);
    if (["completed", "completed_with_errors", "failed"].includes(job.status)) {
      window.clearInterval(pollTimer);
      downloadButton.disabled = false;
    }
  } catch (error) {
    showNotice(error.message, true);
    window.clearInterval(pollTimer);
  }
}

function renderJob(job) {
  const done = (job.success_count || 0) + (job.failure_count || 0);
  const total = job.total || 0;
  const percent = total > 0 ? Math.round((done / total) * 100) : 0;
  jobTitle.textContent = job.playlist?.name || "下载任务";
  jobStatus.textContent = job.status;
  jobCounts.textContent = `${done} / ${total} · 成功 ${job.success_count || 0} · 失败 ${job.failure_count || 0}`;
  currentSong.textContent = job.current_song ? `当前：${job.current_song}` : "";
  progressBar.style.width = `${percent}%`;
  retryButton.disabled = (job.failure_count || 0) === 0 || job.status === "running";
  resultList.innerHTML = (job.results || []).map((result) => `
    <div class="result">
      <strong class="status-${escapeText(result.status)}">${statusLabel(result.status)}</strong>
      <div>
        <div>${escapeText(result.name)} - ${escapeText(result.artist)}</div>
        <small>${escapeText(result.error || result.file_path || "")}</small>
      </div>
    </div>
  `).join("");
}

function statusLabel(status) {
  return {
    queued: "等待",
    running: "下载中",
    success: "完成",
    failed: "失败",
    skipped: "跳过",
  }[status] || status;
}

loadConfig();
