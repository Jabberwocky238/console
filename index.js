// API Client
const API_BASE = window.location.origin;

// HMAC-SHA256 Signature helper
async function signData(data, secretKey) {
    const encoder = new TextEncoder();
    const keyData = encoder.encode(secretKey);
    const key = await crypto.subtle.importKey(
        'raw',
        keyData,
        { name: 'HMAC', hash: 'SHA-256' },
        false,
        ['sign']
    );
    const signature = await crypto.subtle.sign('HMAC', key, encoder.encode(data));
    // Base64 URL encoding (no padding)
    return btoa(String.fromCharCode(...new Uint8Array(signature)))
        .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

async function apiCall(endpoint, method = 'GET', data = null, requireSignature = false) {
    const options = {
        method,
        headers: {
            'Content-Type': 'application/json',
        }
    };

    if (window.token) {
        options.headers['Authorization'] = `Bearer ${window.token}`;
    }

    const bodyStr = data ? JSON.stringify(data) : '';
    if (data) {
        options.body = bodyStr;
    }

    // Add HMAC signature for sensitive operations
    if (requireSignature && window.secretKey && window.currentUser) {
        const timestamp = Date.now().toString();
        const signature = await signData(bodyStr + timestamp, window.secretKey);
        options.headers['X-Combinator-Signature'] = signature;
        options.headers['X-Combinator-User-ID'] = window.currentUser;
        options.headers['X-Combinator-Timestamp'] = timestamp;
    }

    const response = await fetch(API_BASE + endpoint, options);
    const result = await response.json();

    if (!response.ok) {
        throw new Error(result.error || 'Request failed');
    }

    return result;
}

// Auth API
const authAPI = {
    async sendCode(email) {
        return apiCall('/auth/send-code', 'POST', { email });
    },

    async register(email, code, password) {
        return apiCall('/auth/register', 'POST', { email, code, password });
    },

    async login(email, password) {
        return apiCall('/auth/login', 'POST', { email, password });
    }
};

// RDB API
const rdbAPI = {
    async list() {
        return apiCall('/api/rdb', 'GET');
    },

    async create(name, rdb_type, url) {
        return apiCall('/api/rdb', 'POST', { name, rdb_type, url }, true);
    },

    async delete(id) {
        return apiCall(`/api/rdb/${id}`, 'DELETE', {}, true);
    }
};

// KV API
const kvAPI = {
    async list() {
        return apiCall('/api/kv', 'GET');
    },

    async create(name, kv_type, url) {
        return apiCall('/api/kv', 'POST', { name, kv_type, url }, true);
    },

    async delete(id) {
        return apiCall(`/api/kv/${id}`, 'DELETE', {}, true);
    }
};

// Worker API
const workerAPI = {
    async list() {
        return apiCall('/api/worker', 'GET');
    },

    async get(id) {
        return apiCall(`/api/worker/${id}`, 'GET');
    },

    async register(worker_id, image, port) {
        return apiCall('/api/worker', 'POST', { worker_id, image, port }, true);
    },

    async delete(id) {
        return apiCall(`/api/worker/${id}`, 'DELETE', {}, true);
    }
};

// Credential Storage
const credentialStore = {
    save(userId, token, secretKey) {
        const data = { userId, token, secretKey };
        localStorage.setItem('console_credentials', JSON.stringify(data));
        window.currentUser = userId;
        window.token = token;
        window.secretKey = secretKey;
    },

    load() {
        const stored = localStorage.getItem('console_credentials');
        if (stored) {
            try {
                const data = JSON.parse(stored);
                window.currentUser = data.userId;
                window.token = data.token;
                window.secretKey = data.secretKey;
                return true;
            } catch (e) {
                return false;
            }
        }
        return false;
    },

    clear() {
        localStorage.removeItem('console_credentials');
        window.currentUser = null;
        window.token = null;
        window.secretKey = null;
    }
};

// Command Handlers
const commands = {
    help(terminal) {
        terminal.print('');
        terminal.print('Available Commands:', 'info');
        terminal.print('');
        terminal.print('  help                    - Show this help message');
        terminal.print('  clear                   - Clear the terminal');
        terminal.print('  register                - Register a new account');
        terminal.print('  login                   - Login to your account');
        terminal.print('  logout                  - Logout from your account');
        terminal.print('  whoami                  - Show current user');
        terminal.print('  status                  - Show current status');
        terminal.print('');
        terminal.print('  rdb list                - List all RDB resources');
        terminal.print('  rdb add                 - Add a new RDB resource');
        terminal.print('  rdb delete <id>         - Delete an RDB resource');
        terminal.print('');
        terminal.print('  kv list                 - List all KV resources');
        terminal.print('  kv add                  - Add a new KV resource');
        terminal.print('  kv delete <id>          - Delete a KV resource');
        terminal.print('');
        terminal.print('  worker list             - List all workers');
        terminal.print('  worker add              - Add a new worker');
        terminal.print('  worker delete <id>      - Delete a worker');
        terminal.print('');
    },

    async register(terminal) {
        try {
            const email = await terminal.waitForInput('Enter email:');

            terminal.print('Sending verification code...', 'info');
            await authAPI.sendCode(email);
            terminal.print('Verification code sent to your email', 'success');

            const code = await terminal.waitForInput('Enter verification code:');
            const password = await terminal.waitForInput('Enter password:', true);

            const result = await authAPI.register(email, code, password);

            // Save credentials to localStorage
            credentialStore.save(result.user_id, result.token, result.secret_key);
            terminal.updatePrompt();

            terminal.print('', 'success');
            terminal.print('Registration successful!', 'success');
            terminal.print(`User ID: ${result.user_id}`, 'info');
            terminal.print(`Email: ${result.email}`, 'info');
            terminal.print('');
            terminal.print('Secret key saved to localStorage', 'success');
            terminal.print('');
            terminal.print('=== IMPORTANT: Backup your secret key ===', 'warning');
            terminal.print('If you clear browser data, you will need this key!', 'warning');
            terminal.print('');
            terminal.print(result.secret_key, 'info');
        } catch (error) {
            terminal.print(`Registration failed: ${error.message}`, 'error');
        }
    },

    async login(terminal) {
        try {
            const email = await terminal.waitForInput('Enter email:');
            const password = await terminal.waitForInput('Enter password:', true);

            const result = await authAPI.login(email, password);

            // Check if we have stored secret key for this user
            const stored = localStorage.getItem('console_credentials');
            let secretKey = null;

            if (stored) {
                try {
                    const data = JSON.parse(stored);
                    if (data.userId === result.user_id && data.secretKey) {
                        secretKey = data.secretKey;
                        terminal.print('Secret key loaded from localStorage', 'info');
                    }
                } catch (e) {}
            }

            if (!secretKey) {
                terminal.print('No stored secret key found for this user', 'warning');
                secretKey = await terminal.waitForInput('Enter your secret key (sk_...):');
            }

            credentialStore.save(result.user_id, result.token, secretKey);
            terminal.updatePrompt();

            terminal.print('', 'success');
            terminal.print('Login successful!', 'success');
            terminal.print(`User ID: ${result.user_id}`, 'info');
        } catch (error) {
            terminal.print(`Login failed: ${error.message}`, 'error');
        }
    },

    logout(terminal) {
        credentialStore.clear();
        terminal.updatePrompt();
        terminal.print('Logged out successfully', 'success');
        terminal.print('Credentials cleared from localStorage', 'info');
    },

    whoami(terminal) {
        terminal.print(window.currentUser || 'guest', 'info');
    },

    status(terminal) {
        terminal.print('');
        terminal.print('=== System Status ===', 'info');
        terminal.print(`User: ${window.currentUser || 'Not logged in'}`, window.currentUser ? 'success' : 'warning');
        terminal.print(`Token: ${window.token ? 'Active' : 'None'}`, window.token ? 'success' : 'warning');
        terminal.print('');
    },

    async rdb(terminal, args) {
        if (!window.token) {
            terminal.print('Please login first', 'error');
            return;
        }

        const subcommand = args[0];

        switch (subcommand) {
            case 'list':
                await rdbCommands.list(terminal);
                break;
            case 'add':
                await rdbCommands.add(terminal);
                break;
            case 'delete':
                if (!args[1]) {
                    terminal.print('Usage: rdb delete <id>', 'error');
                    return;
                }
                await rdbCommands.delete(terminal, args[1]);
                break;
            default:
                terminal.print('Usage: rdb [list|add|delete]', 'error');
        }
    },

    async kv(terminal, args) {
        if (!window.token) {
            terminal.print('Please login first', 'error');
            return;
        }

        const subcommand = args[0];

        switch (subcommand) {
            case 'list':
                await kvCommands.list(terminal);
                break;
            case 'add':
                await kvCommands.add(terminal);
                break;
            case 'delete':
                if (!args[1]) {
                    terminal.print('Usage: kv delete <id>', 'error');
                    return;
                }
                await kvCommands.delete(terminal, args[1]);
                break;
            default:
                terminal.print('Usage: kv [list|add|delete]', 'error');
        }
    },

    async worker(terminal, args) {
        if (!window.token) {
            terminal.print('Please login first', 'error');
            return;
        }

        const subcommand = args[0];

        switch (subcommand) {
            case 'list':
                await workerCommands.list(terminal);
                break;
            case 'add':
                await workerCommands.add(terminal);
                break;
            case 'delete':
                if (!args[1]) {
                    terminal.print('Usage: worker delete <id>', 'error');
                    return;
                }
                await workerCommands.delete(terminal, args[1]);
                break;
            default:
                terminal.print('Usage: worker [list|add|delete]', 'error');
        }
    }
};

// RDB Commands
const rdbCommands = {
    async list(terminal) {
        try {
            const result = await rdbAPI.list();
            terminal.print('');
            terminal.print('=== RDB Resources ===', 'info');
            if (result.rdbs && result.rdbs.length > 0) {
                result.rdbs.forEach(rdb => {
                    const statusClass = rdb.status === 'active' ? 'success' : (rdb.status === 'error' ? 'error' : 'warning');
                    terminal.print(`ID: ${rdb.id}`, 'success');
                    terminal.print(`  Name: ${rdb.name}`);
                    terminal.print(`  Type: ${rdb.rdb_type}`);
                    terminal.print(`  URL: ${rdb.url}`);
                    terminal.print(`  Status: ${rdb.status}`, statusClass);
                    if (rdb.error_msg) {
                        terminal.print(`  Error: ${rdb.error_msg}`, 'error');
                    }
                    terminal.print('');
                });
            } else {
                terminal.print('No RDB resources found', 'warning');
            }
        } catch (error) {
            terminal.print(`Failed to list RDBs: ${error.message}`, 'error');
        }
    },

    async add(terminal) {
        try {
            const name = await terminal.waitForInput('Enter RDB name:');
            const type = await terminal.waitForInput('Enter RDB type (sqlite/postgres/mysql):');
            const url = await terminal.waitForInput('Enter RDB URL:');

            const result = await rdbAPI.create(name, type, url);

            terminal.print('', 'success');
            terminal.print(`ID: ${result.id}`, 'info');
            if (result.error) {
                terminal.print(result.error, 'warning');
            } else {
                terminal.print(result.message, 'success');
            }
        } catch (error) {
            terminal.print(`Failed to create RDB: ${error.message}`, 'error');
        }
    },

    async delete(terminal, id) {
        try {
            await rdbAPI.delete(id);
            terminal.print('RDB deleted successfully', 'success');
        } catch (error) {
            terminal.print(`Failed to delete RDB: ${error.message}`, 'error');
        }
    }
};

// KV Commands
const kvCommands = {
    async list(terminal) {
        try {
            const result = await kvAPI.list();
            terminal.print('');
            terminal.print('=== KV Resources ===', 'info');
            if (result.kvs && result.kvs.length > 0) {
                result.kvs.forEach(kv => {
                    const statusClass = kv.status === 'active' ? 'success' : (kv.status === 'error' ? 'error' : 'warning');
                    terminal.print(`ID: ${kv.id}`, 'success');
                    terminal.print(`  Name: ${kv.name}`);
                    terminal.print(`  Type: ${kv.kv_type}`);
                    terminal.print(`  URL: ${kv.url}`);
                    terminal.print(`  Status: ${kv.status}`, statusClass);
                    if (kv.error_msg) {
                        terminal.print(`  Error: ${kv.error_msg}`, 'error');
                    }
                    terminal.print('');
                });
            } else {
                terminal.print('No KV resources found', 'warning');
            }
        } catch (error) {
            terminal.print(`Failed to list KVs: ${error.message}`, 'error');
        }
    },

    async add(terminal) {
        try {
            const name = await terminal.waitForInput('Enter KV name:');
            const type = await terminal.waitForInput('Enter KV type (redis/memory):');
            const url = await terminal.waitForInput('Enter KV URL:');

            const result = await kvAPI.create(name, type, url);

            terminal.print('', 'success');
            terminal.print(`ID: ${result.id}`, 'info');
            if (result.error) {
                terminal.print(result.error, 'warning');
            } else {
                terminal.print(result.message, 'success');
            }
        } catch (error) {
            terminal.print(`Failed to create KV: ${error.message}`, 'error');
        }
    },

    async delete(terminal, id) {
        try {
            await kvAPI.delete(id);
            terminal.print('KV deleted successfully', 'success');
        } catch (error) {
            terminal.print(`Failed to delete KV: ${error.message}`, 'error');
        }
    }
};

// Worker Commands
const workerCommands = {
    async list(terminal) {
        try {
            const result = await workerAPI.list();
            terminal.print('');
            terminal.print('=== Workers ===', 'info');
            if (result && result.length > 0) {
                result.forEach(w => {
                    const statusClass = w.status === 'active' ? 'success' : (w.status === 'error' ? 'error' : 'warning');
                    terminal.print(`ID: ${w.worker_id}`, 'success');
                    terminal.print(`  Image: ${w.image}`);
                    terminal.print(`  Port: ${w.port}`);
                    terminal.print(`  Status: ${w.status}`, statusClass);
                    if (w.error_msg) {
                        terminal.print(`  Error: ${w.error_msg}`, 'error');
                    }
                    terminal.print('');
                });
            } else {
                terminal.print('No workers found', 'warning');
            }
        } catch (error) {
            terminal.print(`Failed to list workers: ${error.message}`, 'error');
        }
    },

    async add(terminal) {
        try {
            const worker_id = await terminal.waitForInput('Enter worker ID:');
            const image = await terminal.waitForInput('Enter Docker image:');
            const port = await terminal.waitForInput('Enter port:');

            const result = await workerAPI.register(worker_id, image, parseInt(port));

            terminal.print('', 'success');
            terminal.print(`Worker ID: ${result.worker_id}`, 'info');
            terminal.print(`Status: ${result.status}`, result.status === 'active' ? 'success' : 'warning');
            if (result.domain) {
                terminal.print(`Domain: ${result.domain}`, 'info');
            }
            if (result.error) {
                terminal.print(`Error: ${result.error}`, 'error');
            }
        } catch (error) {
            terminal.print(`Failed to create worker: ${error.message}`, 'error');
        }
    },

    async delete(terminal, id) {
        try {
            await workerAPI.delete(id);
            terminal.print('Worker deleted successfully', 'success');
        } catch (error) {
            terminal.print(`Failed to delete worker: ${error.message}`, 'error');
        }
    }
};
