const API = "http://localhost:8080";

let authToken = null;

export async function login() {
  const r = await fetch(`${API}/auth/token`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tenant_id: "admin", tenant_name: "admin" }),
  });
  const d = await r.json();
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
  const d = await r.json();
  return d.tenants || [];
}

export async function fetchUsage() {
  const r = await fetch(`${API}/billing/usage`, { headers: headers() });
  return r.json();
}

export async function fetchInvoice(id) {
  const r = await fetch(`${API}/billing/invoice/${id}`, { headers: headers() });
  return r.json();
}

export async function createTenant(name, cpu, memory) {
  const r = await fetch(`${API}/tenants`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ name, cpu_quota: cpu, memory_quota: memory }),
  });
  return r.json();
}

export async function deleteTenant(id) {
  await fetch(`${API}/tenants/${id}`, { method: "DELETE", headers: headers() });
}
