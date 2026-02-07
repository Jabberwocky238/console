import type { TerminalAPI } from '../types';
import { authAPI, getAuthState } from '../api';
import { credentialStore } from '../store';

export function helpCommand(terminal: TerminalAPI) {
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
  terminal.print('  gui                     - Switch to GUI mode');
  terminal.print('');
  terminal.print('  rdb list                - List all RDB resources');
  terminal.print('  rdb add                 - Add a new RDB resource');
  terminal.print('  rdb delete <id>         - Delete an RDB resource');
  terminal.print('');
  terminal.print('  kv list                 - List all KV resources');
  terminal.print('  kv add                  - Add a new KV resource');
  terminal.print('  kv delete <id>          - Delete a KV resource');
  terminal.print('');
  terminal.print('  worker list                      - List all workers');
  terminal.print('  worker add                       - Create a new worker');
  terminal.print('  worker <id>                      - Get worker details');
  terminal.print('  worker <id> delete               - Delete a worker');
  terminal.print('  worker <id> env                  - Show env vars');
  terminal.print('  worker <id> env set              - Set env var');
  terminal.print('  worker <id> env delete <key>     - Delete env var');
  terminal.print('  worker <id> secret               - Show secret keys');
  terminal.print('  worker <id> secret set           - Set secret');
  terminal.print('  worker <id> secret delete <key>  - Delete secret');
  terminal.print('');
  terminal.print('  domain list             - List all custom domains');
  terminal.print('  domain add              - Add a new custom domain');
  terminal.print('  domain get <id>         - Get domain status');
  terminal.print('  domain delete <id>      - Delete a custom domain');
  terminal.print('');
}

export async function registerCommand(terminal: TerminalAPI) {
  try {
    const email = await terminal.waitForInput('Enter email:');
    terminal.print('Sending verification code...', 'info');
    await authAPI.sendCode(email);
    terminal.print('Verification code sent to your email', 'success');

    const code = await terminal.waitForInput('Enter verification code:');
    const password = await terminal.waitForInput('Enter password:', true);
    const result = await authAPI.register(email, code, password);

    credentialStore.save(result.user_id, result.token, result.secret_key);

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
    terminal.print(`Registration failed: ${(error as Error).message}`, 'error');
  }
}

export async function loginCommand(terminal: TerminalAPI) {
  try {
    const email = await terminal.waitForInput('Enter email:');
    const password = await terminal.waitForInput('Enter password:', true);
    const result = await authAPI.login(email, password);

    let secretKey = credentialStore.getStoredSecretKey(result.user_id);
    if (secretKey) {
      terminal.print('Secret key loaded from localStorage', 'info');
    } else {
      terminal.print('No stored secret key found for this user', 'warning');
      secretKey = await terminal.waitForInput('Enter your secret key (sk_...):');
    }

    credentialStore.save(result.user_id, result.token, secretKey);

    terminal.print('', 'success');
    terminal.print('Login successful!', 'success');
    terminal.print(`User ID: ${result.user_id}`, 'info');
  } catch (error) {
    terminal.print(`Login failed: ${(error as Error).message}`, 'error');
  }
}

export function logoutCommand(terminal: TerminalAPI) {
  credentialStore.clear();
  terminal.print('Logged out successfully', 'success');
  terminal.print('Credentials cleared from localStorage', 'info');
}

export function whoamiCommand(terminal: TerminalAPI) {
  terminal.print(getAuthState().currentUser || 'guest', 'info');
}

export function statusCommand(terminal: TerminalAPI) {
  const { currentUser, token } = getAuthState();
  terminal.print('');
  terminal.print('=== System Status ===', 'info');
  terminal.print(`User: ${currentUser || 'Not logged in'}`, currentUser ? 'success' : 'warning');
  terminal.print(`Token: ${token ? 'Active' : 'None'}`, token ? 'success' : 'warning');
  terminal.print('');
}
