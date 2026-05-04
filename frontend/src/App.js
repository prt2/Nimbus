// React dashboard for tenants, workloads, billing, and scheduler activity.
// The helpers and page components below share data fetched by App at the end of this file.

import { useState, useEffect, useCallback } from "react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  BarChart,
  Bar,
  LineChart,
  Line,
  CartesianGrid,
} from "recharts";
import {
  login,
  fetchTenants,
  fetchUsage,
  fetchInvoice,
  fetchSchedulerStatus,
  createTenant,
  deleteTenant,
} from "./api";

// Shared colors and spacing effects used by the dashboard styles and charts.
const theme = {
  bg: "#f4f7fb",
  surface: "#ffffff",
  surfaceAlt: "#f8fafc",
  border: "#d9e2ec",
  text: "#17212b",
  muted: "#5b6b7b",
  blue: "#2563eb",
  teal: "#0f766e",
  amber: "#d97706",
  red: "#dc2626",
  shadow: "0 8px 24px rgba(15, 23, 42, 0.06)",
};

// Dashboard styles are inserted by App in a style tag. Narrow screens stack the columns.
const css = `
  @import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');

  * { box-sizing: border-box; }

  body {
    margin: 0;
    background: ${theme.bg};
    color: ${theme.text};
    font-family: 'Inter', sans-serif;
  }

  button, input {
    font: inherit;
  }

  .app {
    min-height: 100vh;
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr);
  }

  .sidebar {
    background: ${theme.surface};
    border-right: 1px solid ${theme.border};
    padding: 24px 16px;
  }

  .brand {
    padding: 4px 8px 20px;
    margin-bottom: 16px;
    border-bottom: 1px solid ${theme.border};
  }

  .brand-title {
    font-size: 22px;
    font-weight: 700;
    letter-spacing: -0.02em;
  }

  .brand-subtitle {
    margin-top: 6px;
    font-size: 13px;
    color: ${theme.muted};
  }

  .nav {
    display: grid;
    gap: 6px;
  }

  .nav-item {
    border: 1px solid transparent;
    background: transparent;
    border-radius: 10px;
    padding: 10px 12px;
    text-align: left;
    cursor: pointer;
    color: ${theme.text};
  }

  .nav-item:hover {
    background: ${theme.surfaceAlt};
  }

  .nav-item.active {
    background: #eef4ff;
    border-color: #bfdbfe;
    color: ${theme.blue};
  }

  .nav-label {
    font-size: 14px;
    font-weight: 600;
  }

  .nav-copy {
    margin-top: 4px;
    font-size: 12px;
    line-height: 1.4;
    color: ${theme.muted};
  }

  .sidebar-footer {
    margin-top: 20px;
    padding: 12px;
    border: 1px solid ${theme.border};
    border-radius: 10px;
    background: ${theme.surfaceAlt};
    font-size: 12px;
    color: ${theme.muted};
    line-height: 1.5;
  }

  .main {
    padding: 28px;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 16px;
    margin-bottom: 24px;
  }

  .page-title {
    font-size: 28px;
    font-weight: 700;
    letter-spacing: -0.03em;
    margin: 0 0 8px;
  }

  .page-subtitle {
    max-width: 720px;
    font-size: 14px;
    line-height: 1.6;
    color: ${theme.muted};
  }

  .button {
    border: 1px solid ${theme.border};
    background: ${theme.surface};
    color: ${theme.text};
    border-radius: 10px;
    padding: 10px 14px;
    cursor: pointer;
    font-weight: 600;
  }

  .button:disabled {
    cursor: not-allowed;
    opacity: 0.7;
  }

  .button:hover {
    background: ${theme.surfaceAlt};
  }

  .button-primary {
    background: ${theme.blue};
    border-color: ${theme.blue};
    color: white;
  }

  .button-primary:hover {
    background: #1d4ed8;
  }

  .button-danger {
    color: ${theme.red};
    background: #fff5f5;
    border-color: #fecaca;
  }

  .grid-4,
  .grid-2 {
    display: grid;
    gap: 16px;
    margin-bottom: 16px;
  }

  .grid-4 {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .grid-2 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .card {
    background: ${theme.surface};
    border: 1px solid ${theme.border};
    border-radius: 14px;
    padding: 18px;
    box-shadow: ${theme.shadow};
  }

  .card-title {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 14px;
  }

  .metric-label {
    font-size: 13px;
    color: ${theme.muted};
  }

  .metric-value {
    margin-top: 6px;
    font-size: 30px;
    font-weight: 700;
    letter-spacing: -0.03em;
  }

  .metric-note {
    margin-top: 8px;
    font-size: 12px;
    line-height: 1.5;
    color: ${theme.muted};
  }

  .stack {
    display: grid;
    gap: 12px;
  }

  .form-grid {
    display: grid;
    grid-template-columns: 1.3fr 1fr 1fr auto;
    gap: 12px;
    align-items: end;
  }

  .field {
    display: grid;
    gap: 6px;
  }

  .label {
    font-size: 12px;
    font-weight: 600;
    color: ${theme.muted};
  }

  .input {
    width: 100%;
    border: 1px solid ${theme.border};
    border-radius: 10px;
    background: ${theme.surface};
    padding: 10px 12px;
    color: ${theme.text};
  }

  .input:focus {
    outline: 2px solid rgba(37, 99, 235, 0.12);
    border-color: #93c5fd;
  }

  .table {
    width: 100%;
    border-collapse: collapse;
  }

  .table th {
    padding: 0 0 12px;
    border-bottom: 1px solid ${theme.border};
    text-align: left;
    font-size: 12px;
    font-weight: 600;
    color: ${theme.muted};
  }

  .table td {
    padding: 14px 0;
    border-bottom: 1px solid #edf2f7;
    font-size: 14px;
    vertical-align: middle;
  }

  .table tr:last-child td {
    border-bottom: none;
  }

  .strong {
    font-weight: 600;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12px;
  }

  .muted {
    color: ${theme.muted};
  }

  .badge {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 6px 10px;
    font-size: 12px;
    font-weight: 600;
  }

  .badge-blue {
    background: #dbeafe;
    color: ${theme.blue};
  }

  .badge-teal {
    background: #ccfbf1;
    color: ${theme.teal};
  }

  .badge-amber {
    background: #fef3c7;
    color: ${theme.amber};
  }

  .meter {
    margin-top: 8px;
    height: 8px;
    border-radius: 999px;
    background: #e5edf5;
    overflow: hidden;
  }

  .meter-fill {
    height: 100%;
    border-radius: 999px;
  }

  .empty {
    padding: 24px 0;
    font-size: 14px;
    color: ${theme.muted};
  }

  .error {
    font-size: 13px;
    color: ${theme.red};
  }

  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
  }

  @media (max-width: 1100px) {
    .grid-4,
    .grid-2,
    .form-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 860px) {
    .app {
      grid-template-columns: 1fr;
    }

    .sidebar {
      border-right: none;
      border-bottom: 1px solid ${theme.border};
    }

    .main {
      padding: 20px;
    }

    .header,
    .row {
      flex-direction: column;
      align-items: flex-start;
    }
  }
`;

// Navigation items match the page names checked by App below.
const PAGES = [
  { id: "overview", label: "Overview", copy: "Cluster summary" },
  { id: "tenants", label: "Tenants", copy: "Namespaces and quotas" },
  { id: "workloads", label: "Workloads", copy: "Current tenant activity" },
  { id: "billing", label: "Billing", copy: "Usage and invoices" },
  { id: "scheduler", label: "Scheduler", copy: "Placement behavior" },
];

// Divide the supplied value by 1000 and format it with an s suffix.
// CPU millicore-seconds become core-seconds; memory totals use this same display helper.
function formatSeconds(value) {
  const seconds = Number(value || 0) / 1000;
  if (seconds === 0) return "0.0s";
  if (seconds < 0.1) return `${seconds.toFixed(3)}s`;
  return `${seconds.toFixed(1)}s`;
}

// Keep small demo charges visible instead of rounding them to zero cents.
function formatMoney(value) {
  const amount = Number(value || 0);
  if (amount === 0) return "$0.000000";
  if (Math.abs(amount) < 0.000001) {
    return `$${amount.toExponential(2)}`;
  }
  return `$${amount.toFixed(6)}`;
}
// Keep a meter value between zero and its maximum.
const clamp = (value, max = 100) => Math.max(0, Math.min(value, max));

// Find usage by tenant ID; a new tenant may not have a billing sample yet.
function usageForTenant(usage, tenantID) {
  return usage[tenantID] || null;
}

// Build a simple visual activity score from pod count and accumulated CPU use.
// This is a dashboard indicator rather than a measured resource percentage.
function activityPercent(record) {
  if (!record) return 0;
  return clamp(record.PodCount * 18 + record.CPUMilliSeconds / 250, 100);
}

// Trim the name, lowercase it, and turn unsupported characters into hyphens.
// This prepares a namespace-friendly name; the server and Kubernetes still validate the request.
function normalizeTenantName(value) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

// Show the navigation buttons and tell App which page the user selected.
function Sidebar({ page, setPage }) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-title">Nimbus</div>
        <div className="brand-subtitle">Tenant control plane</div>
      </div>

      <div className="nav">
        {PAGES.map((item) => (
          <button
            key={item.id}
            className={`nav-item ${page === item.id ? "active" : ""}`}
            onClick={() => setPage(item.id)}
          >
            <div className="nav-label">{item.label}</div>
            <div className="nav-copy">{item.copy}</div>
          </button>
        ))}
      </div>

      <div className="sidebar-footer">
        Refresh runs every 15 seconds.
      </div>
    </aside>
  );
}

// Reuse the title, last-refresh time, and manual refresh button on every page.
function PageHeader({ title, subtitle, onRefresh, refreshing, lastUpdated }) {
  return (
    <div className="header">
      <div>
        <h1 className="page-title">{title}</h1>
        <div className="page-subtitle">{subtitle}</div>
        <div className="metric-note" style={{ marginTop: 8 }}>
          {lastUpdated ? `Last updated ${lastUpdated.toLocaleTimeString()}` : "Waiting for first refresh"}
        </div>
      </div>
      <button className="button" onClick={onRefresh} disabled={refreshing}>
        {refreshing ? "Refreshing..." : "Refresh"}
      </button>
    </div>
  );
}

// Add up tenant activity and prepare one chart entry per tenant for the overview.
function Overview({ tenants, usage, refresh, refreshing, lastUpdated }) {
  const records = Object.values(usage);
  const activeTenants = records.filter((record) => record.PodCount > 0).length;
  const totalPods = records.reduce((sum, record) => sum + record.PodCount, 0);
  const totalCPU = records.reduce((sum, record) => sum + record.CPUMilliSeconds, 0);
  const totalMemory = records.reduce((sum, record) => sum + record.MemoryMBSeconds, 0);

  const chartData = tenants.map((tenant) => ({
    name: tenant.name,
    pods: usageForTenant(usage, tenant.id)?.PodCount || 0,
    cpu: Number(((usageForTenant(usage, tenant.id)?.CPUMilliSeconds || 0) / 1000).toFixed(3)),
  }));

  return (
    <div>
      <PageHeader
        title="Overview"
        subtitle="Tenant count, active workloads, and tracked usage."
        onRefresh={refresh}
        refreshing={refreshing}
        lastUpdated={lastUpdated}
      />

      <div className="grid-4">
        <div className="card">
          <div className="metric-label">Tenants</div>
          <div className="metric-value">{tenants.length}</div>
          <div className="metric-note">Namespaces managed by Nimbus.</div>
        </div>
        <div className="card">
          <div className="metric-label">Active Tenants</div>
          <div className="metric-value">{activeTenants}</div>
          <div className="metric-note">Tenants with running pods.</div>
        </div>
        <div className="card">
          <div className="metric-label">Pods</div>
          <div className="metric-value">{totalPods}</div>
          <div className="metric-note">Pods tracked across tenant namespaces.</div>
        </div>
        <div className="card">
          <div className="metric-label">CPU Tracked</div>
          <div className="metric-value">{formatSeconds(totalCPU)}</div>
          <div className="metric-note">Accumulated millicore-seconds.</div>
        </div>
      </div>

      <div className="grid-2">
        <div className="card">
          <div className="card-title">Pods by Tenant</div>
          {chartData.length === 0 ? (
            <div className="empty">No tenant data.</div>
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={chartData}>
                <CartesianGrid vertical={false} stroke="#edf2f7" />
                <XAxis dataKey="name" tick={{ fill: theme.muted, fontSize: 12 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fill: theme.muted, fontSize: 12 }} axisLine={false} tickLine={false} />
                <Tooltip />
                <Bar dataKey="pods" fill={theme.blue} radius={[6, 6, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>

        <div className="card">
          <div className="card-title">CPU by Tenant</div>
          {chartData.length === 0 ? (
            <div className="empty">No usage data.</div>
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <LineChart data={chartData}>
                <CartesianGrid vertical={false} stroke="#edf2f7" />
                <XAxis dataKey="name" tick={{ fill: theme.muted, fontSize: 12 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fill: theme.muted, fontSize: 12 }} axisLine={false} tickLine={false} />
                <Tooltip />
                <Line type="monotone" dataKey="cpu" stroke={theme.teal} strokeWidth={2.5} dot={{ r: 3 }} />
              </LineChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

      <div className="card">
        <div className="card-title">Usage Summary</div>
        <div className="stack">
          <div className="row">
            <span className="muted">Memory tracked</span>
            <span className="strong">{formatSeconds(totalMemory)}</span>
          </div>
          <div className="row">
            <span className="muted">Refresh interval</span>
            <span className="strong">15s</span>
          </div>
        </div>
      </div>
    </div>
  );
}

// Manage the create form, delete buttons, and errors for tenant administration.
function Tenants({ tenants, refresh, refreshing, lastUpdated }) {
  const [name, setName] = useState("");
  const [cpu, setCPU] = useState("500m");
  const [memory, setMemory] = useState("512Mi");
  const [loading, setLoading] = useState(false);
  const [deletingID, setDeletingID] = useState("");
  const [error, setError] = useState("");

  // Clean the name, submit the quotas, clear the name field, and reload dashboard data.
  async function handleCreate() {
    const normalizedName = normalizeTenantName(name);
    if (!normalizedName) {
      setError("Enter a lowercase name using letters, numbers, or hyphens.");
      return;
    }

    try {
      setLoading(true);
      setError("");
      await createTenant(normalizedName, cpu, memory);
      setName("");
      await refresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  // Mark the selected row as busy, delete its tenant through the API, and refresh the list.
  async function handleDelete(id) {
    try {
      setDeletingID(id);
      setError("");
      await deleteTenant(id);
      await refresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setDeletingID("");
    }
  }

  return (
    <div>
      <PageHeader
        title="Tenants"
        subtitle="Create tenants and manage namespace quotas."
        onRefresh={refresh}
        refreshing={refreshing}
        lastUpdated={lastUpdated}
      />

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="card-title">Create Tenant</div>
        <div className="form-grid">
          <div className="field">
            <label className="label">Name</label>
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="tenant-name"
            />
          </div>
          <div className="field">
            <label className="label">CPU Quota</label>
            <input className="input" value={cpu} onChange={(e) => setCPU(e.target.value)} placeholder="500m" />
          </div>
          <div className="field">
            <label className="label">Memory Quota</label>
            <input className="input" value={memory} onChange={(e) => setMemory(e.target.value)} placeholder="512Mi" />
          </div>
          <button className="button button-primary" onClick={handleCreate} disabled={loading}>
            {loading ? "Creating..." : "Create"}
          </button>
        </div>
        <div className="metric-note" style={{ marginTop: 12 }}>
          Tenant names become Kubernetes namespace slugs. Use lowercase letters, numbers, and hyphens.
        </div>
        {error ? <div className="error" style={{ marginTop: 12 }}>{error}</div> : null}
      </div>

      <div className="card">
        <div className="row" style={{ marginBottom: 16 }}>
          <div className="card-title" style={{ marginBottom: 0 }}>Tenant List</div>
          <span className="badge badge-blue">{tenants.length} total</span>
        </div>

        {tenants.length === 0 ? (
          <div className="empty">No tenants found.</div>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Namespace</th>
                <th>CPU</th>
                <th>Memory</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {tenants.map((tenant) => (
                <tr key={tenant.id}>
                  <td className="strong">{tenant.name}</td>
                  <td className="mono muted">{tenant.namespace}</td>
                  <td className="mono">{tenant.cpu_quota}</td>
                  <td className="mono">{tenant.memory_quota}</td>
                  <td className="muted">{new Date(tenant.created_at).toLocaleDateString()}</td>
                  <td>
                    <button
                      className="button button-danger"
                      onClick={() => handleDelete(tenant.id)}
                      disabled={deletingID === tenant.id}
                    >
                      {deletingID === tenant.id ? "Deleting..." : "Delete"}
                    </button>
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

// Show each tenant’s latest running-pod count and accumulated billing usage.
function Workloads({ tenants, usage, refresh, refreshing, lastUpdated }) {
  return (
    <div>
      <PageHeader
        title="Workloads"
        subtitle="Current activity for each tenant namespace."
        onRefresh={refresh}
        refreshing={refreshing}
        lastUpdated={lastUpdated}
      />

      {tenants.length === 0 ? (
        <div className="card">
          <div className="empty">No workloads to show.</div>
        </div>
      ) : (
        <div className="stack">
          {tenants.map((tenant) => {
            const record = usageForTenant(usage, tenant.id);
            const activity = activityPercent(record);

            return (
              <div className="card" key={tenant.id}>
                <div className="row" style={{ marginBottom: 14 }}>
                  <div>
                    <div className="strong">{tenant.name}</div>
                    <div className="mono muted" style={{ marginTop: 4 }}>{tenant.namespace}</div>
                  </div>
                  <span className={`badge ${record?.PodCount ? "badge-teal" : "badge-amber"}`}>
                    {record?.PodCount ? `${record.PodCount} pods` : "Idle"}
                  </span>
                </div>

                <div className="grid-4" style={{ marginBottom: 0 }}>
                  <div>
                    <div className="metric-label">CPU Quota</div>
                    <div className="strong mono" style={{ marginTop: 6 }}>{tenant.cpu_quota}</div>
                  </div>
                  <div>
                    <div className="metric-label">Memory Quota</div>
                    <div className="strong mono" style={{ marginTop: 6 }}>{tenant.memory_quota}</div>
                  </div>
                  <div>
                    <div className="metric-label">CPU Tracked</div>
                    <div className="strong" style={{ marginTop: 6 }}>{formatSeconds(record?.CPUMilliSeconds)}</div>
                  </div>
                  <div>
                    <div className="metric-label">Memory Tracked</div>
                    <div className="strong" style={{ marginTop: 6 }}>{formatSeconds(record?.MemoryMBSeconds)}</div>
                  </div>
                </div>

                <div className="meter">
                  <div className="meter-fill" style={{ width: `${activity}%`, background: theme.blue }} />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// Load invoices for the visible tenants and show resource totals alongside costs.
function Billing({ tenants, usage, refresh, refreshVersion, refreshing, lastUpdated }) {
  const [invoices, setInvoices] = useState({});
  const [invoiceError, setInvoiceError] = useState("");

  useEffect(() => {
    // Ignore late invoice responses after this effect is cleaned up or restarted.
    let active = true;

    async function loadInvoices() {
      try {
        setInvoiceError("");
        // Fetch invoices together, then turn [tenant ID, invoice] pairs into a lookup table.
        const entries = await Promise.all(
          tenants.map(async (tenant) => [tenant.id, await fetchInvoice(tenant.id)])
        );

        if (!active) return;
        setInvoices(Object.fromEntries(entries));
      } catch (err) {
        if (!active) return;
        setInvoiceError(err.message);
      }
    }

    if (tenants.length === 0) {
      setInvoices({});
      return () => {
        active = false;
      };
    }

    loadInvoices();
    return () => {
      active = false;
    };
  }, [tenants, refreshVersion]);

  // Join tenant names, accumulated usage, and invoice totals for the charts.
  const usageData = tenants.map((tenant) => ({
    name: tenant.name,
    cpu: (usage[tenant.id]?.CPUMilliSeconds || 0) / 1000,
    memory: (usage[tenant.id]?.MemoryMBSeconds || 0) / 1000,
    total: invoices[tenant.id]?.TotalCost || 0,
  }));

  const totalBilled = Object.values(invoices).reduce(
    (sum, invoice) => sum + (invoice?.TotalCost || 0),
    0
  );

  return (
    <div>
      <PageHeader
        title="Billing"
        subtitle="Usage totals and generated invoices."
        onRefresh={refresh}
        refreshing={refreshing}
        lastUpdated={lastUpdated}
      />

      <div className="grid-4">
        <div className="card">
          <div className="metric-label">Invoices</div>
          <div className="metric-value">{Object.keys(invoices).length}</div>
        </div>
        <div className="card">
          <div className="metric-label">Usage Records</div>
          <div className="metric-value">{Object.keys(usage).length}</div>
        </div>
        <div className="card">
          <div className="metric-label">Total Billed</div>
          <div className="metric-value" style={{ fontSize: 24 }}>{formatMoney(totalBilled)}</div>
        </div>
        <div className="card">
          <div className="metric-label">Billing Loop</div>
          <div className="metric-value">30s</div>
        </div>
      </div>

      <div className="grid-2">
        <div className="card">
          <div className="card-title">Usage</div>
          {usageData.length === 0 ? (
            <div className="empty">No usage data.</div>
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <AreaChart data={usageData}>
                <CartesianGrid vertical={false} stroke="#edf2f7" />
                <XAxis dataKey="name" tick={{ fill: theme.muted, fontSize: 12 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fill: theme.muted, fontSize: 12 }} axisLine={false} tickLine={false} />
                <Tooltip />
                <Area type="monotone" dataKey="cpu" stroke={theme.blue} fill="#dbeafe" strokeWidth={2} />
                <Area type="monotone" dataKey="memory" stroke={theme.teal} fill="#ccfbf1" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>

        <div className="card">
          <div className="card-title">Invoice Totals</div>
          {usageData.length === 0 ? (
            <div className="empty">No invoice data.</div>
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={usageData}>
                <CartesianGrid vertical={false} stroke="#edf2f7" />
                <XAxis dataKey="name" tick={{ fill: theme.muted, fontSize: 12 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fill: theme.muted, fontSize: 12 }} axisLine={false} tickLine={false} />
                <Tooltip formatter={(value) => formatMoney(value)} />
                <Bar dataKey="total" fill={theme.amber} radius={[6, 6, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

      <div className="card">
        <div className="card-title">Invoices</div>
        {invoiceError ? <div className="error" style={{ marginBottom: 12 }}>{invoiceError}</div> : null}
        {tenants.length === 0 ? (
          <div className="empty">No tenants found.</div>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Tenant</th>
                <th>Period</th>
                <th>CPU Cost</th>
                <th>Memory Cost</th>
                <th>Total</th>
              </tr>
            </thead>
            <tbody>
              {tenants.map((tenant) => {
                const invoice = invoices[tenant.id];
                return (
                  <tr key={tenant.id}>
                    <td className="strong">{tenant.name}</td>
                    <td className="mono muted">{invoice?.Period || "-"}</td>
                  <td>{formatMoney(invoice?.CPUCost)}</td>
                  <td>{formatMoney(invoice?.MemoryCost)}</td>
                  <td className="strong">{formatMoney(invoice?.TotalCost)}</td>
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

// Display the cached scheduler snapshot: waiting pods, node scores, and recent placements.
function Scheduler({ schedulerStatus, refresh, refreshing, lastUpdated }) {
  const activePods = schedulerStatus?.pods || [];
  const recentBindings = schedulerStatus?.recentBindings || [];
  const nodes = schedulerStatus?.nodes || [];
  const pressure = schedulerStatus?.pressure || 0;
  const pendingPods = schedulerStatus?.pendingPods || 0;
  const scheduledPods = schedulerStatus?.scheduledPods || 0;

  return (
    <div>
      <PageHeader
        title="Scheduler"
        subtitle="Live scheduler state from the custom Nimbus scheduling loop."
        onRefresh={refresh}
        refreshing={refreshing}
        lastUpdated={lastUpdated}
      />

      <div className="grid-4">
        <div className="card">
          <div className="metric-label">Mode</div>
          <div className="metric-value" style={{ fontSize: 22 }}>Bin-pack</div>
        </div>
        <div className="card">
          <div className="metric-label">Interval</div>
          <div className="metric-value">5s</div>
        </div>
        <div className="card">
          <div className="metric-label">Pending Pods</div>
          <div className="metric-value">{pendingPods}</div>
        </div>
        <div className="card">
          <div className="metric-label">Scheduled Pods</div>
          <div className="metric-value">{scheduledPods}</div>
        </div>
        <div className="card">
          <div className="metric-label">Pressure</div>
          <div className="metric-value">{pressure}%</div>
        </div>
      </div>

      <div className="grid-2">
        <div className="card">
          <div className="card-title">Placement Pressure</div>
          <div className="meter">
            <div className="meter-fill" style={{ width: `${pressure}%`, background: theme.blue }} />
          </div>
          <div className="metric-note" style={{ marginTop: 12 }}>
            Based on running pods currently managed by the custom scheduler.
          </div>
        </div>

        <div className="card">
          <div className="card-title">Recent Bindings</div>
          {recentBindings.length === 0 ? (
            <div className="empty">No scheduler bindings recorded yet.</div>
          ) : (
            <div className="stack">
              {recentBindings.slice(0, 4).map((binding) => (
                <div className="row" key={`${binding.namespace}-${binding.podName}-${binding.timestamp}`}>
                  <span className="strong">{binding.podName}</span>
                  <span className="mono muted">{binding.nodeName}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="card-title">Node Scores</div>
        {nodes.length === 0 ? (
          <div className="empty">No node metrics available.</div>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Node</th>
                <th>CPU Utilization</th>
                <th>Schedulable</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((node) => (
                <tr key={node.nodeName}>
                  <td className="strong">{node.nodeName}</td>
                  <td>{node.cpuUtilization}%</td>
                  <td>
                    <span className={`badge ${node.schedulable ? "badge-teal" : "badge-amber"}`}>
                      {node.schedulable ? "Yes" : "No"}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <div className="card-title">Scheduled Pods</div>
        {activePods.length === 0 ? (
          <div className="empty">No custom-scheduled pods found.</div>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Pod</th>
                <th>Namespace</th>
                <th>Node</th>
                <th>Phase</th>
              </tr>
            </thead>
            <tbody>
              {activePods.map((pod) => (
                <tr key={`${pod.namespace}-${pod.name}`}>
                  <td className="strong">{pod.name}</td>
                  <td className="mono muted">{pod.namespace}</td>
                  <td className="mono muted">{pod.nodeName || "-"}</td>
                  <td>{pod.phase}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

// Own page selection and shared API data for the whole dashboard.
// Flow: log in -> fetch tenants, usage, and scheduler status together -> render the selected page.
export default function App() {
  const [page, setPage] = useState("overview");
  const [tenants, setTenants] = useState([]);
  const [usage, setUsage] = useState({});
  const [schedulerStatus, setSchedulerStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  const [lastUpdated, setLastUpdated] = useState(null);

  // Keep the refresh function stable so the timer effect does not restart on every render.
  const refresh = useCallback(async () => {
    try {
      setRefreshing(true);
      setError("");
      // Request the demo admin token before calling the protected endpoints.
      await login();
      const [tenantData, usageData, nextSchedulerStatus] = await Promise.all([
        fetchTenants(),
        fetchUsage(),
        fetchSchedulerStatus(),
      ]);
      setTenants(tenantData);
      setUsage(usageData);
      setSchedulerStatus(nextSchedulerStatus);
      // Tell the billing page to reload invoices after a successful dashboard refresh.
      setRefreshVersion((version) => version + 1);
      setLastUpdated(new Date());
    } catch (error) {
      console.error("API error:", error);
      setError(error.message);
    } finally {
      setRefreshing(false);
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    // Refresh every 15 seconds and remove the timer when this component is removed.
    const interval = setInterval(refresh, 15000);
    return () => clearInterval(interval);
  }, [refresh]);

  return (
    <>
      <style>{css}</style>
      <div className="app">
        <Sidebar page={page} setPage={setPage} />
        <main className="main">
          {loading ? (
            <div className="card">
              <div className="card-title">Loading</div>
              <div className="empty">Fetching tenant and billing data.</div>
            </div>
          ) : null}
          {!loading && error ? (
            <div className="card">
              <div className="card-title">Connection Error</div>
              <div className="error">{error}</div>
            </div>
          ) : null}
          {!loading && !error && page === "overview" && (
            <Overview
              tenants={tenants}
              usage={usage}
              refresh={refresh}
              refreshing={refreshing}
              lastUpdated={lastUpdated}
            />
          )}
          {!loading && !error && page === "tenants" && (
            <Tenants
              tenants={tenants}
              refresh={refresh}
              refreshing={refreshing}
              lastUpdated={lastUpdated}
            />
          )}
          {!loading && !error && page === "workloads" && (
            <Workloads
              tenants={tenants}
              usage={usage}
              refresh={refresh}
              refreshing={refreshing}
              lastUpdated={lastUpdated}
            />
          )}
          {!loading && !error && page === "billing" && (
            <Billing
              tenants={tenants}
              usage={usage}
              refresh={refresh}
              refreshVersion={refreshVersion}
              refreshing={refreshing}
              lastUpdated={lastUpdated}
            />
          )}
          {!loading && !error && page === "scheduler" && (
            <Scheduler
              schedulerStatus={schedulerStatus}
              refresh={refresh}
              refreshing={refreshing}
              lastUpdated={lastUpdated}
            />
          )}
        </main>
      </div>
    </>
  );
}
