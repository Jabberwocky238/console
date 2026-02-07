import type { TerminalAPI } from '../types';
import { rdbAPI, kvAPI, workerAPI, domainAPI, getAuthState } from '../api';

function requireAuth(terminal: TerminalAPI): boolean {
  if (!getAuthState().token) {
    terminal.print('Please login first', 'error');
    return false;
  }
  return true;
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`;
}

// === RDB Commands ===

async function rdbList(terminal: TerminalAPI) {
  try {
    const result = await rdbAPI.list();
    terminal.print('');
    terminal.print('=== RDB Resources ===', 'info');
    if (result.database_size !== undefined) {
      terminal.print(`Database Total: ${formatBytes(result.database_size)}`, 'info');
    }
    terminal.print('');
    if (result.rdbs && result.rdbs.length > 0) {
      result.rdbs.forEach((rdb: { id: string; name: string; url: string; size: number }) => {
        terminal.print(`ID: ${rdb.id}`, 'success');
        terminal.print(`  Name: ${rdb.name}`);
        terminal.print(`  URL: ${rdb.url}`);
        terminal.print(`  Size: ${formatBytes(rdb.size)}`);
        terminal.print('');
      });
    } else {
      terminal.print('No RDB resources found', 'warning');
    }
  } catch (error) {
    terminal.print(`Failed to list RDBs: ${(error as Error).message}`, 'error');
  }
}

async function rdbAdd(terminal: TerminalAPI) {
  try {
    const name = await terminal.waitForInput('Enter RDB name:');
    const result = await rdbAPI.create(name);
    terminal.print('', 'success');
    terminal.print(`ID: ${result.id}`, 'info');
    terminal.print(result.message, 'success');
  } catch (error) {
    terminal.print(`Failed to create RDB: ${(error as Error).message}`, 'error');
  }
}

async function rdbDelete(terminal: TerminalAPI, id: string) {
  try {
    await rdbAPI.delete(id);
    terminal.print('RDB deleted successfully', 'success');
  } catch (error) {
    terminal.print(`Failed to delete RDB: ${(error as Error).message}`, 'error');
  }
}

export async function rdbCommand(terminal: TerminalAPI, args: string[]) {
  if (!requireAuth(terminal)) return;
  switch (args[0]) {
    case 'list': await rdbList(terminal); break;
    case 'add': await rdbAdd(terminal); break;
    case 'delete':
      if (!args[1]) { terminal.print('Usage: rdb delete <id>', 'error'); return; }
      await rdbDelete(terminal, args[1]); break;
    default: terminal.print('Usage: rdb [list|add|delete]', 'error');
  }
}

// === KV Commands ===

async function kvList(terminal: TerminalAPI) {
  try {
    const result = await kvAPI.list();
    terminal.print('');
    terminal.print('=== KV Resources ===', 'info');
    if (result.kvs && result.kvs.length > 0) {
      result.kvs.forEach((kv: { id: string; kv_type: string; url: string }) => {
        terminal.print(`ID: ${kv.id}`, 'success');
        terminal.print(`  Type: ${kv.kv_type}`);
        terminal.print(`  URL: ${kv.url}`);
        terminal.print('');
      });
    } else {
      terminal.print('No KV resources found', 'warning');
    }
  } catch (error) {
    terminal.print(`Failed to list KVs: ${(error as Error).message}`, 'error');
  }
}

async function kvAdd(terminal: TerminalAPI) {
  try {
    const type = await terminal.waitForInput('Enter KV type (redis/memory):');
    const url = await terminal.waitForInput('Enter KV URL:');
    const result = await kvAPI.create(type, url);
    terminal.print('', 'success');
    terminal.print(`ID: ${result.id}`, 'info');
    if (result.error) {
      terminal.print(result.error, 'warning');
    } else {
      terminal.print(result.message, 'success');
    }
  } catch (error) {
    terminal.print(`Failed to create KV: ${(error as Error).message}`, 'error');
  }
}

async function kvDelete(terminal: TerminalAPI, id: string) {
  try {
    await kvAPI.delete(id);
    terminal.print('KV deleted successfully', 'success');
  } catch (error) {
    terminal.print(`Failed to delete KV: ${(error as Error).message}`, 'error');
  }
}

export async function kvCommand(terminal: TerminalAPI, args: string[]) {
  if (!requireAuth(terminal)) return;
  switch (args[0]) {
    case 'list': await kvList(terminal); break;
    case 'add': await kvAdd(terminal); break;
    case 'delete':
      if (!args[1]) { terminal.print('Usage: kv delete <id>', 'error'); return; }
      await kvDelete(terminal, args[1]); break;
    default: terminal.print('Usage: kv [list|add|delete]', 'error');
  }
}

// === Worker Commands ===

async function workerList(terminal: TerminalAPI) {
  try {
    const result = await workerAPI.list();
    terminal.print('');
    terminal.print('=== Workers ===', 'info');
    if (result && result.length > 0) {
      result.forEach((w: { worker_id: string; worker_name: string; status: string; active_version_id: number | null }) => {
        const statusClass = w.status === 'active' ? 'success' : w.status === 'error' ? 'error' : 'warning';
        terminal.print(`ID: ${w.worker_id}`, 'success');
        terminal.print(`  Name: ${w.worker_name}`);
        terminal.print(`  Status: ${w.status}`, statusClass);
        terminal.print(`  Active Version: ${w.active_version_id ?? 'none'}`);
        terminal.print('');
      });
    } else {
      terminal.print('No workers found', 'warning');
    }
  } catch (error) {
    terminal.print(`Failed to list workers: ${(error as Error).message}`, 'error');
  }
}

async function workerAdd(terminal: TerminalAPI) {
  try {
    const name = await terminal.waitForInput('Enter worker name:');
    const result = await workerAPI.create(name);
    terminal.print('', 'success');
    terminal.print(`Worker ID: ${result.worker_id}`, 'info');
    terminal.print(`Name: ${result.worker_name}`, 'info');
  } catch (error) {
    terminal.print(`Failed to create worker: ${(error as Error).message}`, 'error');
  }
}

async function workerGet(terminal: TerminalAPI, id: string) {
  try {
    const result = await workerAPI.get(id);
    const w = result.worker;
    terminal.print('');
    terminal.print(`=== Worker: ${w.worker_name} ===`, 'info');
    terminal.print(`  ID: ${w.worker_id}`);
    terminal.print(`  Status: ${w.status}`, w.status === 'active' ? 'success' : w.status === 'error' ? 'error' : 'warning');
    terminal.print(`  Active Version: ${w.active_version_id ?? 'none'}`);
    terminal.print('');
    if (result.versions && result.versions.length > 0) {
      terminal.print('--- Deploy Versions ---', 'info');
      result.versions.forEach((v: { id: number; image: string; port: number; status: string; msg: string; created_at: string }) => {
        const statusClass = v.status === 'success' ? 'success' : v.status === 'error' ? 'error' : 'warning';
        const active = w.active_version_id === v.id ? ' [active]' : '';
        terminal.print(`  #${v.id}${active}`, statusClass);
        terminal.print(`    Image: ${v.image}`);
        terminal.print(`    Port: ${v.port}`);
        terminal.print(`    Status: ${v.status}`);
        if (v.msg) terminal.print(`    Msg: ${v.msg}`);
        terminal.print(`    Created: ${v.created_at}`);
        terminal.print('');
      });
    } else {
      terminal.print('No deploy versions found', 'warning');
    }
  } catch (error) {
    terminal.print(`Failed to get worker: ${(error as Error).message}`, 'error');
  }
}

async function workerDelete(terminal: TerminalAPI, id: string) {
  try {
    await workerAPI.delete(id);
    terminal.print('Worker deleted successfully', 'success');
  } catch (error) {
    terminal.print(`Failed to delete worker: ${(error as Error).message}`, 'error');
  }
}

async function workerEnv(terminal: TerminalAPI, id: string) {
  try {
    const env = await workerAPI.getEnv(id);
    terminal.print('');
    terminal.print(`=== Env: ${id} ===`, 'info');
    const keys = Object.keys(env);
    if (keys.length > 0) {
      keys.forEach(k => terminal.print(`  ${k}=${env[k]}`));
    } else {
      terminal.print('  (empty)', 'warning');
    }
    terminal.print('');
  } catch (error) {
    terminal.print(`Failed to get env: ${(error as Error).message}`, 'error');
  }
}

async function workerEnvSet(terminal: TerminalAPI, id: string) {
  try {
    const line = await terminal.waitForInput('Enter KEY=VALUE:');
    if (!line) {
      terminal.print('Cancelled', 'warning');
      return;
    }
    const idx = line.indexOf('=');
    if (idx <= 0) {
      terminal.print('Invalid format, use KEY=VALUE', 'error');
      return;
    }
    const key = line.slice(0, idx).trim();
    const value = line.slice(idx + 1);
    const result = await workerAPI.setEnv(id, key, value);
    terminal.print('Env updated, syncing to cluster...', 'success');
    Object.keys(result).forEach(k => terminal.print(`  ${k}=${result[k]}`));
  } catch (error) {
    terminal.print(`Failed to set env: ${(error as Error).message}`, 'error');
  }
}

async function workerEnvDelete(terminal: TerminalAPI, id: string, key: string) {
  try {
    const result = await workerAPI.setEnv(id, key, '', true);
    terminal.print(`Env key "${key}" deleted, syncing to cluster...`, 'success');
    const keys = Object.keys(result);
    if (keys.length > 0) {
      keys.forEach(k => terminal.print(`  ${k}=${result[k]}`));
    } else {
      terminal.print('  (empty)', 'warning');
    }
  } catch (error) {
    terminal.print(`Failed to delete env: ${(error as Error).message}`, 'error');
  }
}

async function workerSecret(terminal: TerminalAPI, id: string) {
  try {
    const keys = await workerAPI.getSecrets(id);
    terminal.print('');
    terminal.print(`=== Secrets: ${id} ===`, 'info');
    if (keys && keys.length > 0) {
      keys.forEach((k: string) => terminal.print(`  ${k}=********`));
    } else {
      terminal.print('  (empty)', 'warning');
    }
    terminal.print('');
  } catch (error) {
    terminal.print(`Failed to get secrets: ${(error as Error).message}`, 'error');
  }
}

async function workerSecretDelete(terminal: TerminalAPI, id: string, key: string) {
  try {
    const keys = await workerAPI.deleteSecret(id, key);
    terminal.print(`Secret key "${key}" deleted, syncing to cluster...`, 'success');
    if (keys && keys.length > 0) {
      keys.forEach((k: string) => terminal.print(`  ${k}=********`));
    } else {
      terminal.print('  (empty)', 'warning');
    }
  } catch (error) {
    terminal.print(`Failed to delete secret: ${(error as Error).message}`, 'error');
  }
}

async function workerSecretSet(terminal: TerminalAPI, id: string) {
  try {
    const line = await terminal.waitForInput('Enter KEY=VALUE:');
    if (!line) {
      terminal.print('Cancelled', 'warning');
      return;
    }
    const idx = line.indexOf('=');
    if (idx <= 0) {
      terminal.print('Invalid format, use KEY=VALUE', 'error');
      return;
    }
    const key = line.slice(0, idx).trim();
    const value = line.slice(idx + 1);
    const keys = await workerAPI.setSecrets(id, key, value);
    terminal.print('Secret updated, syncing to cluster...', 'success');
    keys.forEach((k: string) => terminal.print(`  ${k}=********`));
  } catch (error) {
    terminal.print(`Failed to set secret: ${(error as Error).message}`, 'error');
  }
}

export async function workerCommand(terminal: TerminalAPI, args: string[]) {
  if (!requireAuth(terminal)) return;
  const sub = args[0];
  if (sub === 'list') { await workerList(terminal); return; }
  if (sub === 'add') { await workerAdd(terminal); return; }

  // worker <id> <action> ...
  const id = sub;
  if (!id) { printWorkerUsage(terminal); return; }
  const action = args[1];

  switch (action) {
    case undefined:
    case 'get':
      await workerGet(terminal, id); break;
    case 'delete':
      await workerDelete(terminal, id); break;
    case 'env':
      await handleWorkerEnv(terminal, id, args.slice(2)); break;
    case 'secret':
      await handleWorkerSecret(terminal, id, args.slice(2)); break;
    default:
      printWorkerUsage(terminal);
  }
}

async function handleWorkerEnv(terminal: TerminalAPI, id: string, rest: string[]) {
  switch (rest[0]) {
    case undefined:
      await workerEnv(terminal, id); break;
    case 'set':
      await workerEnvSet(terminal, id); break;
    case 'delete':
      if (!rest[1]) { terminal.print('Usage: worker <id> env delete <key>', 'error'); return; }
      await workerEnvDelete(terminal, id, rest[1]); break;
    default:
      terminal.print('Usage: worker <id> env [set|delete <key>]', 'error');
  }
}

async function handleWorkerSecret(terminal: TerminalAPI, id: string, rest: string[]) {
  switch (rest[0]) {
    case undefined:
      await workerSecret(terminal, id); break;
    case 'set':
      await workerSecretSet(terminal, id); break;
    case 'delete':
      if (!rest[1]) { terminal.print('Usage: worker <id> secret delete <key>', 'error'); return; }
      await workerSecretDelete(terminal, id, rest[1]); break;
    default:
      terminal.print('Usage: worker <id> secret [set|delete <key>]', 'error');
  }
}

function printWorkerUsage(terminal: TerminalAPI) {
  terminal.print('Usage:', 'error');
  terminal.print('  worker list                      - list workers', 'error');
  terminal.print('  worker add                       - create worker', 'error');
  terminal.print('  worker <id>                      - get worker details', 'error');
  terminal.print('  worker <id> delete               - delete worker', 'error');
  terminal.print('  worker <id> env                  - list env vars', 'error');
  terminal.print('  worker <id> env set              - set env var', 'error');
  terminal.print('  worker <id> env delete <key>     - delete env var', 'error');
  terminal.print('  worker <id> secret               - list secrets', 'error');
  terminal.print('  worker <id> secret set           - set secret', 'error');
  terminal.print('  worker <id> secret delete <key>  - delete secret', 'error');
}

// === Domain Commands ===

async function domainList(terminal: TerminalAPI) {
  try {
    const result = await domainAPI.list();
    terminal.print('');
    terminal.print('=== Custom Domains ===', 'info');
    if (result.domains && result.domains.length > 0) {
      result.domains.forEach((d: { id: string; domain: string; target: string; status: string }) => {
        terminal.print(`ID: ${d.id}`, 'success');
        terminal.print(`  Domain: ${d.domain}`);
        terminal.print(`  Target: ${d.target}`);
        terminal.print(`  Status: ${d.status}`);
        terminal.print('');
      });
    } else {
      terminal.print('No custom domains found', 'warning');
    }
  } catch (error) {
    terminal.print(`Failed to list domains: ${(error as Error).message}`, 'error');
  }
}

async function domainAdd(terminal: TerminalAPI) {
  try {
    const domain = await terminal.waitForInput('Enter your domain (e.g. api.example.com):');
    const target = await terminal.waitForInput('Enter target domain to proxy to:');
    const result = await domainAPI.create(domain, target);
    terminal.print('', 'success');
    terminal.print('Domain verification required!', 'warning');
    terminal.print('');
    terminal.print('Add this TXT record to your DNS:', 'info');
    terminal.print(`  Name:  ${result.txt_name}`);
    terminal.print(`  Value: ${result.txt_value}`);
    terminal.print('');
    terminal.print(`ID: ${result.id}`, 'info');
    terminal.print(`Status: ${result.status}`, 'warning');
    terminal.print('');
    terminal.print('Verification will run for 60 seconds...', 'info');
  } catch (error) {
    terminal.print(`Failed to add domain: ${(error as Error).message}`, 'error');
  }
}

async function domainGet(terminal: TerminalAPI, id: string) {
  try {
    const result = await domainAPI.get(id);
    terminal.print('');
    terminal.print(`ID: ${result.id}`, 'success');
    terminal.print(`  Domain: ${result.domain}`);
    terminal.print(`  Target: ${result.target}`);
    terminal.print(`  Status: ${result.status}`);
    terminal.print(`  TXT Name: ${result.txt_name}`);
    terminal.print(`  TXT Value: ${result.txt_value}`);
  } catch (error) {
    terminal.print(`Failed to get domain: ${(error as Error).message}`, 'error');
  }
}

async function domainDelete(terminal: TerminalAPI, id: string) {
  try {
    await domainAPI.delete(id);
    terminal.print('Domain deleted successfully', 'success');
  } catch (error) {
    terminal.print(`Failed to delete domain: ${(error as Error).message}`, 'error');
  }
}

export async function domainCommand(terminal: TerminalAPI, args: string[]) {
  if (!requireAuth(terminal)) return;
  switch (args[0]) {
    case 'list': await domainList(terminal); break;
    case 'add': await domainAdd(terminal); break;
    case 'get':
      if (!args[1]) { terminal.print('Usage: domain get <id>', 'error'); return; }
      await domainGet(terminal, args[1]); break;
    case 'delete':
      if (!args[1]) { terminal.print('Usage: domain delete <id>', 'error'); return; }
      await domainDelete(terminal, args[1]); break;
    default: terminal.print('Usage: domain [list|add|get|delete]', 'error');
  }
}
