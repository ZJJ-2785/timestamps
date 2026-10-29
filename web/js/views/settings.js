// Settings: general, tracking, screenshots, browser extension, data, shortcuts, about.
import { h, clear, button, toggle, select, toast, confirmDialog, icon, pickDate } from '../ui.js';
import { t } from '../i18n.js';
import { bytes, today, addDays, fmtDate, dateKey, fmtDateLong } from '../fmt.js';
import * as api from '../api.js';
import { state, emit } from '../state.js';
import { go } from '../router.js';
import { isMac, keys, byOS } from '../platform.js';

const SECTIONS = [
  ['general', 'settings', 'General'],
  ['tracking', 'activity', 'Tracking'],
  ['screenshots', 'camera', 'Screenshots'],
  ['browser', 'puzzle', 'Browser extension'],
  ['data', 'database', 'Data'],
  ['shortcuts', 'keyboard', 'Keyboard shortcuts'],
  ['about', 'info', 'About'],
];

export function mount(el, params) {
  el.className = 'view settings-view';
  const section = params.arg || 'general';
  let s = { ...state.settings };
  let info = null;
  const nav = h('nav', { class: 'settings-nav' }, SECTIONS.map(([id, ic, label]) => h('button', { type: 'button', class: id === section ? 'on' : '', onClick: () => go('settings/' + id) }, icon(ic, 16), h('span', null, t(label)))));
  const body = h('div', { class: 'settings-body' });
  el.append(h('header', { class: 'toolbar' }, h('h2', null, t('Settings'))), h('div', { class: 'settings-layout' }, nav, body));

  let saveTm;
  const save = (patch) => {
    Object.assign(s, patch);
    clearTimeout(saveTm);
    saveTm = setTimeout(async () => {
      try {
        const r = await api.post('settings', s);
        state.settings = r;
        s = { ...r };
        emit('settings');
        toast(t('Settings saved'), { type: 'ok', timeout: 1400 });
      } catch (err) { toast(err.message, { type: 'error' }); }
    }, 350);
  };

  const row = (label, control, hint) => h('div', { class: 'set-row' }, h('div', { class: 'set-label' }, h('div', null, label), hint ? h('div', { class: 'hint' }, hint) : null), h('div', { class: 'set-control' }, control));
  const group = (title, ...rows) => h('section', { class: 'card set-group' }, title ? h('h3', null, title) : null, ...rows);
  const num = (key, min, max, unit) => {
    const i = h('input', { class: 'input num', type: 'number', min, max, value: s[key] });
    i.addEventListener('change', () => { const v = Math.max(min, Math.min(max, +i.value || min)); i.value = v; save({ [key]: v }); });
    return h('div', { class: 'inline' }, i, unit ? h('span', { class: 'muted' }, unit) : null);
  };
  const sw = (key) => toggle(!!s[key], (v) => save({ [key]: v }));

  // Color theme cards for one mode; each card shows the theme in miniature.
  const THEMES = {
    dark: [['classic', 'Classic'], ['onedark', 'One Dark Pro'], ['dracula', 'Dracula'], ['tokyonight', 'Tokyo Night'], ['githubdark', 'GitHub Dark'], ['gruvboxdark', 'Gruvbox Dark']],
    light: [['classic', 'Classic'], ['githublight', 'GitHub Light'], ['onelight', 'Atom One Light'], ['solarized', 'Solarized Light'], ['latte', 'Catppuccin Latte'], ['rosepinedawn', 'Ros\u00e9 Pine Dawn']],
  };
  const CLASSIC_CATS = ['#5b8def', '#e0699a', '#3fb27f', '#e2b847', '#9a6bdf', '#e5484d'];
  function themePicker(mode, label, hint) {
    const key = mode === 'dark' ? 'darkTheme' : 'lightTheme';
    let egg = false;
    try { egg = localStorage.getItem('tsEgg') === '1'; } catch (_) {}
    const list = THEMES[mode].concat(egg || s[key] === 'dkhd' ? [['dkhd', 'dkhd']] : []);
    const cur = s[key] || 'classic';
    const pick = (id) => {
      save({ [key]: id });
      state.settings = { ...state.settings, [key]: id };
      emit('settings'); // applies it now; the save follows
    };
    const card = ([id, name]) => h('button', {
      type: 'button', class: 'tp' + (id === cur ? ' on' : ''), title: name, 'aria-pressed': String(id === cur),
      dataset: id === 'classic' ? { theme: mode } : { theme: mode, palette: id }, onClick: () => pick(id),
    },
      h('span', { class: 'tp-prev' },
        h('span', { class: 'tp-side' }, h('i'), h('i', { class: 'on' }), h('i')),
        h('span', { class: 'tp-main' },
          h('span', { class: 'tp-lane' }, h('i', { class: 'a', style: { width: '36%' } }), h('i', { class: 'w', style: { width: '16%' } }), h('i', { class: 'a', style: { width: '30%' } })),
          h('span', { class: 'tp-cats' }, CLASSIC_CATS.map((c, i) => h('i', { style: { background: `var(--cat-${i}, ${c})` } }))),
          h('span', { class: 'tp-line' }), h('span', { class: 'tp-line s' }))),
      h('span', { class: 'tp-name' }, id === 'classic' ? t('Classic') : name));
    return h('div', { class: 'tp-block' }, h('div', { class: 'set-label' }, h('div', null, label), h('div', { class: 'hint' }, hint)),
      h('div', { class: 'tp-grid' }, list.map(card)));
  }

  function general() {
    body.append(
      group(t('Appearance'),
        row(t('Language'), select([['auto', t('System default')], ['en', 'English'], ['zh', '简体中文']], s.language, (v) => { save({ language: v }); setTimeout(() => location.reload(), 600); })),
        row(t('Theme'), select([['system', t('System')], ['light', t('Light')], ['dark', t('Dark')]], s.theme, (v) => save({ theme: v }))),
        themePicker('dark', t('Dark theme'), t('Used when the app is dark.')),
        themePicker('light', t('Light theme'), t('Used when the app is light.')),
        row(t('Duration format'), select([['hm', '1h 05m'], ['clock', '1:05:00'], ['decimal', '1.08 h']], s.durationFormat, (v) => save({ durationFormat: v }))),
        row(t('Clock'), select([['24', t('24-hour')], ['12', t('12-hour')]], s.clock24 ? '24' : '12', (v) => save({ clock24: v === '24' }))),
        row(t('First day of the week'), select([[1, t('Monday')], [0, t('Sunday')]], s.weekStart, (v) => save({ weekStart: +v })))),
      group(t('Startup and window'),
        row(t(byOS('Open at login', 'Start with Windows')), sw('runAtStartup'), t(byOS('Starts in the menu bar.', 'Starts minimized in the notification area.'))),
        row(t('Start minimized'), sw('startMinimized')),
        row(t('Keep tracking when the window is closed'), sw('closeToTray'), t(byOS('The window closes to free memory; tracking continues in the menu bar.', 'The window closes to free memory; tracking continues in the tray.')))),
      group(t('Tagging'),
        row(t('Recent tags to show'), num('recentTags', 3, 30)),
        row(t('Stopwatch reminder'), num('stopwatchNag', 0, 480, t('minutes (0 = off)')))));
  }

  function tracking() {
    const excl = h('div', { class: 'excl-list' });
    const drawExcl = () => {
      clear(excl);
      (s.exclusions || []).forEach((x, i) => excl.append(h('div', { class: 'excl' },
        h('span', { class: 'mono' }, x.app),
        select([['titles', t('Hide window titles and documents')], ['screenshots', t('No screenshots')], ['all', t('Do not track (show as Private)')]], x.mode, (v) => { s.exclusions[i].mode = v; save({ exclusions: s.exclusions }); }),
        button('', { icon: 'trash', small: true, variant: 'ghost', onClick: () => { s.exclusions.splice(i, 1); save({ exclusions: s.exclusions }); drawExcl(); } }))));
      const input = h('input', { class: 'input', placeholder: t(byOS('App, e.g. keepassxc', 'Program, e.g. keepass.exe')), list: 'app-list' });
      const dl = h('datalist', { id: 'app-list' });
      api.get('apps').then((apps) => apps.forEach((a) => dl.append(h('option', { value: a.key }, a.name))));
      excl.append(h('div', { class: 'excl add' }, input, dl, button(t('Add'), { icon: 'plus', small: true, onClick: () => {
        const v = input.value.trim().toLowerCase();
        if (!v) return;
        s.exclusions = [...(s.exclusions || []), { app: isMac || v.endsWith('.exe') ? v : v + '.exe', mode: 'titles' }];
        save({ exclusions: s.exclusions });
        drawExcl();
      } })));
    };
    drawExcl();
    const days = h('div', { class: 'days' });
    const dayNames = [];
    for (let i = 0; i < 7; i++) dayNames.push(fmtDate(dateKey(new Date(2024, 0, 1 + i)), { weekday: 'short' }));
    dayNames.forEach((d, i) => {
      const b = h('button', { type: 'button', class: 'chip' + (s.scheduleDays[i] ? ' on' : '') }, d);
      b.onclick = () => { s.scheduleDays[i] = !s.scheduleDays[i]; b.classList.toggle('on'); save({ scheduleDays: s.scheduleDays }); };
      days.append(b);
    });
    const timeIn = (key) => { const i = h('input', { class: 'input time', type: 'time', value: s[key] }); i.addEventListener('change', () => save({ [key]: i.value })); return i; };
    body.append(
      group(t('Activity'),
        row(t('Away after inactivity of'), num('awayMinutes', 1, 120, t('minutes')), t('Time without keyboard or mouse input is reclassified as away from the moment you stopped.')),
        row(t('Treat playing sound as activity'), sw('treatSoundAsActive'), t('Keeps you active while watching videos or in calls.')),
        row(t('Ask what I was doing after being away'), sw('awayPrompt')),
        row(t('Only ask when away for at least'), num('awayPromptMinutes', 1, 240, t('minutes'))),
        row(t('Count long away time as off'), sw('longAwayOff')),
        row(t('Only when away for at least'), num('longAwayHours', 1, 12, t('hours')))),
      group(t('Details'),
        row(t('Record window titles'), sw('trackTitles')),
        row(t('Record documents and websites'), sw('trackDocuments'), t(byOS('Full file paths come from the open document of the window, the window title and Spotlight.', 'Full file paths come from Office/WPS, Explorer, the program command line and Windows Recent items.'))),
        row(t('Read the browser address bar'), sw('readAddressBar'), t(byOS('Asks Safari, Chrome, Edge, Brave, Arc, Vivaldi and Opera for the current tab; macOS asks you once per browser. Firefox needs the browser extension.', 'Uses Windows UI Automation for Chrome, Edge, Firefox, Brave, Opera, Vivaldi and others. The browser extension is an alternative.'))),
        row(t('Record private / incognito browsing'), sw('trackPrivate'), t('Needs the browser extension to recognise private windows.')),
        row(t('Merge subdomains'), sw('mergeSubdomains'), t('Count t.example.com, www.example.com and example.com as one website.')),
        row(t('Download missing website icons'), sw('siteIcons'), t('Icons come from your browsers\' own icon cache first; only sites missing there are asked for their icon.'))),
      group(t('Tracking schedule'),
        row(t('Only track on a schedule'), sw('scheduleOn')),
        row(t('Days'), days),
        row(t('Hours'), h('div', { class: 'inline' }, timeIn('scheduleStart'), h('span', { class: 'muted' }, '–'), timeIn('scheduleEnd')))),
      group(t('Privacy exceptions'), h('p', { class: 'muted' }, t('Limit what is recorded for specific programs.')), excl));
  }

  function screenshots() {
    body.append(
      group(t('Automatic screenshots'),
        row(t('Take screenshots'), sw('shotsOn'), t('Only while you are active, and skipped when the screen has not changed.')),
        row(t('Every'), select([[1, '1 min'], [2, '2 min'], [5, '5 min'], [10, '10 min'], [15, '15 min'], [30, '30 min'], [60, '60 min']], s.shotsInterval, (v) => save({ shotsInterval: +v }))),
        row(t('Size'), select([[960, '960 px'], [1280, '1280 px (' + t('recommended') + ')'], [1600, '1600 px'], [1920, '1920 px'], [0, t('Original')]], s.shotsMaxWidth, (v) => save({ shotsMaxWidth: +v })), t('Width of the saved image. About 80–150 KB each at 1280 px.')),
        row(t('Quality'), select([[40, t('Low')], [60, t('Medium')], [80, t('High')]], s.shotsQuality, (v) => save({ shotsQuality: +v }))),
        row(t('Capture'), select([['active', t('Monitor with the active window')], ['all', t('All monitors')]], s.shotsAllScreens ? 'all' : 'active', (v) => save({ shotsAllScreens: v === 'all' }))),
        row(t('Skip unchanged screens'), sw('shotsSkipSame')),
        row(t('Keep screenshots for'), select([[7, t('{0} days', 7)], [14, t('{0} days', 14)], [30, t('{0} days', 30)], [90, t('{0} days', 90)], [365, t('1 year')], [0, t('Forever')]], s.shotsKeepDays, (v) => save({ shotsKeepDays: +v })))),
      group(t('Storage'),
        row(t('Folder'), h('div', { class: 'inline wrap' }, h('span', { class: 'mono path' }, info.shotsDir),
          button(t('Change…'), { small: true, icon: 'folder', onClick: pickShotsDir }),
          button(t('Open'), { small: true, variant: 'ghost', icon: 'external', onClick: () => api.post('open', { target: info.shotsDir }) }),
          info.shotsDir.toLowerCase() !== info.shotsDefault.toLowerCase() ? button(t('Default'), { small: true, variant: 'ghost', title: info.shotsDefault, onClick: () => moveShots('') }) : null),
        t('New screenshots are saved here. Changing it moves the existing ones.')),
        row(t('Space used'), h('span', null, bytes(info.shotsSize + (info.thumbsSize || 0)) + ' · ' + info.db.Shots + ' ' + t('screenshots')), t('Thumbnails are kept in the data folder.')),
        row(t('Gallery'), button(t('View screenshots'), { small: true, icon: 'image', onClick: () => go('screenshots/' + today()) }))));
  }

  async function pickShotsDir() {
    const r = await api.post('pick-folder', { title: t('Choose the screenshot folder'), dir: info.shotsDir });
    if (r.path) moveShots(r.path);
  }

  // Moves the screenshots to dir ('' = the default folder in the data folder).
  async function moveShots(dir) {
    const to = dir || info.shotsDefault;
    if (!(await confirmDialog(t('Save screenshots in {0}? The {1} existing screenshots ({2}) are moved there.', to, info.db.Shots, bytes(info.shotsSize)), { ok: t('Move') }))) return;
    toast(t('Moving screenshots…'), { timeout: 2500 });
    try {
      const r = await api.post('shots/folder', { dir });
      toast(t('Screenshots are now saved in {0}', r.dir), { type: 'ok', timeout: 5000 });
    } catch (err) { toast(err.message, { type: 'error', timeout: 8000 }); }
    show();
  }

  // Update status with "Check now" and, when there is one, "Update to x".
  function updates() {
    const box = h('div', { class: 'inline wrap' });
    const draw = (u, busy) => {
      clear(box);
      let status;
      if (busy) status = t('Checking…');
      else if (!u) status = '';
      else if (u.error === 'notfound') status = t('No public release found.');
      else if (u.error) status = t('Couldn’t reach GitHub.');
      else if (u.available) status = t('Version {0} is available.', u.latest);
      else status = t('You have the latest version.');
      box.append(h('span', { class: 'muted' }, status));
      if (u && u.available && u.setup) {
        box.append(button(t('Update to {0}', u.latest), { small: true, variant: 'primary', icon: 'download', onClick: async () => {
          if (!(await confirmDialog(t('Download Timestamps {0} and install it? The app closes and opens again when done.', u.latest), { ok: t('Update') }))) return;
          toast(t('Downloading the update…'), { timeout: 8000 });
          try { await api.post('update/install'); } catch (err) { toast(err.message, { type: 'error', timeout: 8000 }); }
        } }));
      }
      if (u && u.page) box.append(button(t('What’s new'), { small: true, variant: 'ghost', icon: 'external', onClick: () => api.post('open', { target: u.page }) }));
      box.append(button(t('Check now'), { small: true, onClick: async () => { draw(u, true); try { draw(await api.get('update', { force: 1 })); } catch (_) { draw({ error: 'offline' }); } } }));
    };
    draw(null);
    api.get('update').then((u) => draw(u)).catch(() => draw({ error: 'offline' }));
    return box;
  }

  function browser() {
    const connected = Object.entries(info.ext || {});
    const chromeDir = info.extDir;
    const ffDir = info.extDir + '\\firefox';
    const copy = (txt) => h('div', { class: 'inline' }, h('span', { class: 'mono path' }, txt),
      button('', { icon: 'copy', small: true, variant: 'ghost', title: t('Copy'), onClick: () => { navigator.clipboard.writeText(txt); toast(t('Copied')); } }),
      button('', { icon: 'folder', small: true, variant: 'ghost', title: t('Open folder'), onClick: () => api.post('open', { target: txt }) }));
    body.append(
      group(t('Status'),
        h('p', { class: 'muted' }, t('Websites are already tracked by reading the address bar. The extension is optional and more precise: it also sees private windows and works when address-bar reading is blocked.')),
        row(t('Connected browsers'), connected.length ? h('div', { class: 'chips' }, connected.map(([b]) => h('span', { class: 'chip on' }, icon('check', 12), b))) : h('span', { class: 'muted' }, t('None yet')))),
      group('Chrome · Edge · Brave · Opera · Vivaldi',
        h('ol', { class: 'steps' },
          h('li', null, t('Open the extensions page:'), ' ', h('code', null, 'chrome://extensions'), ' / ', h('code', null, 'edge://extensions')),
          h('li', null, t('Turn on Developer mode.')),
          h('li', null, t('Click "Load unpacked" and choose this folder:')), copy(chromeDir),
          h('li', null, t('Allow it in InPrivate/Incognito windows if you want those tracked.')))),
      group('Firefox',
        h('ol', { class: 'steps' },
          h('li', null, t('Open'), ' ', h('code', null, 'about:debugging#/runtime/this-firefox')),
          h('li', null, t('Click "Load Temporary Add-on" and choose manifest.json in:')), copy(ffDir),
          h('li', { class: 'muted' }, t('Temporary add-ons are removed when Firefox restarts.')))));
  }

  function data() {
    const imp = h('div', { class: 'import' });
    const drawImport = (st) => {
      clear(imp);
      if (st && st.running) {
        const p = st.total ? Math.round((st.done / st.total) * 100) : 0;
        imp.append(h('div', { class: 'progress' }, h('div', { style: { width: p + '%' } })), h('span', { class: 'muted' }, t('Importing {0}… {1}%', t(st.stage), p)));
        setTimeout(async () => drawImport(await api.get('import/status')), 700);
        return;
      }
      if (st && st.finished) {
        if (st.error) imp.append(h('p', { class: 'error' }, icon('alert', 14), st.error));
        else imp.append(h('p', { class: 'ok' }, icon('check', 14), t('Imported {0} app, {1} document, {2} usage and {3} tag records in {4}s.', st.counts.apps || 0, st.counts.docs || 0, st.counts.usage || 0, st.counts.tags || 0, Math.round(st.seconds))));
      }
      const path = h('input', { class: 'input mono', value: (st && st.path) || info.manictime || '', placeholder: 'ManicTimeReports.db' });
      imp.append(h('div', { class: 'inline grow' }, path,
        button(t('Browse…'), { small: true, onClick: async () => { const r = await api.post('pick-file', { title: t('Choose ManicTimeReports.db') }); if (r.path) path.value = r.path; } }),
        button(t('Import'), { small: true, variant: 'primary', icon: 'upload', onClick: async () => {
          if (!path.value) return;
          if (!(await confirmDialog(t('Import history from ManicTime? Previously imported data is replaced. Only time before Timestamps started tracking is imported, so nothing is counted twice.'), { ok: t('Import') }))) return;
          drawImport(await api.post('import/manictime', { path: path.value }));
        } })));
    };
    api.get('import/status').then(drawImport);

    let expTl = 'apps', expFrom = addDays(today(), -6), expTo = today(), expFmt = 'csv';
    const dateBtn = (get, set) => {
      const b = h('button', { class: 'btn date-btn', type: 'button' }, icon('day', 14), h('span', null, get()));
      b.onclick = async () => { const d = await pickDate(b, get()); if (d) { set(d); b.lastChild.textContent = d; } };
      return b;
    };
    let delFrom = today(), delTo = today();
    const delTl = { usage: true, apps: true, docs: true, tags: false, shots: true };
    body.append(
      group(t('Import from ManicTime'),
        h('p', { class: 'muted' }, info.manictime ? t('ManicTime data was found on this computer.') : t('Choose the ManicTimeReports.db file from your ManicTime data folder.')),
        imp),
      group(t('Export'),
        row(t('Timeline'), select([['apps', t('Applications')], ['docs', t('Documents')], ['usage', t('Computer usage')], ['tags', t('Tags')]], expTl, (v) => (expTl = v))),
        row(t('Range'), h('div', { class: 'inline' }, dateBtn(() => expFrom, (v) => (expFrom = v)), h('span', { class: 'muted' }, '–'), dateBtn(() => expTo, (v) => (expTo = v)))),
        row(t('Format'), select([['csv', 'CSV'], ['xlsx', 'Excel (.xlsx)']], expFmt, (v) => (expFmt = v))),
        row('', button(t('Export'), { icon: 'download', variant: 'primary', small: true, onClick: () => api.download('export', { timeline: expTl, from: expFrom, to: expTo, format: expFmt }) }))),
      group(t('Backup'),
        row(t('Database'), h('div', { class: 'inline' }, h('span', { class: 'mono path' }, info.dataDir + byOS('/', '\\') + 'timestamps.db'), h('span', { class: 'muted' }, bytes(info.dbSize)))),
        row(t('Records'), h('span', { class: 'muted' }, t('{0} app, {1} document, {2} usage, {3} tag records · since {4}', info.db.Apps.toLocaleString(), info.db.Docs.toLocaleString(), info.db.Usage.toLocaleString(), info.db.Tags.toLocaleString(), info.db.First ? fmtDateLong(dateKey(new Date(info.db.First * 1000))) : '—'))),
        row(t('Automatic backup'), select([[0, t('Off')], [1, t('Daily')], [7, t('Weekly')], [30, t('Monthly')]], s.backupDays, (v) => save({ backupDays: +v })),
          info.lastBackup ? t('Last backup: {0}', new Date(+info.lastBackup * 1000).toLocaleString()) : null),
        row(t('Backups to keep'), num('backupKeep', 1, 100)),
        row(t('Folder'), h('div', { class: 'inline' }, h('span', { class: 'mono path' }, info.dataDir + '\\backups'), button(t('Open'), { small: true, icon: 'folder', onClick: () => api.post('open', { target: info.dataDir + '\\backups' }).catch(() => toast(t('No backups yet'))) }))),
        row('', button(t('Back up now'), { icon: 'database', small: true, onClick: async () => { const r = await api.post('backup'); toast(t('Backup saved to {0}', r.path), { type: 'ok', timeout: 6000 }); } }))),
      group(t('Delete data'),
        row(t('Range'), h('div', { class: 'inline' }, dateBtn(() => delFrom, (v) => (delFrom = v)), h('span', { class: 'muted' }, '–'), dateBtn(() => delTo, (v) => (delTo = v)))),
        row(t('Timelines'), h('div', { class: 'chips' }, Object.entries({ usage: t('Computer usage'), apps: t('Applications'), docs: t('Documents'), tags: t('Tags'), shots: t('Screenshots') }).map(([k, l]) => {
          const b = h('button', { type: 'button', class: 'chip' + (delTl[k] ? ' on' : '') }, l);
          b.onclick = () => { delTl[k] = !delTl[k]; b.classList.toggle('on'); };
          return b;
        }))),
        row('', button(t('Delete…'), { icon: 'trash', variant: 'danger', small: true, onClick: async () => {
          const tls = Object.keys(delTl).filter((k) => delTl[k]);
          if (!tls.length) return;
          if (!(await confirmDialog(t('Permanently delete the selected timelines from {0} to {1}?', delFrom, delTo), { danger: true, ok: t('Delete') }))) return;
          await api.post('data/delete', { from: delFrom, to: delTo, timelines: tls });
          toast(t('Data deleted'));
        } }))));
  }

  function shortcuts() {
    const list = [
      ['F5', t('Go to today / refresh')], ['F7 / ←', t('Previous day')], ['F8 / →', t('Next day')],
      [t('Mouse wheel'), t('Zoom the timeline')], ['Shift + ' + t('Mouse wheel'), t('Scroll the timeline')], [t('Drag'), t('Select time (snaps to activity edges)')],
      [t('Click'), t('Select an activity')], ['Shift + ' + t('Click'), t('Extend the selection')], [t('Double-click'), t('Tag an activity / edit a tag')],
      ['E / Enter', t('Tag the selection')], ['Delete', t('Clear tags in the selection')], ['B', t('Toggle billable')],
      [keys('Ctrl + C / Ctrl + V'), t('Copy / paste a tag')], ['+ / −', t('Zoom in / out')], ['0', t('Fit active hours')], ['Shift + 0', t('Whole day')],
      [keys('Ctrl + F'), t('Filter the timeline')], ['Esc', t('Clear selection / close')], [keys('Ctrl + K'), t('Search')],
    ];
    body.append(group(null, h('table', { class: 'table keys' }, h('tbody', null, list.map(([k, d]) => h('tr', null, h('td', null, h('kbd', null, k)), h('td', null, d)))))));
  }

  function about() {
    body.append(group(null,
      h('div', { class: 'about' }, h('img', { src: 'icon.png', width: 56, height: 56, alt: '' }),
        h('div', null, h('h3', null, 'Timestamps ' + info.version), h('p', { class: 'muted' }, t('Automatic time tracking that stays on your computer.')))),
      row(t('Updates'), updates(), t('Looks for new versions on GitHub. Installing one replaces the app and restarts it; your data stays.')),
      row(t('Check for updates automatically'), sw('updateCheck')),
      row(t('Data folder'), h('div', { class: 'inline' }, h('span', { class: 'mono path' }, info.dataDir), button(t('Open'), { small: true, icon: 'folder', onClick: () => api.post('open', { target: info.dataDir }) }))),
      row(t('Open in a browser'), h('div', { class: 'inline' }, h('span', { class: 'mono path' }, info.url), button('', { icon: 'copy', small: true, variant: 'ghost', onClick: () => { navigator.clipboard.writeText(info.url); toast(t('Copied')); } })), t('The same interface works in any browser on this computer while Timestamps is running.')),
      window.__desktop ? row(t('Exit'), button(t('Exit Timestamps'), { icon: 'logout', small: true, variant: 'danger', onClick: () => window.tsQuit && window.tsQuit() }), t('Stops tracking until you start the app again.')) : null));
  }

  async function show() {
    info = await api.get('info');
    clear(body);
    ({ general, tracking, screenshots, browser, data, shortcuts, about }[section] || general)();
  }
  show();
  return {};
}
