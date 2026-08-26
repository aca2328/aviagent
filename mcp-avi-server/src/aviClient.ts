import { Agent, setGlobalDispatcher } from "undici";

export interface AviConfig {
  host: string;
  username: string;
  password: string;
  version: string;
  tenant: string;
  authMethod: "session" | "basic";
  insecure: boolean;
  timeoutMs: number;
}

export function loadConfigFromEnv(): AviConfig {
  const host = process.env.AVI_HOST;
  const username = process.env.AVI_USERNAME;
  const password = process.env.AVI_PASSWORD;
  if (!host || !username || !password) {
    throw new Error(
      "AVI_HOST, AVI_USERNAME and AVI_PASSWORD environment variables are required"
    );
  }
  const insecure = (process.env.AVI_INSECURE ?? "false").toLowerCase() === "true";
  if (insecure) {
    setGlobalDispatcher(new Agent({ connect: { rejectUnauthorized: false } }));
  }
  const authMethod = process.env.AVI_AUTH_METHOD === "basic" ? "basic" : "session";
  return {
    host,
    username,
    password,
    version: process.env.AVI_VERSION ?? "22.1.3",
    tenant: process.env.AVI_TENANT ?? "admin",
    authMethod,
    insecure,
    timeoutMs: (parseInt(process.env.AVI_TIMEOUT ?? "30", 10) || 30) * 1000,
  };
}

export class AviApiError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number,
    public readonly body: unknown
  ) {
    super(message);
    this.name = "AviApiError";
  }
}

interface Session {
  sessionId: string;
  csrfToken: string;
}

/**
 * Thin REST client for the Avi Controller API. Authentication happens lazily
 * on the first request (not at construction) so the MCP server can start and
 * advertise its tools even if the controller is temporarily unreachable.
 */
export class AviClient {
  private session: Session | null = null;
  private loginPromise: Promise<void> | null = null;

  constructor(private readonly config: AviConfig) {}

  private get baseUrl(): string {
    return `https://${this.config.host}/api`;
  }

  private async ensureSession(): Promise<void> {
    if (this.config.authMethod === "basic" || this.session) return;
    if (!this.loginPromise) {
      this.loginPromise = this.login().finally(() => {
        this.loginPromise = null;
      });
    }
    return this.loginPromise;
  }

  private async login(): Promise<void> {
    const res = await fetch(`https://${this.config.host}/login`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Avi-Version": this.config.version,
      },
      body: JSON.stringify({
        username: this.config.username,
        password: this.config.password,
      }),
      signal: AbortSignal.timeout(this.config.timeoutMs),
    });

    if (!res.ok) {
      const body = await safeReadBody(res);
      throw new AviApiError(
        `Avi controller login failed with status ${res.status}`,
        res.status,
        body
      );
    }

    let sessionId = "";
    let csrfToken = "";
    for (const cookie of res.headers.getSetCookie()) {
      const [nameValue] = cookie.split(";");
      const [name, value] = nameValue.split("=");
      if (name === "avi-sessionid" || name === "sessionid") sessionId = value;
      if (name === "csrftoken") csrfToken = value;
    }

    if (!sessionId || !csrfToken) {
      const body = (await safeReadBody(res)) as Record<string, unknown> | null;
      sessionId = sessionId || (body?.sessionid as string) || "";
      csrfToken = csrfToken || (body?.csrftoken as string) || "";
    }

    if (!sessionId) {
      throw new AviApiError(
        "Avi controller login succeeded but no session id was returned",
        res.status,
        null
      );
    }

    this.session = { sessionId, csrfToken };
  }

  /**
   * Performs an authenticated request against /api/<endpoint>. Retries once
   * after a fresh login if the controller reports the session as expired (401).
   */
  async request<T = unknown>(
    method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE",
    endpoint: string,
    options: { body?: unknown; params?: Record<string, string | undefined> } = {}
  ): Promise<T | null> {
    await this.ensureSession();
    const res = await this.doRequest(method, endpoint, options);

    if (res.status === 401 && this.config.authMethod === "session") {
      // Session likely expired: re-authenticate once and retry.
      this.session = null;
      await this.ensureSession();
      return this.parseResponse<T>(await this.doRequest(method, endpoint, options), method, endpoint);
    }

    return this.parseResponse<T>(res, method, endpoint);
  }

  private async doRequest(
    method: string,
    endpoint: string,
    options: { body?: unknown; params?: Record<string, string | undefined> }
  ): Promise<Response> {
    const url = new URL(`${this.baseUrl}${endpoint}`);
    for (const [key, value] of Object.entries(options.params ?? {})) {
      if (value !== undefined) url.searchParams.set(key, value);
    }

    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      "X-Avi-Version": this.config.version,
      "X-Avi-Tenant": this.config.tenant,
    };

    if (this.config.authMethod === "basic") {
      const creds = Buffer.from(`${this.config.username}:${this.config.password}`).toString("base64");
      headers["Authorization"] = `Basic ${creds}`;
    } else if (this.session) {
      headers["Cookie"] = `sessionid=${this.session.sessionId}`;
      if (this.session.csrfToken) headers["X-CSRFToken"] = this.session.csrfToken;
    }

    return fetch(url, {
      method,
      headers,
      body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
      signal: AbortSignal.timeout(this.config.timeoutMs),
    });
  }

  private async parseResponse<T>(res: Response, method: string, endpoint: string): Promise<T | null> {
    if (!res.ok) {
      const body = await safeReadBody(res);
      const isHtml404 = res.status === 404 && (res.headers.get("content-type") ?? "").includes("text/html");
      const hint = isHtml404
        ? " -- this usually means the object_type in this path is not a valid Avi REST resource name; call avi_list_object_types to find the correct one."
        : "";
      throw new AviApiError(
        `Avi API ${method} ${endpoint} failed with status ${res.status}: ${describeErrorBody(body)}${hint}`,
        res.status,
        body
      );
    }
    if (res.status === 204) return null;
    const text = await res.text();
    if (!text) return null;
    return JSON.parse(text) as T;
  }
}

async function safeReadBody(res: Response): Promise<unknown> {
  try {
    const text = await res.text();
    if (!text) return null;
    try {
      return JSON.parse(text);
    } catch {
      return text;
    }
  } catch {
    return null;
  }
}

function describeErrorBody(body: unknown): string {
  if (body && typeof body === "object" && "error" in body) {
    return String((body as Record<string, unknown>).error);
  }
  if (typeof body === "string") return body;
  return JSON.stringify(body);
}
