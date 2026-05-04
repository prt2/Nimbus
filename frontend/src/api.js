// Shared HTTP helpers for the dashboard.
// Flow: log in once per refresh -> attach the token -> call a route -> read data or throw an error.

const API = process.env.REACT_APP_API_URL || "http://localhost:8080";

// Keep the token in memory for later requests; reloading the page clears it.
let authToken = null;

// Read JSON or plain text, then turn unsuccessful responses into useful error messages.
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

// Ask the demo token endpoint for an admin token and save it for subsequent calls.
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

// Attach JSON headers and the saved bearer token when one is available.
function headers() {
  return {
    "Content-Type": "application/json",
    ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
  };
}

// Load the tenant list and use an empty list if the response has no tenants.
export async function fetchTenants() {
  const r = await fetch(`${API}/tenants`, { headers: headers() });
  const d = await readJSON(r);
  return d.tenants || [];
}

// Load accumulated usage keyed by tenant ID.
export async function fetchUsage() {
  const r = await fetch(`${API}/billing/usage`, { headers: headers() });
  return readJSON(r);
}

// Ask the server to calculate one tenant’s current invoice.
export async function fetchInvoice(id) {
  const r = await fetch(`${API}/billing/invoice/${id}`, { headers: headers() });
  return readJSON(r);
}

// Load the scheduler’s most recent pod, node, and binding snapshot.
export async function fetchSchedulerStatus() {
  const r = await fetch(`${API}/scheduler/status`, { headers: headers() });
  return readJSON(r);
}

// Send the tenant name and quotas so the server can create its record and namespace.
export async function createTenant(name, cpu, memory) {
  const r = await fetch(`${API}/tenants`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ name, cpu_quota: cpu, memory_quota: memory }),
  });
  return readJSON(r);
}

// Ask the server to remove a tenant’s namespace and database record.
export async function deleteTenant(id) {
  const r = await fetch(`${API}/tenants/${id}`, { method: "DELETE", headers: headers() });
  return readJSON(r);
}
