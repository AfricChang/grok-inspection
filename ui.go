package main

import "fmt"

func renderUIPage(pluginID string) []byte {
	base := "/v0/management/plugins/" + pluginID
	html := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Grok 账号巡检</title>
  <style>
    :root { color-scheme: light; }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; background:#f5f7fb; color:#0f172a; }
    .wrap { max-width: 1480px; margin: 0 auto; padding: 18px clamp(12px,2vw,24px) 28px; }
    .hero { display:flex; justify-content:space-between; gap:16px; flex-wrap:wrap; margin-bottom:14px; }
    .badge { display:inline-flex; align-items:center; height:22px; padding:0 8px; border-radius:999px; background:#eef2ff; color:#3730a3; font-size:11px; font-weight:700; }
    h1 { margin:6px 0 0; font-size:22px; line-height:30px; }
    .sub { margin:4px 0 0; color:#64748b; font-size:13px; }
    .controls { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    label.ctl, button { height:34px; border-radius:8px; font-size:13px; }
    label.ctl { display:inline-flex; align-items:center; gap:6px; padding:0 10px; border:1px solid #dbe1e8; background:#fff; color:#475569; }
    input[type=number] { width:56px; height:26px; border:1px solid #cbd5e1; border-radius:6px; padding:0 6px; }
    button { padding:0 12px; border:1px solid #d1d5db; background:#fff; color:#334155; cursor:pointer; }
    button.primary { border-color:#2563eb; background:#2563eb; color:#fff; font-weight:700; }
    button.soft { border-color:#c7d2fe; background:#eef2ff; color:#3730a3; font-weight:650; }
    button.danger { border-color:#fecaca; background:#fef2f2; color:#b91c1c; font-weight:650; }
    button:disabled { opacity:.55; cursor:not-allowed; }
    .summary { display:grid; grid-template-columns:repeat(6,minmax(100px,1fr)); gap:10px; margin-bottom:12px; }
    .card { background:#fff; border:1px solid #e2e8f0; border-radius:10px; padding:12px; box-shadow:0 1px 2px rgba(15,23,42,.04); cursor:pointer; }
    .card.active { outline:2px solid #2563eb; }
    .card .k { color:#64748b; font-size:12px; }
    .card .v { margin-top:4px; font-size:22px; font-weight:750; }
    .bar { display:flex; justify-content:space-between; gap:12px; flex-wrap:wrap; margin-bottom:10px; align-items:center; }
    .actions-row { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    .actions-row .hint { font-size:12px; color:#64748b; }
    .progress { min-height:20px; font-size:12px; color:#64748b; }
    .modal { position:fixed; inset:0; z-index:1000; display:flex; align-items:center; justify-content:center; background:rgba(15,23,42,.45); padding:16px; }
    .modal.hidden { display:none; }
    .modal-card { width:min(440px,100%%); background:#fff; border-radius:12px; border:1px solid #e2e8f0; box-shadow:0 20px 40px rgba(15,23,42,.18); padding:18px 18px 14px; }
    .modal-title { font-size:16px; font-weight:700; color:#0f172a; margin-bottom:10px; }
    .modal-msg { font-size:13px; line-height:1.6; color:#334155; white-space:pre-wrap; margin-bottom:16px; }
    .modal-actions { display:flex; justify-content:flex-end; gap:8px; }
    .modal-actions button { min-width:76px; }
    .table-wrap { background:#fff; border:1px solid #e2e8f0; border-radius:10px; overflow:hidden; box-shadow:0 1px 2px rgba(15,23,42,.04); }
    table { width:100%%; border-collapse:collapse; min-width:980px; font-size:13px; }
    th { padding:10px 12px; border-bottom:1px solid #e2e8f0; text-align:left; background:linear-gradient(180deg,#f8fafc 0%%,#f1f5f9 100%%); color:#475569; font-size:12px; }
    td { padding:10px 12px; border-bottom:1px solid #f1f5f9; vertical-align:top; }
    .pill { display:inline-flex; align-items:center; height:22px; padding:0 8px; border-radius:999px; font-size:12px; font-weight:650; }
    .empty { padding:48px 20px; text-align:center; color:#64748b; }
    .pager { display:flex; justify-content:space-between; gap:12px; flex-wrap:wrap; padding:10px 12px; border-top:1px solid #e2e8f0; background:#fbfdff; align-items:center; }
    .err { color:#b91c1c; white-space:pre-wrap; }
    .key-row { display:flex; gap:8px; align-items:center; flex-wrap:wrap; width:100%%; }
    .key-row input { width:min(360px,100%%); height:34px; border:1px solid #cbd5e1; border-radius:8px; padding:0 10px; }
    :root {
      color-scheme: light;
      --page-bg: #f5f7fb;
      --surface: #ffffff;
      --surface-muted: #fbfdff;
      --surface-subtle: #f8fafc;
      --text: #0f172a;
      --muted: #64748b;
      --border: #e2e8f0;
      --border-subtle: #f1f5f9;
      --input-border: #cbd5e1;
    }
    html, body { min-width:0; background:var(--page-bg) !important; color:var(--text) !important; }
    .grok-inspection-page { min-width:0; color:var(--text) !important; }
    .grok-inspection-page .sub,
    .grok-inspection-page .progress,
    .grok-inspection-page .actions-row .hint,
    .grok-inspection-page .card .k { color:var(--muted) !important; }
    .grok-inspection-page .ctl,
    .grok-inspection-page button,
    .grok-inspection-page .card,
    .grok-inspection-page .table-wrap,
    .grok-inspection-page .modal-card { color:var(--text) !important; background:var(--surface) !important; border-color:var(--border) !important; }
    .grok-inspection-page button.primary { background:#2563eb !important; border-color:#2563eb !important; color:#fff !important; }
    .grok-inspection-page button.soft { background:#eef2ff !important; border-color:#c7d2fe !important; color:#3730a3 !important; }
    .grok-inspection-page button.danger { background:#fef2f2 !important; border-color:#fecaca !important; color:#b91c1c !important; }
    .grok-inspection-page .modal-msg { color:var(--text) !important; }
    .grok-inspection-page input[type=number],
    .grok-inspection-page .key-row input { color:var(--text) !important; background:var(--surface) !important; border-color:var(--input-border) !important; }
    .grok-inspection-page th { background:var(--surface-subtle) !important; color:var(--muted) !important; border-color:var(--border) !important; }
    .grok-inspection-page td { border-color:var(--border-subtle) !important; }
    .grok-inspection-page .pager { background:var(--surface-muted) !important; border-color:var(--border) !important; }
    .grok-inspection-page .empty { color:var(--muted) !important; }
    .grok-inspection-page .settings-row,
    .grok-inspection-page .actions-row { display:flex; gap:8px; flex-wrap:wrap; width:100%%; }
    .grok-inspection-page .settings-row > .ctl,
    .grok-inspection-page .actions-row > button { min-width:0; }
    @media (prefers-color-scheme: dark) {
      :root {
        color-scheme: dark;
        --page-bg: #111827;
        --surface: #182131;
        --surface-muted: #151d2b;
        --surface-subtle: #1d2737;
        --text: #f8fafc;
        --muted: #a7b3c7;
        --border: #334155;
        --border-subtle: #273449;
        --input-border: #475569;
      }
      .grok-inspection-page button.soft { background:#242c58 !important; border-color:#4b5aa6 !important; color:#dbe4ff !important; }
      .grok-inspection-page button.danger { background:#3f1d1d !important; border-color:#7f1d1d !important; color:#fecaca !important; }
      .grok-inspection-page .badge { background:#252b63 !important; color:#c7d2fe !important; }
      .grok-inspection-page .card.active { outline-color:#60a5fa !important; }
    }
    @media (max-width:760px){
      body { overflow-x:hidden !important; }
      .grok-inspection-page { padding:14px 12px calc(24px + env(safe-area-inset-bottom)); }
      .grok-inspection-page .hero { display:block; }
      .grok-inspection-page h1 { font-size:24px; line-height:30px; }
      .grok-inspection-page .controls { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; width:100%%; }
      .grok-inspection-page .key-row { grid-column:1 / -1; grid-row:1; width:100%%; }
      .grok-inspection-page .key-row input { width:100%%; min-width:0; height:42px; font-size:16px; }
      .grok-inspection-page .controls > label { width:100%%; min-width:0; padding:0 8px; }
      .grok-inspection-page .controls > label:first-of-type { grid-column:1 / -1; grid-row:2; }
      .grok-inspection-page .controls > label:nth-of-type(2) { grid-column:1; grid-row:3; }
      .grok-inspection-page .controls > label:nth-of-type(3) { grid-column:2; grid-row:3; }
      .grok-inspection-page input[type=number] { flex:1; width:100%%; min-width:0; }
      .grok-inspection-page .controls > #stopBtn { grid-column:1; grid-row:4; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #runBtn { grid-column:2; grid-row:4; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #incrBtn { grid-column:1 / -1; grid-row:5; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .controls > #applyBtn { grid-column:1 / -1; grid-row:6; width:100%%; min-width:0; padding:0 8px; white-space:nowrap; }
      .grok-inspection-page .summary { grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px; }
      .grok-inspection-page .card { min-width:0; padding:10px; }
      .grok-inspection-page .card .v { font-size:26px; }
      .grok-inspection-page .bar { display:block; }
      .grok-inspection-page .actions-row {
        margin-top:8px; width:100%%; overflow-x:auto; flex-wrap:nowrap;
        padding-bottom:4px; scrollbar-width:thin;
      }
      .grok-inspection-page .actions-row > button { flex:0 0 auto; min-width:0; }
      .grok-inspection-page .pager { align-items:stretch; }
      .grok-inspection-page .pager > div { width:100%%; }
      .grok-inspection-page .pager > div:last-child { justify-content:space-between; }
    }
  </style>
</head>
<body>
  <div class="wrap grok-inspection-page">
    <div class="hero">
      <div>
        <div class="badge">xAI / Grok · CPA Plugin</div>
        <h1>Grok 账号巡检</h1>
        <p class="sub">完整巡检清空并重测；增量巡检只测 Auth 中相对上次结果的新增账号。结果落盘本地，批量操作按当前筛选。</p>
      </div>
      <div class="controls">
        <div class="key-row">
          <input id="managementKey" type="password" autocomplete="current-password" placeholder="CPA Management Key">
        </div>
        <label class="ctl">并发 <input id="workers" type="number" min="1" max="16" step="1" value="6" title="1-16 的整数"></label>
        <label class="ctl"><input id="includeDisabled" type="checkbox"> 包含已禁用</label>
        <label class="ctl"><input id="onlyDisabled" type="checkbox"> 仅巡检已禁用</label>
        <button id="stopBtn" disabled>停止</button>
        <button id="applyBtn" class="soft" disabled>执行建议操作</button>
        <button id="incrBtn" class="soft" disabled title="只检测 Auth 中相对上次结果新增的账号">增量巡检</button>
        <button id="runBtn" class="primary">开始巡检</button>
      </div>
    </div>
    <div id="summary" class="summary"></div>
    <div class="bar">
      <div class="actions-row">
        <button id="batchExportBtn" type="button" disabled>批量导出</button>
        <button id="batchDisableBtn" class="soft" type="button" disabled>批量禁用</button>
        <button id="batchEnableBtn" class="soft" type="button" disabled>批量启用</button>
        <button id="batchDeleteBtn" class="danger" type="button" disabled>批量删除</button>
        <span class="hint" id="exportHint">点击上方卡片切换分类；禁用/启用数量按当前分类下列表的启用/禁用状态统计</span>
      </div>
      <div id="progress" class="progress">等待开始</div>
    </div>
    <div id="confirmModal" class="modal hidden" aria-hidden="true">
      <div class="modal-card" role="dialog" aria-modal="true">
        <div id="confirmTitle" class="modal-title">确认操作</div>
        <div id="confirmMsg" class="modal-msg"></div>
        <div class="modal-actions">
          <button type="button" id="confirmCancel">取消</button>
          <button type="button" id="confirmOk" class="primary">确定</button>
        </div>
      </div>
    </div>
    <div class="table-wrap">
      <div style="overflow:auto">
        <table>
          <thead>
            <tr>
              <th>账号</th><th>当前状态</th><th>检测结果</th><th>HTTP</th><th>模型</th><th>建议</th><th>原因</th><th>操作</th>
            </tr>
          </thead>
          <tbody id="rows"></tbody>
        </table>
      </div>
      <div id="empty" class="empty">请输入 CPA Management Key 后加载巡检状态</div>
      <div id="pager" class="pager"></div>
    </div>
    <pre id="error" class="err" style="margin-top:12px"></pre>
  </div>
  <script>
  const BASE = %q;
  const WORKERS_MIN = 1;
  const WORKERS_MAX = 16;
  const WORKERS_DEFAULT = 6;
  const state = {
    filter: 'all',
    page: 1,
    pageSize: 20,
    snapshot: { results: [], summary: {}, running: false, applying: false, done: 0, total: 0 }
  };
  const $ = (id) => document.getElementById(id);
  const prefsKey = 'grokInspectionPrefs';
  function loadPrefs() {
    try { return JSON.parse(localStorage.getItem(prefsKey) || '{}') || {}; } catch (_) { return {}; }
  }
  function savePrefs(patch) {
    localStorage.setItem(prefsKey, JSON.stringify(Object.assign(loadPrefs(), patch || {})));
  }
  function clampWorkers(n) {
    return Math.min(WORKERS_MAX, Math.max(WORKERS_MIN, n));
  }
  function parseWorkersStrict() {
    const raw = String($('workers').value == null ? '' : $('workers').value).trim();
    if (!/^\d+$/.test(raw)) {
      throw new Error('并发必须是 ' + WORKERS_MIN + '-' + WORKERS_MAX + ' 的整数（当前默认 ' + WORKERS_DEFAULT + '）');
    }
    const n = Number(raw);
    if (!Number.isInteger(n) || n < WORKERS_MIN || n > WORKERS_MAX) {
      throw new Error('并发必须在 ' + WORKERS_MIN + '-' + WORKERS_MAX + ' 之间');
    }
    return n;
  }
  function normalizeWorkersInput(strict) {
    try {
      const n = parseWorkersStrict();
      $('workers').value = String(n);
      return n;
    } catch (e) {
      if (strict) throw e;
      $('workers').value = String(WORKERS_DEFAULT);
      return WORKERS_DEFAULT;
    }
  }
  const prefs = loadPrefs();
  state.pageSize = [20,50,100].includes(Number(prefs.pageSize)) ? Number(prefs.pageSize) : 20;
  {
    const prefWorkers = Number(prefs.workers);
    $('workers').value = String(
      Number.isInteger(prefWorkers) && prefWorkers >= WORKERS_MIN && prefWorkers <= WORKERS_MAX
        ? prefWorkers
        : WORKERS_DEFAULT
    );
  }
  $('includeDisabled').checked = !!prefs.includeDisabled;
  $('onlyDisabled').checked = !!prefs.onlyDisabled;
  if ($('onlyDisabled').checked) $('includeDisabled').checked = false;
  const keyInput = $('managementKey');
  keyInput.value = localStorage.getItem('grokInspectionManagementKey') || '';
  const hasManagementKey = () => !!keyInput.value.trim();
  function updateAuthState() {
    const ready = hasManagementKey();
    $('runBtn').disabled = !ready;
    if (!ready) {
      $('stopBtn').disabled = true;
      $('applyBtn').disabled = true;
      $('incrBtn').disabled = true;
      $('batchExportBtn').disabled = true;
      $('batchDisableBtn').disabled = true;
      $('batchEnableBtn').disabled = true;
      $('batchDeleteBtn').disabled = true;
    }
  }
  let confirmResolver = null;
  function closeConfirm(ok) {
    $('confirmModal').classList.add('hidden');
    $('confirmModal').setAttribute('aria-hidden', 'true');
    const resolve = confirmResolver;
    confirmResolver = null;
    if (resolve) resolve(!!ok);
  }
  function confirmDialog(title, message) {
    return new Promise((resolve) => {
      confirmResolver = resolve;
      $('confirmTitle').textContent = title || '确认操作';
      $('confirmMsg').textContent = message || '';
      $('confirmModal').classList.remove('hidden');
      $('confirmModal').setAttribute('aria-hidden', 'false');
      $('confirmOk').focus();
    });
  }
  async function startInspection(incremental) {
    try {
      const workers = parseWorkersStrict();
      $('workers').value = String(workers);
      savePrefs({
        workers,
        includeDisabled: $('includeDisabled').checked,
        onlyDisabled: $('onlyDisabled').checked
      });
      await api('/start', { method: 'POST', body: JSON.stringify({
        workers,
        include_disabled: $('includeDisabled').checked,
        only_disabled: $('onlyDisabled').checked,
        incremental: !!incremental
      })});
      await refresh();
    } catch (e) { $('error').textContent = String(e.message || e); }
  }
  function rowKey(r) {
    return r.auth_index || r.file_name || r.name || r.email || '';
  }
  // 批量禁用：只针对当前分类下「已启用」的号；批量启用：只针对「已禁用」的号。
  function filteredRowsForAction(action) {
    const rows = filtered();
    if (action === 'disable') return rows.filter((r) => !r.disabled);
    if (action === 'enable') return rows.filter((r) => !!r.disabled);
    return rows; // delete / export 用全部分类内账号
  }
  function filteredAuthIndexesForAction(action) {
    return filteredRowsForAction(action).map(rowKey).filter(Boolean);
  }
  async function batchForce(action) {
    const targetRows = filteredRowsForAction(action);
    const indexes = targetRows.map(rowKey).filter(Boolean);
    if (!targetRows.length || !indexes.length) {
      const tip = action === 'disable'
        ? '没有「已启用」可禁用的账号'
        : (action === 'enable' ? '没有「已禁用」可启用的账号' : '没有可操作的账号');
      $('error').textContent = '当前分类「' + filterLabel() + '」下' + tip;
      return;
    }
    const label = action === 'delete' ? '删除' : (action === 'enable' ? '启用' : '禁用');
    const stateHint = action === 'disable'
      ? '仅包含当前列表中状态为「已启用」的账号。'
      : (action === 'enable' ? '仅包含当前列表中状态为「已禁用」的账号。' : '包含当前分类下全部账号。');
    const extra = action === 'delete'
      ? '将删除 CPA Auth 凭证文件，并更新本地结果 JSON。此操作不可恢复。'
      : (action === 'enable'
        ? '将把账号写入 CPA Auth 为启用，并更新本地结果 JSON。'
        : '将把账号写入 CPA Auth 为禁用，并更新本地结果 JSON。');
    const ok = await confirmDialog(
      '批量' + label + '确认',
      '当前分类：' + filterLabel() + '\n' +
      '影响账号：' + indexes.length + ' 个\n' +
      stateHint + '\n\n' +
      '将对上述账号执行批量' + label + '。\n' + extra + '\n\n' +
      '请确认是否继续？'
    );
    if (!ok) return;
    try {
      await api('/apply', {
        method: 'POST',
        body: JSON.stringify({
          force_action: action,
          auth_indexes: indexes
        })
      });
      await refresh();
    } catch (e) {
      $('error').textContent = String(e.message || e);
    }
  }
  async function batchExport() {
    const rows = filtered();
    if (!rows.length) {
      $('error').textContent = '当前分类「' + filterLabel() + '」下没有可导出的数据';
      return;
    }
    const ok = await confirmDialog(
      '批量导出确认',
      '当前分类：' + filterLabel() + '\n' +
      '导出条数：' + rows.length + ' 条\n\n' +
      '将导出当前分类下的全部账号（不是仅当前页）为 JSON 文件。\n\n' +
      '请确认是否继续？'
    );
    if (!ok) return;
    exportRows('json');
  }
  keyInput.addEventListener('change', () => {
    localStorage.setItem('grokInspectionManagementKey', keyInput.value);
    updateAuthState();
    refresh();
  });
  const classLabel = {
    healthy: '健康', permission_denied: '权限被拒', quota_exhausted: '额度用尽',
    reauth: '需重新登录', model_unavailable: '模型不可用', probe_error: '探测异常', unknown: '未知'
  };
  const actionLabel = { keep: '保留', disable: '禁用', enable: '启用', delete: '删除' };
  const color = {
    healthy: '#047857', permission_denied: '#b45309', quota_exhausted: '#b45309',
    reauth: '#b91c1c', model_unavailable: '#475569', probe_error: '#b91c1c', unknown: '#475569'
  };
  function pill(text, c) {
    return '<span class="pill" style="background:' + c + '1a;color:' + c + '">' + escapeHtml(text) + '</span>';
  }
  function escapeHtml(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, (ch) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }
  async function api(path, opts) {
    const headers = { 'Content-Type': 'application/json' };
    if (keyInput.value) headers.Authorization = 'Bearer ' + keyInput.value;
    const res = await fetch(BASE + path, Object.assign({ headers }, opts || {}));
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (_) { data = { raw: text }; }
    // 202 Accepted is success for async apply/action
    if (!res.ok) throw new Error((data && (data.error || data.message)) || text || ('HTTP ' + res.status));
    return data;
  }
  function filtered() {
    const rows = state.snapshot.results || [];
    if (state.filter === 'all') return rows;
    // 「异常」= 探测异常 / 模型不可用 / 未知 等非主分类
    if (state.filter === 'other') {
      return rows.filter((r) => {
        const c = r.classification || '';
        return c !== 'healthy' && c !== 'permission_denied' && c !== 'quota_exhausted' && c !== 'reauth';
      });
    }
    return rows.filter((r) => r.classification === state.filter);
  }
  function filterLabel() {
    const map = {
      all: '全部',
      healthy: '健康',
      permission_denied: '权限被拒',
      quota_exhausted: '额度用尽',
      reauth: '需重登',
      other: '异常'
    };
    return map[state.filter] || state.filter;
  }
  function downloadBlob(filename, content, mime) {
    const blob = new Blob([content], { type: mime || 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  function exportRows(format) {
    const rows = filtered();
    if (!rows.length) {
      $('error').textContent = '当前筛选下没有可导出的数据';
      return;
    }
    const stamp = new Date().toISOString().replace(/[:.]/g, '-');
    const tag = state.filter === 'all' ? 'all' : state.filter;
    if (format === 'json') {
      downloadBlob('grok-inspection-' + tag + '-' + stamp + '.json', JSON.stringify({
        filter: state.filter,
        filter_label: filterLabel(),
        exported_at: new Date().toISOString(),
        count: rows.length,
        results: rows
      }, null, 2), 'application/json;charset=utf-8');
      return;
    }
    const lines = [];
    lines.push('filter=' + filterLabel() + ' count=' + rows.length + ' exported_at=' + new Date().toISOString());
    lines.push(['name','disabled','classification','http_status','model','action','reason','auth_index','file_name','email'].join('\t'));
    rows.forEach((r) => {
      lines.push([
        r.name || '',
        r.disabled ? '1' : '0',
        r.classification || '',
        r.http_status || '',
        r.model || '',
        r.action || '',
        (r.reason || r.error_message || '').replace(/[\t\r\n]+/g, ' '),
        r.auth_index || '',
        r.file_name || '',
        r.email || ''
      ].join('\t'));
    });
    downloadBlob('grok-inspection-' + tag + '-' + stamp + '.txt', lines.join('\n'), 'text/plain;charset=utf-8');
  }
  function render() {
    const snap = state.snapshot || {};
    const summary = snap.summary || {};
    const cards = [
      ['total','全部', summary.total || 0],
      ['healthy','健康', summary.healthy || 0],
      ['permission_denied','权限被拒', summary.permission_denied || 0],
      ['quota_exhausted','额度用尽', summary.quota_exhausted || 0],
      ['reauth','需重登', summary.reauth || 0],
      ['other','异常', summary.other || 0],
    ];
    $('summary').innerHTML = cards.map(([key,label,value]) => {
      const active = (key === 'total' && state.filter === 'all') || state.filter === key;
      return '<div class="card' + (active ? ' active' : '') + '" data-filter="' + key + '"><div class="k">' + label + '</div><div class="v">' + value + '</div></div>';
    }).join('');
    $('summary').querySelectorAll('[data-filter]').forEach((el) => el.onclick = () => {
      state.filter = el.dataset.filter === 'total' ? 'all' : el.dataset.filter;
      state.page = 1; render();
    });

    const rows = filtered();
    $('exportHint').textContent = '当前分类：' + filterLabel() + '（' + rows.length + ' 条）';
    const totalPages = Math.max(1, Math.ceil(rows.length / state.pageSize));
    if (state.page > totalPages) state.page = totalPages;
    const start = (state.page - 1) * state.pageSize;
    const pageRows = rows.slice(start, start + state.pageSize);
    const tbody = $('rows');
    if (!pageRows.length) {
      tbody.innerHTML = '';
      $('empty').style.display = 'block';
      $('empty').textContent = hasManagementKey()
        ? '点击“开始巡检”检测 Grok 账号'
        : '请输入 CPA Management Key 后加载巡检状态';
    } else {
      $('empty').style.display = 'none';
      tbody.innerHTML = pageRows.map((r) => {
        const actionable = !snap.applying && (r.action === 'disable' || r.action === 'enable' || r.action === 'delete');
        const actionBtn = actionable
          ? '<button data-act="' + r.action + '" data-name="' + escapeHtml(r.name) + '" data-index="' + escapeHtml(r.auth_index || '') + '">' + actionLabel[r.action] + '</button>'
          : '-';
        return '<tr>' +
          '<td>' + escapeHtml(r.name) + '</td>' +
          '<td>' + pill(r.disabled ? '已禁用' : '已启用', r.disabled ? '#b45309' : '#047857') + '</td>' +
          '<td>' + pill(classLabel[r.classification] || r.classification || '-', color[r.classification] || '#475569') + '</td>' +
          '<td>' + (r.http_status || '-') + '</td>' +
          '<td>' + escapeHtml(r.model || '-') + '</td>' +
          '<td>' + (actionLabel[r.action] || r.action || '-') + '</td>' +
          '<td>' + escapeHtml(r.reason || r.error_message || '-') + '</td>' +
          '<td>' + actionBtn + '</td>' +
        '</tr>';
      }).join('');
      tbody.querySelectorAll('button[data-act]').forEach((btn) => btn.onclick = async () => {
        try {
          await api('/action', { method: 'POST', body: JSON.stringify({
            auth_index: btn.dataset.index,
            name: btn.dataset.name,
            disabled: btn.dataset.act === 'disable',
            delete: btn.dataset.act === 'delete'
          })});
          await refresh();
        } catch (e) { $('error').textContent = String(e.message || e); }
      });
    }
    const from = rows.length ? start + 1 : 0;
    const to = Math.min(rows.length, start + state.pageSize);
    $('pager').innerHTML =
      '<div style="font-size:12px;color:#64748b">显示 ' + from + '-' + to + ' / ' + rows.length +
      ' · 每页 <select id="pageSize">' +
      [20,50,100].map((n) => '<option value="' + n + '"' + (state.pageSize===n?' selected':'') + '>' + n + '</option>').join('') +
      '</select></div>' +
      '<div style="display:flex;gap:8px;align-items:center">' +
      '<button id="prev"' + (state.page<=1?' disabled':'') + '>上一页</button>' +
      '<span style="font-size:12px;color:#475569">' + state.page + ' / ' + totalPages + '</span>' +
      '<button id="next"' + (state.page>=totalPages?' disabled':'') + '>下一页</button></div>';
    const ps = $('pageSize'); if (ps) ps.onchange = () => {
      state.pageSize = Number(ps.value)||20;
      savePrefs({ pageSize: state.pageSize });
      state.page=1;
      render();
    };
    const prev = $('prev'); if (prev) prev.onclick = () => { if (state.page>1){ state.page--; render(); } };
    const next = $('next'); if (next) next.onclick = () => { if (state.page<totalPages){ state.page++; render(); } };

    const actionCount = (snap.results || []).filter((r) => r.action === 'disable' || r.action === 'enable' || r.action === 'delete').length;
    const filteredCount = rows.length;
    const disableCount = rows.filter((r) => !r.disabled).length; // 当前分类下已启用 → 可禁用
    const enableCount = rows.filter((r) => !!r.disabled).length;  // 当前分类下已禁用 → 可启用
    const busy = !!(snap.running || snap.applying);
    const hasResults = (snap.results || []).length > 0;
    $('runBtn').disabled = !hasManagementKey() || busy;
    $('incrBtn').disabled = !hasManagementKey() || busy || !hasResults;
    $('stopBtn').disabled = !hasManagementKey() || !snap.running;
    $('applyBtn').disabled = !hasManagementKey() || busy || actionCount === 0;
    $('batchExportBtn').disabled = filteredCount === 0;
    $('batchDisableBtn').disabled = !hasManagementKey() || busy || disableCount === 0;
    $('batchEnableBtn').disabled = !hasManagementKey() || busy || enableCount === 0;
    $('batchDeleteBtn').disabled = !hasManagementKey() || busy || filteredCount === 0;
    $('applyBtn').textContent = snap.applying
      ? ('执行中 ' + (snap.apply_done||0) + '/' + (snap.apply_total||0))
      : (actionCount ? ('执行建议操作 (' + actionCount + ')') : '执行建议操作');
    $('batchExportBtn').textContent = filteredCount ? ('批量导出 (' + filteredCount + ')') : '批量导出';
    $('batchDisableBtn').textContent = disableCount ? ('批量禁用 (' + disableCount + ')') : '批量禁用';
    $('batchEnableBtn').textContent = enableCount ? ('批量启用 (' + enableCount + ')') : '批量启用';
    $('batchDeleteBtn').textContent = filteredCount ? ('批量删除 (' + filteredCount + ')') : '批量删除';
    if (!hasManagementKey()) {
      $('progress').textContent = '请输入 CPA Management Key 后加载巡检状态';
    } else if (snap.applying) {
      let msg = '后台执行操作 ' + (snap.apply_done||0) + '/' + (snap.apply_total||0) + (snap.apply_current ? '：' + snap.apply_current : '');
      if ((snap.apply_failures || []).length) msg += '；失败 ' + snap.apply_failures.length;
      $('progress').textContent = msg;
    } else if (snap.running) {
      const mode = snap.incremental ? '增量巡检中' : '巡检中';
      const extra = snap.incremental ? '（仅新增，保留已有结果）' : '（后台继续）';
      $('progress').textContent = mode + ' ' + (snap.done||0) + '/' + (snap.total||0) + ' · 并发 ' + (snap.workers||WORKERS_DEFAULT) + extra;
    } else if (snap.stopped) {
      const mode = snap.incremental ? '增量已停止' : '已停止';
      $('progress').textContent = mode + '，本轮 ' + (snap.done||0) + (snap.total ? '/' + snap.total : '') + '，列表共 ' + ((snap.results||[]).length) + ' 个账号';
    } else if ((snap.results||[]).length) {
      let msg = '巡检完成，共 ' + (snap.results||[]).length + ' 个账号';
      if (snap.incremental && (snap.done||0) >= 0 && snap.total != null) {
        msg = '增量完成：本轮新增检测 ' + (snap.done||0) + ' 个，列表共 ' + (snap.results||[]).length + ' 个';
      }
      if (snap.store_path) msg += ' · 已落盘';
      if ((snap.apply_failures || []).length) msg += ' · 上次操作失败 ' + snap.apply_failures.length + ' 条';
      $('progress').textContent = msg;
    } else {
      $('progress').textContent = '等待开始';
    }
    if ((snap.apply_failures || []).length && !snap.applying) {
      $('error').textContent = (snap.apply_failures || []).join('\n');
    }
  }
  let pollTimer = null;
  const POLL_MS = 1500;
  function stopPolling() {
    if (pollTimer != null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }
  function startPolling() {
    if (pollTimer != null) return;
    pollTimer = setInterval(() => { refresh(); }, POLL_MS);
  }
  // Only poll while a server job is active; idle pages do not keep hitting /status.
  function syncPolling(snap) {
    if (snap && (snap.running || snap.applying)) startPolling();
    else stopPolling();
  }
  async function refresh() {
    if (!keyInput.value.trim()) {
      stopPolling();
      state.snapshot = { results: [], summary: {}, running: false, applying: false, done: 0, total: 0 };
      $('error').textContent = '';
      updateAuthState();
      render();
      return;
    }
    try {
      const data = await api('/status', { method: 'GET' });
      state.snapshot = data || {};
      if (data.running) {
        $('includeDisabled').checked = !!data.include_disabled;
        $('onlyDisabled').checked = !!data.only_disabled;
        if (data.workers) $('workers').value = String(clampWorkers(Number(data.workers) || WORKERS_DEFAULT));
      }
      if (!(data.apply_failures || []).length) {
        $('error').textContent = '';
      }
      syncPolling(data);
      render();
    } catch (e) {
      $('error').textContent = String(e.message || e);
      // Keep polling only if we still believe a job is active.
      syncPolling(state.snapshot);
    }
  }
  function wireExclusive() {
    const include = $('includeDisabled');
    const only = $('onlyDisabled');
    $('workers').addEventListener('change', () => {
      try {
        const n = normalizeWorkersInput(true);
        savePrefs({ workers: n });
        $('error').textContent = '';
      } catch (e) {
        $('error').textContent = String(e.message || e);
        $('workers').value = String(WORKERS_DEFAULT);
        savePrefs({ workers: WORKERS_DEFAULT });
      }
    });
    include.onchange = () => {
      if (include.checked) only.checked = false;
      savePrefs({ includeDisabled: include.checked, onlyDisabled: only.checked });
    };
    only.onchange = () => {
      if (only.checked) include.checked = false;
      savePrefs({ includeDisabled: include.checked, onlyDisabled: only.checked });
    };
  }
  $('runBtn').onclick = () => startInspection(false);
  $('incrBtn').onclick = () => startInspection(true);
  $('stopBtn').onclick = async () => {
    try { await api('/stop', { method: 'POST', body: '{}' }); await refresh(); }
    catch (e) { $('error').textContent = String(e.message || e); }
  };
  $('applyBtn').onclick = async () => {
    const actionCount = (state.snapshot.results || []).filter((r) => r.action === 'disable' || r.action === 'enable' || r.action === 'delete').length;
    const ok = await confirmDialog(
      '执行建议操作确认',
      '将对全部结果中「有建议动作」的账号异步执行禁用/启用/删除（共 ' + actionCount + ' 条建议）。\n' +
      '说明：此操作按建议执行，不受上方卡片当前分类限制。\n\n' +
      '请确认是否继续？'
    );
    if (!ok) return;
    try {
      const result = await api('/apply', { method: 'POST', body: '{}' });
      const total = Number(result && result.apply_total || 0);
      const failed = Array.isArray(result && result.apply_failures) ? result.apply_failures.length : 0;
      if (failed > 0) {
        const details = result.apply_failures.slice(0, 5).join('；');
        $('error').textContent = '建议操作已启动：失败 ' + failed + (details ? ('。示例：' + details) : '');
      } else {
        $('error').textContent = total ? ('建议操作已在后台执行：共 ' + total + ' 项') : '';
      }
      await refresh();
    }
    catch (e) { $('error').textContent = String(e.message || e); }
  };
  $('batchDisableBtn').onclick = () => batchForce('disable');
  $('batchEnableBtn').onclick = () => batchForce('enable');
  $('batchDeleteBtn').onclick = () => batchForce('delete');
  $('batchExportBtn').onclick = () => batchExport();
  $('confirmOk').onclick = () => closeConfirm(true);
  $('confirmCancel').onclick = () => closeConfirm(false);
  $('confirmModal').addEventListener('click', (ev) => {
    if (ev.target === $('confirmModal')) closeConfirm(false);
  });
  wireExclusive();
  // One-shot load on open; polling starts only when status reports running/applying.
  refresh();
  </script>
</body>
</html>`, base)
	return []byte(html)
}
