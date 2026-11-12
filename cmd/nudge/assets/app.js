const appEl = document.getElementById('app');
const statusChip = document.getElementById('statusChip');
const lastUpdated = document.getElementById('lastUpdated');
const errorText = document.getElementById('errorText');
const syncStatusEl = document.getElementById('syncStatus');

const tokenInput = document.getElementById('tokenInput');
const tokenHint = document.getElementById('tokenHint');
const launchAtLoginInput = document.getElementById('launchAtLoginInput');
const tabNav = document.getElementById('tabNav');
const paneContainer = document.getElementById('paneContainer');
const databaseList = document.getElementById('databaseList');
const addDatabaseBtn = document.getElementById('addDatabaseBtn');
const databaseCardTemplate = document.getElementById('databaseCardTemplate');

const saveConfigBtn = document.getElementById('saveConfigBtn');
const saveTokenBtn = document.getElementById('saveTokenBtn');
const clearTokenBtn = document.getElementById('clearTokenBtn');
const openSettingsBtn = document.getElementById('openSettingsBtn');

const wails = window.wails;
const appMode = (() => {
  const mode = new URLSearchParams(window.location.search).get('mode');
  if (mode === 'settings') {
    return 'settings';
  }
  return 'main';
})();
appEl.dataset.mode = appMode;

let state = {
  mode: appMode,
  view: appMode === 'settings' ? 'settings' : null,
  config: null,
  tokenSet: false,
  pollTimer: null,
  paneMap: new Map(),
  dbMap: new Map(),
};

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
  appEl.dataset.view = nextView || '';
  tabNav.querySelectorAll('.tab').forEach((tab) => {
    tab.classList.toggle('is-active', nextView && tab.dataset.view === nextView);
  });
  paneContainer.querySelectorAll('.pane').forEach((pane) => {
    pane.classList.toggle('is-active', nextView && pane.dataset.pane === nextView);
  });
}

function resolveView(view) {
  if (state.mode === 'settings') {
    return 'settings';
  }
  if (view && state.dbMap.has(view)) {
    return view;
  }
  return pickDefaultView();
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

function formatTime(value, withDate = false) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const time = `${date.getHours().toString().padStart(2, '0')}:${date
    .getMinutes()
    .toString()
    .padStart(2, '0')}`;
  if (!withDate) {
    return time;
  }
  const month = (date.getMonth() + 1).toString().padStart(2, '0');
  const day = date.getDate().toString().padStart(2, '0');
  return `${month}/${day} ${time}`;
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

function ensurePropertyLists(card, key) {
  const titleInput = card.querySelector('.db-title-property');
  const statusInput = card.querySelector('.db-status-property');
  const checkboxInput = card.querySelector('.db-checkbox-property');

  const titleListId = `db-title-properties-${key}`;
  const statusListId = `db-status-properties-${key}`;
  const checkboxListId = `db-checkbox-properties-${key}`;

  if (titleInput) {
    titleInput.setAttribute('list', titleListId);
    const list = document.createElement('datalist');
    list.id = titleListId;
    card.appendChild(list);
  }
  if (statusInput) {
    statusInput.setAttribute('list', statusListId);
    const list = document.createElement('datalist');
    list.id = statusListId;
    card.appendChild(list);
  }
  if (checkboxInput) {
    checkboxInput.setAttribute('list', checkboxListId);
    const list = document.createElement('datalist');
    list.id = checkboxListId;
    card.appendChild(list);
  }
}

function fillDatalist(listEl, names) {
  if (!listEl) {
    return;
  }
  listEl.innerHTML = '';
  (names || []).forEach((name) => {
    const option = document.createElement('option');
    option.value = name;
    listEl.appendChild(option);
  });
}

function setPropertyHint(card, message, loading = false) {
  const hint = card.querySelector('.db-props-hint');
  if (!hint) {
    return;
  }
  const text = hint.querySelector('.db-props-text');
  if (text) {
    text.textContent = message || '';
  }
  if (loading) {
    hint.classList.add('is-loading');
  } else {
    hint.classList.remove('is-loading');
  }
}

function applyPropertyOptions(card, properties) {
  const typeMap = new Map((properties || []).map((prop) => [prop.name, prop.type]));
  card._propertyTypeByName = typeMap;
  const optionMap = new Map(
    (properties || [])
      .filter((prop) => Array.isArray(prop.options))
      .map((prop) => [prop.name, prop.options || []])
  );
  card._propertyOptionsByName = optionMap;

  const titleInput = card.querySelector('.db-title-property');
  const statusInput = card.querySelector('.db-status-property');
  const statusTypeSelect = card.querySelector('.db-status-type');
  const checkboxInput = card.querySelector('.db-checkbox-property');

  const titleOptions = (properties || []).filter((prop) => prop.type === 'title').map((prop) => prop.name);
  const statusOptions = (properties || []).filter((prop) => prop.type === 'status').map((prop) => prop.name);
  const checkboxOptions = (properties || [])
    .filter((prop) => prop.type === 'checkbox')
    .map((prop) => prop.name);

  const titleList = titleInput ? card.querySelector(`#${titleInput.getAttribute('list')}`) : null;
  const statusList = statusInput ? card.querySelector(`#${statusInput.getAttribute('list')}`) : null;
  const checkboxList = checkboxInput ? card.querySelector(`#${checkboxInput.getAttribute('list')}`) : null;

  fillDatalist(titleList, titleOptions);
  fillDatalist(statusList, statusOptions);
  fillDatalist(checkboxList, checkboxOptions);

  if (titleInput && !titleInput.value.trim() && titleOptions.length > 0) {
    titleInput.value = titleOptions[0];
  }
  if (statusInput && !statusInput.value.trim() && statusOptions.length > 0) {
    statusInput.value = statusOptions[0];
    if (statusTypeSelect) {
      statusTypeSelect.value = 'status';
    }
    updateStatusValueOptions(card, statusInput.value.trim());
  }
  if (statusInput && statusInput.value.trim()) {
    updateStatusValueOptions(card, statusInput.value.trim());
  }
  if (checkboxInput && !checkboxInput.value.trim() && checkboxOptions.length === 1) {
    checkboxInput.value = checkboxOptions[0];
  }
  applyV2PropertyOptions(card, properties);
}

// v2: 期日・優先度のプロパティの候補を、型で絞って選択肢にする。
function applyV2PropertyOptions(card, properties) {
  const namesOf = (types) => (properties || []).filter((prop) => types.includes(prop.type)).map((prop) => prop.name);
  populatePropertySelect(card.querySelector('.db-due-date-property'), namesOf(['date']));
  populatePropertySelect(card.querySelector('.db-priority-property'), namesOf(['select']));
  renderPriorityOrder(card);
}

// 候補の選択肢を作り直す。保存済みの値が候補に無くても選択肢に残す（取得に失敗しても値を消さない）。
function populatePropertySelect(selectEl, names) {
  if (!selectEl) {
    return;
  }
  const current = selectEl.value;
  selectEl.innerHTML = '';
  const empty = document.createElement('option');
  empty.value = '';
  empty.textContent = '-- 未設定 --';
  selectEl.appendChild(empty);
  names.forEach((name) => {
    const opt = document.createElement('option');
    opt.value = name;
    opt.textContent = name;
    selectEl.appendChild(opt);
  });
  setSelectValue(selectEl, current);
  const hint = selectEl.parentElement?.querySelector('.db-v2-hint');
  if (hint) {
    hint.textContent = names.length === 0 ? '該当する型のプロパティがありません' : '';
  }
}

function setSelectValue(selectEl, value) {
  if (!selectEl || !value) {
    return;
  }
  const exists = Array.from(selectEl.options).some((opt) => opt.value === value);
  if (!exists) {
    const opt = document.createElement('option');
    opt.value = value;
    opt.textContent = value;
    selectEl.appendChild(opt);
  }
  selectEl.value = value;
}

// 選んだ優先度のプロパティの選択肢。プロパティをまだ読んでいなければ null。
function priorityOptionNames(card, propertyName) {
  const map = card._propertyOptionsByName;
  if (!propertyName || !map || !map.has(propertyName)) {
    return null;
  }
  return map.get(propertyName) || [];
}

// 優先度の順を上ほど高い並びで出す。Notion に無い値は除き、順に無い選択肢は末尾に足す。
function renderPriorityOrder(card) {
  const block = card.querySelector('.db-priority-order-block');
  const listEl = card.querySelector('.db-priority-order');
  if (!block || !listEl) {
    return;
  }
  const propertyName = card.querySelector('.db-priority-property')?.value || '';
  block.hidden = !propertyName;
  listEl.innerHTML = '';
  if (!propertyName) {
    card._priorityOrder = [];
    return;
  }
  const options = priorityOptionNames(card, propertyName);
  let order = [...(card._priorityOrder || [])];
  if (options) {
    order = order.filter((value) => options.includes(value));
    options.forEach((value) => {
      if (!order.includes(value)) {
        order.push(value);
      }
    });
  }
  card._priorityOrder = order;
  if (order.length === 0) {
    const row = document.createElement('li');
    row.className = 'hint';
    row.textContent = 'プロパティを読み込むと並べ替えられます';
    listEl.appendChild(row);
    return;
  }
  order.forEach((value, index) => {
    const row = document.createElement('li');
    row.className = 'priority-order-row';
    const label = document.createElement('span');
    label.textContent = value;
    row.appendChild(label);
    row.appendChild(createMoveButton(card, index, -1, '↑', index === 0));
    row.appendChild(createMoveButton(card, index, 1, '↓', index === order.length - 1));
    listEl.appendChild(row);
  });
}

function createMoveButton(card, index, delta, text, disabled) {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'btn ghost';
  btn.textContent = text;
  btn.disabled = disabled;
  btn.addEventListener('click', () => {
    const order = [...(card._priorityOrder || [])];
    const target = index + delta;
    [order[index], order[target]] = [order[target], order[index]];
    card._priorityOrder = order;
    renderPriorityOrder(card);
  });
  return btn;
}

function updateStatusTypeFromProperty(card, name) {
  const typeMap = card._propertyTypeByName;
  if (!typeMap || !name) {
    return;
  }
  const typ = typeMap.get(name);
  if (!typ) {
    return;
  }
  const statusTypeSelect = card.querySelector('.db-status-type');
  if (!statusTypeSelect) {
    return;
  }
  if (typ === 'status' || typ === 'select') {
    statusTypeSelect.value = typ;
  }
}

function populateStatusValueSelects(card, options) {
  const selects = [
    { el: card.querySelector('.db-status-in-progress'), placeholder: '-- 選択 --' },
    { el: card.querySelector('.db-status-done'), placeholder: '-- 未設定 --' },
    { el: card.querySelector('.db-status-paused'), placeholder: '-- 未設定 --' },
  ];
  for (const { el, placeholder } of selects) {
    if (!el) continue;
    const currentValue = el.value;
    el.innerHTML = '';
    const defaultOpt = document.createElement('option');
    defaultOpt.value = '';
    defaultOpt.textContent = placeholder;
    el.appendChild(defaultOpt);
    (options || []).forEach((name) => {
      const opt = document.createElement('option');
      opt.value = name;
      opt.textContent = name;
      el.appendChild(opt);
    });
    if (currentValue) {
      if ((options || []).includes(currentValue)) {
        el.value = currentValue;
      } else {
        const opt = document.createElement('option');
        opt.value = currentValue;
        opt.textContent = currentValue;
        el.appendChild(opt);
        el.value = currentValue;
      }
    }
  }
}

function updateStatusValueOptions(card, statusPropertyName) {
  const map = card._propertyOptionsByName;
  const options = map && statusPropertyName ? map.get(statusPropertyName) || [] : [];
  populateStatusValueSelects(card, options);
}

function pickDefaultView() {
  if (state.mode === 'settings') {
    return 'settings';
  }
  const enabled = (state.config?.databases || []).filter((db) => db.enabled);
  const task = enabled.find((db) => db.kind === 'task');
  return task?.key || enabled[0]?.key || null;
}

function renderTabsAndPanes() {
  const databases = (state.config?.databases || []).filter((db) => db.enabled);
  state.dbMap = new Map(databases.map((db) => [db.key, db]));
  state.paneMap = new Map();

  tabNav.innerHTML = '';
  if (state.mode === 'main') {
    databases.forEach((db) => {
      const tab = document.createElement('button');
      tab.className = 'tab';
      tab.dataset.view = db.key;
      tab.textContent = db.name || defaultDatabaseName(db.kind);
      tabNav.appendChild(tab);
    });
  }

  const settingsPane = paneContainer.querySelector('.pane[data-pane="settings"]');
  paneContainer
    .querySelectorAll('.pane[data-pane]:not([data-pane="settings"])')
    .forEach((pane) => pane.remove());

  if (state.mode === 'main') {
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
      refreshBtn.addEventListener('click', () => refreshDatabaseView(db.key, true));

      header.appendChild(headerText);
      header.appendChild(refreshBtn);

      const list = document.createElement('div');
      list.className = 'task-list';
      // 最初の応答が来るまでは「読み込み中」を出し、空の表示と区別する。
      const loading = document.createElement('div');
      loading.className = 'loading-state';
      loading.textContent = '読み込み中…';
      list.appendChild(loading);
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
}

function renderDatabaseSettings(databases) {
  databaseList.innerHTML = '';
  (databases || []).forEach((db) => {
    const card = createDatabaseCard(db);
    databaseList.appendChild(card);
    // 設定画面を開いたら、期日・優先度などの候補を出すためにプロパティを読み込んでおく。
    if (state.mode === 'settings' && db.database_id) {
      card.dataset.lastFetchedDbId = normalizeNotionId(db.database_id);
      loadDatabaseProperties(card, { silent: true });
    }
  });
}

function applyDatabaseKind(card, kind) {
  card.querySelectorAll('.db-fields').forEach((section) => {
    section.hidden = section.dataset.kind !== kind;
  });
}

function createDatabaseCard(db) {
  const card = databaseCardTemplate.content.firstElementChild.cloneNode(true);
  const key = db.key || generateKey();
  card.dataset.key = key;
  ensurePropertyLists(card, key);

  const nameInput = card.querySelector('.db-name-input');
  const keyLabel = card.querySelector('.db-key');
  const kindSelect = card.querySelector('.db-kind-select');
  const enabledToggle = card.querySelector('.db-enabled-toggle');
  const statusInput = card.querySelector('.db-status-property');

  nameInput.value = db.name || '';
  keyLabel.textContent = `#${key}`;
  kindSelect.value = db.kind || 'task';
  enabledToggle.checked = Boolean(db.enabled);

  card.querySelector('.db-database-id').value = db.database_id || '';
  card.querySelector('.db-data-source-id').value = db.data_source_id || '';
  card.querySelector('.db-title-property').value = db.title_property_name || '';
  card.querySelector('.db-status-property').value = db.status_property_name || '';
  card.querySelector('.db-status-type').value = db.status_property_type || 'status';
  [
    { sel: '.db-status-in-progress', val: db.status_in_progress },
    { sel: '.db-status-done', val: db.status_done },
    { sel: '.db-status-paused', val: db.status_paused },
  ].forEach(({ sel, val }) => {
    const el = card.querySelector(sel);
    if (el && val) {
      const opt = document.createElement('option');
      opt.value = val;
      opt.textContent = val;
      el.appendChild(opt);
      el.value = val;
    }
  });
  const checkboxInput = card.querySelector('.db-checkbox-property');
  if (checkboxInput) {
    checkboxInput.value = db.checkbox_property_name || defaultHabitDays;
  }
  setSelectValue(card.querySelector('.db-due-date-property'), db.due_date_property_name);
  const priorityInput = card.querySelector('.db-priority-property');
  setSelectValue(priorityInput, db.priority_property_name);
  card._priorityOrder = Array.isArray(db.priority_order) ? [...db.priority_order] : [];
  card._goalPropertyName = db.goal_property_name || '';
  if (priorityInput) {
    priorityInput.addEventListener('change', () => {
      card._priorityOrder = [];
      renderPriorityOrder(card);
    });
  }
  renderPriorityOrder(card);

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
    }
  });

  const dbIdInput = card.querySelector('.db-database-id');
  if (dbIdInput) {
    if (db.database_id) {
      card.dataset.lastFetchedDbId = db.database_id;
    }
    dbIdInput.addEventListener('blur', () => {
      const id = normalizeNotionId(dbIdInput.value);
      if (id && id.length === 32 && id !== card.dataset.lastFetchedDbId) {
        card.dataset.lastFetchedDbId = id;
        loadDatabaseProperties(card, { silent: true });
      }
    });
  }
  const loadPropertiesBtn = card.querySelector('.db-load-properties');
  if (loadPropertiesBtn) {
    loadPropertiesBtn.addEventListener('click', () => loadDatabaseProperties(card));
  }
  if (statusInput) {
    statusInput.addEventListener('change', () => updateStatusTypeFromProperty(card, statusInput.value.trim()));
    statusInput.addEventListener('focus', () => {
      const current = statusInput.value;
      if (!current) {
        return;
      }
      statusInput.dataset.prevValue = current;
      statusInput.value = '';
    });
    statusInput.addEventListener('blur', () => {
      if (statusInput.value.trim()) {
        return;
      }
      const prev = statusInput.dataset.prevValue || '';
      if (prev) {
        statusInput.value = prev;
      }
    });
    statusInput.addEventListener('input', () => {
      updateStatusValueOptions(card, statusInput.value.trim());
    });
  }
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
        kind === 'habit'
          ? card.querySelector('.db-checkbox-property')?.value.trim() || defaultHabitDays
          : '',
      ...collectV2Fields(card, kind),
    };
  });
}

// v2 の設定キー。種類に合わないキーは空で送る（サーバ側でも正規化する）。
function collectV2Fields(card, kind) {
  const priorityProperty = kind === 'task' ? card.querySelector('.db-priority-property')?.value.trim() || '' : '';
  return {
    due_date_property_name: kind === 'task' ? card.querySelector('.db-due-date-property')?.value.trim() || '' : '',
    priority_property_name: priorityProperty,
    priority_order: priorityProperty ? [...(card._priorityOrder || [])] : [],
    goal_property_name: card._goalPropertyName || '',
  };
}

// タスクのまとまり（Go 側の group の値と見出し）。表示の順もこの並び。
const TASK_GROUPS = [
  { key: 'overdue', label: '期限切れ' },
  { key: 'today', label: '今日' },
  { key: 'in_progress', label: '進行中' },
  { key: 'upcoming', label: '直近 7 日', collapsible: true },
];

const DAY_MS = 24 * 60 * 60 * 1000;

function usesDueDateView(db) {
  return Boolean(db?.due_date_property_name && db?.status_in_progress && db?.status_done && db?.status_paused);
}

function renderTasks(listEl, emptyEl, tasks, dbKey) {
  listEl.innerHTML = '';
  if (!tasks || tasks.length === 0) {
    emptyEl.style.display = 'block';
    return;
  }
  emptyEl.style.display = 'none';
  const db = state.dbMap.get(dbKey);

  // 期日を使わない DB は v1 と同じく見出しなしで並べる（NFR-02）。
  // 手元の設定が古くても、Go 側が期日のまとまりを付けて返していれば見出しを出す。
  const hasDueGroups = tasks.some((task) => task.group && task.group !== 'in_progress');
  if (!usesDueDateView(db) && !hasDueGroups) {
    tasks.forEach((task) => listEl.appendChild(createTaskCard(task, db, dbKey)));
    return;
  }
  TASK_GROUPS.forEach((group) => {
    const items = tasks.filter((task) => task.group === group.key);
    if (items.length === 0) {
      return;
    }
    listEl.appendChild(createTaskGroup(group, items, db, dbKey));
  });
}

function createTaskGroup(group, items, db, dbKey) {
  const section = document.createElement('section');
  section.className = `task-group task-group-${group.key}`;

  const header = document.createElement(group.collapsible ? 'button' : 'div');
  header.className = 'task-group-header';
  header.textContent = `${group.label}（${items.length}）`;
  section.appendChild(header);

  const body = document.createElement('div');
  body.className = 'task-group-body';
  items.forEach((task) => body.appendChild(createTaskCard(task, db, dbKey)));
  section.appendChild(body);

  if (group.collapsible) {
    header.type = 'button';
    const applyOpen = (open) => {
      body.hidden = !open;
      header.classList.toggle('is-open', open);
      header.setAttribute('aria-expanded', String(open));
    };
    applyOpen(readUpcomingOpen(dbKey));
    header.addEventListener('click', () => {
      const open = body.hidden;
      applyOpen(open);
      writeUpcomingOpen(dbKey, open);
    });
  }
  return section;
}

function upcomingOpenKey(dbKey) {
  return `nudge.upcomingOpen.${dbKey}`;
}

// 「直近 7 日」の開閉は見た目だけの状態なので localStorage に置く。読めなければ閉じた状態にする。
function readUpcomingOpen(dbKey) {
  try {
    return window.localStorage.getItem(upcomingOpenKey(dbKey)) === '1';
  } catch (err) {
    return false;
  }
}

function writeUpcomingOpen(dbKey, open) {
  try {
    window.localStorage.setItem(upcomingOpenKey(dbKey), open ? '1' : '0');
  } catch (err) {
    // 保存できなくても表示は続ける。
  }
}

function parseLocalDate(value) {
  const [y, m, d] = (value || '').split('-').map(Number);
  if (!y || !m || !d) {
    return null;
  }
  return new Date(y, m - 1, d);
}

function formatDueLabel(task) {
  if (!task.due_date) {
    return '';
  }
  if (task.group === 'today') {
    return '今日';
  }
  const due = parseLocalDate(task.due_date);
  if (!due) {
    return task.due_date;
  }
  const label = `${due.getMonth() + 1}/${due.getDate()}`;
  if (task.group !== 'overdue') {
    return label;
  }
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const days = Math.round((today - due) / DAY_MS);
  return days > 0 ? `${label}（${days} 日超過）` : label;
}

function createTaskCard(task, db, dbKey) {
  const card = document.createElement('div');
  card.className = 'task-card';
  if (task.group === 'overdue') {
    card.classList.add('is-overdue');
  }

  const title = document.createElement('div');
  title.className = 'task-title';
  title.textContent = task.title || '(無題)';

  const meta = document.createElement('div');
  meta.className = 'task-meta';
  const info = document.createElement('span');
  info.className = 'task-meta-info';
  const dueLabel = formatDueLabel(task);
  if (dueLabel) {
    const due = document.createElement('span');
    due.className = 'task-due';
    due.textContent = dueLabel;
    info.appendChild(due);
  }
  if (task.priority) {
    const priority = document.createElement('span');
    priority.className = 'task-priority';
    priority.textContent = task.priority;
    info.appendChild(priority);
  }
  const edited = document.createElement('span');
  edited.textContent = `更新 ${formatTime(task.last_edited_time)}`;
  meta.appendChild(info);
  meta.appendChild(edited);

  card.appendChild(title);
  card.appendChild(meta);
  card.appendChild(createTaskActions(task, db, dbKey));
  return card;
}

// 未着手のタスクは「開始」「完了」、進行中のタスクは「完了」「中断」を出す。値が未設定のボタンは出さない。
function createTaskActions(task, db, dbKey) {
  const actions = document.createElement('div');
  actions.className = 'task-actions';
  const addButton = (label, className, onClick) => {
    const btn = document.createElement('button');
    btn.className = className;
    btn.textContent = label;
    btn.addEventListener('click', onClick);
    actions.appendChild(btn);
  };

  addButton('Notionで開く', 'btn ghost', () => openURL(task.url));
  const inProgress = task.is_in_progress !== false;
  if (!inProgress && db?.status_in_progress) {
    addButton('開始', 'btn', () => updateTaskStatus(task.id, 'resume', dbKey));
  }
  if (db?.status_done) {
    addButton('完了', inProgress ? 'btn' : 'btn ghost', () => updateTaskStatus(task.id, 'done', dbKey));
  }
  if (inProgress && db?.status_paused) {
    addButton('中断', 'btn ghost', () => updateTaskStatus(task.id, 'paused', dbKey));
  }
  return actions;
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

async function refreshDatabaseView(dbKey, force = false) {
  const pane = state.paneMap.get(dbKey);
  const db = state.dbMap.get(dbKey);
  if (!pane || !db) {
    return;
  }
  try {
    setError('');
    if (db.kind === 'habit') {
      const habits = await rpc('getHabits', { database_key: dbKey, force_refresh: force });
      renderHabits(pane.listEl, pane.emptyEl, habits, dbKey);
    } else {
      const tasks = await rpc('getTasks', { database_key: dbKey, force_refresh: force });
      renderTasks(pane.listEl, pane.emptyEl, tasks, dbKey);
    }
    lastUpdated.textContent = `更新 ${formatTime(new Date().toISOString())}`;
  } catch (err) {
    clearLoading(pane);
    setError(err.message);
  } finally {
    refreshSyncState();
  }
}

function clearLoading(pane) {
  const loading = pane.listEl.querySelector('.loading-state');
  if (loading) {
    loading.remove();
  }
}

// 取得失敗の種類ごとの表示（詳細設計 1.4）。
const ERROR_KIND_LABELS = {
  auth: '権限がありません',
  not_shared: 'DB が共有されていません',
  rate_limited: '混み合っています',
  server: 'Notion 側のエラー',
  network: '通信エラー',
  config: '設定を確認してください',
  unknown: '不明なエラー',
};

async function refreshSyncState() {
  if (state.mode === 'settings' || !syncStatusEl) {
    return;
  }
  try {
    renderSyncState(await rpc('getSyncState'));
  } catch (err) {
    syncStatusEl.textContent = '';
  }
}

function renderSyncState(states) {
  const list = states || [];
  const messages = [];
  const failed = list.filter((s) => !s.ok);
  if (failed.length > 0) {
    const first = failed[0];
    const label = ERROR_KIND_LABELS[first.error_kind] || ERROR_KIND_LABELS.unknown;
    const rest = failed.length > 1 ? ` ほか ${failed.length - 1} 件` : '';
    messages.push(`取得に失敗: ${first.name}（${label}） ${formatTime(first.last_attempt_at)}${rest}`);
  }
  if (list.some((s) => s.history_ok === false)) {
    messages.push('過去の習慣の取得に失敗（連続週数は前回の値）');
  }
  list.filter((s) => s.truncated).forEach((s) => messages.push(`${s.name} の件数が多すぎて一部を省きました`));
  syncStatusEl.textContent = messages.join(' / ');
  syncStatusEl.classList.toggle('is-error', messages.length > 0);
}

function refreshActiveView(force = false) {
  if (state.mode === 'settings' || !state.view || state.view === 'settings') {
    return;
  }
  refreshDatabaseView(state.view, force);
}

async function updateTaskStatus(taskID, action, dbKey) {
  try {
    setError('');
    await rpc('updateStatus', { database_key: dbKey, task_id: taskID, action });
    await refreshDatabaseView(dbKey, true);
  } catch (err) {
    setError(err.message);
  }
}

async function updateHabitCheck(dbKey, taskID, checkbox) {
  try {
    setError('');
    checkbox.disabled = true;
    await rpc('updateHabitCheck', { database_key: dbKey, task_id: taskID, checked: true });
    await refreshDatabaseView(dbKey, true);
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

async function openSettingsWindow() {
  try {
    setError('');
    await rpc('openSettingsWindow');
  } catch (err) {
    setError(err.message);
  }
}

function normalizeNotionId(value) {
  const raw = (value || '').trim();
  if (!raw) {
    return '';
  }
  const cleaned = raw.replace(/-/g, '');
  const match = cleaned.match(/[a-f0-9]{32}/i);
  if (match) {
    return match[0];
  }
  return cleaned;
}

async function loadConfig() {
  const cfg = await rpc('getConfig');
  state.config = cfg;
  launchAtLoginInput.checked = Boolean(cfg.launch_at_login);
  renderDatabaseSettings(cfg.databases || []);
  renderTabsAndPanes();
}

async function saveConfig() {
  if (saveConfigBtn) {
    saveConfigBtn.disabled = true;
    saveConfigBtn.classList.remove('is-saved', 'is-error');
    saveConfigBtn.classList.add('is-saving');
  }
  try {
    const cfg = {
      ...state.config,
      databases: collectDatabases(),
      launch_at_login: launchAtLoginInput.checked,
    };
    await rpc('saveConfig', cfg);
    state.config = cfg;
    renderTabsAndPanes();
    const nextView = state.dbMap.has(state.view) ? state.view : pickDefaultView();
    setView(nextView);
    setError('');
    if (saveConfigBtn) {
      saveConfigBtn.classList.remove('is-saving');
      saveConfigBtn.classList.add('is-saved');
      window.setTimeout(() => {
        saveConfigBtn?.classList.remove('is-saved');
      }, 700);
    }
  } catch (err) {
    setError(err.message);
    if (saveConfigBtn) {
      saveConfigBtn.classList.remove('is-saving');
      saveConfigBtn.classList.add('is-error');
      window.setTimeout(() => {
        saveConfigBtn?.classList.remove('is-error');
      }, 700);
    }
  } finally {
    if (saveConfigBtn) {
      saveConfigBtn.disabled = false;
      saveConfigBtn.classList.remove('is-saving');
    }
  }
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

async function loadDatabaseProperties(card, opts = {}) {
  const input = card.querySelector('.db-database-id');
  const databaseID = normalizeNotionId(input?.value);
  if (!databaseID) {
    if (!opts.silent) setError('Database ID を入力してください');
    return;
  }
  const button = card.querySelector('.db-load-properties');
  try {
    if (!opts.silent) setError('');
    setPropertyHint(card, 'プロパティを取得中...', true);
    if (button) {
      button.disabled = true;
    }
    if (input) {
      input.value = databaseID;
    }
    // DataSourceID を自動解決
    const dataSourceInput = card.querySelector('.db-data-source-id');
    if (dataSourceInput && !dataSourceInput.value.trim()) {
      try {
        const id = await rpc('resolveDataSourceID', { database_id: databaseID });
        dataSourceInput.value = id || '';
      } catch (_) {}
    }
    const properties = await rpc('getDatabaseProperties', { database_id: databaseID });
    const props = properties || [];
    applyPropertyOptions(card, props);
    const fetchedAt = new Date().toISOString();
    card.dataset.propsFetchedAt = fetchedAt;
    const timeLabel = formatTime(fetchedAt, true);
    if (props.length === 0) {
      setPropertyHint(card, `プロパティが見つかりませんでした (${timeLabel})`);
      return;
    }
    const titleCount = props.filter((prop) => prop.type === 'title').length;
    const statusCount = props.filter((prop) => prop.type === 'status').length;
    const checkboxCount = props.filter((prop) => prop.type === 'checkbox').length;
    setPropertyHint(
      card,
      `取得: タイトル${titleCount} / ステータス${statusCount} / チェック${checkboxCount} (${timeLabel})`
    );
  } catch (err) {
    setPropertyHint(card, '取得に失敗しました');
    if (!opts.silent) setError(err.message);
  } finally {
    setPropertyHint(card, card.querySelector('.db-props-text')?.textContent || '', false);
    if (button) {
      button.disabled = false;
    }
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
  if (state.mode === 'settings') {
    return;
  }
  if (state.pollTimer) {
    clearInterval(state.pollTimer);
  }
  const interval = (state.config?.poll_interval_seconds || 60) * 1000;
  state.pollTimer = setInterval(() => {
    refreshActiveView(false);
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
  if (openSettingsBtn) {
    openSettingsBtn.addEventListener('click', openSettingsWindow);
  }

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
    refreshActiveView(true);
  });

  // 設定ウィンドウで保存されたら、設定を読み直してタブと一覧を作り直す。
  wails.Events.On('config-updated', async () => {
    if (state.mode === 'settings') {
      return;
    }
    const view = state.view;
    try {
      await loadConfig();
      setView(view);
      refreshActiveView(false);
    } catch (err) {
      setError(err.message);
    }
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
  if (state.mode === 'main') {
    refreshActiveView();
    startPolling();
  }
}

init();
