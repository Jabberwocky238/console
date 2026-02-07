import { BrowserRouter, Routes, Route, Navigate, Outlet, NavLink, useNavigate, useParams } from 'react-router-dom'
import { useState, useEffect, useCallback } from 'react'
import { credentialStore } from '../store'
import { rdbAPI, workerAPI, domainAPI } from '../api'
import { useMode } from '../context/ModeContext'
import AuthPage from './AuthPage'

function RequireAuth() {
  const isAuth = credentialStore.load()
  if (!isAuth) return <Navigate to="/auth" replace />
  return <Outlet />
}

function MainLayout() {
  const { setMode } = useMode()

  return (
    <div className="flex flex-col h-screen bg-zinc-950 text-zinc-100">
      {/* AppBar - top, full width */}
      <header className="h-12 border-b border-zinc-800 flex items-center px-4 shrink-0">
        <div className="text-lg font-bold">Console</div>
        <div className="flex-1" />
        <div className="flex items-center gap-3">
          <button className="text-sm text-zinc-400 hover:text-zinc-200">Lang</button>
          <button className="text-sm text-zinc-400 hover:text-zinc-200">Account</button>
          <button
            onClick={() => setMode('terminal')}
            className="text-sm text-zinc-400 hover:text-zinc-200"
          >
            Terminal
          </button>
        </div>
      </header>

      {/* Drawer + Content below AppBar */}
      <div className="flex flex-1 overflow-hidden">
        <nav className="w-48 border-r border-zinc-800 py-2 shrink-0">
          <NavItem to="/rdb" label="Database" />
          <NavItem to="/domain" label="Domain" />
          <NavItem to="/worker" label="Worker" />
        </nav>
        <main className="flex-1 p-6 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}

function NavItem({ to, label }: { to: string; label: string }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        `block px-4 py-2 text-sm ${isActive ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900'}`
      }
    >
      {label}
    </NavLink>
  )
}

function useList<T>(fetcher: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    fetcher().then(setData).catch(e => setError(e.message)).finally(() => setLoading(false))
  }, [])
  return { data, error, loading }
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    active: 'bg-green-900/50 text-green-400',
    success: 'bg-green-900/50 text-green-400',
    loading: 'bg-yellow-900/50 text-yellow-400',
    error: 'bg-red-900/50 text-red-400',
    unloaded: 'bg-zinc-800 text-zinc-400',
  }
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-mono ${colors[status] ?? colors.unloaded}`}>
      {status}
    </span>
  )
}

function RdbPage() {
  const { data, error, loading } = useList(rdbAPI.list)
  if (loading) return <div className="text-zinc-500">Loading...</div>
  if (error) return <div className="text-red-400">{error}</div>
  const rdbs = data?.rdbs as { id: string; name: string; url: string; size: number }[] | undefined
  return (
    <div>
      <h2 className="text-lg font-semibold mb-4">Database</h2>
      {data?.database_size !== undefined && (
        <p className="text-sm text-zinc-400 mb-4">Total: {formatBytes(data.database_size)}</p>
      )}
      {rdbs && rdbs.length > 0 ? (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-zinc-400 border-b border-zinc-800">
              <th className="pb-2 pr-4">ID</th>
              <th className="pb-2 pr-4">Name</th>
              <th className="pb-2 pr-4">URL</th>
              <th className="pb-2">Size</th>
            </tr>
          </thead>
          <tbody>
            {rdbs.map(r => (
              <tr key={r.id} className="border-b border-zinc-800/50">
                <td className="py-2 pr-4 font-mono text-zinc-300">{r.id}</td>
                <td className="py-2 pr-4">{r.name}</td>
                <td className="py-2 pr-4 text-zinc-400 font-mono text-xs">{r.url}</td>
                <td className="py-2">{formatBytes(r.size)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="text-zinc-500">No databases found</p>
      )}
    </div>
  )
}

function DomainPage() {
  const { data, error, loading } = useList(domainAPI.list)
  if (loading) return <div className="text-zinc-500">Loading...</div>
  if (error) return <div className="text-red-400">{error}</div>
  const domains = data?.domains as { id: string; domain: string; target: string; status: string }[] | undefined
  return (
    <div>
      <h2 className="text-lg font-semibold mb-4">Domain</h2>
      {domains && domains.length > 0 ? (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-zinc-400 border-b border-zinc-800">
              <th className="pb-2 pr-4">ID</th>
              <th className="pb-2 pr-4">Domain</th>
              <th className="pb-2 pr-4">Target</th>
              <th className="pb-2">Status</th>
            </tr>
          </thead>
          <tbody>
            {domains.map(d => (
              <tr key={d.id} className="border-b border-zinc-800/50">
                <td className="py-2 pr-4 font-mono text-zinc-300">{d.id}</td>
                <td className="py-2 pr-4">{d.domain}</td>
                <td className="py-2 pr-4 text-zinc-400">{d.target}</td>
                <td className="py-2">{d.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="text-zinc-500">No domains found</p>
      )}
    </div>
  )
}

function WorkerPage() {
  const { data, error, loading } = useList(workerAPI.list)
  const navigate = useNavigate()
  if (loading) return <div className="text-zinc-500">Loading...</div>
  if (error) return <div className="text-red-400">{error}</div>
  const workers = data as { worker_id: string; worker_name: string; status: string; active_version_id: number | null; url: string }[] | null
  return (
    <div>
      <h2 className="text-lg font-semibold mb-4">Worker</h2>
      {workers && workers.length > 0 ? (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-zinc-400 border-b border-zinc-800">
              <th className="pb-2 pr-4">ID</th>
              <th className="pb-2 pr-4">Name</th>
              <th className="pb-2 pr-4">URL</th>
              <th className="pb-2 pr-4">Status</th>
              <th className="pb-2">Active Version</th>
            </tr>
          </thead>
          <tbody>
            {workers.map(w => (
              <tr
                key={w.worker_id}
                className="border-b border-zinc-800/50 hover:bg-zinc-900 cursor-pointer"
                onClick={() => navigate(`/worker/${w.worker_id}`)}
              >
                <td className="py-2 pr-4 font-mono text-zinc-300">{w.worker_id}</td>
                <td className="py-2 pr-4">{w.worker_name}</td>
                <td className="py-2 pr-4">
                  <a href={w.url} target="_blank" rel="noreferrer"
                    className="text-blue-400 hover:text-blue-300 font-mono text-xs"
                    onClick={e => e.stopPropagation()}
                  >{w.url}</a>
                </td>
                <td className="py-2 pr-4"><StatusBadge status={w.status} /></td>
                <td className="py-2">{w.active_version_id ?? 'none'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="text-zinc-500">No workers found</p>
      )}
    </div>
  )
}

const btnClass = 'px-3 py-1 text-xs rounded border border-zinc-700 text-zinc-300 hover:bg-zinc-800'
const textareaClass = 'w-full bg-zinc-900 border border-zinc-700 rounded p-2 text-sm font-mono text-zinc-200 focus:outline-none focus:border-zinc-500'

function EnvSection({ env, envDraft, envEditing, envSaving, envToText, onEdit, onCancel, onDraftChange, onSave }: {
  env: Record<string, string>; envDraft: string; envEditing: boolean; envSaving: boolean
  envToText: (m: Record<string, string>) => string
  onEdit: () => void; onCancel: () => void; onDraftChange: (v: string) => void; onSave: () => void
}) {
  const keys = Object.keys(env)
  return (
    <section>
      <div className="flex items-center gap-2 mb-2">
        <h3 className="text-sm font-semibold text-zinc-300">Environment Variables</h3>
        {!envEditing && <button onClick={onEdit} className={btnClass}>Edit</button>}
      </div>
      {envEditing ? (
        <div className="space-y-2">
          <textarea rows={6} value={envDraft} onChange={e => onDraftChange(e.target.value)}
            className={textareaClass} placeholder="KEY=VALUE (one per line)" />
          <div className="flex gap-2">
            <button onClick={onSave} disabled={envSaving} className={btnClass}>{envSaving ? 'Saving...' : 'Save'}</button>
            <button onClick={onCancel} className={btnClass}>Cancel</button>
          </div>
        </div>
      ) : keys.length > 0 ? (
        <pre className="text-xs font-mono text-zinc-400 bg-zinc-900 rounded p-2">{envToText(env)}</pre>
      ) : (
        <p className="text-xs text-zinc-500">(empty)</p>
      )}
    </section>
  )
}

function SecretSection({ secrets, secretDraft, secretEditing, secretSaving, onEdit, onCancel, onDraftChange, onSave }: {
  secrets: string[]; secretDraft: string; secretEditing: boolean; secretSaving: boolean
  onEdit: () => void; onCancel: () => void; onDraftChange: (v: string) => void; onSave: () => void
}) {
  return (
    <section>
      <div className="flex items-center gap-2 mb-2">
        <h3 className="text-sm font-semibold text-zinc-300">Secrets</h3>
        {!secretEditing && <button onClick={onEdit} className={btnClass}>Edit</button>}
      </div>
      {secretEditing ? (
        <div className="space-y-2">
          <textarea rows={6} value={secretDraft} onChange={e => onDraftChange(e.target.value)}
            className={textareaClass} placeholder="KEY=VALUE (one per line, values only stored in cluster)" />
          <div className="flex gap-2">
            <button onClick={onSave} disabled={secretSaving} className={btnClass}>{secretSaving ? 'Saving...' : 'Save'}</button>
            <button onClick={onCancel} className={btnClass}>Cancel</button>
          </div>
        </div>
      ) : secrets.length > 0 ? (
        <pre className="text-xs font-mono text-zinc-400 bg-zinc-900 rounded p-2">
          {secrets.map(k => `${k}=********`).join('\n')}
        </pre>
      ) : (
        <p className="text-xs text-zinc-500">(empty)</p>
      )}
    </section>
  )
}

function VersionsSection({ versions, activeVersionId }: { versions: any[]; activeVersionId: number | null }) {
  if (!versions.length) return <p className="text-xs text-zinc-500">No deploy versions</p>
  return (
    <section>
      <h3 className="text-sm font-semibold text-zinc-300 mb-2">Deploy Versions</h3>
      <table className="w-full text-xs">
        <thead>
          <tr className="text-left text-zinc-500 border-b border-zinc-800">
            <th className="pb-1 pr-3">#</th>
            <th className="pb-1 pr-3">Image</th>
            <th className="pb-1 pr-3">Port</th>
            <th className="pb-1 pr-3">Status</th>
            <th className="pb-1">Created</th>
          </tr>
        </thead>
        <tbody>
          {versions.map((v: any) => (
            <tr key={v.id} className="border-b border-zinc-800/50">
              <td className="py-1 pr-3 font-mono">
                {v.id}{activeVersionId === v.id ? <span className="ml-1 text-green-400">[active]</span> : ''}
              </td>
              <td className="py-1 pr-3 font-mono text-zinc-400">{v.image}</td>
              <td className="py-1 pr-3">{v.port}</td>
              <td className="py-1 pr-3"><StatusBadge status={v.status} /></td>
              <td className="py-1 text-zinc-500">{v.created_at}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}

function WorkerDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [worker, setWorker] = useState<any>(null)
  const [workerURL, setWorkerURL] = useState('')
  const [versions, setVersions] = useState<any[]>([])
  const [env, setEnv] = useState<Record<string, string>>({})
  const [secrets, setSecrets] = useState<string[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  // env editing
  const [envDraft, setEnvDraft] = useState('')
  const [envEditing, setEnvEditing] = useState(false)
  const [envSaving, setEnvSaving] = useState(false)

  // secret editing
  const [secretDraft, setSecretDraft] = useState('')
  const [secretEditing, setSecretEditing] = useState(false)
  const [secretSaving, setSecretSaving] = useState(false)

  const load = useCallback(async () => {
    if (!id) return
    try {
      const [detail, envData, secretData] = await Promise.all([
        workerAPI.get(id),
        workerAPI.getEnv(id),
        workerAPI.getSecrets(id),
      ])
      setWorker(detail.worker)
      setWorkerURL(detail.url ?? '')
      setVersions(detail.versions ?? [])
      setEnv(envData ?? {})
      setSecrets(secretData ?? [])
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => { load() }, [load])

  if (loading) return <div className="text-zinc-500">Loading...</div>
  if (error) return <div className="text-red-400">{error}</div>
  if (!worker) return <div className="text-red-400">Worker not found</div>

  const envToText = (m: Record<string, string>) => Object.entries(m).map(([k, v]) => `${k}=${v}`).join('\n')
  const textToMap = (t: string): Record<string, string> => {
    const m: Record<string, string> = {}
    t.split('\n').forEach(line => {
      const i = line.indexOf('=')
      if (i > 0) m[line.slice(0, i).trim()] = line.slice(i + 1)
    })
    return m
  }

  const saveEnv = async () => {
    setEnvSaving(true)
    try {
      const result = await workerAPI.setEnv(id!, textToMap(envDraft))
      setEnv(result)
      setEnvEditing(false)
      load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setEnvSaving(false)
    }
  }

  const saveSecrets = async () => {
    setSecretSaving(true)
    try {
      const keys = await workerAPI.setSecrets(id!, textToMap(secretDraft))
      setSecrets(keys)
      setSecretEditing(false)
      setSecretDraft('')
      load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSecretSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <button onClick={() => navigate('/worker')} className="text-zinc-400 hover:text-zinc-200 text-sm">&larr; Back</button>
        <h2 className="text-lg font-semibold">{worker.worker_name}</h2>
        <StatusBadge status={worker.status} />
        <span className="text-xs text-zinc-500 font-mono">{worker.worker_id}</span>
      </div>
      {workerURL && (
        <div className="text-sm">
          <span className="text-zinc-400">URL: </span>
          <a href={workerURL} target="_blank" rel="noreferrer"
            className="text-blue-400 hover:text-blue-300 font-mono text-xs"
          >{workerURL}</a>
        </div>
      )}

      {/* Env Section */}
      <EnvSection
        env={env} envDraft={envDraft} envEditing={envEditing} envSaving={envSaving}
        envToText={envToText}
        onEdit={() => { setEnvDraft(envToText(env)); setEnvEditing(true) }}
        onCancel={() => setEnvEditing(false)}
        onDraftChange={setEnvDraft}
        onSave={saveEnv}
      />

      {/* Secrets Section */}
      <SecretSection
        secrets={secrets} secretDraft={secretDraft} secretEditing={secretEditing} secretSaving={secretSaving}
        onEdit={() => { setSecretDraft(''); setSecretEditing(true) }}
        onCancel={() => setSecretEditing(false)}
        onDraftChange={setSecretDraft}
        onSave={saveSecrets}
      />

      {/* Versions */}
      <VersionsSection versions={versions} activeVersionId={worker.active_version_id} />
    </div>
  )
}

export default function GUI() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/auth" element={<AuthPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<MainLayout />}>
            <Route path="/rdb" element={<RdbPage />} />
            <Route path="/domain" element={<DomainPage />} />
            <Route path="/worker" element={<WorkerPage />} />
            <Route path="/worker/:id" element={<WorkerDetailPage />} />
            <Route index element={<Navigate to="/rdb" replace />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
