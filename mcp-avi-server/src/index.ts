#!/usr/bin/env node
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { AviClient, loadConfigFromEnv } from "./aviClient.js";
import { registerAviTools } from "./tools.js";

async function main() {
  const config = loadConfigFromEnv();
  const client = new AviClient(config);

  const server = new McpServer({
    name: "avi-mcp-server",
    version: "1.0.0",
    title: "Avi Load Balancer",
  });

  registerAviTools(server, client);

  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => {
  console.error("avi-mcp-server failed to start:", err);
  process.exit(1);
});
