const API_BASE = '/api';
const TOKEN_KEY = 'jwt_token';

let refreshTimer = null;

function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

function setToken(token) {
  localStorage.setItem(TOKEN_KEY, token);
}

function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

function showLogin() {
  document.getElementById('screen-login').style.display = 'grid';
  document.getElementById('screen-dashboard').style.display = 'none';
}

function showDashboard() {
  document.getElementById('screen-login').style.display = 'none';
  document.getElementById('screen-dashboard').style.display = 'block';
  document.getElementById('login-error').textContent = '';
}

function logout() {
  clearToken();
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
  showLogin();
}

async function apiFetch(path, options = {}) {
  const headers = new Headers(options.headers || {});

  if (!headers.has('Content-Type') && options.body) {
    headers.set('Content-Type', 'application/json');
  }

  const token = getToken();
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers
  });

  if (response.status === 401) {
    logout();
    throw new Error('unauthorized');
  }

  let data = null;
  const text = await response.text();
  if (text) {
    data = JSON.parse(text);
  }

  if (!response.ok) {
    const message = data && data.error ? data.error : 'Request failed';
    throw new Error(message);
  }

  return data;
}

async function handleLogin(event) {
  event.preventDefault();

  const username = document.getElementById('input-username').value.trim();
  const password = document.getElementById('input-password').value;
  const errorEl = document.getElementById('login-error');

  errorEl.textContent = '';

  try {
    const data = await fetch(`${API_BASE}/auth/login`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ username, password })
    }).then(async (response) => {
      let payload = null;
      const text = await response.text();
      if (text) {
        payload = JSON.parse(text);
      }
      if (!response.ok) {
        const message = payload && payload.error ? payload.error : 'Login failed';
        throw new Error(message);
      }
      return payload;
    });

    setToken(data.token);
    showDashboard();
    loadAll();

    if (!refreshTimer) {
      refreshTimer = setInterval(loadAll, 10000);
    }
  } catch (error) {
    errorEl.textContent = error.message || 'Login failed';
  }
}

async function loadAgentStatus(agentId) {
  return apiFetch(`/agents/${agentId}/status`);
}

function createEmptyState(message) {
  const empty = document.createElement('div');
  empty.className = 'empty-state';
  empty.textContent = message;
  return empty;
}

function createAgentCard(agent, status) {
  const card = document.createElement('article');
  card.className = 'agent-card';

  const header = document.createElement('div');
  header.className = 'agent-card-header';

  const meta = document.createElement('div');
  meta.className = 'agent-meta';

  const title = document.createElement('div');
  title.className = 'agent-title';
  title.textContent = `Agent #${agent.id}`;

  const ip = document.createElement('div');
  ip.textContent = `IP: ${agent.ip}`;

  const connected = document.createElement('div');
  connected.textContent = `Connected: ${agent.connected_at}`;

  meta.append(title, ip, connected);

  const state = document.createElement('span');
  const stateValue = status && status.state ? status.state : 'UNKNOWN';
  state.className = `agent-state ${stateValue.toLowerCase() === 'isolated' ? 'isolated' : 'normal'}`;
  state.textContent = stateValue;

  header.append(meta, state);

  const whitelist = document.createElement('div');
  whitelist.className = 'agent-whitelist';
  if (status && Array.isArray(status.whitelist) && status.whitelist.length > 0) {
    whitelist.textContent = `Whitelist: ${status.whitelist.join(', ')}`;
  } else {
    whitelist.textContent = 'Whitelist: none';
  }

  const actions = document.createElement('div');
  actions.className = 'agent-actions';

  const isolateButton = document.createElement('button');
  isolateButton.className = 'btn btn-isolate';
  isolateButton.type = 'button';
  isolateButton.textContent = 'Isolate';
  isolateButton.addEventListener('click', () => isolateAgent(agent.id));

  const releaseButton = document.createElement('button');
  releaseButton.className = 'btn btn-release';
  releaseButton.type = 'button';
  releaseButton.textContent = 'Release';
  releaseButton.addEventListener('click', () => releaseAgent(agent.id));

  actions.append(isolateButton, releaseButton);
  card.append(header, whitelist, actions);
  return card;
}

async function loadAgents() {
  const agentsList = document.getElementById('agents-list');
  if (!agentsList.hasChildNodes()) {
    agentsList.replaceChildren(createEmptyState('Loading agents...'));
  }

  try {
    const data = await apiFetch('/agents');
    const agents = data && Array.isArray(data.agents) ? data.agents : [];

    if (agents.length === 0) {
      agentsList.replaceChildren(createEmptyState('No agents connected.'));
      return;
    }

    const statuses = await Promise.all(
      agents.map(async (agent) => {
        try {
          return await loadAgentStatus(agent.id);
        } catch (error) {
          return { state: 'UNKNOWN', whitelist: [] };
        }
      })
    );

    const cards = agents.map((agent, index) => createAgentCard(agent, statuses[index]));
    agentsList.replaceChildren(...cards);
  } catch (error) {
    agentsList.replaceChildren(createEmptyState(error.message || 'Failed to load agents.'));
  }
}

function renderEvents(events) {
  const wrap = document.createElement('div');
  wrap.className = 'events-table-wrap';

  const table = document.createElement('table');
  table.className = 'events-table';

  const thead = document.createElement('thead');
  const headRow = document.createElement('tr');
  ['ID', 'Agent', 'Command', 'Payload', 'Time'].forEach((label) => {
    const th = document.createElement('th');
    th.scope = 'col';
    th.textContent = label;
    headRow.appendChild(th);
  });
  thead.appendChild(headRow);

  const tbody = document.createElement('tbody');
  events.forEach((event) => {
    const row = document.createElement('tr');
    const values = [
      event.id,
      event.agent_id,
      event.command,
      event.payload || '-',
      event.created_at
    ];

    values.forEach((value) => {
      const td = document.createElement('td');
      td.textContent = String(value);
      row.appendChild(td);
    });

    tbody.appendChild(row);
  });

  table.append(thead, tbody);
  wrap.appendChild(table);
  return wrap;
}

async function loadEvents() {
  const eventsList = document.getElementById('events-list');
  if (!eventsList.hasChildNodes()) {
    eventsList.replaceChildren(createEmptyState('Loading events...'));
  }

  try {
    const data = await apiFetch('/events?limit=20');
    const events = data && Array.isArray(data.events) ? data.events : [];

    if (events.length === 0) {
      eventsList.replaceChildren(createEmptyState('No events recorded yet.'));
      return;
    }

    eventsList.replaceChildren(renderEvents(events));
  } catch (error) {
    eventsList.replaceChildren(createEmptyState(error.message || 'Failed to load events.'));
  }
}

function loadAll() {
  loadAgents();
  loadEvents();
}

async function isolateAgent(agentId) {
  const input = window.prompt('Enter whitelist IPs separated by commas');
  if (input === null) {
    return;
  }

  const ips = input
    .split(',')
    .map((ip) => ip.trim())
    .filter((ip) => ip.length > 0);

  if (ips.length === 0) {
    return;
  }

  try {
    await apiFetch(`/agents/${agentId}/isolate`, {
      method: 'POST',
      body: JSON.stringify({ ips })
    });
    loadAgents();
    loadEvents();
  } catch (error) {
    window.alert(error.message || 'Failed to isolate agent.');
  }
}

async function releaseAgent(agentId) {
  try {
    await apiFetch(`/agents/${agentId}/release`, {
      method: 'POST'
    });
    loadAgents();
    loadEvents();
  } catch (error) {
    window.alert(error.message || 'Failed to release agent.');
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('form-login').addEventListener('submit', handleLogin);
  document.getElementById('btn-logout').addEventListener('click', logout);

  if (getToken()) {
    showDashboard();
    loadAll();
    refreshTimer = setInterval(loadAll, 10000);
  } else {
    showLogin();
  }
});
