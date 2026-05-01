const API = {
    get: (path) => fetch(path).then(r => r.json()),
    post: (path, body) => fetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    }).then(r => r.json()),
    delete: (path) => fetch(path, { method: 'DELETE' }).then(r => r.json())
};

let currentStatus = null;
let busy = false;
let refreshInterval = null;

const els = {
    btnBootstrap: document.getElementById('btn-bootstrap'),
    btnStart: document.getElementById('btn-start'),
    btnStop: document.getElementById('btn-stop'),
    btnDestroy: document.getElementById('btn-destroy'),
    btnAddSync: document.getElementById('btn-add-sync'),
    btnAddTunnel: document.getElementById('btn-add-tunnel'),
    actionText: document.getElementById('action-text'),
    refreshText: document.getElementById('refresh-text'),
    progress: document.getElementById('progress'),
    overviewText: document.getElementById('overview-text'),
    resourceText: document.getElementById('resource-text'),
    syncList: document.getElementById('sync-list'),
    tunnelList: document.getElementById('tunnel-list'),
    modalOverlay: document.getElementById('modal-overlay'),
    modalTitle: document.getElementById('modal-title'),
    modalBody: document.getElementById('modal-body'),
    modalConfirm: document.getElementById('modal-confirm'),
    modalCancel: document.getElementById('modal-cancel'),
    filepickerOverlay: document.getElementById('filepicker-overlay'),
    filepickerBreadcrumb: document.getElementById('filepicker-breadcrumb'),
    filepickerList: document.getElementById('filepicker-list'),
    filepickerSelect: document.getElementById('filepicker-select'),
    filepickerCancel: document.getElementById('filepicker-cancel'),
};

function showToast(message, type = 'success') {
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = message;
    document.body.appendChild(toast);
    setTimeout(() => toast.remove(), 3000);
}

function setBusy(isBusy) {
    busy = isBusy;
    els.progress.classList.toggle('hidden', !isBusy);
    [els.btnBootstrap, els.btnStart, els.btnStop, els.btnDestroy].forEach(btn => {
        btn.disabled = isBusy;
    });
}

function setAction(msg) { els.actionText.textContent = msg; }

function updateButtons() {
    if (!currentStatus) return;
    const { status } = currentStatus;
    const bootstrapDone = status.BootstrapDone;
    const running = status.Running;
    els.btnStart.disabled = !bootstrapDone || running || busy;
    els.btnStop.disabled = !bootstrapDone || !running || busy;
    els.btnBootstrap.disabled = busy;
    els.btnDestroy.disabled = busy;
}

async function refreshStatus() {
    try {
        const data = await API.get('/api/status');
        currentStatus = data;
        const { status, metrics } = data;
        els.overviewText.textContent = formatOverview(status);
        els.resourceText.textContent = formatResources(status, metrics);
        els.refreshText.textContent = 'Last refresh: ' + new Date().toISOString();
        updateButtons();
    } catch (err) {
        els.refreshText.textContent = 'Refresh failed: ' + err.message;
    }
}

function formatOverview(status) {
    const lines = [
        `Name: ${status.Name}`,
        `State: ${status.State}`,
        `Running: ${status.Running}`,
        `Bootstrap complete: ${status.BootstrapDone}`,
        `Disk: ${status.DiskPath}`,
        `IP: ${status.StaticIP}`,
        `SSH: ${status.SSHTarget}`,
    ];
    if (status.Running) lines.push(`PID: ${status.PID}`);
    return lines.join('\n');
}

function formatResources(status, metrics) {
    if (!status.Running) return 'The VM is stopped.';
    if (!metrics || !metrics.Available) return 'Guest metrics are not available yet. SSH may still be starting.';
    const cpuText = metrics.HasCPUPercent
        ? `CPU usage: ${metrics.CPUPercent.toFixed(1)}%`
        : 'CPU usage: waiting for a second sample';
    return [
        cpuText,
        `Memory: ${metrics.MemUsedMiB} MiB / ${metrics.MemTotalMiB} MiB (${metrics.MemUsedPercent.toFixed(1)}%)`,
        'Sampling source: /proc/stat and /proc/meminfo over SSH.',
    ].join('\n');
}

function showModal(title, bodyHtml, onConfirm) {
    els.modalTitle.textContent = title;
    els.modalBody.innerHTML = bodyHtml;
    els.modalOverlay.classList.remove('hidden');
    els.modalConfirm.onclick = () => { onConfirm(); hideModal(); };
    els.modalCancel.onclick = hideModal;
    els.modalOverlay.querySelector('.modal-close').onclick = hideModal;
}

function hideModal() { els.modalOverlay.classList.add('hidden'); }

let filePickerCallback = null;
let filePickerCurrentPath = '';
let filePickerRoot = '';

async function openFilePicker(root, callback) {
    filePickerRoot = root || '/home/vm';
    filePickerCurrentPath = filePickerRoot;
    filePickerCallback = callback;
    els.filepickerOverlay.classList.remove('hidden');
    await loadFilePickerPath(filePickerCurrentPath);
}

async function loadFilePickerPath(path) {
    try {
        const entries = await API.get(`/api/vm-files?path=${encodeURIComponent(path)}&root=${encodeURIComponent(filePickerRoot)}`);
        if (entries.error) { showToast(entries.error, 'error'); return; }
        filePickerCurrentPath = path;
        renderFilePicker(entries);
    } catch (err) {
        showToast('Failed to load files: ' + err.message, 'error');
    }
}

function renderFilePicker(entries) {
    const parts = filePickerCurrentPath.split('/').filter(Boolean);
    let crumbHtml = '<span data-path="/">/</span>';
    let buildPath = '';
    parts.forEach(part => {
        buildPath += '/' + part;
        crumbHtml += ` / <span data-path="${buildPath}">${part}</span>`;
    });
    els.filepickerBreadcrumb.innerHTML = crumbHtml;
    els.filepickerBreadcrumb.querySelectorAll('span').forEach(span => {
        span.onclick = () => loadFilePickerPath(span.dataset.path);
    });

    els.filepickerList.innerHTML = entries.map(e => `
        <div class="filepicker-item" data-name="${e.name}" data-isdir="${e.isDir}">
            <span class="icon">${e.isDir ? '📁' : '📄'}</span>
            <span>${e.name}</span>
        </div>
    `).join('');

    els.filepickerList.querySelectorAll('.filepicker-item').forEach(item => {
        item.onclick = () => {
            const name = item.dataset.name;
            const isDir = item.dataset.isdir === 'true';
            if (isDir) loadFilePickerPath(filePickerCurrentPath + '/' + name);
        };
    });
}

els.filepickerSelect.onclick = () => {
    if (filePickerCallback) filePickerCallback(filePickerCurrentPath);
    els.filepickerOverlay.classList.add('hidden');
};
els.filepickerCancel.onclick = () => els.filepickerOverlay.classList.add('hidden');
els.filepickerOverlay.querySelector('.modal-close').onclick = () => els.filepickerOverlay.classList.add('hidden');

els.btnBootstrap.onclick = () => {
    showModal('Bootstrap Preferences', `
        <div class="form-group"><label>Default shell</label><select id="bootstrap-shell"><option>fish</option><option>zsh</option></select></div>
        <div class="form-group"><label>Default editor</label><select id="bootstrap-editor"><option>neovim</option><option>helix</option></select></div>
        <div class="form-group"><label>Window manager</label><select id="bootstrap-wm"><option>sway</option><option>xfce</option></select></div>
    `, async () => {
        setBusy(true); setAction('Running bootstrap...');
        try {
            await API.post('/api/bootstrap', {
                shell: document.getElementById('bootstrap-shell').value,
                editor: document.getElementById('bootstrap-editor').value,
                windowManager: document.getElementById('bootstrap-wm').value,
            });
            showToast('Bootstrap started');
        } catch (err) {
            showToast('Bootstrap failed: ' + err.message, 'error');
            setAction('Bootstrap failed: ' + err.message);
        }
        setBusy(false);
    });
};

els.btnStart.onclick = async () => {
    setBusy(true); setAction('Starting VM...');
    try { await API.post('/api/start'); showToast('VM start initiated'); }
    catch (err) { showToast('Start failed: ' + err.message, 'error'); setAction('Start failed: ' + err.message); }
    setBusy(false);
};

els.btnStop.onclick = async () => {
    setBusy(true); setAction('Stopping VM...');
    try { await API.post('/api/stop'); showToast('VM stop initiated'); }
    catch (err) { showToast('Stop failed: ' + err.message, 'error'); setAction('Stop failed: ' + err.message); }
    setBusy(false);
};

els.btnDestroy.onclick = () => {
    if (!confirm('Stop the VM and delete generated files?')) return;
    setBusy(true); setAction('Destroying VM...');
    API.post('/api/destroy')
        .then(() => showToast('Destroy initiated'))
        .catch(err => { showToast('Destroy failed: ' + err.message, 'error'); setAction('Destroy failed: ' + err.message); })
        .finally(() => setBusy(false));
};

async function loadTunnels() {
    try {
        const data = await API.get('/api/tunnels');
        const tunnels = data.tunnels || [];
        els.tunnelList.innerHTML = tunnels.length === 0
            ? '<div class="list-item">No tunnels configured.</div>'
            : tunnels.map(t => {
                const statusIcon = t.running ? '🟢' : '🔴';
                const mapping = t.Type === 'local'
                    ? `host:${t.LocalPort} → ${t.RemoteHost || 'localhost'}:${t.RemotePort}`
                    : `vm:${t.RemotePort} → host:${t.LocalPort}`;
                return `
                    <div class="list-item">
                        <div class="list-item-info">${statusIcon} ${t.Name} — ${mapping}</div>
                        <div class="list-item-actions">
                            <button class="btn btn-small" onclick="toggleTunnel('${t.ID}', ${t.running})">${t.running ? 'Stop' : 'Start'}</button>
                            <button class="btn btn-small btn-danger" onclick="removeTunnel('${t.ID}')">Remove</button>
                        </div>
                    </div>
                `;
            }).join('');
    } catch (err) {
        els.tunnelList.innerHTML = '<div class="list-item">Error loading tunnels</div>';
    }
}

window.toggleTunnel = async (id, isRunning) => {
    try {
        await API.post(`/api/tunnels/${id}/${isRunning ? 'stop' : 'start'}`);
        showToast(`Tunnel ${isRunning ? 'stopped' : 'started'}`);
        loadTunnels();
    } catch (err) { showToast(err.message, 'error'); }
};

window.removeTunnel = async (id) => {
    if (!confirm('Remove this tunnel?')) return;
    try {
        await API.delete(`/api/tunnels/${id}`);
        showToast('Tunnel removed'); loadTunnels();
    } catch (err) { showToast(err.message, 'error'); }
};

els.btnAddTunnel.onclick = () => {
    showModal('Add Tunnel', `
        <div class="form-group"><label>Name</label><input id="tunnel-name" placeholder="Tunnel name"></div>
        <div class="form-group"><label>Type</label><select id="tunnel-type"><option value="local">Local Forward</option><option value="remote">Remote Forward</option></select></div>
        <div class="form-group"><label>Local port</label><input id="tunnel-local" type="number" placeholder="Local port"></div>
        <div class="form-group"><label>Remote port</label><input id="tunnel-remote" type="number" placeholder="Remote port"></div>
        <div class="form-group"><label>Remote host</label><input id="tunnel-host" placeholder="localhost"></div>
        <div class="form-group"><label><input id="tunnel-autostart" type="checkbox"> Auto-start when VM runs</label></div>
    `, async () => {
        try {
            await API.post('/api/tunnels', {
                name: document.getElementById('tunnel-name').value,
                type: document.getElementById('tunnel-type').value,
                localPort: parseInt(document.getElementById('tunnel-local').value),
                remotePort: parseInt(document.getElementById('tunnel-remote').value),
                remoteHost: document.getElementById('tunnel-host').value,
                autoStart: document.getElementById('tunnel-autostart').checked,
            });
            showToast('Tunnel added'); loadTunnels();
        } catch (err) { showToast(err.message, 'error'); }
    });
};

async function loadSync() {
    try {
        const data = await API.get('/api/sync');
        const pairs = data.Pairs || [];
        els.syncList.innerHTML = pairs.length === 0
            ? '<div class="list-item">No sync pairs configured.</div>'
            : pairs.map(p => `
                <div class="list-item">
                    <div class="list-item-info">${p.ID} — ${p.HostPath} ↔ ${p.VMPath}</div>
                    <div class="list-item-actions">
                        <button class="btn btn-small" onclick="runSync('${p.ID}')">Sync</button>
                        ${p.Mode === 'copy' ? `<button class="btn btn-small" onclick="showHistory('${p.ID}')">History</button>` : ''}
                        <button class="btn btn-small btn-danger" onclick="removeSync('${p.ID}')">Remove</button>
                    </div>
                </div>
            `).join('');
    } catch (err) {
        els.syncList.innerHTML = '<div class="list-item">Error loading sync pairs</div>';
    }
}

window.runSync = async (id) => {
    try {
        const res = await API.post(`/api/sync/${id}/run`);
        showToast(res.message || 'Sync started');
    } catch (err) { showToast(err.message, 'error'); }
};

window.showHistory = async (id) => {
    try {
        const backups = await API.get(`/api/sync/${id}/history`);
        const html = backups.length === 0
            ? '<p>No backups found.</p>'
            : '<ul>' + backups.map(b => `<li>${new Date(b).toISOString()}</li>`).join('') + '</ul>';
        showModal('Backup History', html, () => {});
        els.modalConfirm.classList.add('hidden');
    } catch (err) { showToast(err.message, 'error'); }
};

window.removeSync = async (id) => {
    if (!confirm('Remove this sync pair?')) return;
    try {
        await API.delete(`/api/sync/${id}`);
        showToast('Sync pair removed'); loadSync();
    } catch (err) { showToast(err.message, 'error'); }
};

els.btnAddSync.onclick = () => {
    showModal('Add Sync Pair', `
        <div class="form-group"><label>Mode</label><select id="sync-mode"><option value="git">Git</option><option value="copy">Copy</option></select></div>
        <div class="form-group"><label>Host directory</label><input id="sync-host" placeholder="Host directory path"></div>
        <div class="form-group">
            <label>VM directory</label>
            <div style="display:flex;gap:8px;"><input id="sync-vm" placeholder="VM directory path" style="flex:1;"><button class="btn btn-small" onclick="pickVMPath()">Browse</button></div>
        </div>
        <div class="form-group" id="git-fields">
            <label>Bare repo path</label>
            <div style="display:flex;gap:8px;"><input id="sync-bare" placeholder="Bare repo path on VM (optional)" style="flex:1;"><button class="btn btn-small" onclick="pickBareRepoPath()">Browse</button></div>
        </div>
        <div class="form-group hidden" id="copy-fields">
            <label>Direction</label>
            <select id="sync-direction"><option value="host-to-vm">Host → VM</option><option value="vm-to-host">VM → Host</option><option value="bidirectional">Bidirectional</option></select>
        </div>
    `, async () => {
        const mode = document.getElementById('sync-mode').value;
        const req = { mode, hostPath: document.getElementById('sync-host').value, vmPath: document.getElementById('sync-vm').value };
        if (mode === 'git') req.bareRepoPath = document.getElementById('sync-bare').value;
        else req.direction = document.getElementById('sync-direction').value;
        try {
            await API.post('/api/sync', req);
            showToast('Sync pair added'); loadSync();
        } catch (err) { showToast(err.message, 'error'); }
    });

    document.getElementById('sync-mode').onchange = (e) => {
        const isGit = e.target.value === 'git';
        document.getElementById('git-fields').classList.toggle('hidden', !isGit);
        document.getElementById('copy-fields').classList.toggle('hidden', isGit);
    };
};

window.pickVMPath = () => openFilePicker('/home/vm', (path) => { document.getElementById('sync-vm').value = path; });
window.pickBareRepoPath = () => openFilePicker('/home/vm/repos', (path) => { document.getElementById('sync-bare').value = path; });

refreshStatus();
loadTunnels();
loadSync();
refreshInterval = setInterval(() => { refreshStatus(); loadTunnels(); loadSync(); }, 5000);
