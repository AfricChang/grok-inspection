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
    input[type=number] { width:52px; height:26px; border:1px solid #cbd5e1; border-radius:6px; padding:0 6px; }
    button { padding:0 12px; border:1px solid #d1d5db; background:#fff; color:#334155; cursor:pointer; }
    button.primary { border-color:#2563eb; background:#2563eb; color:#fff; font-weight:700; }
    button.soft { border-color:#c7d2fe; background:#eef2ff; color:#3730a3; font-weight:650; }
    button:disabled { opacity:.55; cursor:not-allowed; }
    .summary { display:grid; grid-template-columns:repeat(5,minmax(120px,1fr)); gap:10px; margin-bottom:12px; }
    .card { background:#fff; border:1px solid #e2e8f0; border-radius:10px; padding:12px; box-shadow:0 1px 2px rgba(15,23,42,.04); cursor:pointer; }
    .card.active { outline:2px solid #2563eb; }
    .card .k { color:#64748b; font-size:12px; }
    .card .v { margin-top:4px; font-size:22px; font-weight:750; }
    .bar { display:flex; justify-content:space-between; gap:12px; flex-wrap:wrap; margin-bottom:10px; align-items:center; }
    .filters { display:flex; gap:6px; flex-wrap:wrap; }
    .filters button { height:28px; }
    .filters button.active { background:#2563eb; border-color:#2563eb; color:#fff; }
    .progress { min-height:20px; font-size:12px; color:#64748b; }
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
    @media (max-width:760px){ .summary{ grid-template-columns:repeat(2,minmax(130px,1fr)); } }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="hero">
      <div>
        <div class="badge">xAI / Grok · CPA Plugin</div>
        <h1>Grok 账号巡检</h1>
        <p class="sub">服务端后台巡检：可切换页面继续运行。permission_denied / 额度用尽建议禁用，健康已禁用建议启用。</p>
      </div>
      <div class="controls">
        <div class="key-row">
          <input id="managementKey" type="password" autocomplete="current-password" placeholder="CPA Management Key">
        </div>
        <label class="ctl">并发 <input id="workers" type="number" min="1" max="16" value="6"></label>
        <label class="ctl"><input id="includeDisabled" type="checkbox"> 包含已禁用</label>
        <label class="ctl"><input id="onlyDisabled" type="checkbox"> 仅巡检已禁用</label>
        <button id="stopBtn" disabled>停止</button>
        <button id="applyBtn" class="soft" disabled>执行建议操作</button>
        <button id="runBtn" class="primary">开始巡检</button>
      </div>
    </div>
    <div id="summary" class="summary"></div>
    <div class="bar">
      <div id="filters" class="filters"></div>
      <div id="progress" class="progress">等待开始</div>
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
      <div id="empty" class="empty">点击“开始巡检”检测 Grok 账号</div>
      <div id="pager" class="pager"></div>
    </div>
    <pre id="error" class="err" style="margin-top:12px"></pre>
  </div>
  <script>
  const BASE = %q;
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
  const prefs = loadPrefs();
  state.pageSize = [20,50,100].includes(Number(prefs.pageSize)) ? Number(prefs.pageSize) : 20;
  $('workers').value = String(Math.min(16, Math.max(1, Number(prefs.workers) || 6)));
  $('includeDisabled').checked = !!prefs.includeDisabled;
  $('onlyDisabled').checked = !!prefs.onlyDisabled;
  if ($('onlyDisabled').checked) $('includeDisabled').checked = false;
  const keyInput = $('managementKey');
  keyInput.value = localStorage.getItem('grokInspectionManagementKey') || '';
  keyInput.addEventListener('change', () => {
    localStorage.setItem('grokInspectionManagementKey', keyInput.value);
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
    if (!res.ok) throw new Error((data && (data.error || data.message)) || text || ('HTTP ' + res.status));
    return data;
  }
  function filtered() {
    const rows = state.snapshot.results || [];
    if (state.filter === 'all') return rows;
    return rows.filter((r) => r.classification === state.filter);
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
    ];
    $('summary').innerHTML = cards.map(([key,label,value]) => {
      const active = (key === 'total' && state.filter === 'all') || state.filter === key;
      return '<div class="card' + (active ? ' active' : '') + '" data-filter="' + key + '"><div class="k">' + label + '</div><div class="v">' + value + '</div></div>';
    }).join('');
    $('summary').querySelectorAll('[data-filter]').forEach((el) => el.onclick = () => {
      state.filter = el.dataset.filter === 'total' ? 'all' : el.dataset.filter;
      state.page = 1; render();
    });
    const filters = [['all','全部'],['healthy','健康'],['permission_denied','权限被拒'],['quota_exhausted','额度用尽'],['reauth','需重登'],['probe_error','异常']];
    $('filters').innerHTML = filters.map(([k,l]) => '<button data-filter="' + k + '" class="' + (state.filter===k?'active':'') + '">' + l + '</button>').join('');
    $('filters').querySelectorAll('button').forEach((btn) => btn.onclick = () => { state.filter = btn.dataset.filter; state.page = 1; render(); });

    const rows = filtered();
    const totalPages = Math.max(1, Math.ceil(rows.length / state.pageSize));
    if (state.page > totalPages) state.page = totalPages;
    const start = (state.page - 1) * state.pageSize;
    const pageRows = rows.slice(start, start + state.pageSize);
    const tbody = $('rows');
    if (!pageRows.length) {
      tbody.innerHTML = '';
      $('empty').style.display = 'block';
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

    $('runBtn').disabled = !!(snap.running || snap.applying);
    $('stopBtn').disabled = !snap.running;
    const actionCount = (snap.results || []).filter((r) => r.action === 'disable' || r.action === 'enable' || r.action === 'delete').length;
    $('applyBtn').disabled = !!(snap.running || snap.applying || actionCount === 0);
    $('applyBtn').textContent = snap.applying
      ? ('执行中 ' + (snap.apply_done||0) + '/' + (snap.apply_total||0))
      : (actionCount ? ('执行建议操作 (' + actionCount + ')') : '执行建议操作');
    if (snap.applying) {
      $('progress').textContent = '执行建议 ' + (snap.apply_done||0) + '/' + (snap.apply_total||0) + (snap.apply_current ? '：' + snap.apply_current : '');
    } else if (snap.running) {
      $('progress').textContent = '巡检中 ' + (snap.done||0) + '/' + (snap.total||0) + '（服务端后台继续）';
    } else if (snap.stopped) {
      $('progress').textContent = '已停止，完成 ' + (snap.done||0) + (snap.total ? '/' + snap.total : '') + ' 个账号';
    } else if ((snap.results||[]).length) {
      $('progress').textContent = '巡检完成，共 ' + (snap.results||[]).length + ' 个账号';
    } else {
      $('progress').textContent = '等待开始';
    }
  }
  async function refresh() {
    if (!keyInput.value.trim()) {
      $('error').textContent = '';
      render();
      return;
    }
    try {
      const data = await api('/status', { method: 'GET' });
      state.snapshot = data || {};
      if (data.running) {
        $('includeDisabled').checked = !!data.include_disabled;
        $('onlyDisabled').checked = !!data.only_disabled;
        if (data.workers) $('workers').value = data.workers;
      }
      $('error').textContent = '';
      render();
    } catch (e) {
      $('error').textContent = String(e.message || e);
    }
  }
  function wireExclusive() {
    const include = $('includeDisabled');
    const only = $('onlyDisabled');
    const persistWorkers = () => {
      const workers = Math.min(16, Math.max(1, Number($('workers').value) || 6));
      savePrefs({ workers });
    };
    $('workers').addEventListener('input', persistWorkers);
    $('workers').addEventListener('change', () => {
      persistWorkers();
      $('workers').value = String(Math.min(16, Math.max(1, Number($('workers').value) || 6)));
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
  $('runBtn').onclick = async () => {
    try {
      const workers = Math.min(16, Math.max(1, Number($('workers').value) || 6));
      savePrefs({
        workers,
        includeDisabled: $('includeDisabled').checked,
        onlyDisabled: $('onlyDisabled').checked
      });
      await api('/start', { method: 'POST', body: JSON.stringify({
        workers,
        include_disabled: $('includeDisabled').checked,
        only_disabled: $('onlyDisabled').checked
      })});
      await refresh();
    } catch (e) { $('error').textContent = String(e.message || e); }
  };
  $('stopBtn').onclick = async () => {
    try { await api('/stop', { method: 'POST', body: '{}' }); await refresh(); }
    catch (e) { $('error').textContent = String(e.message || e); }
  };
  $('applyBtn').onclick = async () => {
    if (!confirm('确认执行当前建议的禁用/启用操作？')) return;
    try { await api('/apply', { method: 'POST', body: '{}' }); await refresh(); }
    catch (e) { $('error').textContent = String(e.message || e); }
  };
  wireExclusive();
  refresh();
  setInterval(refresh, 1500);
  </script>
</body>
</html>`, base)
	return []byte(html)
}
