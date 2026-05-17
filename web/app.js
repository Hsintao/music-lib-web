const form = document.querySelector("#playlistForm");
const input = document.querySelector("#playlistInput");
const downloadDirInput = document.querySelector("#downloadDirInput");
const qualitySelect = document.querySelector("#qualitySelect");
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
const jobSummary = document.querySelector("#jobSummary");
const jobList = document.querySelector("#jobList");
const themeToggle = document.querySelector("#themeToggle");

let activePlaylistLink = "";
let pollTimer = 0;
const themeStorageKey = "music-lib-theme";
const resultScrollByJob = new Map();

function readStoredTheme() {
  try {
    return localStorage.getItem(themeStorageKey);
  } catch (_) {
    return "";
  }
}

function writeStoredTheme(theme) {
  try {
    localStorage.setItem(themeStorageKey, theme);
  } catch (_) {
    // Theme persistence is optional; keep the UI usable if storage is blocked.
  }
}

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

function applyTheme(theme) {
  const nextTheme = theme === "dark" ? "dark" : "light";
  document.documentElement.dataset.theme = nextTheme;
  writeStoredTheme(nextTheme);
  if (!themeToggle) return;
  themeToggle.textContent = nextTheme === "dark" ? "白天" : "夜间";
  themeToggle.setAttribute("aria-pressed", String(nextTheme === "dark"));
}

if (themeToggle) {
  themeToggle.addEventListener("click", () => {
    const currentTheme = document.documentElement.dataset.theme === "dark" ? "dark" : "light";
    applyTheme(currentTheme === "dark" ? "light" : "dark");
  });
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
    const cookieState = cfg.has_cookie ? "已记住 Cookie" : "未设置 Cookie";
    configText.textContent = `${cfg.addr} · ${cfg.download_dir} · 并发 ${cfg.concurrency} · ${cookieState}`;
    downloadDirInput.value = cfg.download_dir || "./Downloads";
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
        quality: qualitySelect.value,
        cookie: cookieInput.value.trim(),
      }),
    });
    jobPanel.classList.remove("hidden");
    renderJobs([job]);
    startPolling();
    downloadButton.disabled = false;
    showNotice("下载任务已在后台开始。");
  } catch (error) {
    showNotice(error.message, true);
    downloadButton.disabled = false;
  }
});

jobList.addEventListener("click", async (event) => {
  const button = event.target.closest("button[data-action][data-job-id]");
  if (!button) return;
  const jobID = button.dataset.jobId;
  const action = button.dataset.action;
  button.disabled = true;
  showNotice(action === "retry" ? "正在重试失败项..." : "正在停止下载...");
  try {
    await api(`/api/jobs/${jobID}/${action}`, { method: "POST" });
    await fetchJobs();
    startPolling();
    showNotice(action === "retry" ? "失败项已重新调度。" : "下载已停止。");
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
  pollTimer = window.setInterval(fetchJobs, 1200);
  fetchJobs();
}

async function fetchJobs() {
  try {
    const data = await api("/api/jobs");
    const jobs = data.jobs || [];
    renderJobs(jobs);
    if (jobs.some((job) => isActiveJob(job.status))) {
      if (!pollTimer) {
        pollTimer = window.setInterval(fetchJobs, 1200);
      }
    } else {
      window.clearInterval(pollTimer);
      pollTimer = 0;
      downloadButton.disabled = false;
    }
  } catch (error) {
    showNotice(error.message, true);
    window.clearInterval(pollTimer);
    pollTimer = 0;
  }
}

function renderJobs(jobs) {
  rememberResultScrollPositions();
  if (!jobs.length) {
    jobPanel.classList.add("hidden");
    jobList.innerHTML = "";
    jobSummary.textContent = "暂无任务";
    resultScrollByJob.clear();
    return;
  }
  jobPanel.classList.remove("hidden");
  const active = jobs.filter((job) => isActiveJob(job.status)).length;
  const failed = jobs.filter((job) => (job.failure_count || 0) > 0).length;
  jobSummary.textContent = `${jobs.length} 个任务 · 运行中 ${active} · 有失败 ${failed}`;
  jobList.innerHTML = jobs.map(renderJobCard).join("");
  restoreResultScrollPositions(jobs);
}

function renderJobCard(job) {
  const done = (job.success_count || 0) + (job.failure_count || 0);
  const total = job.total || 0;
  const percent = total > 0 ? Math.round((done / total) * 100) : 0;
  const canCancel = job.status === "running" || job.status === "queued";
  const canRetry = (job.failure_count || 0) > 0 && !isActiveJob(job.status);
  const results = (job.results || []).map((result) => `
    <div class="result">
      <strong class="status-${escapeText(result.status)}">${statusLabel(result.status)}</strong>
      <div>
        <div>${escapeText(result.name)} - ${escapeText(result.artist)}</div>
        <small>${escapeText(result.error || result.file_path || "")}${result.source ? ` · 音源: ${escapeText(result.source)}` : ""}</small>
      </div>
    </div>
  `).join("");
  return `
    <article class="job-card">
      <div class="job-card-head">
        <div>
          <h3>${escapeText(job.playlist?.name || "下载任务")}</h3>
          <div class="stats">
            <span>${statusLabel(job.status)}</span>
            <span>${done} / ${total}</span>
            <span>成功 ${job.success_count || 0}</span>
            <span>失败 ${job.failure_count || 0}</span>
            <span>${escapeText(job.quality || "mp3")}</span>
          </div>
        </div>
        <div class="job-actions">
          <button type="button" data-action="cancel" data-job-id="${escapeText(job.id)}" ${canCancel ? "" : "disabled"}>停止下载</button>
          <button type="button" data-action="retry" data-job-id="${escapeText(job.id)}" ${canRetry ? "" : "disabled"}>重试失败项</button>
        </div>
      </div>
      <div class="progress" aria-label="下载进度 ${percent}%">
        <div class="progress-bar" style="width: ${percent}%"></div>
      </div>
      <div class="stats">
        <span>${job.current_song ? `当前：${escapeText(job.current_song)}` : "后台下载"}</span>
        <span>${escapeText(job.download_dir || "")}</span>
      </div>
      <div class="result-list" data-job-id="${escapeText(job.id)}">${results}</div>
    </article>
  `;
}

function rememberResultScrollPositions() {
  document.querySelectorAll(".result-list[data-job-id]").forEach((list) => {
    resultScrollByJob.set(list.dataset.jobId, list.scrollTop);
  });
}

function restoreResultScrollPositions(jobs) {
  const liveIDs = new Set(jobs.map((job) => String(job.id)));
  for (const id of resultScrollByJob.keys()) {
    if (!liveIDs.has(id)) {
      resultScrollByJob.delete(id);
    }
  }
  document.querySelectorAll(".result-list[data-job-id]").forEach((list) => {
    const scrollTop = resultScrollByJob.get(list.dataset.jobId);
    if (scrollTop !== undefined) {
      list.scrollTop = scrollTop;
    }
  });
}

function isActiveJob(status) {
  return status === "queued" || status === "running";
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

applyTheme(readStoredTheme() || document.documentElement.dataset.theme);
loadConfig();
fetchJobs();
