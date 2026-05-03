const API = process.env.REACT_APP_API_URL || "http://localhost:8080";

let authToken = null;

async function readJSON(response) {
  const contentType = response.headers.get("content-type") || "";
  const data = contentType.includes("application/json")
    ? await response.json()
    : await response.text();

  if (!response.ok) {
    const message =
      typeof data === "string"
        ? data
        : data?.error || data?.message || `Request failed with status ${response.status}`;
    throw new Error(message);
  }

  return data;
}

export async function login() {
  const r = await fetch(`${API}/auth/token`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tenant_id: "admin", tenant_name: "admin" }),
  });
  const d = await readJSON(r);
  authToken = d.token;
  return authToken;
}

function headers() {
  return {
    "Content-Type": "application/json",
    ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
  };
}

export async function fetchTenants() {
  const r = await fetch(`${API}/tenants`, { headers: headers() });
  const d = await readJSON(r);
  return d.tenants || [];
}

export async function fetchUsage() {
  const r = await fetch(`${API}/billing/usage`, { headers: headers() });
  return readJSON(r);
}

export async function fetchInvoice(id) {
  const r = await fetch(`${API}/billing/invoice/${id}`, { headers: headers() });
  return readJSON(r);
}

export async function fetchSchedulerStatus() {
  const r = await fetch(`${API}/scheduler/status`, { headers: headers() });
  return readJSON(r);
}

export async function createTenant(name, cpu, memory) {
  const r = await fetch(`${API}/tenants`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ name, cpu_quota: cpu, memory_quota: memory }),
  });
  return readJSON(r);
}

export async function deleteTenant(id) {
  const r = await fetch(`${API}/tenants/${id}`, { method: "DELETE", headers: headers() });
  return readJSON(r);
}
