// MeshVPN Client Frontend

// Состояние приложения
let appState = {
    connected: false,
    networkID: null,
    virtualIP: null,
    peerID: null
};

// Инициализация
document.addEventListener('DOMContentLoaded', async () => {
    await loadConfig();
    updateUI();
    
    // Слушаем события от Go
    if (window.runtime) {
        window.runtime.EventsOn('network:joined', (data) => {
            console.log('Joined network:', data);
            onNetworkJoined(data);
        });
        
        window.runtime.EventsOn('network:left', () => {
            onNetworkLeft();
        });
        
        window.runtime.EventsOn('peers:updated', (peers) => {
            updatePeersList(peers);
        });
    }
});

// Загрузка конфигурации
async function loadConfig() {
    try {
        if (window.go && window.go.app && window.go.app.App) {
            const config = await window.go.app.App.GetConfig();
            document.getElementById('serverURL').value = config.server_url || 'http://localhost:8080';
            document.getElementById('settingsServerURL').value = config.server_url || 'http://localhost:8080';
            document.getElementById('autoStart').checked = config.auto_start || false;
            document.getElementById('minimizeToTray').checked = config.minimize_to_tray !== false;
        
            const appInfo = await window.go.app.App.GetAppInfo();
            document.getElementById('platform').textContent = appInfo.platform || '-';
            document.getElementById('settingsPeerID').textContent = appInfo.peer_id || '-';
            appState.peerID = appInfo.peer_id;
        }
    } catch (e) {
        console.error('Failed to load config:', e);
    }
}

// Обновление UI
function updateUI() {
    const statusEl = document.getElementById('connectionStatus');
    const dot = statusEl.querySelector('.status-dot');
    const text = statusEl.querySelector('span:last-child');
    
    if (appState.connected) {
        dot.className = 'status-dot online';
        text.textContent = 'Подключен';
        showScreen('networkScreen');
    } else {
        dot.className = 'status-dot offline';
        text.textContent = 'Не подключен';
        showScreen('connectScreen');
    }
}

// Показать экран
function showScreen(screenId) {
    document.querySelectorAll('.screen').forEach(s => s.classList.remove('active'));
    document.getElementById(screenId).classList.add('active');
}

// Табы
function showTab(tab) {
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
    
    event.target.classList.add('active');
    document.getElementById(tab + 'Tab').classList.add('active');
}

// Подключение к сети
async function joinNetwork() {
    const serverURL = document.getElementById('serverURL').value.trim();
    const networkID = document.getElementById('networkID').value.trim();
    const password = document.getElementById('joinPassword').value;
    
    if (!serverURL || !networkID || !password) {
        showToast('Заполните все поля', 'error');
        return;
    }
    
    showToast('Подключение...');
    
    try {
        // Сохраняем URL сервера
        await saveServerURL(serverURL);
        
        const result = await window.go.app.App.JoinNetwork(networkID, password);
        console.log('Join result:', result);
        
        showToast('Подключено!', 'success');
    } catch (e) {
        console.error('Join failed:', e);
        showToast('Ошибка: ' + e.message, 'error');
    }
}

// Создание сети
async function createNetwork() {
    const serverURL = document.getElementById('serverURL').value.trim();
    const name = document.getElementById('networkName').value.trim();
    const password = document.getElementById('createPassword').value;
    
    if (!serverURL || !name || !password) {
        showToast('Заполните все поля', 'error');
        return;
    }
    
    if (password.length < 6) {
        showToast('Пароль должен быть минимум 6 символов', 'error');
        return;
    }
    
    showToast('Создание сети...');
    
    try {
        await saveServerURL(serverURL);
        
        const result = await window.go.app.App.CreateNetwork(name, password);
        console.log('Create result:', result);
        
        const networkID = result.network_id;
        
        // Автоматически подключаемся к созданной сети
        const joinResult = await window.go.app.App.JoinNetwork(networkID, password);
        console.log('Auto-join result:', joinResult);
        
        showToast(`Сеть создана! ID: ${networkID}`, 'success');
    } catch (e) {
        console.error('Create failed:', e);
        showToast('Ошибка: ' + e.message, 'error');
    }
}

// Отключение
async function leaveNetwork() {
    try {
        await window.go.app.App.LeaveNetwork();
        showToast('Отключено', 'success');
    } catch (e) {
        showToast('Ошибка: ' + e.message, 'error');
    }
}

// Обработчики событий
function onNetworkJoined(data) {
    appState.connected = true;
    appState.networkID = data.network_id;
    appState.virtualIP = data.virtual_ip;
    
    document.getElementById('networkIdDisplay').textContent = data.network_id;
    document.getElementById('virtualIP').textContent = data.virtual_ip;
    document.getElementById('peerID').textContent = appState.peerID;
    
    updateUI();
    updatePeersList(data.peers);
}

function onNetworkLeft() {
    appState.connected = false;
    appState.networkID = null;
    appState.virtualIP = null;
    updateUI();
}

// Обновление списка пиров
function updatePeersList(peers) {
    const list = document.getElementById('peersList');
    
    if (!peers || peers.length === 0) {
        list.innerHTML = '<div class="empty">Нет других устройств</div>';
        return;
    }
    
    list.innerHTML = peers.map(peer => `
        <div class="peer-item">
            <div class="peer-info">
                <div class="peer-ip">${peer.virtual_ip}</div>
                <div class="peer-id">${peer.id.substring(0, 8)}...</div>
            </div>
            <div class="peer-status ${peer.connected ? 'online' : 'offline'}">
                <span class="status-dot ${peer.connected ? 'online' : 'offline'}"></span>
                ${peer.connected ? 'Online' : 'Offline'}
            </div>
        </div>
    `).join('');
}

// Пинг
async function sendPing() {
    const target = document.getElementById('pingTarget').value.trim();
    if (!target) {
        showToast('Введите IP адрес', 'error');
        return;
    }
    
    const resultEl = document.getElementById('pingResult');
    resultEl.textContent = 'Пинг...';
    
    try {
        const result = await window.go.app.App.SendTestPing(target);
        resultEl.textContent = result;
    } catch (e) {
        resultEl.textContent = 'Ошибка: ' + e.message;
    }
}

// Тест соединения
async function testConnection() {
    const serverURL = document.getElementById('serverURL').value.trim();
    if (!serverURL) {
        showToast('Введите URL сервера', 'error');
        return;
    }
    
    showToast('Тестирование...');
    
    try {
        const result = await window.go.app.App.TestServerConnection(serverURL);
        showToast('Сервер доступен!', 'success');
        console.log('Server info:', result);
    } catch (e) {
        showToast('Сервер недоступен: ' + e.message, 'error');
    }
}

// Настройки
function showSettings() {
    showScreen('settingsScreen');
}

function showConnect() {
    if (appState.connected) {
        showScreen('networkScreen');
    } else {
        showScreen('connectScreen');
    }
}

async function saveSettings() {
    const config = {
        server_url: document.getElementById('settingsServerURL').value.trim(),
        auto_start: document.getElementById('autoStart').checked,
        minimize_to_tray: document.getElementById('minimizeToTray').checked
    };
    
    try {
        await window.go.app.App.SaveConfig(config);
        showToast('Настройки сохранены', 'success');
        showConnect();
    } catch (e) {
        showToast('Ошибка сохранения: ' + e.message, 'error');
    }
}

async function saveServerURL(url) {
    try {
        const config = await window.go.app.App.GetConfig();
        config.server_url = url;
        await window.go.app.App.SaveConfig(config);
    } catch (e) {
        console.error('Failed to save URL:', e);
    }
}

// Копирование ID
async function copyNetworkID() {
    if (!appState.networkID) return;
    
    try {
        await navigator.clipboard.writeText(appState.networkID);
        showToast('ID скопирован в буфер обмена', 'success');
    } catch (e) {
        showToast('Не удалось скопировать', 'error');
    }
}

// Toast уведомления
function showToast(message, type = '') {
    const toast = document.getElementById('toast');
    toast.textContent = message;
    toast.className = 'toast ' + type;
    
    setTimeout(() => toast.classList.add('show'), 10);
    setTimeout(() => toast.classList.remove('show'), 3000);
}
