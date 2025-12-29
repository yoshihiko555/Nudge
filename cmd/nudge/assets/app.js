const appEl = document.getElementById('app');
const statusChip = document.getElementById('statusChip');
const taskList = document.getElementById('taskList');
const taskEmpty = document.getElementById('taskEmpty');
const lastUpdated = document.getElementById('lastUpdated');
const errorText = document.getElementById('errorText');

const tokenInput = document.getElementById('tokenInput');
const tokenHint = document.getElementById('tokenHint');
const databaseIdInput = document.getElementById('databaseIdInput');
const dataSourceIdInput = document.getElementById('dataSourceIdInput');
const titlePropertyInput = document.getElementById('titlePropertyInput');
const statusPropertyInput = document.getElementById('statusPropertyInput');
const statusTypeSelect = document.getElementById('statusTypeSelect');
const statusInProgressInput = document.getElementById('statusInProgressInput');
const statusDoneInput = document.getElementById('statusDoneInput');
const statusPausedInput = document.getElementById('statusPausedInput');
const notionVersionInput = document.getElementById('notionVersionInput');

const refreshBtn = document.getElementById('refreshBtn');
const saveConfigBtn = document.getElementById('saveConfigBtn');
const saveTokenBtn = document.getElementById('saveTokenBtn');
const clearTokenBtn = document.getElementById('clearTokenBtn');
const resolveDataSourceBtn = document.getElementById('resolveDataSourceBtn');
const resolveTitleBtn = document.getElementById('resolveTitleBtn');

let state = {
  view: 'inprogress',
  config: null,
  tokenSet: false,
  pollTimer: null,
};

const wails = window.wails;

function runtimeReady() {
  return wails && wails.System && wails.Events && wails.Events.On;
}

function setRuntimeMissing() {
  appEl.innerHTML = `
    <div class="runtime-missing">
      <div>
        <p>Wails runtime が見つかりません。</p>
        <p>先に <strong>wails3 generate runtime</strong> を実行してください。</p>
      </div>
    </div>
  `;
}

let rpcSeq = 0;
function rpc(action, payload = {}) {
  return new Promise((resolve, reject) => {
    const id = `${Date.now()}-${rpcSeq++}`;
    const off = wails.Events.On('rpc:response', (event) => {
      const res = event?.data;
      if (!res || res.id !== id) {
        return;
      }
      if (typeof off === 'function') {
        off();
      }
      if (res.ok) {
        resolve(res.data);
      } else {
        reject(new Error(res.error || 'unknown error'));
      }
    });
    wails.System.invoke(JSON.stringify({ id, action, payload }));
  });
}

function setView(view) {
  state.view = view;
  appEl.dataset.view = view;
  document.querySelectorAll('.tab').forEach((tab) => {
    tab.classList.toggle('is-active', tab.dataset.view === view);
  });
}

function setError(message) {
  errorText.textContent = message || '';
}

function setStatusChip() {
  if (state.tokenSet) {
    statusChip.textContent = '接続準備OK';
    statusChip.classList.add('is-connected');
  } else {
    statusChip.textContent = '未接続';
    statusChip.classList.remove('is-connected');
  }
}

function formatTime(value) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return `${date.getHours().toString().padStart(2, '0')}:${date
    .getMinutes()
    .toString()
    .padStart(2, '0')}`;
}

function renderTasks(listEl, emptyEl, tasks, view) {
  listEl.innerHTML = '';
  if (!tasks || tasks.length === 0) {
    emptyEl.style.display = 'block';
    return;
  }
  emptyEl.style.display = 'none';
  tasks.forEach((task) => {
    const card = document.createElement('div');
    card.className = 'task-card';

    const title = document.createElement('div');
    title.className = 'task-title';
    title.textContent = task.title || '(無題)';

    const meta = document.createElement('div');
    meta.className = 'task-meta';
    meta.innerHTML = `<span>更新 ${formatTime(task.last_edited_time)}</span>`;

    const actions = document.createElement('div');
    actions.className = 'task-actions';

    const openBtn = document.createElement('button');
    openBtn.className = 'btn ghost';
    openBtn.textContent = 'Notionで開く';
    openBtn.addEventListener('click', () => openURL(task.url));

    actions.appendChild(openBtn);

    if (view === 'inprogress') {
      const doneBtn = document.createElement('button');
      doneBtn.className = 'btn';
      doneBtn.textContent = '完了';
      doneBtn.addEventListener('click', () => updateStatus(task.id, 'done'));

      const pauseBtn = document.createElement('button');
      pauseBtn.className = 'btn ghost';
      pauseBtn.textContent = '中断';
      pauseBtn.addEventListener('click', () => updateStatus(task.id, 'paused'));

      actions.appendChild(doneBtn);
      actions.appendChild(pauseBtn);
    }

    card.appendChild(title);
    card.appendChild(meta);
    card.appendChild(actions);
    listEl.appendChild(card);
  });
}

async function refreshTasks() {
  try {
    setError('');
    const tasks = await rpc('getTasks', { status: 'inprogress' });
    renderTasks(taskList, taskEmpty, tasks, 'inprogress');
    lastUpdated.textContent = `更新 ${formatTime(new Date().toISOString())}`;
  } catch (err) {
    setError(err.message);
  }
}

async function updateStatus(taskID, action) {
  try {
    setError('');
    await rpc('updateStatus', { task_id: taskID, action });
    await refreshTasks();
  } catch (err) {
    setError(err.message);
  }
}

async function openURL(url) {
  try {
    setError('');
    await rpc('openURL', { url });
  } catch (err) {
    setError(err.message);
  }
}

async function loadConfig() {
  const cfg = await rpc('getConfig');
  state.config = cfg;
  databaseIdInput.value = cfg.database_id || '';
  dataSourceIdInput.value = cfg.data_source_id || '';
  titlePropertyInput.value = cfg.title_property_name || '';
  statusPropertyInput.value = cfg.status_property_name || '';
  statusTypeSelect.value = cfg.status_property_type || 'status';
  statusInProgressInput.value = cfg.status_in_progress || '';
  statusDoneInput.value = cfg.status_done || '';
  statusPausedInput.value = cfg.status_paused || '';
  notionVersionInput.value = cfg.notion_version || '';
}

async function saveConfig() {
  const cfg = {
    ...state.config,
    database_id: databaseIdInput.value.trim(),
    data_source_id: dataSourceIdInput.value.trim(),
    title_property_name: titlePropertyInput.value.trim(),
    status_property_name: statusPropertyInput.value.trim(),
    status_property_type: statusTypeSelect.value,
    status_in_progress: statusInProgressInput.value.trim(),
    status_done: statusDoneInput.value.trim(),
    status_paused: statusPausedInput.value.trim(),
    notion_version: notionVersionInput.value.trim(),
  };
  await rpc('saveConfig', cfg);
  state.config = cfg;
  setError('');
}

async function refreshTokenStatus() {
  const tokenSet = await rpc('getTokenStatus');
  state.tokenSet = Boolean(tokenSet);
  tokenHint.textContent = tokenSet ? '保存済み' : '未保存';
  // 保存済みの場合は伏せ字を表示
  if (tokenSet) {
    tokenInput.value = '●●●●●●●●●●●●';
    tokenInput.disabled = true;
  } else {
    tokenInput.value = '';
    tokenInput.disabled = false;
  }
  setStatusChip();
}

async function saveToken() {
  const token = tokenInput.value.trim();
  if (!token || token === '●●●●●●●●●●●●') {
    setError('トークンが空です');
    return;
  }
  await rpc('setToken', { token });
  await refreshTokenStatus();
}

async function clearToken() {
  await rpc('clearToken');
  await refreshTokenStatus();
}

async function resolveDataSource() {
  const databaseID = databaseIdInput.value.trim();
  if (!databaseID) {
    setError('Database ID を入力してください');
    return;
  }
  const id = await rpc('resolveDataSourceID', { database_id: databaseID });
  dataSourceIdInput.value = id || '';
}

async function resolveTitleProperty() {
  const databaseID = databaseIdInput.value.trim();
  if (!databaseID) {
    setError('Database ID を入力してください');
    return;
  }
  const name = await rpc('resolveTitlePropertyName', { database_id: databaseID });
  titlePropertyInput.value = name || '';
}

function startPolling() {
  if (state.pollTimer) {
    clearInterval(state.pollTimer);
  }
  const interval = (state.config?.poll_interval_seconds || 60) * 1000;
  state.pollTimer = setInterval(() => {
    refreshTasks();
  }, interval);
}

function bindUI() {
  document.querySelectorAll('.tab').forEach((tab) => {
    tab.addEventListener('click', async () => {
      const view = tab.dataset.view;
      setView(view);
      if (view === 'inprogress') {
        await refreshTasks();
      }
    });
  });

  refreshBtn.addEventListener('click', () => refreshTasks());
  saveConfigBtn.addEventListener('click', saveConfig);
  saveTokenBtn.addEventListener('click', saveToken);
  clearTokenBtn.addEventListener('click', clearToken);
  resolveDataSourceBtn.addEventListener('click', resolveDataSource);
  resolveTitleBtn.addEventListener('click', resolveTitleProperty);

  wails.Events.On('view-change', (event) => {
    const view = event?.data;
    if (view) {
      setView(view);
      if (view === 'inprogress') {
        refreshTasks();
      }
    }
  });

  wails.Events.On('refresh', () => {
    refreshTasks();
  });
}

async function init() {
  if (!runtimeReady()) {
    setRuntimeMissing();
    return;
  }

  await loadConfig();
  await refreshTokenStatus();
  bindUI();
  await refreshTasks('inprogress');
  startPolling();
}

init();
