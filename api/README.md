# Lumen API spec

Two formats, same Phase 1 endpoints. Pick whichever your tool prefers.

## OpenAPI 3.1 — `openapi.yaml`

Import into:

| Tool                 | How                                                                                                |
| -------------------- | -------------------------------------------------------------------------------------------------- |
| **Postman**          | File → Import → drop `openapi.yaml`. Choose "Generate collection from import".                     |
| **Insomnia**         | Application → Preferences → Data → Import Data → From File → `openapi.yaml`.                       |
| **Bruno**            | New Collection → From OpenAPI → pick `openapi.yaml`.                                               |
| **Hoppscotch**       | Settings → Collections → Import/Export → OpenAPI → upload `openapi.yaml`.                          |
| **Stoplight Studio** | File → Open Project → pick the repo root; Stoplight auto-finds `openapi.yaml`.                     |
| **Swagger UI**       | `npx http-server` in `api/` then open the bundled Swagger UI of your choice pointed at the file.   |

Default server is `http://localhost:8090` — change to your deployed hub
URL in your tool's environment after import.

## REST Client `.http` — `lumen.http`

Compatible with:

| Editor             | Extension / built-in                                              |
| ------------------ | ----------------------------------------------------------------- |
| **VS Code**        | [humao.rest-client](https://marketplace.visualstudio.com/items?itemName=humao.rest-client) |
| **JetBrains IDEs** | Built-in HTTP Client (IntelliJ, GoLand, WebStorm, PyCharm…).      |
| **Visual Studio**  | Built-in `.http` editor (2022 17.8+).                             |

Open the file and click "Send Request" above each `###` block. The `@hub`,
`@host`, and `@token` variables at the top are easy to override per request.

## WebSocket `/api/stream`

OpenAPI 3.1 documents the upgrade but most HTTP-only tools won't drive a
WebSocket. Use any of the snippets at the bottom of `lumen.http` (Node,
`websocat`, `wscat`, or browser DevTools).
