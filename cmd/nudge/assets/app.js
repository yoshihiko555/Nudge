const appEl = document.getElementById('app');
const statusChip = document.getElementById('statusChip');
const lastUpdated = document.getElementById('lastUpdated');
const errorText = document.getElementById('errorText');

const tokenInput = document.getElementById('tokenInput');
const tokenHint = document.getElementById('tokenHint');
const launchAtLoginInput = document.getElementById('launchAtLoginInput');
const notionVersionInput = document.getElementById('notionVersionInput');

const tabNav = document.getElementById('tabNav');
const paneContainer = document.getElementById('paneContainer');
const databaseList = document.getElementById('databaseList');
const addDatabaseBtn = document.getElementById('addDatabaseBtn');
const databaseCardTemplate = document.getElementById('databaseCardTemplate');

const saveConfigBtn = document.getElementById('saveConfigBtn');
const saveTokenBtn = document.getElementById('saveTokenBtn');
const clearTokenBtn = document.getElementById('clearTokenBtn');

let state = {
  view: 'settings',
  config: null,
  tokenSet: false,
  pollTimer: null,
  paneMap: new Map(),
  dbMap: new Map(),
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
  const nextView = resolveView(view);
  state.view = nextView;
  appEl.dataset.view = nextView;
  tabNav.querySelectorAll('.tab').forEach((tab) => {
    tab.classList.toggle('is-active', tab.dataset.view === nextView);
  });
  paneContainer.querySelectorAll('.pane').forEach((pane) => {
    pane.classList.toggle('is-active', pane.dataset.pane === nextView);
  });
}

function resolveView(view) {
  if (!view) {
    return 'settings';
  }
  if (view === 'settings') {
    return view;
  }
  if (state.dbMap.has(view)) {
    return view;
  }
  return 'settings';
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

function defaultDatabaseName(kind) {
  return kind === 'habit' ? '習慣' : 'タスク';
}

const defaultHabitDays = '日,月,火,水,木,金,土';

function generateKey() {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return `db-${crypto.randomUUID().slice(0, 8)}`;
  }
  return `db-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
}

function pickDefaultView() {
  const enabled = (state.config?.databases || []).filter((db) => db.enabled);
  const task = enabled.find((db) => db.kind === 'task');
  return task?.key || enabled[0]?.key || 'settings';
}

function renderTabsAndPanes() {
  const databases = (state.config?.databases || []).filter((db) => db.enabled);
  state.dbMap = new Map(databases.map((db) => [db.key, db]));
  state.paneMap = new Map();

  tabNav.innerHTML = '';
  databases.forEach((db) => {
    const tab = document.createElement('button');
    tab.className = 'tab';
    tab.dataset.view = db.key;
    tab.textContent = db.name || defaultDatabaseName(db.kind);
    tabNav.appendChild(tab);
  });

  const settingsTab = document.createElement('button');
  settingsTab.className = 'tab';
  settingsTab.dataset.view = 'settings';
  settingsTab.textContent = '設定';
  tabNav.appendChild(settingsTab);

  const settingsPane = paneContainer.querySelector('.pane[data-pane="settings"]');
  paneContainer
    .querySelectorAll('.pane[data-pane]:not([data-pane="settings"])')
    .forEach((pane) => pane.remove());

  databases.forEach((db) => {
    const pane = document.createElement('section');
    pane.className = 'pane';
    pane.dataset.pane = db.key;

    const header = document.createElement('div');
    header.className = 'pane-header';

    const headerText = document.createElement('div');
    const title = document.createElement('h2');
    title.textContent = db.name || defaultDatabaseName(db.kind);
    const hint = document.createElement('p');
    hint.className = 'hint';
    hint.textContent =
      db.kind === 'habit' ? '今日の習慣のみ表示' : '最新の更新から 60 秒おきに自動更新';
    headerText.appendChild(title);
    headerText.appendChild(hint);

    const refreshBtn = document.createElement('button');
    refreshBtn.className = 'btn ghost';
    refreshBtn.textContent = '更新';
    refreshBtn.addEventListener('click', () => refreshDatabaseView(db.key));

    header.appendChild(headerText);
    header.appendChild(refreshBtn);

    const list = document.createElement('div');
    list.className = 'task-list';
    const empty = document.createElement('div');
    empty.className = 'empty-state';
    empty.textContent = db.kind === 'habit' ? '今日の習慣がありません' : 'タスクがありません';

    pane.appendChild(header);
    pane.appendChild(list);
    pane.appendChild(empty);

    paneContainer.insertBefore(pane, settingsPane);
    state.paneMap.set(db.key, { listEl: list, emptyEl: empty, kind: db.kind });
  });
}

function renderDatabaseSettings(databases) {
  databaseList.innerHTML = '';
  (databases || []).forEach((db) => {
    const card = createDatabaseCard(db);
    databaseList.appendChild(card);
  });
}

function applyDatabaseKind(card, kind) {
  card.querySelectorAll('.db-fields').forEach((section) => {
    section.hidden = section.dataset.kind !== kind;
  });
  const checkboxInput = card.querySelector('.db-checkbox-property');
  if (checkboxInput) {
    checkboxInput.readOnly = kind === 'habit';
  }
}

function createDatabaseCard(db) {
  const card = databaseCardTemplate.content.firstElementChild.cloneNode(true);
  const key = db.key || generateKey();
  card.dataset.key = key;

  const nameInput = card.querySelector('.db-name-input');
  const keyLabel = card.querySelector('.db-key');
  const kindSelect = card.querySelector('.db-kind-select');
  const enabledToggle = card.querySelector('.db-enabled-toggle');

  nameInput.value = db.name || '';
  keyLabel.textContent = `#${key}`;
  kindSelect.value = db.kind || 'task';
  enabledToggle.checked = Boolean(db.enabled);

  card.querySelector('.db-database-id').value = db.database_id || '';
  card.querySelector('.db-data-source-id').value = db.data_source_id || '';
  card.querySelector('.db-title-property').value = db.title_property_name || '';
  card.querySelector('.db-status-property').value = db.status_property_name || '';
  card.querySelector('.db-status-type').value = db.status_property_type || 'status';
  card.querySelector('.db-status-in-progress').value = db.status_in_progress || '';
  card.querySelector('.db-status-done').value = db.status_done || '';
  card.querySelector('.db-status-paused').value = db.status_paused || '';
  card.querySelector('.db-checkbox-property').value = db.checkbox_property_name || defaultHabitDays;

  applyDatabaseKind(card, kindSelect.value);

  kindSelect.addEventListener('change', () => {
    applyDatabaseKind(card, kindSelect.value);
    if (!nameInput.value.trim()) {
      nameInput.value = defaultDatabaseName(kindSelect.value);
    }
    if (kindSelect.value === 'habit') {
      const titleInput = card.querySelector('.db-title-property');
      const checkboxInput = card.querySelector('.db-checkbox-property');
      if (titleInput && !titleInput.value.trim()) {
        titleInput.value = '名前';
      }
      if (checkboxInput && !checkboxInput.value.trim()) {
        checkboxInput.value = defaultHabitDays;
      }
      if (checkboxInput) {
        checkboxInput.readOnly = true;
      }
    }
  });

  card
    .querySelector('.db-resolve-data-source')
    .addEventListener('click', () => resolveDataSource(card));
  card
    .querySelector('.db-resolve-title')
    .addEventListener('click', () => resolveTitleProperty(card));
  card.querySelector('.db-delete-btn').addEventListener('click', () => {
    if (!confirm('このデータベース設定を削除しますか？')) {
      return;
    }
    card.remove();
  });

  return card;
}

function collectDatabases() {
  const cards = databaseList.querySelectorAll('.database-card');
  return Array.from(cards).map((card) => {
    const kind = card.querySelector('.db-kind-select').value;
    const name = card.querySelector('.db-name-input').value.trim() || defaultDatabaseName(kind);
    const key = card.dataset.key || generateKey();
    card.dataset.key = key;
    card.querySelector('.db-key').textContent = `#${key}`;

    return {
      key,
      name,
      kind,
      enabled: card.querySelector('.db-enabled-toggle').checked,
      database_id: card.querySelector('.db-database-id').value.trim(),
      data_source_id: card.querySelector('.db-data-source-id').value.trim(),
      title_property_name: card.querySelector('.db-title-property').value.trim(),
      status_property_name: card.querySelector('.db-status-property').value.trim(),
      status_property_type: card.querySelector('.db-status-type').value,
      status_in_progress: card.querySelector('.db-status-in-progress').value.trim(),
      status_done: card.querySelector('.db-status-done').value.trim(),
      status_paused: card.querySelector('.db-status-paused').value.trim(),
      checkbox_property_name:
        card.querySelector('.db-checkbox-property').value.trim() || defaultHabitDays,
    };
  });
}

function renderTasks(listEl, emptyEl, tasks, dbKey) {
  listEl.innerHTML = '';
  if (!tasks || tasks.length === 0) {
    emptyEl.style.display = 'block';
    return;
  }
  emptyEl.style.display = 'none';
  const db = state.dbMap.get(dbKey);
  const canDone = Boolean(db?.status_done);
  const canPause = Boolean(db?.status_paused);

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

    if (canDone) {
      const doneBtn = document.createElement('button');
      doneBtn.className = 'btn';
      doneBtn.textContent = '完了';
      doneBtn.addEventListener('click', () => updateTaskStatus(task.id, 'done', dbKey));
      actions.appendChild(doneBtn);
    }

    if (canPause) {
      const pauseBtn = document.createElement('button');
      pauseBtn.className = 'btn ghost';
      pauseBtn.textContent = '中断';
      pauseBtn.addEventListener('click', () => updateTaskStatus(task.id, 'paused', dbKey));
      actions.appendChild(pauseBtn);
    }

    card.appendChild(title);
    card.appendChild(meta);
    card.appendChild(actions);
    listEl.appendChild(card);
  });
}

function renderHabits(listEl, emptyEl, habits, dbKey) {
  listEl.innerHTML = '';
  if (!habits || habits.length === 0) {
    emptyEl.style.display = 'block';
    return;
  }
  emptyEl.style.display = 'none';

  habits.forEach((habit) => {
    const card = document.createElement('div');
    card.className = 'task-card habit-card';

    const main = document.createElement('div');
    main.className = 'habit-main';

    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    checkbox.addEventListener('change', () => {
      if (!checkbox.checked) {
        checkbox.checked = false;
        return;
      }
      updateHabitCheck(dbKey, habit.id, checkbox);
    });

    const title = document.createElement('div');
    title.className = 'habit-title';
    title.textContent = habit.title || '(未設定)';

    main.appendChild(checkbox);
    main.appendChild(title);

    const actions = document.createElement('div');
    actions.className = 'task-actions';

    const openBtn = document.createElement('button');
    openBtn.className = 'btn ghost';
    openBtn.textContent = 'Notionで開く';
    openBtn.addEventListener('click', () => openURL(habit.url));

    actions.appendChild(openBtn);
    card.appendChild(main);
    card.appendChild(actions);
    listEl.appendChild(card);
  });
}

async function refreshDatabaseView(dbKey) {
  const pane = state.paneMap.get(dbKey);
  const db = state.dbMap.get(dbKey);
  if (!pane || !db) {
    return;
  }
  try {
    setError('');
    if (db.kind === 'habit') {
      const habits = await rpc('getHabits', { database_key: dbKey });
      renderHabits(pane.listEl, pane.emptyEl, habits, dbKey);
    } else {
      const tasks = await rpc('getTasks', { database_key: dbKey });
      renderTasks(pane.listEl, pane.emptyEl, tasks, dbKey);
    }
    lastUpdated.textContent = `更新 ${formatTime(new Date().toISOString())}`;
  } catch (err) {
    setError(err.message);
  }
}

function refreshActiveView() {
  if (!state.view || state.view === 'settings') {
    return;
  }
  refreshDatabaseView(state.view);
}

async function updateTaskStatus(taskID, action, dbKey) {
  try {
    setError('');
    await rpc('updateStatus', { database_key: dbKey, task_id: taskID, action });
    await refreshDatabaseView(dbKey);
  } catch (err) {
    setError(err.message);
  }
}

async function updateHabitCheck(dbKey, taskID, checkbox) {
  try {
    setError('');
    checkbox.disabled = true;
    await rpc('updateHabitCheck', { database_key: dbKey, task_id: taskID, checked: true });
    await refreshDatabaseView(dbKey);
  } catch (err) {
    checkbox.disabled = false;
    checkbox.checked = false;
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
  launchAtLoginInput.checked = Boolean(cfg.launch_at_login);
  notionVersionInput.value = cfg.notion_version || '';
  renderDatabaseSettings(cfg.databases || []);
  renderTabsAndPanes();
}

async function saveConfig() {
  const cfg = {
    ...state.config,
    databases: collectDatabases(),
    launch_at_login: launchAtLoginInput.checked,
    notion_version: notionVersionInput.value.trim(),
  };
  await rpc('saveConfig', cfg);
  state.config = cfg;
  renderTabsAndPanes();
  const nextView = state.dbMap.has(state.view) ? state.view : pickDefaultView();
  setView(nextView);
  setError('');
}

async function refreshTokenStatus() {
  const tokenSet = await rpc('getTokenStatus');
  state.tokenSet = Boolean(tokenSet);
  tokenHint.textContent = tokenSet ? '保存済み' : '未保存';
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

async function resolveDataSource(card) {
  const databaseID = card.querySelector('.db-database-id').value.trim();
  if (!databaseID) {
    setError('Database ID を入力してください');
    return;
  }
  try {
    const id = await rpc('resolveDataSourceID', { database_id: databaseID });
    card.querySelector('.db-data-source-id').value = id || '';
  } catch (err) {
    setError(err.message);
  }
}

async function resolveTitleProperty(card) {
  const databaseID = card.querySelector('.db-database-id').value.trim();
  if (!databaseID) {
    setError('Database ID を入力してください');
    return;
  }
  try {
    const name = await rpc('resolveTitlePropertyName', { database_id: databaseID });
    card.querySelector('.db-title-property').value = name || '';
  } catch (err) {
    setError(err.message);
  }
}

function addDatabase() {
  const card = createDatabaseCard({
    key: generateKey(),
    name: defaultDatabaseName('habit'),
    kind: 'habit',
    enabled: true,
    title_property_name: '名前',
    checkbox_property_name: defaultHabitDays,
    status_property_type: 'status',
  });
  databaseList.appendChild(card);
}

function startPolling() {
  if (state.pollTimer) {
    clearInterval(state.pollTimer);
  }
  const interval = (state.config?.poll_interval_seconds || 60) * 1000;
  state.pollTimer = setInterval(() => {
    refreshActiveView();
  }, interval);
}

function bindUI() {
  tabNav.addEventListener('click', (event) => {
    const tab = event.target.closest('.tab');
    if (!tab) {
      return;
    }
    const view = tab.dataset.view;
    setView(view);
    if (view !== 'settings') {
      refreshDatabaseView(view);
    }
  });

  saveConfigBtn.addEventListener('click', saveConfig);
  saveTokenBtn.addEventListener('click', saveToken);
  clearTokenBtn.addEventListener('click', clearToken);
  addDatabaseBtn.addEventListener('click', addDatabase);

  wails.Events.On('view-change', (event) => {
    const view = event?.data;
    if (view) {
      setView(view);
      if (view !== 'settings') {
        refreshDatabaseView(view);
      }
    }
  });

  wails.Events.On('refresh', () => {
    refreshActiveView();
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
  setView(pickDefaultView());
  refreshActiveView();
  startPolling();
}

init();
