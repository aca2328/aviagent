import { z } from "zod";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import { AviApiError, AviClient } from "./aviClient.js";
import { ALL_OBJECT_TYPES, COMMON_OBJECT_TYPES } from "./objectTypes.js";

const OBJECT_TYPE_DESCRIPTION =
  "Avi object type, e.g. 'VirtualService', 'Pool', 'HealthMonitor' (case-insensitive; the REST path is the " +
  "lowercased name). Call avi_list_object_types first if unsure which type to use.";

function textResult(value: unknown): CallToolResult {
  return { content: [{ type: "text", text: JSON.stringify(value, null, 2) }] };
}

function errorResult(err: unknown): CallToolResult {
  if (err instanceof AviApiError) {
    return {
      isError: true,
      content: [
        {
          type: "text",
          text: `Avi API error (HTTP ${err.statusCode}): ${err.message}\nResponse body: ${JSON.stringify(err.body)}`,
        },
      ],
    };
  }
  const message = err instanceof Error ? err.message : String(err);
  return { isError: true, content: [{ type: "text", text: message }] };
}

function toApiPath(objectType: string): string {
  return `/${objectType.toLowerCase()}`;
}

function stripUndefined(obj: Record<string, string | undefined>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(obj)) {
    if (v !== undefined) out[k] = v;
  }
  return out;
}

export function registerAviTools(server: McpServer, client: AviClient): void {
  server.registerTool(
    "avi_list_object_types",
    {
      title: "List Avi object types",
      description:
        "Returns the catalog of Avi Controller REST object types available on this controller (170+ types), " +
        "grouped by functional area with short descriptions for the most commonly used ones, plus the full " +
        "authoritative list of every type name. Use this to find the correct `object_type` value for " +
        "avi_list/avi_get/avi_create/avi_update/avi_patch/avi_delete/avi_action.",
      inputSchema: {},
      annotations: { readOnlyHint: true, openWorldHint: false, title: "List Avi object types" },
    },
    async () => textResult({ common: COMMON_OBJECT_TYPES, all: ALL_OBJECT_TYPES })
  );

  server.registerTool(
    "avi_list",
    {
      title: "List Avi objects",
      description:
        "List objects of a given Avi type (GET /api/<object_type>). Supports Avi's standard filtering: " +
        "`name` for an exact/substring name filter, `fields` to return only specific comma-separated fields " +
        "(much smaller responses -- prefer this for broad listings), and `page`/`page_size` for pagination " +
        "(response includes `count` and, if more pages exist, `next`). Use `params` for any other Avi query " +
        "parameter not covered above (e.g. 'cloud_ref.name', 'search', 'sort', 'referred_by').",
      inputSchema: {
        object_type: z.string().describe(OBJECT_TYPE_DESCRIPTION),
        name: z.string().optional().describe("Filter by object name"),
        fields: z.string().optional().describe("Comma-separated list of fields to return, e.g. 'name,uuid,enabled'"),
        page: z.string().optional().describe("Page number, for paginated results"),
        page_size: z.string().optional().describe("Number of results per page"),
        params: z
          .record(z.string(), z.string())
          .optional()
          .describe("Additional raw Avi query parameters as key/value pairs"),
      },
      annotations: { readOnlyHint: true, openWorldHint: false, title: "List Avi objects" },
    },
    async ({ object_type, name, fields, page, page_size, params }) => {
      try {
        const result = await client.request("GET", toApiPath(object_type), {
          params: stripUndefined({ name, fields, page, page_size, ...params }),
        });
        return textResult(result);
      } catch (err) {
        return errorResult(err);
      }
    }
  );

  server.registerTool(
    "avi_get",
    {
      title: "Get an Avi object",
      description:
        "Fetch a single Avi object by UUID or by exact name (GET /api/<object_type>/<uuid>, or a name-filtered " +
        "list when only `name` is given). Provide either `uuid` or `name`.",
      inputSchema: {
        object_type: z.string().describe(OBJECT_TYPE_DESCRIPTION),
        uuid: z.string().optional().describe("UUID of the object"),
        name: z.string().optional().describe("Exact name of the object (used when uuid is not known)"),
        fields: z.string().optional().describe("Comma-separated list of fields to return"),
      },
      annotations: { readOnlyHint: true, openWorldHint: false, title: "Get an Avi object" },
    },
    async ({ object_type, uuid, name, fields }) => {
      if (!uuid && !name) {
        return errorResult(new Error("Either 'uuid' or 'name' must be provided"));
      }
      try {
        if (uuid) {
          const result = await client.request("GET", `${toApiPath(object_type)}/${uuid}`, {
            params: stripUndefined({ fields }),
          });
          return textResult(result);
        }
        const list = await client.request<{ count: number; results: unknown[] }>(
          "GET",
          toApiPath(object_type),
          { params: stripUndefined({ name, fields, page_size: "1" }) }
        );
        if (!list || list.count === 0) {
          return errorResult(new Error(`No ${object_type} object found with name '${name}'`));
        }
        return textResult(list.results[0]);
      } catch (err) {
        return errorResult(err);
      }
    }
  );

  server.registerTool(
    "avi_create",
    {
      title: "Create an Avi object",
      description:
        "Create a new Avi object (POST /api/<object_type>). `body` must be a JSON object matching that type's " +
        "schema (at minimum a `name` field for most types). Call avi_get on a similar existing object first if " +
        "unsure of the required shape.",
      inputSchema: {
        object_type: z.string().describe(OBJECT_TYPE_DESCRIPTION),
        body: z.record(z.string(), z.unknown()).describe("Object definition to create"),
      },
      annotations: {
        readOnlyHint: false,
        destructiveHint: false,
        idempotentHint: false,
        openWorldHint: false,
        title: "Create an Avi object",
      },
    },
    async ({ object_type, body }) => {
      try {
        const result = await client.request("POST", toApiPath(object_type), { body });
        return textResult(result);
      } catch (err) {
        return errorResult(err);
      }
    }
  );

  server.registerTool(
    "avi_update",
    {
      title: "Replace an Avi object",
      description:
        "Fully replace an existing Avi object's configuration (PUT /api/<object_type>/<uuid>). `body` must be " +
        "the complete desired object -- fields you omit may be reset to defaults. For partial field updates, " +
        "use avi_patch instead.",
      inputSchema: {
        object_type: z.string().describe(OBJECT_TYPE_DESCRIPTION),
        uuid: z.string().describe("UUID of the object to replace"),
        body: z.record(z.string(), z.unknown()).describe("Complete object definition"),
      },
      annotations: {
        readOnlyHint: false,
        destructiveHint: true,
        idempotentHint: true,
        openWorldHint: false,
        title: "Replace an Avi object",
      },
    },
    async ({ object_type, uuid, body }) => {
      try {
        const result = await client.request("PUT", `${toApiPath(object_type)}/${uuid}`, { body });
        return textResult(result);
      } catch (err) {
        return errorResult(err);
      }
    }
  );

  server.registerTool(
    "avi_patch",
    {
      title: "Partially update an Avi object",
      description:
        "Partially update fields on an existing Avi object (PATCH /api/<object_type>/<uuid>), leaving everything " +
        "else unchanged. `body` should use Avi's patch operators, e.g. " +
        '{"replace": {"enabled": false}}, {"add": {"services": [{"port": 8080}]}}, or {"remove": {"services": [...]}}.',
      inputSchema: {
        object_type: z.string().describe(OBJECT_TYPE_DESCRIPTION),
        uuid: z.string().describe("UUID of the object to update"),
        body: z.record(z.string(), z.unknown()).describe("Patch document using add/replace/remove operators"),
      },
      annotations: {
        readOnlyHint: false,
        destructiveHint: true,
        idempotentHint: false,
        openWorldHint: false,
        title: "Partially update an Avi object",
      },
    },
    async ({ object_type, uuid, body }) => {
      try {
        const result = await client.request("PATCH", `${toApiPath(object_type)}/${uuid}`, { body });
        return textResult(result);
      } catch (err) {
        return errorResult(err);
      }
    }
  );

  server.registerTool(
    "avi_delete",
    {
      title: "Delete an Avi object",
      description: "Permanently delete an Avi object (DELETE /api/<object_type>/<uuid>). This cannot be undone.",
      inputSchema: {
        object_type: z.string().describe(OBJECT_TYPE_DESCRIPTION),
        uuid: z.string().describe("UUID of the object to delete"),
      },
      annotations: {
        readOnlyHint: false,
        destructiveHint: true,
        idempotentHint: true,
        openWorldHint: false,
        title: "Delete an Avi object",
      },
    },
    async ({ object_type, uuid }) => {
      try {
        await client.request("DELETE", `${toApiPath(object_type)}/${uuid}`, {});
        return textResult({ deleted: true, object_type, uuid });
      } catch (err) {
        return errorResult(err);
      }
    }
  );

  server.registerTool(
    "avi_action",
    {
      title: "Run an Avi object action or sub-resource",
      description:
        "Call a sub-resource or workflow action on an object, for anything not covered by plain CRUD: e.g. " +
        "action='scaleout' or 'scalein' (Pool, VirtualService), 'migrate' or 'switchover' (VirtualService), " +
        "'runtime' or 'runtime/detail' (live status separate from config, most object types), 'clear' " +
        "(bulk-clear a collection, no uuid), 'hmon' (Pool health-monitor status), and many other per-type " +
        "actions documented in the Avi API. Hits /api/<object_type>/<uuid>/<action> (or /api/<object_type>/<action> " +
        "when uuid is omitted, e.g. the 'clear' bulk actions). Defaults to GET; use method='POST' for actions " +
        "that trigger a change (scaleout, scalein, migrate, switchover, resync, ...).",
      inputSchema: {
        object_type: z.string().describe(OBJECT_TYPE_DESCRIPTION),
        uuid: z.string().optional().describe("UUID of the object, omit for collection-level actions like 'clear'"),
        action: z.string().describe("Sub-resource/action path segment, e.g. 'scaleout', 'runtime', 'migrate'"),
        method: z.enum(["GET", "POST"]).optional().describe("HTTP method to use (default GET)"),
        body: z.record(z.string(), z.unknown()).optional().describe("Request body for POST actions"),
        params: z.record(z.string(), z.string()).optional().describe("Additional query parameters"),
      },
      annotations: {
        readOnlyHint: false,
        destructiveHint: true,
        idempotentHint: false,
        openWorldHint: false,
        title: "Run an Avi object action",
      },
    },
    async ({ object_type, uuid, action, method, body, params }) => {
      try {
        const path = uuid
          ? `${toApiPath(object_type)}/${uuid}/${action}`
          : `${toApiPath(object_type)}/${action}`;
        const result = await client.request(method ?? "GET", path, { body, params: stripUndefined(params ?? {}) });
        return textResult(result ?? { success: true });
      } catch (err) {
        return errorResult(err);
      }
    }
  );

  server.registerTool(
    "avi_get_analytics",
    {
      title: "Get Avi analytics metrics",
      description:
        "Fetch time-series analytics/metrics for a virtualservice, pool, serviceengine, or the controller " +
        "(GET /api/analytics/metrics/<resource_type>[/<uuid>]). Common metric_id values include " +
        "l4_client.avg_complete_conns, l7_client.avg_complete_responses, l4_client.avg_bandwidth, " +
        "l4_client.avg_rx_pkts_drop_ratio -- pass multiple comma-separated. Omit uuid for a collection-level " +
        "query across all objects of that type.",
      inputSchema: {
        resource_type: z
          .enum(["virtualservice", "pool", "serviceengine", "controller", "collection"])
          .describe("Type of resource the metrics belong to"),
        uuid: z.string().optional().describe("UUID of the specific object; omit for a collection-wide query"),
        metric_id: z.string().optional().describe("Comma-separated metric IDs, e.g. 'l4_client.avg_complete_conns'"),
        step: z.string().optional().describe("Sampling interval in seconds"),
        limit: z.string().optional().describe("Number of data points to return"),
        params: z.record(z.string(), z.string()).optional().describe("Additional raw Avi analytics query parameters"),
      },
      annotations: { readOnlyHint: true, openWorldHint: false, title: "Get Avi analytics metrics" },
    },
    async ({ resource_type, uuid, metric_id, step, limit, params }) => {
      try {
        const path = uuid
          ? `/analytics/metrics/${resource_type}/${uuid}`
          : `/analytics/metrics/${resource_type}`;
        const result = await client.request("GET", path, {
          params: stripUndefined({ metric_id, step, limit, ...params }),
        });
        return textResult(result);
      } catch (err) {
        return errorResult(err);
      }
    }
  );
}
