import { useState, useEffect, useCallback } from "react";
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, BarChart, Bar, Cell } from "recharts";
import { login, fetchTenants, fetchUsage, fetchInvoice, createTenant, deleteTenant } from './api';

const API = "http://localhost:8080";

const theme = {
  bg: "#0a0e1a",
  surface: "#111827",
  border: "#1e293b",
  accent: "#3b82f6",
  accentGlow: "#3b82f620",
  green: "#10b981",
  red: "#ef4444",
  yellow: "#f59e0b",
  text: "#f1f5f9",
  muted: "#64748b",
};

const css = `
  @import url('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;700&family=Syne:wght@400;600;800&display=swap');

  * { margin: 0; padding: 0; box-sizing: border-box; }

  body {
    background: ${theme.bg};
    color: ${theme.text};
    font-family: 'Syne', sans-serif;
    min-height: 100vh;
  }

  .app { display: flex; min-height: 100vh; }

  .sidebar {
    width: 220px;
    background: ${theme.surface};
    border-right: 1px solid ${theme.border};
    padding: 24px 16px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    position: fixed;
    height: 100vh;
  }

  .logo {
    font-family: 'Syne', sans-serif;
    font-weight: 800;
    font-size: 22px;
    color: ${theme.accent};
    letter-spacing: -0.5px;
    padding: 0 8px 24px;
    border-bottom: 1px solid ${theme.border};
    margin-bottom: 12px;
  }

  .logo span { color: ${theme.muted}; font-weight: 400; font-size: 11px; display: block; margin-top: 2px; }

  .nav-item {
    padding: 10px 12px;
    border-radius: 8px;
    cursor: pointer;
    font-size: 13px;
    font-weight: 600;
    color: ${theme.muted};
    transition: all 0.15s;
    display: flex;
    align-items: center;
    gap: 10px;
    border: 1px solid transparent;
  }

  .nav-item:hover { background: ${theme.accentGlow}; color: ${theme.text}; }
  .nav-item.active { background: ${theme.accentGlow}; color: ${theme.accent}; border-color: ${theme.accent}40; }

  .main { margin-left: 220px; flex: 1; padding: 32px; }

  .page-title {
    font-size: 28px;
    font-weight: 800;
    margin-bottom: 8px;
    letter-spacing: -0.5px;
  }

  .page-sub { color: ${theme.muted}; font-size: 13px; margin-bottom: 32px; font-family: 'JetBrains Mono', monospace; }

  .grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-bottom: 16px; }
  .grid-3 { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 16px; margin-bottom: 16px; }

  .card {
    background: ${theme.surface};
    border: 1px solid ${theme.border};
    border-radius: 12px;
    padding: 20px;
  }

  .card-title {
    font-size: 11px;
    font-weight: 600;
    color: ${theme.muted};
    text-transform: uppercase;
    letter-spacing: 1px;
    margin-bottom: 12px;
  }

  .stat-value {
    font-size: 36px;
    font-weight: 800;
    letter-spacing: -1px;
    color: ${theme.text};
  }

  .stat-sub { font-size: 12px; color: ${theme.muted}; margin-top: 4px; font-family: 'JetBrains Mono', monospace; }

  .badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 20px;
    font-size: 11px;
    font-weight: 700;
    font-family: 'JetBrains Mono', monospace;
  }

  .badge-green { background: ${theme.green}20; color: ${theme.green}; }
  .badge-blue { background: ${theme.accent}20; color: ${theme.accent}; }
  .badge-red { background: ${theme.red}20; color: ${theme.red}; }
  .badge-yellow { background: ${theme.yellow}20; color: ${theme.yellow}; }

  .table { width: 100%; border-collapse: collapse; }
  .table th { text-align: left; font-size: 11px; color: ${theme.muted}; font-weight: 600; text-transform: uppercase; letter-spacing: 0.8px; padding: 8px 12px; border-bottom: 1px solid ${theme.border}; }
  .table td { padding: 12px; font-size: 13px; border-bottom: 1px solid ${theme.border}10; font-family: 'JetBrains Mono', monospace; }
  .table tr:hover td { background: ${theme.accentGlow}; }

  .btn {
    padding: 8px 16px;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 700;
    cursor: pointer;
    border: none;
    font-family: 'Syne', sans-serif;
    transition: all 0.15s;
  }

  .btn-primary { background: ${theme.accent}; color: white; }
  .btn-primary:hover { background: #2563eb; transform: translateY(-1px); }
  .btn-danger { background: ${theme.red}20; color: ${theme.red}; border: 1px solid ${theme.red}40; }
  .btn-danger:hover { background: ${theme.red}40; }

  .input {
    background: ${theme.bg};
    border: 1px solid ${theme.border};
    border-radius: 8px;
    padding: 8px 12px;
    color: ${theme.text};
    font-size: 13px;
    font-family: 'JetBrains Mono', monospace;
    outline: none;
    width: 100%;
    transition: border-color 0.15s;
  }

  .input:focus { border-color: ${theme.accent}; }

  .form-row { display: flex; gap: 8px; align-items: flex-end; flex-wrap: wrap; margin-bottom: 20px; }
  .form-group { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 120px; }
  .form-label { font-size: 11px; color: ${theme.muted}; font-weight: 600; text-transform: uppercase; letter-spacing: 0.8px; }

  .pulse { width: 8px; height: 8px; border-radius: 50%; background: ${theme.green}; animation: pulse 2s infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }

  .node-bar { height: 8px; border-radius: 4px; background: ${theme.border}; overflow: hidden; margin-top: 6px; }
  .node-bar-fill { height: 100%; border-radius: 4px; background: linear-gradient(90deg, ${theme.accent}, ${theme.green}); transition: width 0.5s ease; }

  .refresh-btn {
    background: none;
    border: 1px solid ${theme.border};
    color: ${theme.muted};
    padding: 6px 12px;
    border-radius: 6px;
    font-size: 12px;
    cursor: pointer;
    font-family: 'JetBrains Mono', monospace;
    transition: all 0.15s;
  }
  .refresh-btn:hover { border-color: ${theme.accent}; color: ${theme.accent}; }

  .header-row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 32px; }
  .empty { text-align: center; padding: 40px; color: ${theme.muted}; font-family: 'JetBrains Mono', monospace; font-size: 13px; }
`;

// ── Sidebar ────────────────────────────────────────────────────
const PAGES = [
  { id: "overview", label: "Overview" },
  { id: "tenants", label: "Tenants" },
  { id: "workloads", label: "Workloads" },
  { id: "billing", label: "Billing" },
  { id: "scheduler", label: "Scheduler" },
];

function Sidebar({ page, setPage }) {
  return (
    <div className="sidebar">
      <div className="logo">
        NIMBUS
        <span>cloud control plane</span>
      </div>
      {PAGES.map((p) => (
        <div
          key={p.id}
          className={`nav-item ${page === p.id ? "active" : ""}`}
          onClick={() => setPage(p.id)}
        >
          {p.label}
        </div>
      ))}
      <div style={{ marginTop: "auto", padding: "8px 12px", display: "flex", alignItems: "center", gap: "8px" }}>
        <div className="pulse" />
        <span style={{ fontSize: "11px", color: theme.muted, fontFamily: "JetBrains Mono" }}>API connected</span>
      </div>
    </div>
  );
}

// ── Overview ───────────────────────────────────────────────────
function Overview({ tenants, usage }) {
  const totalPods = Object.values(usage).reduce((s, u) => s + u.PodCount, 0);
  const totalCPU = Object.values(usage).reduce((s, u) => s + u.CPUMilliSeconds, 0);

  const chartData = tenants.map((t) => ({
    name: t.name,
    pods: usage[t.id]?.PodCount || 0,
    cpu: usage[t.id]?.CPUMilliSeconds || 0,
  }));

  return (
    <div>
      <div className="page-title">Overview</div>
      <div className="page-sub">$ nimbus status --all-tenants</div>

      <div className="grid-3">
        <div className="card">
          <div className="card-title">Active Tenants</div>
          <div className="stat-value">{tenants.length}</div>
          <div className="stat-sub">namespaces provisioned</div>
        </div>
        <div className="card">
          <div className="card-title">Running Pods</div>
          <div className="stat-value" style={{ color: theme.green }}>{totalPods}</div>
          <div className="stat-sub">across all tenants</div>
        </div>
        <div className="card">
          <div className="card-title">CPU Tracked</div>
          <div className="stat-value" style={{ color: theme.accent }}>{(totalCPU / 1000).toFixed(1)}s</div>
          <div className="stat-sub">millicore-seconds accumulated</div>
        </div>
      </div>

      <div className="card">
        <div className="card-title">Pod Distribution by Tenant</div>
        {chartData.length === 0 ? (
          <div className="empty">No tenants yet — create one to see data</div>
        ) : (
          <ResponsiveContainer width="100%" height={200}>
            <BarChart data={chartData}>
              <XAxis dataKey="name" tick={{ fill: theme.muted, fontSize: 12 }} />
              <YAxis tick={{ fill: theme.muted, fontSize: 12 }} />
              <Tooltip contentStyle={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 8 }} />
              <Bar dataKey="pods" radius={[4, 4, 0, 0]}>
                {chartData.map((_, i) => (
                  <Cell key={i} fill={i % 2 === 0 ? theme.accent : theme.green} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  );
}

// ── Tenants ────────────────────────────────────────────────────
function Tenants({ tenants, refresh }) {
  const [name, setName] = useState("");
  const [cpu, setCpu] = useState("500m");
  const [mem, setMem] = useState("512Mi");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleCreate() {
    if (!name) return;
    setLoading(true);
    setError("");
    const res = await createTenant(name, cpu, mem);
    setLoading(false);
    if (res.error) { setError(res.error); return; }
    setName("");
    refresh();
  }

  async function handleDelete(id) {
    await deleteTenant(id);
    refresh();
  }

  return (
    <div>
      <div className="page-title">Tenants</div>
      <div className="page-sub">$ nimbus tenant list</div>

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="card-title">Create Tenant</div>
        <div className="form-row">
          <div className="form-group">
            <div className="form-label">Name</div>
            <input className="input" value={name} onChange={e => setName(e.target.value)} placeholder="acme-corp" />
          </div>
          <div className="form-group">
            <div className="form-label">CPU Quota</div>
            <input className="input" value={cpu} onChange={e => setCpu(e.target.value)} placeholder="500m" />
          </div>
          <div className="form-group">
            <div className="form-label">Memory Quota</div>
            <input className="input" value={mem} onChange={e => setMem(e.target.value)} placeholder="512Mi" />
          </div>
          <button className="btn btn-primary" onClick={handleCreate} disabled={loading}>
            {loading ? "Creating..." : "+ Create"}
          </button>
        </div>
        {error && <div style={{ color: theme.red, fontSize: 13, fontFamily: "JetBrains Mono" }}>{error}</div>}
      </div>

      <div className="card">
        <div className="card-title">All Tenants ({tenants.length})</div>
        {tenants.length === 0 ? (
          <div className="empty">No tenants yet</div>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Namespace</th>
                <th>CPU Quota</th>
                <th>Memory</th>
                <th>Created</th>
                <th>Status</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {tenants.map(t => (
                <tr key={t.id}>
                  <td style={{ color: theme.text, fontWeight: 600 }}>{t.name}</td>
                  <td style={{ color: theme.muted }}>{t.namespace}</td>
                  <td>{t.cpu_quota}</td>
                  <td>{t.memory_quota}</td>
                  <td style={{ color: theme.muted }}>{new Date(t.created_at).toLocaleDateString()}</td>
                  <td><span className="badge badge-green">● Active</span></td>
                  <td>
                    <button className="btn btn-danger" onClick={() => handleDelete(t.id)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

// ── Workloads ──────────────────────────────────────────────────
function Workloads({ tenants, usage }) {
  return (
    <div>
      <div className="page-title">Workloads</div>
      <div className="page-sub">$ nimbus workloads --watch</div>

      {tenants.length === 0 ? (
        <div className="card"><div className="empty">No tenants — create one first</div></div>
      ) : (
        tenants.map(t => {
          const u = usage[t.id];
          return (
            <div className="card" key={t.id} style={{ marginBottom: 12 }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
                <div>
                  <span style={{ fontWeight: 700, fontSize: 15 }}>{t.name}</span>
                  <span style={{ color: theme.muted, fontSize: 12, marginLeft: 8, fontFamily: "JetBrains Mono" }}>{t.namespace}</span>
                </div>
                <span className="badge badge-blue">{u?.PodCount || 0} pods running</span>
              </div>
              <div className="grid-3">
                <div>
                  <div style={{ fontSize: 11, color: theme.muted, marginBottom: 4 }}>CPU QUOTA</div>
                  <div style={{ fontFamily: "JetBrains Mono", fontSize: 13 }}>{t.cpu_quota}</div>
                  <div className="node-bar"><div className="node-bar-fill" style={{ width: "30%" }} /></div>
                </div>
                <div>
                  <div style={{ fontSize: 11, color: theme.muted, marginBottom: 4 }}>MEMORY QUOTA</div>
                  <div style={{ fontFamily: "JetBrains Mono", fontSize: 13 }}>{t.memory_quota}</div>
                  <div className="node-bar"><div className="node-bar-fill" style={{ width: "20%", background: `linear-gradient(90deg, ${theme.green}, ${theme.yellow})` }} /></div>
                </div>
                <div>
                  <div style={{ fontSize: 11, color: theme.muted, marginBottom: 4 }}>SCHEDULER</div>
                  <div style={{ fontFamily: "JetBrains Mono", fontSize: 13, color: theme.accent }}>nimbus-scheduler</div>
                </div>
              </div>
            </div>
          );
        })
      )}
    </div>
  );
}

// ── Billing ────────────────────────────────────────────────────
function Billing({ tenants, usage }) {
  const [invoices, setInvoices] = useState({});

  useEffect(() => {
    tenants.forEach(async t => {
      const inv = await fetchInvoice(t.id);
      setInvoices(prev => ({ ...prev, [t.id]: inv }));
    });
  }, [tenants]);

  const usageHistory = tenants.map(t => ({
    name: t.name,
    cpu: (usage[t.id]?.CPUMilliSeconds || 0) / 1000,
    memory: (usage[t.id]?.MemoryMBSeconds || 0) / 1000,
    cost: invoices[t.id]?.TotalCost || 0,
  }));

  return (
    <div>
      <div className="page-title">Billing</div>
      <div className="page-sub">$ nimbus billing --period 2026-02</div>

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="card-title">Usage by Tenant</div>
        {usageHistory.length === 0 ? (
          <div className="empty">No usage data yet</div>
        ) : (
          <ResponsiveContainer width="100%" height={200}>
            <AreaChart data={usageHistory}>
              <XAxis dataKey="name" tick={{ fill: theme.muted, fontSize: 12 }} />
              <YAxis tick={{ fill: theme.muted, fontSize: 12 }} />
              <Tooltip contentStyle={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 8 }} />
              <Area type="monotone" dataKey="cpu" stroke={theme.accent} fill={theme.accentGlow} strokeWidth={2} />
              <Area type="monotone" dataKey="memory" stroke={theme.green} fill={`${theme.green}15`} strokeWidth={2} />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </div>

      <div className="card">
        <div className="card-title">Invoices</div>
        {tenants.length === 0 ? (
          <div className="empty">No tenants to invoice</div>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Tenant</th>
                <th>Period</th>
                <th>CPU Cost</th>
                <th>Memory Cost</th>
                <th>Total</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {tenants.map(t => {
                const inv = invoices[t.id];
                return (
                  <tr key={t.id}>
                    <td style={{ color: theme.text, fontWeight: 600 }}>{t.name}</td>
                    <td>{inv?.Period || "—"}</td>
                    <td style={{ color: theme.accent }}>${inv?.CPUCost?.toFixed(6) || "0.000000"}</td>
                    <td style={{ color: theme.green }}>${inv?.MemoryCost?.toFixed(6) || "0.000000"}</td>
                    <td style={{ color: theme.yellow, fontWeight: 700 }}>${inv?.TotalCost?.toFixed(6) || "0.000000"}</td>
                    <td><span className="badge badge-yellow">● Pending</span></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

// ── Scheduler ─────────────────────────────────────────────────
function Scheduler({ usage }) {
  const totalPods = Object.values(usage).reduce((s, u) => s + u.PodCount, 0);
  const score = Math.min(totalPods * 10, 100);

  return (
    <div>
      <div className="page-title">Scheduler</div>
      <div className="page-sub">$ nimbus scheduler --strategy bin-pack</div>

      <div className="grid-2">
        <div className="card">
          <div className="card-title">Strategy</div>
          <div style={{ fontSize: 20, fontWeight: 800, color: theme.accent, marginBottom: 8 }}>Bin Packing</div>
          <div style={{ fontSize: 13, color: theme.muted, lineHeight: 1.6 }}>
            Fills nodes with maximum utilization before using new ones.
            Scores nodes by existing CPU usage — higher usage = preferred node.
          </div>
        </div>
        <div className="card">
          <div className="card-title">Node: nimbus-control-plane</div>
          <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
            <span style={{ fontSize: 13, color: theme.muted }}>CPU Utilization</span>
            <span style={{ fontFamily: "JetBrains Mono", fontSize: 13, color: theme.accent }}>{score}%</span>
          </div>
          <div className="node-bar">
            <div className="node-bar-fill" style={{ width: `${score}%` }} />
          </div>
          <div style={{ marginTop: 12, fontSize: 12, color: theme.muted, fontFamily: "JetBrains Mono" }}>
            {totalPods} pods scheduled by nimbus-scheduler
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card-title">Scoring Algorithm</div>
        <div style={{ fontFamily: "JetBrains Mono", fontSize: 13, color: theme.muted, lineHeight: 2 }}>
          <div><span style={{ color: theme.accent }}>score</span> = (totalCPURequested × 100) / nodeAllocatableCPU</div>
          <div><span style={{ color: theme.green }}>bind</span> = highest scoring schedulable node</div>
          <div><span style={{ color: theme.yellow }}>interval</span> = 5s reconcile loop</div>
          <div><span style={{ color: theme.muted }}>filter</span> = exclude unschedulable / cordoned nodes</div>
        </div>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <div className="card-title">Recent Scheduling Decisions</div>
        <table className="table">
          <thead>
            <tr><th>Pod</th><th>Namespace</th><th>Node</th><th>Strategy</th><th>Score</th></tr>
          </thead>
          <tbody>
            {Object.entries(usage).map(([tenantID, u]) =>
              u.PodCount > 0 ? (
                <tr key={tenantID}>
                  <td>nimbus-workload-*</td>
                  <td style={{ color: theme.muted }}>{u.TenantName ? `nimbus-${u.TenantName}` : tenantID}</td>
                  <td style={{ color: theme.green }}>nimbus-control-plane</td>
                  <td><span className="badge badge-blue">bin-pack</span></td>
                  <td style={{ color: theme.accent }}>{score}</td>
                </tr>
              ) : null
            )}
            {totalPods === 0 && <tr><td colSpan={5}><div className="empty">No scheduled pods yet</div></td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ── App ────────────────────────────────────────────────────────
export default function App() {
  const [page, setPage] = useState("overview");
  const [tenants, setTenants] = useState([]);
  const [usage, setUsage] = useState({});

  const refresh = useCallback(async () => {
  try {
    await login();
    const [t, u] = await Promise.all([fetchTenants(), fetchUsage()]);
    setTenants(t);
    setUsage(u);
  } catch (e) {
    console.error("API error:", e);
  }
}, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 15000);
    return () => clearInterval(interval);
  }, [refresh]);

  return (
    <>
      <style>{css}</style>
      <div className="app">
        <Sidebar page={page} setPage={setPage} />
        <div className="main">
          <div className="header-row">
            <div />
            <button className="refresh-btn" onClick={refresh}>Refresh</button>
          </div>
          {page === "overview" && <Overview tenants={tenants} usage={usage} />}
          {page === "tenants" && <Tenants tenants={tenants} refresh={refresh} />}
          {page === "workloads" && <Workloads tenants={tenants} usage={usage} />}
          {page === "billing" && <Billing tenants={tenants} usage={usage} />}
          {page === "scheduler" && <Scheduler usage={usage} />}
        </div>
      </div>
    </>
  );
}
