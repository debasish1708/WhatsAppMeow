const API_BASE = 'http://localhost:8080';
let pollingInterval = null;
let historyInterval = null;

// DOM Elements
const getEl = (id) => document.getElementById(id);

// Tab Switching
function openTab(tabId) {
    document.querySelectorAll('.tab-content').forEach(el => el.classList.add('hidden'));
    document.querySelectorAll('.tab-btn').forEach(el => el.classList.remove('active'));
    
    const targetTab = getEl(tabId);
    if (targetTab) targetTab.classList.remove('hidden');
    
    const btnId = 'tab-' + tabId.split('-')[0];
    const btn = getEl(btnId);
    if (btn) btn.classList.add('active');
    
    if (tabId === 'history-tab') {
        fetchHistory();
    }
}

// Start Login Process
function startLogin() {
    const loginBtn = getEl('login-btn');
    const statusMessage = getEl('status-message');
    
    if (loginBtn) loginBtn.classList.add('hidden');
    if (statusMessage) statusMessage.textContent = 'Initiating login...';
    
    checkLoginStatus();
    if (!pollingInterval) pollingInterval = setInterval(checkLoginStatus, 2500);
}

async function checkLoginStatus() {
    try {
        const response = await fetch(`${API_BASE}/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({})
        });
        const data = await response.json();
        const statusMessage = getEl('status-message');

        switch (data.status) {
            case 'logged_in':
                handleSuccessfulLogin();
                break;
            case 'qr_ready':
                getEl('qr-image').src = data.qr_code_image;
                getEl('qr-container').classList.remove('hidden');
                if (statusMessage) statusMessage.textContent = 'Scan the QR code to log in.';
                break;
            case 'timeout':
                handleLoggedOut();
                if (statusMessage) statusMessage.textContent = 'QR code timed out. Click Login again.';
                break;
            default:
                if (statusMessage) statusMessage.textContent = data.message || 'Connecting...';
        }
    } catch (error) {
        console.error('Login poll error:', error);
    }
}

function handleSuccessfulLogin() {
    stopPolling();
    getEl('status-container').classList.add('hidden');
    getEl('qr-container').classList.add('hidden');
    getEl('success-container').classList.remove('hidden');
    getEl('tab-send').classList.remove('hidden');
    getEl('tab-history').classList.remove('hidden');
    
    // Start history polling (this also acts as a session check)
    if (!historyInterval) historyInterval = setInterval(fetchHistory, 5000);
}

function handleLoggedOut() {
    console.log("Session lost or logged out. Resetting UI.");
    stopPolling();
    if (historyInterval) { clearInterval(historyInterval); historyInterval = null; }
    
    getEl('tab-send').classList.add('hidden');
    getEl('tab-history').classList.add('hidden');
    getEl('success-container').classList.add('hidden');
    getEl('qr-container').classList.add('hidden');
    
    const statusContainer = getEl('status-container');
    const statusMessage = getEl('status-message');
    const loginBtn = getEl('login-btn');
    
    if (statusContainer) statusContainer.classList.remove('hidden');
    if (statusMessage) statusMessage.textContent = 'Ready to connect.';
    if (loginBtn) loginBtn.classList.remove('hidden');
    
    openTab('login-tab');
}

async function logout() {
    if (!confirm('Logout and unlink device?')) return;
    try {
        const response = await fetch(`${API_BASE}/logout`, { method: 'POST' });
        if (response.ok) handleLoggedOut();
    } catch (error) { console.error('Logout error:', error); }
}

function stopPolling() {
    if (pollingInterval) { clearInterval(pollingInterval); pollingInterval = null; }
}

async function handleFormSubmit(event) {
    event.preventDefault();
    await sendMessage();
    return false;
}

async function handleMediaSubmit(event) {
    event.preventDefault();
    await sendMedia();
    return false;
}

async function sendMedia() {
    const number = getEl('media-number').value.trim();
    const mediaType = getEl('media-type').value;
    const fileInput = getEl('media-file');
    const caption = getEl('media-caption').value.trim();
    const sendStatus = getEl('send-media-status');
    
    if (!number || !fileInput.files.length) return;

    const fullNumber = '91' + number;
    if (sendStatus) { sendStatus.textContent = 'Uploading and Sending...'; sendStatus.style.color = '#555'; }

    const formData = new FormData();
    formData.append('phone', fullNumber);
    formData.append('media_type', mediaType);
    formData.append('file', fileInput.files[0]);
    if (caption) formData.append('caption', caption);

    try {
        const response = await fetch(`${API_BASE}/send-media-message`, {
            method: 'POST',
            body: formData
        });
        const data = await response.json();

        if (response.ok && data.success) {
            if (sendStatus) { sendStatus.textContent = 'Media Sent!'; sendStatus.style.color = 'green'; }
            getEl('media-number').value = '';
            fileInput.value = '';
            getEl('media-caption').value = '';
            fetchHistory(); // Immediate refresh
        } else {
            if (sendStatus) { sendStatus.textContent = 'Error: ' + (data.message || 'Failed'); sendStatus.style.color = 'red'; }
            // If backend returns 401, session is dead
            if (response.status === 401) handleLoggedOut();
        }
    } catch (error) { if (sendStatus) sendStatus.textContent = 'Network error.'; }
}

async function sendMessage() {
    const number = getEl('mobile-number').value.trim();
    const message = getEl('message-text').value.trim();
    const sendStatus = getEl('send-status');
    
    if (!number || !message) return;

    const fullNumber = '91' + number;
    if (sendStatus) { sendStatus.textContent = 'Sending...'; sendStatus.style.color = '#555'; }

    try {
        const response = await fetch(`${API_BASE}/send`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ phone: fullNumber, message: message })
        });
        const data = await response.json();

        if (response.ok && data.success) {
            if (sendStatus) { sendStatus.textContent = 'Sent!'; sendStatus.style.color = 'green'; }
            getEl('mobile-number').value = '';
            getEl('message-text').value = '';
            fetchHistory(); // Immediate refresh
        } else {
            if (sendStatus) { sendStatus.textContent = 'Error: ' + (data.message || 'Failed'); sendStatus.style.color = 'red'; }
            // If backend returns 401, session is dead
            if (response.status === 401) handleLoggedOut();
        }
    } catch (error) { if (sendStatus) sendStatus.textContent = 'Network error.'; }
}

async function fetchHistory() {
    try {
        const response = await fetch(`${API_BASE}/history`);
        
        // CRITICAL: If backend says 401 Unauthorized, it means the phone unlinked or session died
        if (response.status === 401) {
            handleLoggedOut();
            return;
        }

        if (!response.ok) return;
        const messages = await response.json();
        renderHistory(messages);
    } catch (error) { 
        console.error('History fetch error:', error);
    }
}

function renderHistory(messages) {
    const list = getEl('history-list');
    if (!list) return;

    if (!messages || messages.length === 0) {
        list.innerHTML = '<p style="text-align:center; color:#888;">No messages yet.</p>';
        return;
    }

    const sorted = [...messages].reverse();

    list.innerHTML = sorted.map(item => `
        <div class="history-item ${item.type}">
            <div class="phone">${item.type === 'sent' ? 'To' : 'From'}: +${item.phone}</div>
            <div class="msg">${escapeHtml(item.message)}</div>
            <div class="status">${item.type.toUpperCase()} - ${item.timestamp}</div>
        </div>
    `).join('');
}

function clearHistory() {
    alert('Server-side history cannot be cleared from here yet.');
}

function escapeHtml(unsafe) {
    return String(unsafe).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
}

window.addEventListener('DOMContentLoaded', async () => {
    try {
        const response = await fetch(`${API_BASE}/status`);
        const data = await response.json();
        if (data.logged_in) handleSuccessfulLogin();
        else handleLoggedOut();
    } catch (e) { handleLoggedOut(); }
});